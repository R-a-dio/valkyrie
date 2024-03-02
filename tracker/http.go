package tracker

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/R-a-dio/valkyrie/website"
	"github.com/rs/zerolog/hlog"
)

func ListenerAdd(ctx context.Context, recorder *Recorder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("icecast-auth-user", "1")
		w.WriteHeader(http.StatusOK)

		_ = r.ParseForm()
		go recorder.ListenerAdd(ctx, r)
	}
}

func ListenerRemove(ctx context.Context, recorder *Recorder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)

		_ = r.ParseForm()
		go recorder.ListenerRemove(ctx, r)
	}
}

func NewServer(ctx context.Context, addr string, recorder *Recorder) *http.Server {
	r := website.NewRouter()

	r.Use(
		hlog.AccessHandler(zerologLoggerFunc),
	)
	r.Post("/listener_joined", ListenerAdd(ctx, recorder))
	r.Post("/listener_left", ListenerRemove(ctx, recorder))

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
