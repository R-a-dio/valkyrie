package tracker

import (
	"context"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/R-a-dio/valkyrie/website"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/hlog"
)

const ADD_DELAY = time.Second * 1

type delayed struct {
	timer *time.Timer
}

type Delayer struct {
	delay time.Duration
	mu    sync.Mutex
	m     map[string]delayed
}

func NewDelayer(delay time.Duration) *Delayer {
	return &Delayer{
		delay: delay,
		m:     make(map[string]delayed),
	}
}

func (d *Delayer) Delay(ctx context.Context, id string, fn func()) {
	d.mu.Lock()
	d.m[id] = delayed{
		timer: time.AfterFunc(d.delay, fn),
	}
	d.mu.Unlock()
}

func (d *Delayer) Remove(ctx context.Context, id string) {
	d.mu.Lock()
	dyed, ok := d.m[id]
	delete(d.m, id)
	d.mu.Unlock()
	if ok {
		dyed.timer.Stop()
	}
}

func ListenerAdd(ctx context.Context, delayer *Delayer, recorder *Recorder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("icecast-auth-user", "1")
		w.WriteHeader(http.StatusOK)

		_ = r.ParseForm()

		id := r.FormValue("client")
		if id == "" {
			// icecast send us no client id somehow, this is broken and
			// we can't record this listener
			hlog.FromRequest(r).WithLevel(zerolog.PanicLevel).Msg("received icecast client with no id")
			return
		}
		delayer.Delay(ctx, id, func() {
			recorder.ListenerAdd(ctx, id, r)
		})
	}
}

func ListenerRemove(ctx context.Context, delayer *Delayer, recorder *Recorder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)

		_ = r.ParseForm()

		id := r.FormValue("client")
		delayer.Remove(ctx, id)

		go recorder.ListenerRemove(ctx, id, r)
	}
}

func NewServer(ctx context.Context, addr string, recorder *Recorder) *http.Server {
	r := website.NewRouter()

	delayi := NewDelayer(ADD_DELAY)

	r.Use(
		hlog.NewHandler(*zerolog.Ctx(ctx)),
		hlog.RemoteAddrHandler("ip"),
		hlog.UserAgentHandler("user_agent"),
		hlog.RequestIDHandler("req_id", "Request-Id"),
		hlog.URLHandler("url"),
		hlog.MethodHandler("method"),
		hlog.ProtoHandler("protocol"),
		hlog.AccessHandler(zerologLoggerFunc),
	)
	r.Post("/listener_joined", ListenerAdd(ctx, delayi, recorder))
	r.Post("/listener_left", ListenerRemove(ctx, delayi, recorder))

	return &http.Server{
		Addr:        addr,
		Handler:     r,
		BaseContext: func(l net.Listener) context.Context { return ctx },
	}
}

func zerologLoggerFunc(r *http.Request, status, size int, duration time.Duration) {
	hlog.FromRequest(r).Info().
		Int("status_code", status).
		Int("response_size_bytes", size).
		Dur("elapsed_ms", duration).
		Str("url", r.URL.String()).
		Msg("http request")
}
