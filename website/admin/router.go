package admin

import (
	"context"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/templates"
	vmiddleware "github.com/R-a-dio/valkyrie/website/middleware"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
)

type State struct {
	config.Config

	Storage          radio.StorageService
	Templates        *templates.Site
	TemplateExecutor *templates.Executor
	SessionManager   *scs.SessionManager
	Authentication   vmiddleware.Authentication
}

type admin struct {
	config.Config

	storage   radio.StorageService
	templates *templates.Executor
}

func Router(ctx context.Context, s State) chi.Router {
	admin := admin{s.Config, s.Storage, s.TemplateExecutor}

	r := chi.NewRouter()

	r.Group(func(r chi.Router) {
		r.Use(s.Authentication.LoginMiddleware)
		r.Get("/", admin.GetHome)
		r.Get("/profile", admin.GetProfile)
		r.Post("/profile", admin.PostProfile)
		r.Get("/pending", admin.GetPending)
	})

	return r
}
