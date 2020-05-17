package website

import (
	"context"
	"net/http"
	"strconv"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/go-chi/chi"
)

type ctxKey int

const (
	TrackKey ctxKey = iota
)

// TrackCtx reads an URL parameter named TrackID and tries to find the track associated
// with it.
func TrackCtx(storage radio.TrackStorageService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			id := chi.URLParamFromCtx(ctx, "TrackID")
			iid, err := strconv.Atoi(id)
			if err != nil {
				// TODO: update this to 1.13 error handling
				/*if errors.Is(err, strconv.ErrRange) {
					return
				}*/

				panic("TrackCtx: non-number found: " + id)
			}
			trackid := radio.TrackID(iid)

			song, err := storage.Track(ctx).Get(trackid)
			if err != nil {
				// TODO: handle error
				return
			}

			ctx = context.WithValue(ctx, TrackKey, *song)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
