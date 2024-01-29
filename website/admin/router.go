package admin

import (
	"context"
	"net/http"

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

func Router(ctx context.Context, s State) chi.Router {
	r := chi.NewRouter()

	r.Group(func(r chi.Router) {
		r.Use(s.Authentication.LoginMiddleware)
		r.HandleFunc("/", s.GetHome)
		r.Get("/profile", s.GetProfile)
		r.Post("/profile", s.PostProfile)
		r.Get("/pending", s.GetPending)
		r.Post("/pending", s.PostPending)
		r.HandleFunc("/streamer/start", s.StartStreamer)
		r.HandleFunc("/streamer/stop", s.StopStreamer)
	})

	return r
}

func (s *State) StartStreamer(w http.ResponseWriter, r *http.Request) {
	s.Conf().Streamer.Client().Start(r.Context())
}

func (s *State) StopStreamer(w http.ResponseWriter, r *http.Request) {
	s.Conf().Streamer.Client().Stop(r.Context(), true)
}
