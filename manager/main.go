package manager

import (
	"context"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/cmd"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/rpc"
	"github.com/R-a-dio/valkyrie/storage"
	"github.com/R-a-dio/valkyrie/streamer/audio"
	"github.com/R-a-dio/valkyrie/util"
	"github.com/R-a-dio/valkyrie/util/eventstream"
	"github.com/Wessie/fdstore"
	"github.com/rs/zerolog"
)

// Execute executes a manager with the context and configuration given; it returns with
// any error that occurs; Execution can be interrupted by canceling the context given.
func Execute(ctx context.Context, cfg config.Config) error {
	store, err := storage.Open(ctx, cfg)
	if err != nil {
		return err
	}

	fdstorage := fdstore.NewStoreListenFDs()

	ln, state, err := util.RestoreOrListen(fdstorage, "manager", "tcp", cfg.Conf().Manager.RPCAddr.String())
	if err != nil {
		return err
	}
	defer ln.Close()

	m, err := NewManager(ctx, store, audio.NewProber(cfg, time.Second), state)
	if err != nil {
		return err
	}

	// separate cancel for the guest service since it depends on the manager
	guestCtx, guestCancel := context.WithCancel(ctx)
	defer guestCancel()

	gs, err := NewGuestService(guestCtx, cfg, m, store)
	if err != nil {
		return err
	}

	// setup tunein integration if it's enabled
	if cfg.Conf().Tunein.Enabled {
		tu, err := NewTuneinUpdater(ctx, cfg, m, http.DefaultClient)
		if err != nil {
			zerolog.Ctx(ctx).WithLevel(zerolog.PanicLevel).Err(err).Ctx(ctx).Msg("failed to setup tunein updater")
			// continue running if this fails, we don't care that much about tunein
		} else {
			defer tu.Close()
		}
	}

	// setup a http server for our RPC API
	srv, err := NewGRPCServer(ctx, m, gs)
	if err != nil {
		return err
	}
	defer srv.Stop()

	errCh := make(chan error, 1)
	go func() {
		// we need to clone the listener since we use the GRPC shutdown mechanism
		// before we add the listener to the fdstore, and GRPC will close the listener
		// fd when Stop is called
		clone, err := ln.(fdstore.Filer).File()
		if err != nil {
			errCh <- err
		}
		ln, err := net.FileListener(clone)
		if err != nil {
			errCh <- err
		}
		errCh <- srv.Serve(ln)
	}()

	// wait for our context to be canceled or Serve to error out
	select {
	case <-ctx.Done():
		return nil
	case <-cmd.USR2Signal(ctx):
		zerolog.Ctx(ctx).Info().Ctx(ctx).Msg("SIGUSR2 received")
		// on a restart signal we want to capture the current state and pass it
		// to the next process, however it is possible there are in-flight updates
		// happening and so we need to wait for those to finish first
		guestCancel()
		zerolog.Ctx(ctx).Info().Ctx(ctx).Msg("canceled guest service")
		// this stops any long-running manager streams we have open
		m.CloseSubs()
		zerolog.Ctx(ctx).Info().Ctx(ctx).Msg("closed manager subs")
		// this should stop any other RPC requests and wait until they're finished
		srv.GracefulStop()
		zerolog.Ctx(ctx).Info().Ctx(ctx).Msg("stopped grpc server")
		// now shutdown the manager streams, this should count as a "happens-after"
		// constraint for any updates that were incoming
		m.Shutdown()
		zerolog.Ctx(ctx).Info().Ctx(ctx).Msg("shutdown manager streams")
		// now our state should be "stable" and not be able to be mutated anymore,
		// so we can encode it to bytes. We use statusFromStreams because we did
		// a stream Shutdown earlier and it means m.status might not have the latest
		// values.
		state, err := rpc.EncodeStatus(m.statusFromStreams())
		if err != nil {
			return err
		}
		if err := fdstorage.AddListener(ln, "manager", state); err != nil {
			zerolog.Ctx(ctx).Error().Ctx(ctx).Err(err).Msg("failed to store self")
			return err
		}
		if err := fdstorage.Send(); err != nil {
			zerolog.Ctx(ctx).Error().Ctx(ctx).Err(err).Msg("failed to send store")
		}
		return nil
	case err = <-errCh:
		return err
	}
}

// NewManager returns a manager ready for use
func NewManager(ctx context.Context, store radio.StorageService, prober audio.Prober, state []byte) (*Manager, error) {
	m := Manager{
		logger:  zerolog.Ctx(ctx),
		Storage: store,
		prober:  prober,
		status:  radio.Status{},
	}

	// if we have state from a previous process, use that
	if len(state) > 0 {
		old, err := rpc.DecodeStatus(state)
		if err != nil {
			return nil, err
		}
		zerolog.Ctx(ctx).Info().Ctx(ctx).Any("status", old).Msg("restored state from fdstore")
		m.status = old
	} else { // otherwise use the state from the storage interface
		old, err := m.loadStreamStatus(ctx)
		if err != nil {
			return nil, err
		}
		zerolog.Ctx(ctx).Info().Ctx(ctx).Any("status", *old).Msg("restored state from storage")
		m.status = *old
	}

	m.userStream = eventstream.NewEventStream(m.status.StreamUser)
	m.threadStream = eventstream.NewEventStream(m.status.Thread)
	m.songStream = eventstream.NewEventStream(&radio.SongUpdate{
		Song: m.status.Song,
		Info: m.status.SongInfo,
	})
	m.listenerStream = eventstream.NewEventStream(radio.Listeners(m.status.Listeners))
	m.statusStream = eventstream.NewEventStream(m.status)

	ready := make(chan struct{})
	go m.runStatusUpdates(ctx, ready)
	// wait for the goroutine to startup and subscribe to the eventstreams
	select {
	case <-ready:
	case <-ctx.Done():
	}
	return &m, nil
}

// Manager manages shared state between different processes
type Manager struct {
	logger  *zerolog.Logger
	running atomic.Bool
	Storage radio.StorageService
	prober  audio.Prober

	// mu protects the fields below and their contents
	mu     sync.Mutex
	status radio.Status

	// streaming support
	userStream     *eventstream.EventStream[*radio.User]
	threadStream   *eventstream.EventStream[radio.Thread]
	songStream     *eventstream.EventStream[*radio.SongUpdate]
	listenerStream *eventstream.EventStream[radio.Listeners]
	statusStream   *eventstream.EventStream[radio.Status]
}

// updateStreamStatus is a legacy layer to keep supporting streamstatus table usage
// in the website.
func (m *Manager) updateStreamStatus(status radio.Status) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
	defer cancel()

	err := m.Storage.Status(ctx).Store(status)
	if err != nil {
		m.logger.Error().Ctx(ctx).Err(err).Msg("update stream status")
		return
	}
}

// loadStreamStatus is to load the legacy streamstatus table, we should only
// do this at startup
func (m *Manager) loadStreamStatus(ctx context.Context) (*radio.Status, error) {
	status, err := m.Storage.Status(ctx).Load()
	if err != nil {
		return nil, err
	}

	return status, nil
}

// CloseSubs calls CloseSubs on all internal manager streams
func (m *Manager) CloseSubs() {
	m.running.Store(false)
	m.listenerStream.CloseSubs()
	m.threadStream.CloseSubs()
	m.songStream.CloseSubs()
	m.statusStream.CloseSubs()
	m.userStream.CloseSubs()
}

// Shutdown calls Shutdown on all internal manager streams
func (m *Manager) Shutdown() {
	m.running.Store(false)
	m.listenerStream.Shutdown()
	m.threadStream.Shutdown()
	m.songStream.Shutdown()
	m.statusStream.Shutdown()
	m.userStream.Shutdown()
}
