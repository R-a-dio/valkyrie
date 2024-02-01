package manager

import (
	"context"
	"net"
	"sync"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/storage"
	"github.com/R-a-dio/valkyrie/util/eventstream"
	"github.com/rs/zerolog"
)

// Execute executes a manager with the context and configuration given; it returns with
// any error that occurs; Execution can be interrupted by canceling the context given.
func Execute(ctx context.Context, cfg config.Config) error {
	m, err := NewManager(ctx, cfg)
	if err != nil {
		return err
	}

	ExecuteListener(ctx, cfg, m)

	// setup a http server for our RPC API
	srv, err := NewHTTPServer(m)
	if err != nil {
		return err
	}

	ln, err := net.Listen("tcp", cfg.Conf().Manager.ListenAddr)
	if err != nil {
		return err
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Serve(ln)
	}()

	// wait for our context to be canceled or Serve to error out
	select {
	case <-ctx.Done():
		srv.Stop()
		return nil
	case err = <-errCh:
		return err
	}
}

// ExecuteListener is an alias for NewListener
var ExecuteListener = NewListener

// NewManager returns a manager ready for use
func NewManager(ctx context.Context, cfg config.Config) (*Manager, error) {
	store, err := storage.Open(ctx, cfg)
	if err != nil {
		return nil, err
	}

	m := Manager{
		Config:  cfg,
		logger:  zerolog.Ctx(ctx),
		Storage: store,
		status:  radio.Status{},
	}

	old, err := m.loadStreamStatus(ctx)
	if err != nil {
		return nil, err
	}
	m.status = *old

	m.userStream = eventstream.NewEventStream(old.User)
	m.threadStream = eventstream.NewEventStream(old.Thread)
	m.songStream = eventstream.NewEventStream(&radio.SongUpdate{Song: old.Song, Info: old.SongInfo})
	m.listenerStream = eventstream.NewEventStream(radio.Listeners(old.Listeners))

	m.client.streamer = cfg.Conf().Streamer.Client()
	return &m, nil
}

// Manager manages shared state between different processes
type Manager struct {
	config.Config
	logger *zerolog.Logger

	Storage radio.StorageService

	// Other components
	client struct {
		streamer radio.StreamerService
	}
	// mu protects the fields below and their contents
	mu                sync.Mutex
	status            radio.Status
	autoStreamerTimer *time.Timer
	// listener count at the start of a song
	songStartListenerCount int

	// streaming support
	userStream     *eventstream.EventStream[radio.User]
	threadStream   *eventstream.EventStream[radio.Thread]
	songStream     *eventstream.EventStream[*radio.SongUpdate]
	listenerStream *eventstream.EventStream[radio.Listeners]
}

// updateStreamStatus is a legacy layer to keep supporting streamstatus table usage
// in the website.
func (m *Manager) updateStreamStatus() {
	go func() {
		m.mu.Lock()
		status := m.status.Copy()
		m.mu.Unlock()

		ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
		defer cancel()

		ss := m.Storage.Status(ctx)

		// do some minor adjustments so that we can safely pass the status object
		// straight to the Exec
		if !status.Song.HasTrack() {
			status.Song.DatabaseTrack = &radio.DatabaseTrack{}
		}
		// streamstatus can be empty and we set a start time of now if it's zero
		if status.SongInfo.Start.IsZero() {
			status.SongInfo.Start = time.Now()
		}
		// streamstatus expects an end equal to start if it's unknown
		if status.SongInfo.End.IsZero() {
			status.SongInfo.End = status.SongInfo.Start
		}

		err := ss.Store(status)
		if err != nil {
			m.logger.Error().Err(err).Msg("update stream status")
			return
		}
	}()
}

// loadStreamStatus is to load the legacy streamstatus table, we should only do this
// at startup
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
	if status.StreamerName != "" {
		user, err := m.Storage.User(ctx).LookupName(status.StreamerName)
		if err != nil {
			m.logger.Warn().Err(err).Msg("retrieving database user")
		} else {
			status.User = *user
		}
	}

	return status, nil
}

// tryStartStreamer tries to start the streamer after waiting the timeout period given
//
// tryStartStreamer needs to be called with m.mu held
func (m *Manager) tryStartStreamer(timeout time.Duration) {
	if m.autoStreamerTimer != nil {
		return
	}

	m.logger.Info().Dur("timeout", timeout).Msg("trying to start streamer")
	m.autoStreamerTimer = time.AfterFunc(timeout, func() {
		// we lock here to lower the chance of a race between UpdateUser and this
		// timer firing
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.autoStreamerTimer == nil {
			// this means we got cancelled before we could run, but a race occurred
			// between the call to Stop and this function, and we won that race. We
			// don't want that to happen so cancel the starting
			return
		}
		// reset ourselves
		m.autoStreamerTimer = nil

		err := m.client.streamer.Start(context.Background())
		if err != nil {
			m.logger.Error().Err(err).Msg("failed to start streamer")
			// if we failed to start, try again with atleast 10 seconds timeout
			if timeout < time.Second*10 {
				timeout = time.Second * 10
			}
			m.tryStartStreamer(timeout)
			return
		}
	})
}

// stopStartStreamer stops the timer created by tryStartStreamer and sets the timer to
// nil again.
//
// stopStartStreamer needs to be called with m.mu held
func (m *Manager) stopStartStreamer() {
	if m.autoStreamerTimer == nil {
		return
	}

	m.autoStreamerTimer.Stop()
	m.autoStreamerTimer = nil
}
