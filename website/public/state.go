package public

import (
	"context"
	"net/http"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/templates"
	"github.com/R-a-dio/valkyrie/util/daypass"
	"github.com/rs/zerolog/hlog"

	"github.com/go-chi/chi/v5"
)

const theme = "default"

type State struct {
	config.Config

	Daypass   *daypass.Daypass
	Templates templates.Executor
	Manager   radio.ManagerService
	Streamer  radio.StreamerService
	Storage   radio.StorageService
}

func (s *State) errorHandler(w http.ResponseWriter, r *http.Request, err error) {
	hlog.FromRequest(r).Error().Err(err).Msg("")
	// TODO: handle errors more gracefully
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}

func Route(ctx context.Context, s State) func(chi.Router) {
	return func(r chi.Router) {
		r.Get("/", s.GetHome)
		r.Get("/news", s.GetNews)
		r.Get("/news/{NewsID:[0-9]+}`", s.GetNewsEntry)
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
	}
}
