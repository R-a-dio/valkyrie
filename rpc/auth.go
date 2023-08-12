package rpc

import (
	"context"
	http "net/http"

	twirp "github.com/twitchtv/twirp"
)

func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		r = r.WithContext(ctx)
		next.ServeHTTP(w, r)
	})
}

func AuthCheckHooks() *twirp.ServerHooks {
	hooks := &twirp.ServerHooks{}
	hooks.RequestRouted = func(ctx context.Context) (context.Context, error) {
		return nil, nil
	}

	return hooks
}
