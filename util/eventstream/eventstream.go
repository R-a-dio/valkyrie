package eventstream

import (
	"context"
	"io"
	"log"
	"sync/atomic"
	"time"
)

const (
	SUBSCRIBE cmd = iota
	SEND
	SEND_COMPARE
	LEAVE
	SHUTDOWN
	CLOSE
	LENGTH
)

const TIMEOUT = time.Millisecond * 100
const SUB_BUFFER_SIZE = 8

type cmd byte

type request[T any] struct {
	cmd       cmd
	ch        chan T
	m         T
	compareFn func(new, old T) bool
}

// NewEventStream returns a new EventStream with an initial value set to
// initial
func NewEventStream[M any](initial M) *EventStream[M] {
	es := newEventStream[M](true)
	es.last.Store(&initial)
	go es.run()
	return es
}

// NewEventStreamNoInit returns a new EventStream that doesn't have an
// initial value and does not send the last seen value when a sub connects
func NewEventStreamNoInit[M any]() *EventStream[M] {
	es := newEventStream[M](false)
	go es.run()
	return es
}

func newEventStream[M any](sendInit bool) *EventStream[M] {
	return &EventStream[M]{
		shutdownCh:  make(chan struct{}),
		closeCh:     make(chan struct{}),
		lengthCh:    make(chan int),
		reqs:        make(chan request[M]),
		last:        atomic.Pointer[M]{},
		sendInitial: sendInit,
	}
}

type EventStream[M any] struct {
	// shutdownCh is closed when Shutdown is called and indicates
	// we're unsubscribing all subs and the background goroutine
	shutdownCh chan struct{}
	// closeCh is closed when CloseSubs is called and indicates
	// we're unsubscribing all subs
	closeCh chan struct{}
	// lengthCh is the channel used to receive length responses after
	// calling .length()
	lengthCh chan int
	// sendInitial indicates if we should send our last seen value to
	// new subscribers or not
	sendInitial bool

	// reqs is the request channel to the manager goroutine
	reqs chan request[M]
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
	var subs = make([]chan M, 0, 16)

	for req := range es.reqs {
		switch req.cmd {
		case SUBSCRIBE:
			if closed {
				// we're not taking new subscribers
				close(req.ch)
				continue
			}
			// send our last/initial value
			if es.sendInitial {
				req.ch <- *es.last.Load()
			}
			// add the channel
			subs = append(subs, req.ch)
		case LEAVE:
			// remove the channel
			subs = removeSub(subs, req.ch)
		case SEND_COMPARE:
			old := *es.last.Load()
			if req.compareFn == nil || !req.compareFn(req.m, old) {
				continue
			}
			fallthrough
		case SEND:
			// make a copy of the value
			v := req.m
			// store the value as our last known value
			es.last.Store(&v)
			// send to all our subs with a small timeout grace period
			// so that clients have a bit of leeway between receives
			ticker.Reset(TIMEOUT)

			// eat a potential previous ticker value
			select {
			case <-ticker.C:
			default:
			}

			for _, ch := range subs {
				ticker.Reset(TIMEOUT)
				select {
				case ch <- req.m:
				case <-ticker.C: // sub didn't receive fast enough
					// TODO: see if we want to do book keeping and kicking of bad subs
					log.Println("TIMEOUT REACHED")
				}
			}
		case CLOSE:
			if !closed {
				close(es.closeCh)
			}
			// after closing the above channel we shouldn't be getting anymore
			// new subscribers so we can close all existing ones
			for _, ch := range subs {
				close(ch)
			}
			closed = true
			subs = nil
		case SHUTDOWN:
			if !closed {
				close(es.closeCh)
			}
			close(es.shutdownCh)
			// close all the subs, a noop if CLOSE was done beforehand
			for _, ch := range subs {
				close(ch)
			}
			return
		case LENGTH:
			es.lengthCh <- len(subs)
		}
	}
}

// removeSub removes the needle given from the slice s by swapping
// the last element with the needle and slicing the end off
func removeSub[M any](s []chan M, needle chan M) []chan M {
	for i, ch := range s {
		if ch == needle {
			// swap it with the last and cut the slice
			last := len(s) - 1
			s[i] = s[last]
			s = s[:last]
			// close the leaving channel to unblock sub
			close(ch)
			break
		}
	}
	return s
}

// length returns the length of the internal subs slice, this is the amount
// of active subscribers
func (es *EventStream[M]) length() int {
	select {
	case es.reqs <- request[M]{cmd: LENGTH}:
		return <-es.lengthCh
	case <-es.shutdownCh:
		return 0
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

// CompareAndSend sends the value m to all subscribers if the fn given returns
// true
func (es *EventStream[M]) CompareAndSend(m M, fn func(new M, old M) bool) {
	if fn == nil {
		// do nothing if we didn't get a comparison function
		return
	}

	select {
	case es.reqs <- request[M]{cmd: SEND_COMPARE, m: m, compareFn: fn}:
	case <-es.shutdownCh:
	}
}

// Sub subscribers to this stream of events, the channel will receive values
// send through calling Send with a small buffer and receive grace period if
// they fall behind.
func (es *EventStream[M]) Sub() chan M {
	ch := make(chan M, SUB_BUFFER_SIZE)

	select {
	case es.reqs <- request[M]{cmd: SUBSCRIBE, ch: ch}:
	case <-es.closeCh: // indicates this stream isn't taking any more subs
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
	case <-es.closeCh:
		// closed before we were able to leave, so we can assume we got closed already
	}
}

// CloseSubs closes all channels handed out by Sub() and prevents
// new subs from subscribing
func (es *EventStream[M]) CloseSubs() {
	select {
	case es.reqs <- request[M]{cmd: CLOSE}:
	case <-es.closeCh:
		// if we're already closed we don't have to wait for the request
		// to go through
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
