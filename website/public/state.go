package public

import (
	"context"

	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/templates"

	"github.com/go-chi/chi"
)

const theme = "default"

type State struct {
	config.Config

	Templates templates.Templates
}

type sharedInput struct {
	IsUser bool
}

func Router(ctx context.Context, s State) chi.Router {
	r := chi.NewRouter()

	r.Get("/", s.GetHome)
	r.Get("/news", s.GetNews)
	r.Get("/queue", s.GetQueue)
	r.Get("/last-played", s.GetLastPlayed)
	r.Get("/search", s.GetSearch)
	r.Get("/submit", s.GetSubmit)
	r.Get("/staff", s.GetStaff)
	r.Get("/favorites", s.GetFaves)
	r.Get("/irc", s.GetChat)
	return r
}
