package admin

import (
	"context"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/telemetry/reverseproxy"
	"github.com/R-a-dio/valkyrie/templates"
	"github.com/R-a-dio/valkyrie/util/secret"
	vmiddleware "github.com/R-a-dio/valkyrie/website/middleware"
	"github.com/R-a-dio/valkyrie/website/shared"
	"github.com/R-a-dio/valkyrie/website/shared/navbar"
	"github.com/spf13/afero"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
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
	ctx context.Context,
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
	var rvp *httputil.ReverseProxy
	if !cfg.Conf().Telemetry.StandaloneProxy.Enabled {
		rvp = reverseproxy.New(ctx, cfg)
	}

	return State{
		Config:           NewConfig(cfg),
		TelemetryProxy:   rvp,
		ThemeConfig:      tv,
		Daypass:          daypass,
		SongSecret:       songSecret,
		News:             newsCache,
		Storage:          storage,
		Search:           search,
		Guest:            cfg.Guest,
		Proxy:            cfg.Proxy,
		Streamer:         cfg.Streamer,
		Manager:          cfg.Manager,
		Queue:            cfg.Queue,
		Tracker:          cfg.Tracker,
		Templates:        siteTmpl,
		TemplateExecutor: exec,
		SessionManager:   sessionManager,
		Authentication:   auth,
		FS:               fs,
	}
}

type State struct {
	// Config is the configuration for the admin panel
	Config Config
	// TelemetryProxy is the reverseproxy used to access the telemetry backend or nil if it isn't enabled
	TelemetryProxy *httputil.ReverseProxy
	// ThemeConfig is the theme state for the website
	ThemeConfig *templates.ThemeValues
	// Daypass is the submission daypass
	Daypass secret.Secret
	// SongSecret is the secret used for generating song download urls
	SongSecret secret.Secret
	// News is a markdown renderer cache for the news posts
	News *shared.NewsCache
	// other services
	Storage  radio.StorageService
	Search   radio.SearchService
	Guest    radio.GuestService
	Proxy    radio.ProxyService
	Streamer radio.StreamerService
	Manager  radio.ManagerService
	Queue    radio.QueueService
	Tracker  radio.ListenerTrackerService

	// Templates is the actual Site collection of templates, used to
	// be able to reload templates from the admin panel
	Templates *templates.Site
	// TemplateExecutor is the actual executor handlers should be using
	// when executing templates
	TemplateExecutor templates.Executor
	// SessionManager is the user session manager state
	SessionManager *scs.SessionManager
	// Authentication is the configuration authentication middleware, this is
	// used to protect admin pages from unauthenticated requests
	Authentication vmiddleware.Authentication
	// FS is the filesystem used to access files in any admin handlers
	FS afero.Fs
}

func Route(ctx context.Context, s State) func(chi.Router) {
	return func(r chi.Router) {
		// the login middleware will require atleast the active permission
		r = r.With(
			s.Authentication.LoginMiddleware,
			middleware.NoCache,
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
		r.Get("/booth/sse", p(radio.PermDJ, s.sseBoothAPI))
		r.Post("/booth/stop-streamer", p(radio.PermDJ, s.PostBoothStopStreamer))
		r.Post("/booth/set-thread", p(radio.PermDJ, s.PostBoothSetThread))

		// setup monitoring endpoint
		if s.TelemetryProxy != nil {
			r.Handle("/telemetry/*", p(radio.PermTelemetryView, s.TelemetryProxy.ServeHTTP))
		} else {
			r.Handle("/telemetry/*", p(radio.PermTelemetryView, func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, s.Config.TelemetryProxyURL(), http.StatusFound)
			}))
		}

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

type Config struct {
	StreamerConnectTimeout func() time.Duration
	MusicPath              func() string
	DJImagePath            func() string
	DJImageMaxSize         func() int64
	TelemetryProxyURL      func() string
	BoothStreamURL         func() *url.URL
}

func NewConfig(cfg config.Config) Config {
	return Config{
		StreamerConnectTimeout: config.Value(cfg, func(cfg config.Config) time.Duration {
			return time.Duration(cfg.Conf().Streamer.ConnectTimeout)
		}),
		MusicPath: config.Value(cfg, func(cfg config.Config) string {
			return cfg.Conf().MusicPath
		}),
		DJImagePath: config.Value(cfg, func(cfg config.Config) string {
			return cfg.Conf().Website.DJImagePath
		}),
		DJImageMaxSize: config.Value(cfg, func(cfg config.Config) int64 {
			return cfg.Conf().Website.DJImageMaxSize
		}),
		TelemetryProxyURL: config.Value(cfg, func(c config.Config) string {
			return string(cfg.Conf().Telemetry.StandaloneProxy.URL)
		}),
		BoothStreamURL: config.Value(cfg, func(c config.Config) *url.URL {
			return cfg.Conf().Manager.GuestProxyAddr.URL()
		}),
	}
}
