package streamer

import (
	"context"
	"net"

	"github.com/R-a-dio/valkyrie/engine"
)

// QueueComponent loads a queue from the database configured and sets q to the
// new queue if no error occured
func QueueComponent(q **Queue) engine.StartFn {
	return func(e *engine.Engine) (engine.DeferFn, error) {
		nq, err := NewQueue(e)
		if err != nil {
			return nil, err
		}

		*q = nq
		return nq.Save, nil
	}
}

// HTTPComponent calls NewHTTPServer and starts serving requests with the
// returned net/http server
func HTTPComponent(errCh chan<- error, streamer *Streamer) engine.StartFn {
	return func(e *engine.Engine) (engine.DeferFn, error) {
		srv, err := NewHTTPServer(e, streamer)
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
func Component(errCh chan<- error) engine.StartFn {
	return func(e *engine.Engine) (engine.DeferFn, error) {
		var queue *Queue

		err := e.Load(QueueComponent(&queue))
		if err != nil {
			return nil, err
		}

		streamer, err := NewStreamer(e, queue)
		if err != nil {
			return nil, err
		}
		deferFn := func() error {
			// TODO: use other context?
			return streamer.ForceStop(context.Background())
		}

		err = e.Load(HTTPComponent(errCh, streamer))
		return deferFn, err
	}
}
