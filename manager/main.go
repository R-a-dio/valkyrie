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

	// setup a http server for our RPC API
	srv, err := NewGRPCServer(ctx, m)
	if err != nil {
		return err
	}

	ln, err := net.Listen("tcp", cfg.Conf().Manager.RPCAddr.String())
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

	if old.User.ID != 0 {
		m.userStream = eventstream.NewEventStream(&old.User)
	} else {
		m.userStream = eventstream.NewEventStream[*radio.User](nil)
	}
	m.threadStream = eventstream.NewEventStream(old.Thread)
	m.songStream = eventstream.NewEventStream(&radio.SongUpdate{Song: old.Song, Info: old.SongInfo})
	m.listenerStream = eventstream.NewEventStream(radio.Listeners(old.Listeners))
	m.statusStream = eventstream.NewEventStream(*old)

	return &m, nil
}

// Manager manages shared state between different processes
type Manager struct {
	config.Config
	logger *zerolog.Logger

	Storage radio.StorageService

	// mu protects the fields below and their contents
	mu     sync.Mutex
	status radio.Status
	// listener count at the start of a song
	songStartListenerCount radio.Listeners

	// streaming support
	userStream     *eventstream.EventStream[*radio.User]
	threadStream   *eventstream.EventStream[radio.Thread]
	songStream     *eventstream.EventStream[*radio.SongUpdate]
	listenerStream *eventstream.EventStream[radio.Listeners]
	statusStream   *eventstream.EventStream[radio.Status]
}

// updateStreamStatus is a legacy layer to keep supporting streamstatus table usage
// in the website.
func (m *Manager) updateStreamStatus(send bool, status radio.Status) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
	defer cancel()

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

	if send {
		m.statusStream.Send(status)
	}

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
