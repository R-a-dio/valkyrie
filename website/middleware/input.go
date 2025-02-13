package middleware

import (
	"context"
	"html/template"
	"net/http"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/templates"
	"github.com/R-a-dio/valkyrie/util"
	"github.com/R-a-dio/valkyrie/website/shared/navbar"
)

type inputKey struct{}

// InputMiddleware generates an Input for each request and makes it available
// through InputFromRequest or InputFromContext
func InputMiddleware(cfg config.Config, status util.StreamValuer[radio.Status], publicNavBar navbar.NavBar, adminNavBar navbar.NavBar) func(http.Handler) http.Handler {
	PublicStreamURL := config.Value(cfg, func(c config.Config) template.URL {
		return template.URL(cfg.Conf().Website.PublicStreamURL)
	})
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			user := UserFromContext(ctx)
			input := Input{
				Now:         time.Now(),
				IsHTMX:      util.IsHTMX(r),
				IsUser:      user != nil,
				User:        user,
				StreamURL:   PublicStreamURL(),
				RequestURL:  template.URL(r.URL.String()),
				Status:      status.Latest(),
				NavBar:      publicNavBar,
				AdminNavBar: adminNavBar.WithUser(user),
				Theme:       templates.GetTheme(ctx),
			}

			ctx = context.WithValue(ctx, inputKey{}, input)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// InputFromRequest returns the Input associated with the request.
// This is equal to InputFromContext(r.Context())
func InputFromRequest(r *http.Request) Input {
	return InputFromContext(r.Context())
}

// InputFromContext returns the Input associated with the context
func InputFromContext(ctx context.Context) Input {
	v := ctx.Value(inputKey{})
	if v == nil {
		return Input{}
	}
	return v.(Input)
}

// Input is a bunch of data that should be accessible to the base template
type Input struct {
	// Now is the current time
	Now time.Time
	// IsUser is true if the request was made by a logged in user
	IsUser bool
	// IsHTMX is true if the request was made by htmx
	IsHTMX bool
	// User is non-nil if IsUser is true, and contains the logged in user
	User *radio.User
	// Status is the current radio Status
	Status radio.Status
	// StreamURL is the URL that points to the public url to listen to the stream
	StreamURL template.URL
	// RequestURL is the URL that this request was for
	RequestURL template.URL
	// NavBar is the public navbar, see documentation of NavBar
	NavBar navbar.NavBar
	// AdminNavBar is the admin navbar, see documentation of NavBar
	AdminNavBar navbar.NavBar
	// Theme is the current theme being rendered
	Theme radio.ThemeName
}

func (Input) TemplateName() string {
	return "full-page"
}
