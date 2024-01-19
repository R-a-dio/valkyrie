package v1

import (
	"fmt"
	"log"
	"maps"
	"net/http"
	"sync"
	"time"
)

const (
	SUBSCRIBE = "subscribe"
	SEND      = "send"
	LEAVE     = "leave"
	SHUTDOWN  = "shutdown"
	INIT      = "init"
)

const (
	EventPing       = "ping"
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
	// shutdown indicator
	shutdownCh chan struct{}
}

func NewStream() *Stream {
	s := &Stream{
		reqs:       make(chan request),
		subs:       make([]chan message, 0, 128),
		mu:         new(sync.RWMutex),
		last:       make(map[EventName]message),
		shutdownCh: make(chan struct{}),
	}
	go s.run()
	go s.ping()
	return s
}

// ServeHTTP implements http.Handler where each client gets send all SSE events that
// occur after connecting. There is no history.
func (s *Stream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	controller := http.NewResponseController(w)

	log.Println("sse: subscribing")
	ch := s.sub()
	defer func() {
		s.leave(ch)
	}()

	w.Header().Set("Content-Type", "text/event-stream")

	// send events that have already happened, one for each event so that
	// we're certain the page is current
	log.Println("sse: cloning initial")
	s.mu.RLock()
	init := maps.Clone(s.last)
	s.mu.RUnlock()

	for _, m := range init {
		log.Println("sending initial event:", string(m))
		if _, err := w.Write(m); err != nil {
			return
		}
	}
	controller.Flush()

	log.Println("sse: starting loop")
	for m := range ch {
		if _, err := w.Write(m); err != nil {
			return
		}
		controller.Flush()
	}
}

// SendEvent sends an SSE event with the data given.
func (s *Stream) SendEvent(event EventName, data []byte) {
	m := newMessage(event, data)

	select {
	case s.reqs <- request{cmd: SEND, m: m, e: event}:
	case <-s.shutdownCh:
	}
}

func (s *Stream) ping() {
	t := time.NewTicker(time.Second * 30)
	defer t.Stop()

	for range t.C {
		s.SendEvent(EventPing, []byte("ping"))
		select {
		case <-s.shutdownCh:
			return
		default:
		}
	}
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
			s.mu.Lock()
			s.last[req.e] = req.m
			s.mu.Unlock()

			for _, ch := range s.subs {
				select {
				case ch <- req.m:
				default:
				}
			}
		case SHUTDOWN:
			close(s.shutdownCh)
			for _, ch := range s.subs {
				close(ch)
			}
			return
		}
	}
}

func (s *Stream) sub() chan message {
	ch := make(chan message, 2)
	select {
	case s.reqs <- request{cmd: SUBSCRIBE, ch: ch}:
	case <-s.shutdownCh:
		close(ch)
	}
	return ch
}

func (s *Stream) leave(ch chan message) {
	select {
	case s.reqs <- request{cmd: LEAVE, ch: ch}:
	case <-s.shutdownCh:
	}
}

// Shutdown disconnects all connected clients
func (s *Stream) Shutdown() {
	select {
	case s.reqs <- request{cmd: SHUTDOWN}:
	case <-s.shutdownCh:
	}
}

type request struct {
	cmd string       // required
	ch  chan message // SUB/LEAVE only
	m   message      // SEND only
	e   EventName    // SEND only
}

func newMessage(event EventName, data []byte) message {
	// TODO: handle newlines in data
	return message(fmt.Sprintf("event: %s\ndata: %s\n\n", event, data))
}

type message []byte
