package middleware

import (
	"errors"
	"io"
	"net/http"
	"runtime/debug"

	"github.com/R-a-dio/valkyrie/util"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/hlog"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

func Recoverer(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rvr := recover(); rvr != nil {
				// convert our recovered value to an error if possible
				err, ok := rvr.(error)
				if !ok {
					// or create a dummy if it isn't
					err = errors.New("panic in webserver")
				}

				// net/http has a special error handlers can return to bypass recovery
				if errors.Is(err, http.ErrAbortHandler) {
					// so repanic those
					panic(rvr)
				}

				// otherwise just do some book keeping before sending back a sort of "something is really broken" message

				// mark any tracer span active as error
				span := trace.SpanFromContext(r.Context())
				span.SetStatus(codes.Error, "panic in webserver")
				// record the error in the span with a stacktrace
				span.RecordError(err, trace.WithStackTrace(true))

				// also log a stack trace through normal logging
				hlog.FromRequest(r).WithLevel(zerolog.PanicLevel).Str("stack", string(debug.Stack())).Msg("panic in webserver")

				// now send a response to whatever triggered it
				if util.IsHTMX(r) {
					// retarget any htmx request into the main box
					w.Header().Set("Hx-Retarget", "#content")
				}
				// if it's a websocket connection just error here
				if r.Header.Get("Connection") != "Upgrade" {
					w.WriteHeader(http.StatusInternalServerError)
				}
				// otherwise we should send some basic html or such, but for now it's just text
				io.WriteString(w, "something broke badly, contact IRC")
			}
		}()

		next.ServeHTTP(w, r)
		// if we didn't panic mark our span as OK
		trace.SpanFromContext(r.Context()).SetStatus(codes.Ok, "")
	}

	return http.HandlerFunc(fn)
}
