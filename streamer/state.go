package streamer

import (
	"context"
	"net"

	"github.com/R-a-dio/valkyrie/config"
)

// QueueComponent loads a queue from the database configured and sets q to the
// new queue if no error occured
func QueueComponent(q **Queue) config.StateStart {
	return func(s *config.State) (config.StateDefer, error) {
		nq, err := NewQueue(s)
		if err != nil {
			return nil, err
		}

		*q = nq
		return nq.Save, nil
	}
}

// HTTPComponent calls NewHTTPServer and starts serving requests with the
// returned net/http server
func HTTPComponent(errCh chan<- error, streamer *Streamer) config.StateStart {
	return func(s *config.State) (config.StateDefer, error) {
		srv, err := NewHTTPServer(s, streamer)
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

// Component initializes a Streamer with the state given, the error channel is
// used to send any potential http server error
//
// Component calls QueueComponent and HTTPComponent for you
func Component(errCh chan<- error) config.StateStart {
	return func(s *config.State) (config.StateDefer, error) {
		var queue *Queue

		err := s.Load(QueueComponent(&queue))
		if err != nil {
			return nil, err
		}

		streamer, err := NewStreamer(s, queue)
		if err != nil {
			return nil, err
		}
		deferFn := func() error {
			// TODO: use other context?
			return streamer.ForceStop(context.Background())
		}

		err = s.Load(HTTPComponent(errCh, streamer))
		return deferFn, err
	}
}
