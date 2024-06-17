package v1

import (
	"bytes"
	"context"
	"log"
	"maps"
	"net/http"
	"strconv"
	"sync"
	"syscall"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/templates"
	"github.com/R-a-dio/valkyrie/util"
	"github.com/R-a-dio/valkyrie/util/pool"
	"github.com/R-a-dio/valkyrie/util/sse"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/hlog"
)

func (a *API) runStatusUpdates(ctx context.Context) {
	log := zerolog.Ctx(ctx).With().Str("sse", "updates").Logger()

	var previous radio.Status

	// we don't care about the actual value, and the goroutine it spawns should keep everything
	// alive aslong as the ctx isn't canceled
	_ = util.StreamValue(ctx, a.manager.CurrentListeners, func(ctx context.Context, i int64) {
		// always send listeners, this acts as a keep-alive for the long-polling but also gives us
		// a bit more up to date listener count display
		a.sse.SendListeners(i)
	})

	_ = util.StreamValue(ctx, a.manager.CurrentStatus, func(ctx context.Context, status radio.Status) {
		// if status is zero it probably means it was an initial value or there is no stream
		// either way skip the propagation to the sse stream
		if status.IsZero() {
			log.Debug().Msg("zero value")
			return
		}

		// only pass an update through if the song is different from the previous one
		if !status.Song.EqualTo(previous.Song) {
			log.Debug().Str("event", EventMetadata).Any("value", status).Msg("sending")
			a.sse.SendNowPlaying(status)
			go a.sendQueue(ctx)
			go a.sendLastPlayed(ctx)
		}

		// same goes for the user one, only pass it through if the user actually changed
		if status.User.ID != previous.User.ID {
			log.Debug().Str("event", EventStreamer).Any("value", status.User).Msg("sending")
			a.sse.SendStreamer(status.User)
			// TODO(wessie): queue is technically only used for the automated streamer
			// and should probably have an extra event trigger here to make it disappear
		}

		previous = status
	})
}

func (a *API) sendQueue(ctx context.Context) {
	q, err := a.streamer.Queue(ctx)
	if err != nil {
		zerolog.Ctx(ctx).Error().Err(err).Str("sse", "queue").Msg("")
		return
	}

	zerolog.Ctx(ctx).Debug().Str("event", EventQueue).Any("value", q).Msg("sending")
	a.sse.SendQueue(q)
}

func (a *API) sendLastPlayed(ctx context.Context) {
	lp, err := a.storage.Song(ctx).LastPlayed(radio.LPKeyLast, 5)
	if err != nil {
		zerolog.Ctx(ctx).Error().Err(err).Str("sse", "lastplayed").Msg("")
		return
	}

	zerolog.Ctx(ctx).Debug().Str("event", EventLastPlayed).Any("value", lp).Msg("sending")
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
	EventTime       = "time"
	EventMetadata   = "metadata"
	EventListeners  = "listeners"
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
	w.Header().Set("X-Accel-Buffering", "no")

	// send a sync timestamp
	now := strconv.FormatInt(time.Now().UnixMilli(), 10)
	_, _ = w.Write(sse.Event{Name: string(EventTime), Data: []byte(now)}.Encode())

	// send events that have already happened, one for each event so that
	// we're certain the page is current
	log.Debug().Msg("init")
	s.mu.RLock()
	init := maps.Clone(s.last)
	s.mu.RUnlock()

	for _, m := range init {
		if m.genFn == nil {
			continue
		}

		data, err := m.genFn(r)
		if err != nil {
			log.Error().Err(err).Msg("sse init generator error")
			continue
		}

		log.Debug().Bytes("value", data).Msg("send")
		if _, err := w.Write(data); err != nil && !errors.IsE(err, syscall.EPIPE) {
			log.Error().Err(err).Msg("sse client write error")
			return
		}
	}
	controller.Flush()

	// start the actual new-event loop
	log.Debug().Msg("start")
	for m := range ch {
		log.Debug().Bytes("value", m.encoded[theme]).Msg("send")
		if _, err := w.Write(m.encoded[theme]); err != nil && !errors.IsE(err, syscall.EPIPE) {
			log.Error().Err(err).Msg("sse client write error")
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

// sub subscribes to the event stream and returns a channel that
// will receive all messages
func (s *Stream) sub() chan message {
	ch := make(chan message, 2)
	select {
	case s.reqs <- request{cmd: SUBSCRIBE, ch: ch}:
	case <-s.shutdownCh:
		close(ch)
	}
	return ch
}

// leave sends a LEAVE command for the channel given, the channel
// should be a channel returned by sub
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
		return message{}
	}

	// encode template results to server-side-event format
	for k, v := range m {
		v = bytes.TrimSpace(v)
		m[k] = sse.Event{Name: event, Data: v}.Encode()
	}
	return message{
		encoded: m,
		genFn:   s.MessageCache(event, data),
	}
}

var bufferPool = pool.NewResetPool(func() *bytes.Buffer { return new(bytes.Buffer) })

func (s *Stream) MessageCache(event EventName, data templates.TemplateSelectable) func(r *http.Request) ([]byte, error) {
	const op errors.Op = "website/api/v1/Stream.MessageCache"

	return func(r *http.Request) ([]byte, error) {
		buf := bufferPool.Get()
		defer bufferPool.Put(buf)

		err := s.templates.Execute(buf, r, data)
		if err != nil {
			return nil, errors.E(op, err)
		}

		data := bytes.TrimSpace(buf.Bytes())
		return sse.Event{Name: event, Data: data}.Encode(), nil
	}
}

func (s *Stream) SendStreamer(data radio.User) {
	s.SendEvent(EventStreamer, s.NewMessage(EventStreamer, Streamer(data)))
}

func (s *Stream) SendNowPlaying(data radio.Status) {
	s.SendEvent(EventMetadata, s.NewMessage(EventMetadata, NowPlaying(data)))
}

func (s *Stream) SendLastPlayed(data []radio.Song) {
	s.SendEvent(EventLastPlayed, s.NewMessage(EventLastPlayed, LastPlayed(data)))
}

func (s *Stream) SendQueue(data []radio.QueueEntry) {
	s.SendEvent(EventQueue, s.NewMessage(EventQueue, Queue(data)))
}

func (s *Stream) SendListeners(data radio.Listeners) {
	s.SendEvent(EventListeners, s.NewMessage(EventListeners, Listeners(data)))
}

// request send over the management channel
type request struct {
	cmd string       // required
	ch  chan message // SUB/LEAVE only
	m   message      // SEND only
	e   EventName    // SEND only
}

type messageGen func(r *http.Request) ([]byte, error)

type message struct {
	encoded map[string][]byte
	genFn   messageGen
}

// NowPlaying is for what is currently playing on the home page
type NowPlaying radio.Status

func (NowPlaying) TemplateName() string {
	return "nowplaying"
}

func (NowPlaying) TemplateBundle() string {
	return "home"
}

// LastPlayed is for the lastplayed listing on the home page
type LastPlayed []radio.Song

func (LastPlayed) TemplateName() string {
	return "lastplayed"
}

func (LastPlayed) TemplateBundle() string {
	return "home"
}

// Queue is for the queue listing on the home page
type Queue []radio.QueueEntry

func (Queue) TemplateName() string {
	return "queue"
}

func (Queue) TemplateBundle() string {
	return "home"
}

// Streamer is for the DJ indicator on the home page
type Streamer radio.User

func (Streamer) TemplateName() string {
	return "streamer"
}

func (Streamer) TemplateBundle() string {
	return "home"
}

// Listeners is for the listener amount indicator on the home page
type Listeners radio.Listeners

func (Listeners) TemplateName() string {
	return "listeners"
}

func (Listeners) TemplateBundle() string {
	return "home"
}
