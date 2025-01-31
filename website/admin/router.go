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
	"github.com/R-a-dio/valkyrie/website/shared/navbar"
	"github.com/spf13/afero"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
)

var NavBar = navbar.New(`hx-boost="true" hx-push-url="true" hx-target="#content"`,
	navbar.NewProtectedItem("Admin", radio.PermActive, navbar.Attrs("href", "/admin")),
	navbar.NewProtectedItem("News", radio.PermNews, navbar.Attrs("href", "/admin/news")),
	navbar.NewProtectedItem("Queue", radio.PermQueueEdit, navbar.Attrs("href", "/admin/queue")),
	navbar.NewProtectedItem("Listeners", radio.PermListenerView, navbar.Attrs("href", "/admin/tracker")),
	navbar.NewProtectedItem("Schedule", radio.PermScheduleEdit, navbar.Attrs("href", "/admin/schedule")),
	navbar.NewProtectedItem("Proxy", radio.PermProxyKick, navbar.Attrs("href", "/admin/proxy")),
	navbar.NewProtectedItem("Pending", radio.PermPendingView, navbar.Attrs("href", "/admin/pending")),
	navbar.NewProtectedItem("Song Database", radio.PermDatabaseView, navbar.Attrs("href", "/admin/songs")),
	navbar.NewProtectedItem("Users", radio.PermAdmin, navbar.Attrs("href", "/admin/users")),
	navbar.NewProtectedItem("Telemetry", radio.PermTelemetryView, navbar.Attrs(
		"href", "/admin/telemetry/",
		// avoid htmx doing the request, it will try and
		// insert it into the page with our css/js (that's bad)
		"hx-boost", "false",
		"target", "_blank", // open a new tab for dashboard
	)),
	navbar.NewProtectedItem("Booth", radio.PermDJ, navbar.Attrs("href", "/admin/booth")),
	navbar.NewProtectedItem("Profile", radio.PermActive, navbar.Attrs("href", "/admin/profile")),
)

func NewState(
	_ context.Context,
	cfg config.Config,
	tv *templates.ThemeValues,
	daypass secret.Secret,
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
		ThemeConfig:      tv,
		Daypass:          daypass,
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

	ThemeConfig      *templates.ThemeValues
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
		r.Get("/proxy", p(radio.PermDJ, s.GetProxy))
		r.Post("/proxy/remove", p(radio.PermProxyKick, s.PostRemoveSource))
		r.Get("/booth", p(radio.PermDJ, s.GetBooth))
		r.Get("/booth/sse", p(radio.PermDJ, NewBoothAPI(s.Config, s.TemplateExecutor).ServeHTTP))
		r.Post("/booth/stop-streamer", p(radio.PermDJ, s.PostBoothStopStreamer))
		r.Post("/booth/set-thread", p(radio.PermDJ, s.PostBoothSetThread))

		// setup monitoring endpoint
		proxy := setupMonitoringProxy(s.Config)
		r.Handle("/telemetry/*", p(radio.PermTelemetryView, proxy.ServeHTTP))

		// debug handlers, might not be needed later
		r.Post("/api/streamer/stop", p(radio.PermAdmin, s.PostStreamerStop))
		r.Post("/api/website/reload-templates", p(radio.PermAdmin, s.PostReloadTemplates))
		r.Post("/api/website/set-holiday-theme", p(radio.PermAdmin, s.PostSetHolidayTheme))

		// error handlers
		r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
			shared.ErrorHandler(s.TemplateExecutor, w, r, shared.ErrMethodNotAllowed)
		})
		r.NotFound(func(w http.ResponseWriter, r *http.Request) {
			shared.ErrorHandler(s.TemplateExecutor, w, r, shared.ErrNotFound)
		})
	}
}

// PostStreamerStop stops the streamer forcefully
func (s *State) PostStreamerStop(w http.ResponseWriter, r *http.Request) {
	s.Streamer.Stop(r.Context(), true)
}

func (s *State) errorHandler(w http.ResponseWriter, r *http.Request, err error, msg string) {
	shared.ErrorHandler(s.TemplateExecutor, w, r, err)
}

func setupMonitoringProxy(cfg config.Config) *httputil.ReverseProxy {
	monitoringURL := config.Value(cfg, func(c config.Config) *url.URL {
		return c.Conf().Website.AdminMonitoringURL.URL()
	})
	monitoringUserHeader := config.Value(cfg, func(c config.Config) string {
		return c.Conf().Website.AdminMonitoringUserHeader
	})
	monitoringRoleHeader := config.Value(cfg, func(c config.Config) string {
		return c.Conf().Website.AdminMonitoringRoleHeader
	})

	// proxy to the grafana host
	return &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(monitoringURL())
			pr.SetXForwarded()
			pr.Out.Host = pr.In.Host

			u := vmiddleware.UserFromContext(pr.In.Context())
			pr.Out.Header.Add(monitoringUserHeader(), u.Username)
			if u.UserPermissions.Has(radio.PermAdmin) {
				pr.Out.Header.Add(monitoringRoleHeader(), "Admin")
			}
		},
	}
}
