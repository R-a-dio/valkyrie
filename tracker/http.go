package tracker

import (
	"context"
	"net"
	"net/http"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/website"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/hlog"
)

const (
	ICECAST_AUTH_HEADER         = "icecast-auth-user"
	ICECAST_CLIENTID_FIELD_NAME = "client"
)

func ListenerAdd(ctx context.Context, recorder *Recorder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()

		id := r.FormValue(ICECAST_CLIENTID_FIELD_NAME)
		if id == "" {
			// icecast send us no client id somehow, this is broken and
			// we can't record this listener
			hlog.FromRequest(r).WithLevel(zerolog.PanicLevel).Msg("received icecast client with no id")
			return
		}

		cid, err := radio.ParseListenerClientID(id)
		if err != nil {
			// icecast send us a client id that isn't an integer
			hlog.FromRequest(r).WithLevel(zerolog.PanicLevel).Msg("received icecast client with non-int id")
			return
		}

		// only return OK if we got the required ID from icecast
		w.Header().Set(ICECAST_AUTH_HEADER, "1")
		w.WriteHeader(http.StatusOK)

		go func() {
			recorder.ListenerAdd(ctx, NewListener(cid, r))
		}()
	}
}

func ListenerRemove(ctx context.Context, recorder *Recorder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// always return OK because it doesn't really matter if the
		// rest of the request is broken
		w.WriteHeader(http.StatusOK)

		_ = r.ParseForm()

		id := r.FormValue(ICECAST_CLIENTID_FIELD_NAME)
		if id == "" {
			// icecast send us no client id somehow, this is broken and
			// we can't record this listener
			hlog.FromRequest(r).WithLevel(zerolog.PanicLevel).Msg("received icecast client with no id")
			return
		}

		cid, err := radio.ParseListenerClientID(id)
		if err != nil {
			// icecast send us a client id that isn't an integer
			hlog.FromRequest(r).WithLevel(zerolog.PanicLevel).Msg("received icecast client with non-int id")
			return
		}

		go recorder.ListenerRemove(ctx, cid)
	}
}

func NewServer(ctx context.Context, addr string, recorder *Recorder) *http.Server {
	r := website.NewRouter()

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
