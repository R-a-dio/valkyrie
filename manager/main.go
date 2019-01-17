package manager

import (
	"context"
	"net"
	"sync"

	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/database"
	"github.com/R-a-dio/valkyrie/rpc/irc"
	"github.com/R-a-dio/valkyrie/rpc/manager"
	pb "github.com/R-a-dio/valkyrie/rpc/manager"
	"github.com/R-a-dio/valkyrie/rpc/streamer"
	"github.com/jmoiron/sqlx"
)

// Execute executes a manager with the context and configuration given; it returns with
// any error that occurs; Execution can be interrupted by canceling the context given.
func Execute(ctx context.Context, cfg config.Config) error {
	m, err := NewManager(cfg)
	if err != nil {
		return err
	}

	ExecuteListener(ctx, cfg, manager.NewWrapClient(m))

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
		status: &pb.StatusResponse{
			User:         new(pb.User),
			Song:         new(pb.Song),
			ListenerInfo: new(pb.ListenerInfo),
			Thread:       new(pb.Thread),
			BotConfig:    new(pb.BotConfig),
		},
	}

	m.client.irc = cfg.Conf().IRC.TwirpClient()
	m.client.streamer = cfg.Conf().Streamer.TwirpClient()
	return &m, nil
}

// Manager manages shared state between different processes
type Manager struct {
	config.Config
	DB *sqlx.DB

	// RPC clients to other processes
	client struct {
		irc      irc.Bot
		streamer streamer.Streamer
	}
	// mu protects the fields below and their contents
	mu     sync.Mutex
	status *pb.StatusResponse
	// listener count at the start of a song
	songStartListenerCount int64
}
