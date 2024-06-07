package admin

import (
	"context"
	"net/http"
	"net/http/httputil"
	"net/url"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/templates"
	"github.com/R-a-dio/valkyrie/util/secret"
	vmiddleware "github.com/R-a-dio/valkyrie/website/middleware"
	"github.com/R-a-dio/valkyrie/website/shared"
	"github.com/rs/zerolog/hlog"
	"github.com/spf13/afero"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
)

func NewState(
	_ context.Context,
	cfg config.Config,
	dp secret.Secret,
	songSecret secret.Secret,
	newsCache *shared.NewsCache,
	storage radio.StorageService,
	search radio.SearchService,
	siteTmpl *templates.Site,
	exec templates.Executor,
	sessionManager *scs.SessionManager,
	auth vmiddleware.Authentication,
	fs afero.Fs,
) State {
	return State{
		Config:           cfg,
		Daypass:          dp,
		SongSecret:       songSecret,
		News:             newsCache,
		Storage:          storage,
		Search:           search,
		Templates:        siteTmpl,
		TemplateExecutor: exec,
		SessionManager:   sessionManager,
		Authentication:   auth,
		FS:               fs,
	}
}

type State struct {
	config.Config

	Daypass          secret.Secret
	SongSecret       secret.Secret
	News             *shared.NewsCache
	Storage          radio.StorageService
	Search           radio.SearchService
	Templates        *templates.Site
	TemplateExecutor templates.Executor
	SessionManager   *scs.SessionManager
	Authentication   vmiddleware.Authentication
	FS               afero.Fs
}

func Route(ctx context.Context, s State) func(chi.Router) {
	return func(r chi.Router) {
		// the login middleware will require atleast the active permission
		r = r.With(
			s.Authentication.LoginMiddleware,
		)
		p := vmiddleware.RequirePermission
		r.Handle("/set-theme", templates.SetThemeHandler(
			templates.ThemeAdminCookieName,
			s.Templates.ResolveThemeName,
		))
		r.HandleFunc("/", s.GetHome)
		r.Get("/profile", s.GetProfile)
		r.Post("/profile", s.PostProfile)
		r.Get("/pending", p(radio.PermPendingView, s.GetPending))
		r.Post("/pending", p(radio.PermPendingEdit, s.PostPending))
		r.Get("/pending-song/{SubmissionID:[0-9]+}", p(radio.PermPendingView, s.GetPendingSong))
		r.Get("/songs", p(radio.PermDatabaseView, s.GetSongs))
		r.Post("/songs", p(radio.PermDatabaseEdit, s.PostSongs))
		r.Get("/users", p(radio.PermAdmin, s.GetUsersList))
		r.Get("/news", p(radio.PermNews, s.GetNews))
		r.Get("/news/{NewsID:[0-9]+|new}", p(radio.PermNews, s.GetNewsEntry))
		r.Post("/news/{NewsID:[0-9]+|new}", p(radio.PermNews, s.PostNewsEntry))
		r.Post("/news/render", p(radio.PermNews, s.PostNewsRender))
		r.Get("/queue", p(radio.PermQueueEdit, s.GetQueue))
		r.Post("/queue/remove", p(radio.PermQueueEdit, s.PostQueueRemove))
		r.Get("/schedule", p(radio.PermScheduleEdit, s.GetSchedule))
		r.Post("/schedule", p(radio.PermScheduleEdit, s.PostSchedule))
		r.Get("/tracker", p(radio.PermListenerView, s.GetListeners))
		r.Post("/tracker/remove", p(radio.PermListenerKick, s.PostRemoveListener))

		// proxy to the grafana host
		grafana, _ := url.Parse("http://localhost:6969")
		proxy := &httputil.ReverseProxy{
			Rewrite: func(r *httputil.ProxyRequest) {
				r.SetURL(grafana)
				u := vmiddleware.UserFromContext(ctx)
				r.Out.Header.Add("X-WEBAUTH-USER", u.Username)
			},
		}
		r.Handle("/grafana/*", p(radio.PermGrafanaView, proxy.ServeHTTP))

		// debug handlers, might not be needed later
		r.Post("/api/streamer/stop", p(radio.PermAdmin, s.PostStreamerStop))
		r.Post("/api/website/reload-templates", p(radio.PermAdmin, s.PostReloadTemplates))
	}
}

func (s *State) PostStreamerStop(w http.ResponseWriter, r *http.Request) {
	s.Streamer.Stop(r.Context(), false)
}

func (s *State) errorHandler(w http.ResponseWriter, r *http.Request, err error, msg string) {
	// TODO: implement this better
	hlog.FromRequest(r).Error().Err(err).Msg(msg)
	http.Error(w, err.Error(), http.StatusInternalServerError)
}
