package v1

import (
	"bytes"
	"context"
	"maps"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
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
	var listeners atomic.Int64

	// we don't care about the actual value, and the goroutine it spawns should keep everything
	// alive aslong as the ctx isn't canceled
	_ = util.StreamValue(ctx, a.manager.CurrentListeners, func(ctx context.Context, i int64) {
		// always send listeners, this acts as a keep-alive for the long-polling but also gives us
		// a bit more up to date listener count display
		a.sse.SendListeners(i)
		// also store it for the status update to use below
		listeners.Store(i)
	})

	_ = util.StreamValue(ctx, a.manager.CurrentStatus, func(ctx context.Context, status radio.Status) {
		// if status is zero it probably means it was an initial value or there is no stream
		// either way skip the propagation to the sse stream
		if status.IsZero() {
			log.Debug().Ctx(ctx).Msg("zero value")
			return
		}

		if status.Thread != previous.Thread {
			log.Debug().Ctx(ctx).Str("event", EventThread).Any("value", status).Msg("sending")
			a.sse.SendThread(status.Thread)
			// TODO: add an actual sendThread as well
			// TODO: see if we need this hack at all
			// send a queue after a thread update, since our default theme
			// uses the queue template for the thread location
			//go a.sendQueue(ctx)
		}

		// only pass an update through if the song is different from the previous one
		if !status.Song.EqualTo(previous.Song) {
			// update the listener count with a more recent value
			// FIXME: this doesn't actually change the status retrieved inside the templates
			// 		by calling the Status function
			status.Listeners = listeners.Load()

			log.Debug().Ctx(ctx).Str("event", EventMetadata).Any("value", status).Msg("sending")
			a.sse.SendNowPlaying(status)
			go a.sendQueue(ctx, status.StreamUser)
			go a.sendLastPlayed(ctx)
		}

		// same goes for the user one, only pass it through if the user actually changed
		if status.User.ID != previous.User.ID ||
			status.User.DJ != previous.User.DJ {
			log.Debug().Ctx(ctx).Str("event", EventStreamer).Any("value", status.User).Msg("sending")
			a.sse.SendStreamer(status.User)
			// send the queue for this user, this should fix a small desync issue where metadata
			// is updated before the user
			go a.sendQueue(ctx, status.StreamUser)
		}

		previous = status
	})
}

func (a *API) sendQueue(ctx context.Context, user *radio.User) {
	var q radio.Queue
	if user != nil && radio.IsRobot(*user) {
		rq, err := a.queue.Entries(ctx)
		if err != nil {
			zerolog.Ctx(ctx).Error().Ctx(ctx).Err(err).Str("sse", "queue").Msg("")
			return
		}
		q = rq
	}

	zerolog.Ctx(ctx).Debug().Ctx(ctx).Str("event", EventQueue).Any("value", q).Msg("sending")
	a.sse.SendQueue(q)
}

func (a *API) sendLastPlayed(ctx context.Context) {
	lp, err := a.storage.Song(ctx).LastPlayed(radio.LPKeyLast, 5)
	if err != nil {
		zerolog.Ctx(ctx).Error().Ctx(ctx).Err(err).Str("sse", "lastplayed").Msg("")
		return
	}

	zerolog.Ctx(ctx).Debug().Ctx(ctx).Str("event", EventLastPlayed).Any("value", lp).Msg("sending")
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
	logger *zerolog.Logger
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

func NewStream(ctx context.Context, exec templates.Executor) *Stream {
	s := &Stream{
		logger:     zerolog.Ctx(ctx),
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
	ctx := r.Context()
	log := hlog.FromRequest(r)
	controller := http.NewResponseController(w)
	theme := templates.GetTheme(r.Context())

	log.Debug().Ctx(ctx).Msg("subscribing")
	ch := s.sub()
	defer func() {
		log.Debug().Ctx(ctx).Msg("leave")
		s.leave(ch)
	}()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("X-Accel-Buffering", "no")

	// send a sync timestamp
	now := strconv.FormatInt(time.Now().UnixMilli(), 10)
	_, _ = w.Write(sse.Event{Name: string(EventTime), Data: []byte(now)}.Encode())

	// send events that have already happened, one for each event so that
	// we're certain the page is current
	log.Debug().Ctx(ctx).Msg("init")
	s.mu.RLock()
	init := maps.Clone(s.last)
	s.mu.RUnlock()

	for _, m := range init {
		if m.genFn == nil {
			continue
		}

		data, err := m.genFn(r)
		if err != nil {
			log.Error().Ctx(ctx).Err(err).Msg("sse init generator error")
			continue
		}

		log.Debug().Ctx(ctx).Bytes("value", data).Msg("send")
		if _, err := w.Write(data); err != nil {
			if !errors.IsE(err, syscall.EPIPE) {
				log.Error().Ctx(ctx).Err(err).Msg("sse client write error")
			}
			return
		}
	}

	if err := controller.Flush(); err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("sse client flush error")
		return
	}

	// start the actual new-event loop
	log.Debug().Ctx(ctx).Msg("start")
	for m := range ch {
		log.Debug().Ctx(ctx).Bytes("value", m.encoded[theme]).Msg("send")
		if _, err := w.Write(m.encoded[theme]); err != nil {
			if !errors.IsE(err, syscall.EPIPE) {
				log.Error().Ctx(ctx).Err(err).Msg("sse client write error")
			}
			return
		}
		if err := controller.Flush(); err != nil {
			log.Error().Ctx(ctx).Err(err).Msg("sse client flush error")
			return
		}
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

// trimSpace is like bytes.TrimSpace but in that in preserves non-nilness of
// the input slice
func trimSpace(v []byte) []byte {
	v = bytes.TrimSpace(v)
	if v == nil {
		return []byte{}
	}
	return v
}

func (s *Stream) NewMessage(event EventName, data templates.TemplateSelectable) message {
	m, err := s.templates.ExecuteAll(data)
	if err != nil {
		s.logger.Error().Err(err).Msg("failed creating SSE message")
		return message{}
	}

	// encode template results to server-side-event format
	for k, v := range m {
		v = trimSpace(v)
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

		data := trimSpace(buf.Bytes())
		return sse.Event{Name: event, Data: data}.Encode(), nil
	}
}

func (s *Stream) SendThread(data radio.Thread) {
	s.SendEvent(EventThread, s.NewMessage(EventThread, Thread(data)))
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

func (s *Stream) SendQueue(data radio.Queue) {
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
	encoded map[radio.ThemeName][]byte
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

type Thread radio.Thread

func (Thread) TemplateName() string {
	return "thread"
}

func (Thread) TemplateBundle() string {
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
type Queue radio.Queue

func (Queue) TemplateName() string {
	return "queue"
}

func (Queue) TemplateBundle() string {
	return "home"
}

func (q Queue) Length() time.Duration {
	return radio.Queue(q).Length()
}

func (q Queue) Limit(maxSize int) Queue {
	return Queue(radio.Queue(q).Limit(maxSize))
}

func (q Queue) RequestAmount() int {
	return radio.Queue(q).RequestAmount()
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
