package api

import (
	"fmt"
	"maps"
	"net/http"
	"sync"
)

const (
	SUBSCRIBE = "subscribe"
	SEND      = "send"
	LEAVE     = "leave"
	SHUTDOWN  = "shutdown"
	INIT      = "init"
)

const (
	EVENT_COUNT = 4

	EventMetadata   = "metadata"
	EventStreamer   = "streamer"
	EventQueue      = "queue"
	EventLastPlayed = "lastplayed"
	EventThread     = "thread"
)

type EventName string

type Stream struct {
	reqs chan request
	subs []chan message

	// mu guards last
	mu   *sync.RWMutex
	last map[EventName]message
}

func NewStream() *Stream {
	s := &Stream{
		reqs: make(chan request),
		subs: make([]chan message, 0, 128),
		mu:   new(sync.RWMutex),
		last: make(map[EventName]message),
	}
	s.run()
	return s
}

// ServeHTTP implements http.Handler where each client gets send all SSE events that
// occur after connecting. There is no history.
func (s *Stream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, http.StatusText(http.StatusNotImplemented), http.StatusNotImplemented)
		return
	}

	ch := s.sub()
	defer func() {
		s.leave(ch)
	}()

	w.Header().Set("Content-Type", "text/event-stream")

	// send events that have already happened, one for each event so that
	// we're certain the page is current
	s.mu.RLock()
	init := maps.Clone(s.last)
	s.mu.RUnlock()
	for _, m := range init {
		if _, err := w.Write(m); err != nil {
			return
		}
	}
	flusher.Flush()

	for m := range ch {
		if _, err := w.Write(m); err != nil {
			return
		}
		flusher.Flush()
	}
}

// SendEvent sends an SSE event with the data given.
func (s *Stream) SendEvent(event EventName, data []byte) {
	m := newMessage(event, data)
	s.mu.Lock()
	s.last[event] = m
	s.mu.Unlock()

	s.reqs <- request{cmd: SEND, m: m}
}

func (s *Stream) run() {
	for req := range s.reqs {
		switch req.cmd {
		case SUBSCRIBE:
			s.subs = append(s.subs, req.ch)
		case LEAVE:
			for i, ch := range s.subs {
				if ch == req.ch {
					last := len(s.subs) - 1
					s.subs[i] = s.subs[last]
					s.subs = s.subs[:last]
					close(ch)
					break
				}
			}
		case SEND:
			for _, ch := range s.subs {
				select {
				case ch <- req.m:
				default:
				}
			}
		case SHUTDOWN:
			for _, ch := range s.subs {
				close(ch)
				s.subs = s.subs[:0]
			}
		}
	}
}

func (s *Stream) sub() chan message {
	ch := make(chan message)
	s.reqs <- request{cmd: SUBSCRIBE, ch: ch}
	return ch
}

func (s *Stream) leave(ch chan message) {
	s.reqs <- request{cmd: LEAVE, ch: ch}
}

// Shutdown disconnects all connected clients
func (s *Stream) Shutdown() {
	s.reqs <- request{cmd: SHUTDOWN}
}

type request struct {
	cmd string
	ch  chan message
	m   message
}

func newMessage(event EventName, data []byte) message {
	return message(fmt.Sprintf("event: %s\ndata: %s\n\n", event, data))
}

type message []byte
