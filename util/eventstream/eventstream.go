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
	CLOSE
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
		closeCh:    make(chan struct{}),
		reqs:       make(chan request[M]),
		subs:       make([]chan M, 0, 16),
		last:       atomic.Pointer[M]{},
	}

	es.last.Store(&initial)
	go es.run()

	return es
}

type EventStream[M any] struct {
	// shutdownCh is closed when Shutdown is called and indicates
	// we're unsubscribing all subs and the background goroutine
	shutdownCh chan struct{}
	// closeCh is closed when CloseSubs is called and indicates
	// we're unsubscribing all subs
	closeCh chan struct{}

	// reqs is the request channel to the manager goroutine
	reqs chan request[M]
	// subs stores the subscribers
	subs []chan M
	// last stores the last value send to subscribers
	last atomic.Pointer[M]
}

func (es *EventStream[M]) Latest() M {
	return *es.last.Load()
}

func (es *EventStream[M]) run() {
	ticker := time.NewTicker(TIMEOUT)
	defer ticker.Stop()

	var closed bool

	for req := range es.reqs {
		switch req.cmd {
		case SUBSCRIBE:
			if closed {
				// we're not taking new subscribers
				close(req.ch)
				continue
			}
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
					// TODO: see if we want to do book keeping and kicking of bad subs
				}
			}
		case CLOSE:
			close(es.closeCh)
			// after closing the above channel we shouldn't be getting anymore
			// new subscribers so we can close all existing ones
			for _, ch := range es.subs {
				close(ch)
			}
			closed = true
			es.subs = nil
		case SHUTDOWN:
			close(es.shutdownCh)

			if closed { // if we already closed there is nothing to do
				return
			}

			// otherwise we want to close all subs
			for _, ch := range es.subs {
				close(ch)
			}
			es.subs = nil
			return
		}
	}
}

// Send sends the value M to all subscribers previously subscribed through
// Sub() or SubStream(), the last value Send is also stored and send when
// a new subscriber appears.
func (es *EventStream[M]) Send(m M) {
	select {
	case es.reqs <- request[M]{cmd: SEND, m: m}:
	case <-es.shutdownCh:
	}
}

// Sub subscribers to this stream of events, the channel will receive values
// send through calling Send with a small buffer and receive grace period if
// they fall behind.
func (es *EventStream[M]) Sub() chan M {
	ch := make(chan M, 8)

	select {
	case es.reqs <- request[M]{cmd: SUBSCRIBE, ch: ch}:
	case <-es.closeCh: // indicates this stream isn't taking any more subs
		close(ch) // we never subscribed so close the channel
	case <-es.shutdownCh: // indicates this stream is completely shutdown
		close(ch) // we never subscribed so close the channel
	}
	return ch
}

// SubStream is like Sub but returns a Stream interface instead of a channel
func (es *EventStream[M]) SubStream(ctx context.Context) Stream[M] {
	return NewStream(ctx, es)
}

// Leave leaves the subscriber list, the channel should be one returned by Sub, the
// channel is closed once the Leave request has been processed
func (es *EventStream[M]) Leave(ch chan M) {
	select {
	case es.reqs <- request[M]{cmd: LEAVE, ch: ch}:
	case <-es.shutdownCh:
		// shutdown before we were able to leave, so we can assume we got closed already
	}
}

// CloseSubs closes all channels handed out by Sub() and prevents
// new subs from subscribing
func (es *EventStream[M]) CloseSubs() {
	select {
	case es.reqs <- request[M]{cmd: CLOSE}:
	case <-es.closeCh:
	}
}

// Shutdown is like CloseSubs but also exits the background goroutine used
// for updating the eventstream
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
