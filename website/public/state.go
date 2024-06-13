package public

import (
	"context"
	"net/http"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/templates"
	"github.com/R-a-dio/valkyrie/util/secret"
	"github.com/R-a-dio/valkyrie/website/shared"

	"github.com/go-chi/chi/v5"
)

func NewState(
	ctx context.Context,
	cfg config.Config,
	dp secret.Secret,
	newsCache *shared.NewsCache,
	exec templates.Executor,
	manager radio.ManagerService,
	streamer radio.StreamerService,
	storage radio.StorageService,
	search radio.SearchService) State {

	return State{
		Config:    cfg,
		Daypass:   dp,
		News:      newsCache,
		Templates: exec,
		Manager:   manager,
		Streamer:  streamer,
		Storage:   storage,
		Search:    search,
	}
}

type State struct {
	config.Config

	Daypass   secret.Secret
	News      *shared.NewsCache
	Templates templates.Executor
	Manager   radio.ManagerService
	Streamer  radio.StreamerService
	Storage   radio.StorageService
	Search    radio.SearchService
}

func Route(ctx context.Context, s State) func(chi.Router) {
	return func(r chi.Router) {
		r.Get("/", s.GetHome)
		r.Get("/index", s.GetHome)
		r.Get("/news", s.GetNews)
		r.Get("/news/{NewsID:[0-9]+}", s.GetNewsEntry)
		r.Post("/news/{NewsID:[0-9]+}", s.PostNewsEntry)
		r.Get("/schedule", s.GetSchedule)
		r.Get("/queue", s.GetQueue)
		r.Get("/last-played", s.GetLastPlayed)
		r.Get("/search", s.GetSearch)
		r.Get("/submit", s.GetSubmit)
		r.Post("/submit", s.PostSubmit)
		r.Get("/staff", s.GetStaff)
		r.Get("/faves", s.GetFaves)
		r.Get("/faves/{Nick}", s.GetFaves)
		r.Post("/faves", s.PostFaves)
		r.Get("/irc", s.GetChat)
		r.Get("/help", s.GetHelp)
		r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
			shared.ErrorHandler(s.Templates, w, r, shared.ErrMethodNotAllowed)
		})
		r.NotFound(func(w http.ResponseWriter, r *http.Request) {
			shared.ErrorHandler(s.Templates, w, r, shared.ErrNotFound)
		})
	}
}

func (s *State) errorHandler(w http.ResponseWriter, r *http.Request, err error) {
	shared.ErrorHandler(s.Templates, w, r, err)
}
