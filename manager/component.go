package manager

import (
	"net"
	"sync"

	"github.com/R-a-dio/valkyrie/engine"
	"github.com/R-a-dio/valkyrie/rpc/irc"
	pb "github.com/R-a-dio/valkyrie/rpc/manager"
	"github.com/R-a-dio/valkyrie/rpc/streamer"
)

// HTTPComponent calls NewHTTPServer and starts serving requests with the
// returned net/http server
func HTTPComponent(errCh chan<- error, m *Manager) engine.StartFn {
	return func(e *engine.Engine) (engine.DeferFn, error) {
		srv, err := NewHTTPServer(m)
		if err != nil {
			return nil, err
		}

		ln, err := net.Listen("tcp", srv.Addr)
		if err != nil {
			return nil, err
		}

		go func() {
			err := srv.Serve(ln)
			if err != nil {
				errCh <- err
			}
		}()
		return srv.Close, nil
	}
}

func Component(errCh chan<- error) engine.StartFn {
	return func(e *engine.Engine) (engine.DeferFn, error) {
		m, err := NewManager(e)
		if err != nil {
			return nil, err
		}

		err = e.Load(
			ListenerComponent(m),

			HTTPComponent(errCh, m),
		)

		return m.Close, err
	}
}

// ListenerComponent runs a stream listener until cancelled
func ListenerComponent(m *Manager) engine.StartFn {
	return func(e *engine.Engine) (engine.DeferFn, error) {
		ln, err := NewListener(e, m)
		if err != nil {
			return nil, err
		}

		return ln.Shutdown, nil
	}
}

type Manager struct {
	*engine.Engine

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

func NewManager(e *engine.Engine) (*Manager, error) {
	m := Manager{
		Engine: e,
		status: &pb.StatusResponse{
			User:         new(pb.User),
			Song:         new(pb.Song),
			ListenerInfo: new(pb.ListenerInfo),
			Thread:       new(pb.Thread),
			BotConfig:    new(pb.BotConfig),
		},
	}

	m.client.irc = e.Conf().IRC.TwirpClient()
	m.client.streamer = e.Conf().Streamer.TwirpClient()

	return &m, nil
}

func (m *Manager) Close() error {
	return nil
}
