package v1

import (
	"bytes"
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
	var wg sync.WaitGroup

	wg.Add(1)
	// prepare our eventstreams from the manager
	go func() {
		defer wg.Done()
		a.runSongUpdates(ctx)
	}()

	wg.Wait()
	return nil
}

func (a *API) runSongUpdates(ctx context.Context) error {
	song_stream := prepareStream(ctx, a.manager.CurrentSong)

	for {
		us, err := song_stream.Next()
		if err != nil {
			log.Println("v1/api:song:", err)
			break
		}

		if us == nil {
			log.Println("v1/api:song: nil value")
			continue
		}

		log.Println("v1/api:song:sending:", us)
		a.sse.SendNowPlaying(us)
		// TODO: add a timeout scenario
		go a.sendQueue(ctx)
		go a.sendLastPlayed(ctx)
	}

	return nil
}

func (a *API) sendQueue(ctx context.Context) {
	q, err := a.streamer.Queue(ctx)
	if err != nil {
		log.Println("v1/api:queue:", err)
		return
	}

	a.sse.SendQueue(q)
}

func (a *API) sendLastPlayed(ctx context.Context) {
	lp, err := a.song.Song(ctx).LastPlayed(0, 5)
	if err != nil {
		log.Println("v1/api:lastplayed:", err)
		return
	}

	a.sse.SendLastPlayed(lp)
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
	templates *templates.Executor
}

func NewStream(exec *templates.Executor) *Stream {
	s := &Stream{
		reqs:       make(chan request),
		mu:         new(sync.RWMutex),
		last:       make(map[EventName]message),
		shutdownCh: make(chan struct{}),
		templates:  exec,
	}
	go s.run()
	return s
}

// ServeHTTP implements http.Handler where each client gets send all SSE events that
// occur after connecting. There is no history.
func (s *Stream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	controller := http.NewResponseController(w)

	theme := middleware.GetTheme(r.Context())

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
		log.Println("sending initial event:", string(m[theme]))
		if _, err := w.Write(m[theme]); err != nil {
			return
		}
	}
	controller.Flush()

	// start the actual new-event loop
	log.Println("sse: starting loop")
	for m := range ch {
		if _, err := w.Write(m[theme]); err != nil {
			return
		}
		controller.Flush()
	}
}

// SendEvent sends an SSE event with the data given.
func (s *Stream) SendEvent(event EventName, m message) {
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

func (s *Stream) NewMessage(event EventName, template string, data any) message {
	m, err := s.templates.ExecuteTemplateAll(template, data)
	if err != nil {
		log.Println("failed creating message", err)
		return nil
	}

	// encode template results to server-side-event format
	for k, v := range m {
		v = bytes.TrimSpace(v)
		m[k] = sse.Event{Name: event, Data: v}.Encode()
	}
	return m
}

func (s *Stream) SendNowPlaying(data *radio.SongUpdate) {
	s.SendEvent(EventMetadata, s.NewMessage(EventMetadata, "nowplaying", data))
}

func (s *Stream) SendLastPlayed(data []radio.Song) {
	s.SendEvent(EventLastPlayed, s.NewMessage(EventLastPlayed, "lastplayed", data))
}

func (s *Stream) SendQueue(data []radio.QueueEntry) {
	s.SendEvent(EventQueue, s.NewMessage(EventQueue, "queue", data))
}

// request send over the management channel
type request struct {
	cmd string       // required
	ch  chan message // SUB/LEAVE only
	m   message      // SEND only
	e   EventName    // SEND only
}

type message map[string][]byte
