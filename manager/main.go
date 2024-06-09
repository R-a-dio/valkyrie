package manager

import (
	"context"
	"net"
	"sync"
	"syscall"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/rpc"
	"github.com/R-a-dio/valkyrie/storage"
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

	m, err := NewManager(ctx, store, state)
	if err != nil {
		return err
	}

	// setup a http server for our RPC API
	srv, err := NewGRPCServer(ctx, m)
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
	case <-util.Signal(syscall.SIGUSR2):
		// on a restart signal we want to capture the current state and pass it
		// to the next process, however it is possible there are in-flight updates
		// happening and so we need to wait for those to finish first

		// this stops any long-running manager streams we have open
		m.Shutdown()
		// this should stop any other RPC requests and wait until they're finished
		srv.GracefulStop()
		// now our state should be "stable" and not be able to be mutated anymore,
		// so we can encode it to bytes. We use statusFromStreams because we did
		// a stream Shutdown earlier and it means m.status might not have the latest
		// values.
		state, err := rpc.EncodeStatus(m.statusFromStreams())
		if err != nil {
			return err
		}
		if err := fdstorage.AddListener(ln, "manager", state); err != nil {
			zerolog.Ctx(ctx).Error().Err(err).Msg("failed to store self")
			return err
		}
		if err := fdstorage.Send(); err != nil {
			zerolog.Ctx(ctx).Error().Err(err).Msg("failed to send store")
		}
		return nil
	case err = <-errCh:
		return err
	}
}

// NewManager returns a manager ready for use
func NewManager(ctx context.Context, store radio.StorageService, state []byte) (*Manager, error) {
	m := Manager{
		logger:  zerolog.Ctx(ctx),
		Storage: store,
		status:  radio.Status{},
	}

	// if we have state from a previous process, use that
	if len(state) > 0 {
		old, err := rpc.DecodeStatus(state)
		if err != nil {
			return nil, err
		}
		m.status = old
	} else { // otherwise use the state from the storage interface
		old, err := m.loadStreamStatus(ctx)
		if err != nil {
			return nil, err
		}
		m.status = *old
	}

	if m.status.User.ID != 0 { //
		m.userStream = eventstream.NewEventStream(&m.status.User)
	} else {
		m.userStream = eventstream.NewEventStream[*radio.User](nil)
	}
	m.threadStream = eventstream.NewEventStream(m.status.Thread)
	m.songStream = eventstream.NewEventStream(&radio.SongUpdate{
		Song: m.status.Song,
		Info: m.status.SongInfo,
	})
	m.listenerStream = eventstream.NewEventStream(radio.Listeners(m.status.Listeners))
	m.statusStream = eventstream.NewEventStream(m.status)
	go m.runStatusUpdates(ctx)
	return &m, nil
}

// Manager manages shared state between different processes
type Manager struct {
	logger *zerolog.Logger

	Storage radio.StorageService

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
		m.logger.Error().Err(err).Msg("update stream status")
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

	// see if we can get more complete data than what we already have
	if status.Song.Metadata != "" {
		song, err := m.Storage.Song(ctx).FromMetadata(status.Song.Metadata)
		if err != nil {
			m.logger.Warn().Err(err).Msg("retrieving database metadata")
		} else {
			status.Song = *song
		}
	}
	if status.User.DJ.ID != 0 {
		user, err := m.Storage.User(ctx).GetByDJID(status.User.DJ.ID)
		if err != nil {
			m.logger.Warn().Err(err).Msg("retrieving database user")
		} else {
			status.User = *user
		}
	}

	return status, nil
}

// Shutdown calls Shutdown on all internal manager streams
func (m *Manager) Shutdown() {
	m.listenerStream.Shutdown()
	m.threadStream.Shutdown()
	m.songStream.Shutdown()
	m.statusStream.Shutdown()
	m.userStream.Shutdown()
}
