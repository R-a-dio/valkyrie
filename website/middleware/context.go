package middleware

import (
	"context"
	"net/http"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/go-chi/chi/v5"
)

type trackKey struct{}

type userKey struct{}

// TrackCtx reads an URL parameter named TrackID and tries to find the track associated
// with it.
func TrackCtx(storage radio.TrackStorageService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			id := chi.URLParamFromCtx(ctx, "TrackID")
			trackid, err := radio.ParseTrackID(id)
			if err != nil {
				// TODO: update this to 1.13 error handling
				/*if errors.Is(err, strconv.ErrRange) {
					return
				}*/

				panic("TrackCtx: non-number found: " + id)
			}

			song, err := storage.Track(ctx).Get(trackid)
			if err != nil {
				// TODO: handle error
				return
			}

			ctx = context.WithValue(ctx, trackKey{}, *song)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetTrack returns the track from the given context if one exists.
// See TrackCtx for supplier of this
func GetTrack(ctx context.Context) (radio.Song, bool) {
	song, ok := ctx.Value(trackKey{}).(radio.Song)
	return song, ok
}

// UserByDJIDCtx reads an URL router parameter named DJID and tries to find the user
// associated with it. The result can be retrieved with GetUser
func UserByDJIDCtx(storage radio.UserStorageService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			tmp1 := chi.URLParamFromCtx(ctx, "DJID")
			id, err := radio.ParseDJID(tmp1)
			if err != nil {
				http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
				return
			}

			user, err := storage.User(ctx).GetByDJID(id)
			if err != nil {
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				return
			}

			ctx = context.WithValue(ctx, userKey{}, *user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetUser returns the user from the given context if one exists.
func GetUser(ctx context.Context) (radio.User, bool) {
	user, ok := ctx.Value(userKey{}).(radio.User)
	return user, ok
}
