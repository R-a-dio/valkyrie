package middleware

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
	UserKey  ctxKey = iota
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

// UserByDJIDCtx reads an URL router parameter named DJID and tries to find the user
// associated with it. The result is found in the request context with key UserKey
func UserByDJIDCtx(storage radio.UserStorageService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			tmp1 := chi.URLParamFromCtx(ctx, "DJID")
			tmp2, err := strconv.Atoi(tmp1)
			if err != nil {
				http.Error(w, http.StatusText(404), 404)
				return
			}
			id := radio.DJID(tmp2)

			user, err := storage.User(ctx).GetByDJID(id)
			if err != nil {
				// nothing really
				panic("UserBYDJIDCtx: fuck do I know")
			}

			ctx = context.WithValue(ctx, UserKey, *user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
