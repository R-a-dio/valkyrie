package admin

import (
	"context"
	"net/http"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/templates"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
)

type State struct {
	config.Config

	Storage   radio.StorageService
	Templates *templates.Site
}

type admin struct {
	config.Config

	storage   radio.StorageService
	templates *templates.Executor
}

func newSessionManager() *scs.SessionManager {
	s := scs.New()
	s.Lifetime = 150 * 24 * time.Hour
	s.Cookie = scs.SessionCookie{
		Name: "admin",
		//SameSite: http.SameSiteStrictMode,
		Secure: true,
	}
	return s
}

func Router(ctx context.Context, s State) chi.Router {
	sessionManager := scs.New()
	sessionManager.Store = NewSessionStore(ctx, s.Storage)
	sessionManager.Codec = JSONCodec{}
	sessionManager.Lifetime = 150 * 24 * time.Hour
	sessionManager.Cookie = scs.SessionCookie{
		Name: "admin",
		//SameSite: http.SameSiteStrictMode,
		Secure: true,
	}

	executor := s.Templates.Executor()
	authentication := NewAuthentication(s.Storage, executor, sessionManager)
	admin := admin{s.Config, s.Storage, executor}

	r := chi.NewRouter()
	r.Use(sessionManager.LoadAndSave) // use session manager
	// logout page has to be accessable without being logged in, so register it outside of the group
	r.Get("/logout", authentication.LogoutHandler)
	r.Group(func(r chi.Router) {
		r.Use(authentication.LoginMiddleware)
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("hello"))
		})
		r.Get("/profile", admin.GetProfile)
		r.Post("/profile", admin.PostProfile)
	})

	return r
}
