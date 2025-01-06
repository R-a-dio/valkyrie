package middleware

import (
	"io"
	"net/http"
	"runtime/debug"

	"github.com/R-a-dio/valkyrie/util"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/hlog"
)

func Recoverer(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rvr := recover(); rvr != nil {
				if rvr == http.ErrAbortHandler {
					// we don't recover http.ErrAbortHandler so the response
					// to the client is aborted, this should not be logged
					panic(rvr)
				}

				hlog.FromRequest(r).WithLevel(zerolog.PanicLevel).Str("stack", string(debug.Stack())).Msg("panic in webserver")

				if util.IsHTMX(r) {
					w.Header().Set("Hx-Retarget", "#content")
				}
				if r.Header.Get("Connection") != "Upgrade" {
					w.WriteHeader(http.StatusInternalServerError)
				}

				io.WriteString(w, "something broke badly, contact IRC")
			}
		}()

		next.ServeHTTP(w, r)
	}

	return http.HandlerFunc(fn)
}
