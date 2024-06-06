package eventstream

import (
	"context"
	"io"
	"sync/atomic"
	"time"
)

const (
	SUBSCRIBE cmd = iota
	SEND
	LEAVE
	SHUTDOWN
	INIT
)

const TIMEOUT = time.Millisecond * 100

type cmd byte

type request[T any] struct {
	cmd cmd
	ch  chan T
	m   T
}

func NewEventStream[M any](initial M) *EventStream[M] {
	es := &EventStream[M]{
		shutdownCh: make(chan struct{}),
		reqs:       make(chan request[M]),
		subs:       make([]chan M, 0, 16),
		last:       atomic.Pointer[M]{},
	}

	es.last.Store(&initial)
	go es.run()

	return es
}

type EventStream[M any] struct {
	// shutdownCh is closed when the server is shutting down
	shutdownCh chan struct{}

	// reqs is the request channel to the manager goroutine
	reqs chan request[M]
	// subs stores the subscribers
	subs []chan M
	// last stores the last value send to subscribers
	last atomic.Pointer[M]
	// FallbehindFn holds a function that is called when a subscriber falls behind
	// and isn't keeping up with SENDs
	FallbehindFn func(chan M, M)
}

func (es *EventStream[M]) Latest() M {
	return *es.last.Load()
}

func (es *EventStream[M]) run() {
	ticker := time.NewTicker(TIMEOUT)
	defer ticker.Stop()

	for req := range es.reqs {
		switch req.cmd {
		case SUBSCRIBE:
			// send our last/initial value
			req.ch <- *es.last.Load()
			es.subs = append(es.subs, req.ch)
		case LEAVE:
			// find the channel that is leaving
			for i, ch := range es.subs {
				if ch == req.ch {
					// swap it with the last sub and cut the slice
					last := len(es.subs) - 1
					es.subs[i] = es.subs[last]
					es.subs = es.subs[:last]
					// close the leaving channel to unblock sub
					close(ch)
					break
				}
			}
		case SEND:
			v := req.m
			es.last.Store(&v)
			// send to all our subs with a small timeout grace period
			// so that clients have a bit of leeway between receives
			ticker.Reset(TIMEOUT)

			// eat a potential previous ticker value
			select {
			case <-ticker.C:
			default:
			}

			for _, ch := range es.subs {
				ticker.Reset(TIMEOUT)
				select {
				case ch <- req.m:
				case <-ticker.C: // sub didn't receive fast enough
					if es.FallbehindFn != nil {
						es.FallbehindFn(ch, req.m)
					}
					// TODO: see if we want to do book keeping and kicking of bad subs
				}
			}
		case SHUTDOWN:
			close(es.shutdownCh)
			// after closing the above channel we shouldn't be getting anymore
			// new subscribers so we can close all existing ones
			for _, ch := range es.subs {
				close(ch)
			}
			// and exit our background goroutine
			return
		}
	}
}

func (es *EventStream[M]) Send(m M) {
	select {
	case es.reqs <- request[M]{cmd: SEND, m: m}:
	case <-es.shutdownCh:
	}
}

func (es *EventStream[M]) Sub() chan M {
	ch := make(chan M, 8)

	select {
	case es.reqs <- request[M]{cmd: SUBSCRIBE, ch: ch}:
	case <-es.shutdownCh:
		close(ch) // we never subscribed so close ourselves
	}
	return ch
}

func (es *EventStream[M]) SubStream(ctx context.Context) Stream[M] {
	return NewStream(ctx, es)
}

func (es *EventStream[M]) Leave(ch chan M) {
	select {
	case es.reqs <- request[M]{cmd: LEAVE, ch: ch}:
	case <-es.shutdownCh:
		// shutdown before we were able to leave, so we can assume we got closed already
	}
}

func (es *EventStream[M]) Shutdown() {
	select {
	case es.reqs <- request[M]{cmd: SHUTDOWN}:
	case <-es.shutdownCh: // someone else shut us down
	}
}

func NewStream[T any](ctx context.Context, p *EventStream[T]) Stream[T] {
	return &stream[T]{
		ctx: ctx,
		p:   p,
		C:   p.Sub(),
	}
}

type Stream[T any] interface {
	Next() (T, error)
	Close() error
}

type stream[T any] struct {
	ctx context.Context
	p   *EventStream[T]
	C   chan T
}

func (s *stream[T]) Next() (v T, err error) {
	select {
	case v, ok := <-s.C:
		if !ok {
			return v, io.EOF
		}
		return v, nil
	case <-s.ctx.Done():
		return v, io.EOF
	}
}

func (s *stream[T]) Close() error {
	s.p.Leave(s.C)
	return nil
}
