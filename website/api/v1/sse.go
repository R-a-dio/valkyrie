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
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/templates"
	"github.com/R-a-dio/valkyrie/util/sse"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/hlog"
)

func prepareStream[T any](ctx context.Context, fn func(context.Context) (T, error)) (T, error) {
	for {
		s, err := fn(ctx)
		if err == nil {
			return s, nil
		}
		zerolog.Ctx(ctx).Error().Err(err).Msg("failed to prepare stream")

		time.Sleep(time.Second * 3)
		select {
		case <-ctx.Done():
			return s, ctx.Err()
		default:
		}
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
	const op errors.Op = "website/api/v1/API.runSongUpdates"

	log := zerolog.Ctx(ctx).With().Str("stream", "song").Logger()

	song_stream, err := prepareStream(ctx, a.manager.CurrentSong)
	if err != nil {
		return errors.E(op, err)
	}

	for {
		us, err := song_stream.Next()
		if err != nil {
			log.Error().Err(err).Msg("source failure")
			break
		}

		if us == nil {
			log.Debug().Msg("nil value")
			continue
		}

		log.Debug().Any("value", us).Msg("sending")
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
		zerolog.Ctx(ctx).Error().Err(err).Str("stream", "queue").Msg("")
		return
	}

	a.sse.SendQueue(q)
}

func (a *API) sendLastPlayed(ctx context.Context) {
	lp, err := a.song.Song(ctx).LastPlayed(0, 5)
	if err != nil {
		zerolog.Ctx(ctx).Error().Err(err).Str("stream", "lastplayed").Msg("")
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
	templates templates.Executor
}

func NewStream(exec templates.Executor) *Stream {
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
	log := hlog.FromRequest(r)
	controller := http.NewResponseController(w)
	theme := templates.GetTheme(r.Context())

	log.Debug().Msg("subscribing")
	ch := s.sub()
	defer func() {
		log.Debug().Msg("leave")
		s.leave(ch)
	}()

	w.Header().Set("Content-Type", "text/event-stream")

	// send a sync timestamp
	now := strconv.FormatInt(time.Now().UnixMilli(), 10)
	_, _ = w.Write(sse.Event{Name: "time", Data: []byte(now)}.Encode())

	// send events that have already happened, one for each event so that
	// we're certain the page is current
	log.Debug().Msg("init")
	s.mu.RLock()
	init := maps.Clone(s.last)
	s.mu.RUnlock()

	for _, m := range init {
		log.Debug().Bytes("value", m[theme]).Msg("send")
		if _, err := w.Write(m[theme]); err != nil {
			return
		}
	}
	controller.Flush()

	// start the actual new-event loop
	log.Debug().Msg("start")
	for m := range ch {
		log.Debug().Bytes("value", m[theme]).Msg("send")
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

func (s *Stream) NewMessage(event EventName, data templates.TemplateSelectable) message {
	m, err := s.templates.ExecuteAll(data)
	if err != nil {
		// TODO: handle error cases better
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
	if data == nil {
		return
	}
	s.SendEvent(EventMetadata, s.NewMessage(EventMetadata, NowPlaying(*data)))
}

func (s *Stream) SendLastPlayed(data []radio.Song) {
	s.SendEvent(EventLastPlayed, s.NewMessage(EventLastPlayed, LastPlayed(data)))
}

func (s *Stream) SendQueue(data []radio.QueueEntry) {
	s.SendEvent(EventQueue, s.NewMessage(EventQueue, Queue(data)))
}

// request send over the management channel
type request struct {
	cmd string       // required
	ch  chan message // SUB/LEAVE only
	m   message      // SEND only
	e   EventName    // SEND only
}

type message map[string][]byte

type NowPlaying radio.SongUpdate

func (NowPlaying) TemplateName() string {
	return "nowplaying"
}

func (NowPlaying) TemplateBundle() string {
	return "home"
}

type LastPlayed []radio.Song

func (LastPlayed) TemplateName() string {
	return "lastplayed"
}

func (LastPlayed) TemplateBundle() string {
	return "home"
}

type Queue []radio.QueueEntry

func (Queue) TemplateName() string {
	return "queue"
}

func (Queue) TemplateBundle() string {
	return "home"
}
