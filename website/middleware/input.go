package middleware

import (
	"context"
	"html/template"
	"net/http"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/util"
)

type inputKey struct{}

// InputMiddleware generates an Input for each request and makes it available
// through InputFromRequest
func InputMiddleware(cfg config.Config, status *util.Value[radio.Status]) func(http.Handler) http.Handler {
	PublicStreamURL := template.URL(cfg.Conf().Website.PublicStreamURL)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			user := UserFromContext(ctx)
			input := Input{
				Now:        time.Now(),
				IsHTMX:     util.IsHTMX(r),
				IsUser:     user != nil,
				User:       user,
				StreamURL:  PublicStreamURL,
				RequestURL: template.URL(r.URL.String()),
				Status:     status.Latest(),
			}

			ctx = context.WithValue(ctx, inputKey{}, input)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// InputFromRequest returns the Input associated with the request
func InputFromRequest(r *http.Request) Input {
	return InputFromContext(r.Context())
}

func InputFromContext(ctx context.Context) Input {
	v := ctx.Value(inputKey{})
	if v == nil {
		return Input{}
	}
	return v.(Input)
}

// Input is a bunch of data that should be accessable to the base template
type Input struct {
	Now        time.Time
	IsUser     bool
	IsHTMX     bool
	User       *radio.User
	Status     radio.Status
	StreamURL  template.URL
	RequestURL template.URL
}

func (Input) TemplateName() string {
	return "full-page"
}
