package v1

import (
	"context"
	"log"
	"maps"
	"net/http"
	"strconv"
	"sync"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/templates"
	"github.com/R-a-dio/valkyrie/util/sse"
	"github.com/R-a-dio/valkyrie/website/middleware"
)

func prepareStream[T any](ctx context.Context, fn func(context.Context) (T, error)) T {
	for {
		s, err := fn(ctx)
		if err == nil {
			return s
		}
		time.Sleep(time.Second * 3)
	}
}

func (a *API) runSSE(ctx context.Context) error {
	// prepare our eventstreams from the manager
	go a.runSongUpdates(ctx)

	return nil
}

func (a *API) runSongUpdates(ctx context.Context) error {
	song_stream := prepareStream(ctx, a.manager.CurrentSong)

	for {
		us, err := song_stream.Next()
		if err != nil {
			break
		}

		if us == nil {
			continue
		}

		a.sse.SendEvent(EventMetadata, []byte(us.Metadata))
	}

	return nil
}

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

type EventName = string

type Stream struct {
	// manager goroutine channel
	reqs chan request

	// mu guards last
	mu   *sync.RWMutex
	last map[EventName]message

	// shutdown indicator
	shutdownCh chan struct{}

	// templates for the site, used in theme support
	site *templates.Site
}

func NewStream() *Stream {
	s := &Stream{
		reqs:       make(chan request),
		mu:         new(sync.RWMutex),
		last:       make(map[EventName]message),
		shutdownCh: make(chan struct{}),
	}
	go s.run()
	return s
}

// ServeHTTP implements http.Handler where each client gets send all SSE events that
// occur after connecting. There is no history.
func (s *Stream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	controller := http.NewResponseController(w)

	themeIdx := s.themeIndex(middleware.GetTheme(r.Context()))

	log.Println("sse: subscribing")
	ch := s.sub()
	defer func() {
		s.leave(ch)
	}()

	w.Header().Set("Content-Type", "text/event-stream")

	// send a sync timestamp
	now := strconv.FormatInt(time.Now().UnixMilli(), 10)
	w.Write(sse.Event{Name: "time", Data: []byte(now)}.Encode())

	// send events that have already happened, one for each event so that
	// we're certain the page is current
	log.Println("sse: cloning initial")
	s.mu.RLock()
	init := maps.Clone(s.last)
	s.mu.RUnlock()

	for _, m := range init {
		log.Println("sending initial event:", string(m.data[themeIdx]))
		if _, err := w.Write(m.data[themeIdx]); err != nil {
			return
		}
	}
	controller.Flush()

	// start the actual new-event loop
	log.Println("sse: starting loop")
	for m := range ch {
		if _, err := w.Write(m.data[themeIdx]); err != nil {
			return
		}
		controller.Flush()
	}
}

// SendEvent sends an SSE event with the data given.
func (s *Stream) SendEvent(event EventName, data []byte) {
	m := s.NewMessage(event, data)

	select {
	case s.reqs <- request{cmd: SEND, m: m, e: event}:
	case <-s.shutdownCh:
	}
}

func (s *Stream) run() {
	subs := make([]chan message, 0, 128)

	for req := range s.reqs {
		switch req.cmd {
		case SUBSCRIBE:
			subs = append(subs, req.ch)
		case LEAVE:
			for i, ch := range subs {
				if ch == req.ch {
					last := len(subs) - 1
					subs[i] = subs[last]
					subs = subs[:last]
					close(ch)
					break
				}
			}
		case SEND:
			s.mu.Lock()
			s.last[req.e] = req.m
			s.mu.Unlock()

			for _, ch := range subs {
				select {
				case ch <- req.m:
				default:
				}
			}
		case SHUTDOWN:
			close(s.shutdownCh)
			for _, ch := range subs {
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

func (s *Stream) themeIndex(theme string) int {
	return 0
}

func (s *Stream) NewMessage(event EventName, data any) message {
	switch data.(type) {
	case radio.SongUpdate:
		return message{}
	}

	return message{}
}

// request send over the management channel
type request struct {
	cmd string       // required
	ch  chan message // SUB/LEAVE only
	m   message      // SEND only
	e   EventName    // SEND only
}

// message encapsulates an SSE event
type message struct {
	// event name used in Stream.last
	event EventName
	// data is a slice of sse-encoded-event; one for each theme
	data [][]byte
}
