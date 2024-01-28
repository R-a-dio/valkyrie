package public

import (
	"context"
	"net/http"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/templates"
	"github.com/R-a-dio/valkyrie/website/middleware"

	"github.com/go-chi/chi/v5"
)

const theme = "default"

type State struct {
	config.Config

	Templates        *templates.Site
	TemplateExecutor *templates.Executor
	Manager          radio.ManagerService
	Streamer         radio.StreamerService
	Storage          radio.StorageService
}

func (s *State) shared(r *http.Request) shared {
	user := middleware.UserFromContext(r.Context())
	return shared{
		IsUser: user != nil,
		User:   user,
	}
}

type shared struct {
	IsUser bool
	User   *radio.User
}

func Router(ctx context.Context, s State) chi.Router {
	r := chi.NewRouter()
	r.Use(middleware.ThemeCtx(s.Storage))

	r.Get("/", s.GetHome)
	r.Get("/news", s.GetNews)
	r.Post("/news", s.PostNews)
	r.Get("/schedule", s.GetSchedule)
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

func IsHTMX(r *http.Request) bool {
	return r.Header.Get("Hx-Request") == "true"
}
