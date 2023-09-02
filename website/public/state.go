package public

import (
	"context"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/templates"
	"github.com/R-a-dio/valkyrie/website/middleware"

	"github.com/go-chi/chi"
)

const theme = "default"

type State struct {
	config.Config

	Templates templates.Templates
	Manager   radio.ManagerService
	Streamer  radio.StreamerService
	Storage   radio.StorageService
}

type sharedInput struct {
	IsUser bool
}

func Router(ctx context.Context, s State) chi.Router {
	r := chi.NewRouter()
	r.Use(middleware.ThemeCtx(s.Storage))

	r.Get("/", s.GetHome)
	r.Get("/news", s.GetNews)
	r.Post("/news", s.PostNews)
	r.Get("/queue", s.GetQueue)
	r.Get("/last-played", s.GetLastPlayed)
	r.Get("/search", s.GetSearch)
	r.Get("/submit", s.GetSubmit)
	r.Post("/submit", s.PostSubmit)
	r.Get("/staff", s.GetStaff)
	r.Get("/favorites", s.GetFaves)
	r.Post("/favorites", s.PostFaves)
	r.Get("/irc", s.GetChat)
	return r
}
