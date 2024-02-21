package middleware

import (
	"context"
	"html/template"
	"net/http"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/util"
)

type inputKey struct{}

// InputMiddleware generates an Input for each request and makes it available
// through InputFromRequest
func InputMiddleware(cfg config.Config) func(http.Handler) http.Handler {
	PublicStreamURL := template.URL(cfg.Conf().Website.PublicStreamURL)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			user := UserFromContext(ctx)
			input := Input{
				IsHTMX:    util.IsHTMX(r),
				IsUser:    user != nil,
				User:      user,
				StreamURL: PublicStreamURL,
			}

			ctx = context.WithValue(ctx, inputKey{}, input)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// InputFromRequest returns the Input associated with the request
func InputFromRequest(r *http.Request) Input {
	v := r.Context().Value(inputKey{})
	if v == nil {
		return Input{}
	}
	return v.(Input)
}

// Input is a bunch of data that should be accessable to the base template
type Input struct {
	IsUser    bool
	IsHTMX    bool
	User      *radio.User
	StreamURL template.URL
}

func (Input) TemplateName() string {
	return "full-page"
}
