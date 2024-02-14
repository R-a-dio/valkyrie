package admin

import (
	"context"
	"net/http"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/templates"
	"github.com/R-a-dio/valkyrie/util/daypass"
	vmiddleware "github.com/R-a-dio/valkyrie/website/middleware"
	"github.com/R-a-dio/valkyrie/website/shared"
	"github.com/spf13/afero"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
)

type State struct {
	config.Config

	Shared           *shared.InputFactory
	Daypass          *daypass.Daypass
	Storage          radio.StorageService
	Templates        *templates.Site
	TemplateExecutor templates.Executor
	SessionManager   *scs.SessionManager
	Authentication   vmiddleware.Authentication
	FS               afero.Fs
}

func Route(ctx context.Context, s State) func(chi.Router) {
	return func(r chi.Router) {
		r = r.With(s.Authentication.LoginMiddleware)
		r.HandleFunc("/", s.GetHome)
		r.Get("/profile", s.GetProfile)
		r.Post("/profile", s.PostProfile)
		r.Get("/pending", vmiddleware.RequirePermission(radio.PermPendingView, s.GetPending))
		r.Post("/pending", vmiddleware.RequirePermission(radio.PermPendingEdit, s.PostPending))
		// debug handlers, might not be needed later
		r.HandleFunc("/streamer/start", vmiddleware.RequirePermission(radio.PermAdmin, s.StartStreamer))
		r.HandleFunc("/streamer/stop", vmiddleware.RequirePermission(radio.PermAdmin, s.StopStreamer))
	}
}

func (s *State) StartStreamer(w http.ResponseWriter, r *http.Request) {
	s.Conf().Streamer.Client().Start(r.Context())
}

func (s *State) StopStreamer(w http.ResponseWriter, r *http.Request) {
	s.Conf().Streamer.Client().Stop(r.Context(), true)
}
