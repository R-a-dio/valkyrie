package admin

import (
	"context"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi"
)

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

func Router(ctx context.Context, cfg config.Config, storage radio.StorageService) chi.Router {
	sessionManager := scs.New()
	sessionManager.Store = NewSessionStore(ctx, storage)
	sessionManager.Codec = JSONCodec{}
	sessionManager.Lifetime = 150 * 24 * time.Hour
	sessionManager.Cookie = scs.SessionCookie{
		Name: "admin",
		//SameSite: http.SameSiteStrictMode,
		Secure: true,
	}

	authentication := NewAuthentication(storage, sessionManager)

	r := chi.NewRouter()
	r.Use(sessionManager.LoadAndSave)
	r.Use(authentication.LoginMiddleware)
	r.Get("/logout", authentication.LogoutHandler)
	return r
}
