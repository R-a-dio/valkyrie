package manager

import (
	"context"
	"net"
	"sync"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/database"
	"github.com/jmoiron/sqlx"
)

// Execute executes a manager with the context and configuration given; it returns with
// any error that occurs; Execution can be interrupted by canceling the context given.
func Execute(ctx context.Context, cfg config.Config) error {
	m, err := NewManager(cfg)
	if err != nil {
		return err
	}

	ExecuteListener(ctx, cfg, m)

	// setup a http server for our RPC API
	srv, err := NewHTTPServer(m)
	if err != nil {
		return err
	}

	ln, err := net.Listen("tcp", srv.Addr)
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
		return srv.Close()
	case err = <-errCh:
		return err
	}
}

// ExecuteListener is an alias for NewListener
var ExecuteListener = NewListener

// NewManager returns a manager ready for use
func NewManager(cfg config.Config) (*Manager, error) {
	db, err := database.Connect(cfg)
	if err != nil {
		return nil, err
	}

	m := Manager{
		Config: cfg,
		DB:     db,
		status: radio.Status{},
	}

	m.client.announce = cfg.Conf().IRC.Client()
	m.client.streamer = cfg.Conf().Streamer.Client()
	return &m, nil
}

// Manager manages shared state between different processes
type Manager struct {
	config.Config
	DB *sqlx.DB

	// Other components
	client struct {
		announce radio.AnnounceService
		streamer radio.StreamerService
	}
	// mu protects the fields below and their contents
	mu     sync.Mutex
	status radio.Status
	// listener count at the start of a song
	songStartListenerCount int
}
