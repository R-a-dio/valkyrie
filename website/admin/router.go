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
	s.Codec = JSONCodec{}
	return s
}

func Router(ctx context.Context, cfg config.Config, storage radio.StorageService) chi.Router {
	sessionManager := newSessionManager()
	sessionManager.Store = NewSessionStore(ctx, storage)

	authentication := NewAuthentication(storage, sessionManager)

	r := chi.NewRouter()
	r.Use(sessionManager.LoadAndSave)
	r.Use(authentication.LoginMiddleware)
	r.Get("/logout", authentication.LogoutHandler)
	return r
}
