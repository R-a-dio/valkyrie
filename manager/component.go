package manager

import (
	"net"
	"sync"

	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/rpc/irc"
	pb "github.com/R-a-dio/valkyrie/rpc/manager"
	"github.com/R-a-dio/valkyrie/rpc/streamer"
)

// HTTPComponent calls NewHTTPServer and starts serving requests with the
// returned net/http server
func HTTPComponent(errCh chan<- error, m *Manager) config.StateStart {
	return func(s *config.State) (config.StateDefer, error) {
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

func Component(errCh chan<- error) config.StateStart {
	return func(s *config.State) (config.StateDefer, error) {
		m, err := NewManager(s)
		if err != nil {
			return nil, err
		}

		err = s.Load(
			ListenerComponent(m),

			HTTPComponent(errCh, m),
		)

		return m.Close, err
	}
}

type Manager struct {
	*config.State

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

func NewManager(state *config.State) (*Manager, error) {
	m := Manager{
		State: state,
		status: &pb.StatusResponse{
			User:         new(pb.User),
			Song:         new(pb.Song),
			ListenerInfo: new(pb.ListenerInfo),
			Thread:       new(pb.Thread),
			BotConfig:    new(pb.BotConfig),
		},
	}

	m.client.irc = state.Conf().IRC.TwirpClient()
	m.client.streamer = state.Conf().Streamer.TwirpClient()

	return &m, nil
}

func (m *Manager) Close() error {
	return nil
}
