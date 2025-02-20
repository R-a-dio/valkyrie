package public

import (
	"context"
	"net/http"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/templates"
	"github.com/R-a-dio/valkyrie/util/secret"
	"github.com/R-a-dio/valkyrie/website/shared"
	"github.com/R-a-dio/valkyrie/website/shared/navbar"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func NewState(
	ctx context.Context,
	cfg config.Config,
	dp secret.Secret,
	newsCache *shared.NewsCache,
	exec templates.Executor,
	storage radio.StorageService,
	search radio.SearchService) State {

	return State{
		Config:    NewConfig(cfg),
		Daypass:   dp,
		News:      newsCache,
		Templates: exec,
		Manager:   cfg.Manager,
		Queue:     cfg.Queue,
		Storage:   storage,
		Search:    search,
	}
}

type State struct {
	Config Config

	Daypass   secret.Secret
	News      *shared.NewsCache
	Templates templates.Executor
	Manager   radio.ManagerService
	Queue     radio.QueueService
	Storage   radio.StorageService
	Search    radio.SearchService
}

var NavBar = navbar.New(`hx-boost="true" hx-push-url="true" hx-target="#content"`,
	navbar.NewItem("Home", navbar.Attrs("href", "/")),
	navbar.NewItem("News", navbar.Attrs("href", "/news")),
	navbar.NewItem("Help", navbar.Attrs("href", "/help")),
	navbar.NewItem("Chat", navbar.Attrs("href", "/irc")),
	navbar.NewItem("Search", navbar.Attrs("href", "/search")),
	navbar.NewItem("Schedule", navbar.Attrs("href", "/schedule")),
	navbar.NewItem("Last Played", navbar.Attrs("href", "/last-played")),
	navbar.NewItem("Queue", navbar.Attrs("href", "/queue")),
	navbar.NewItem("Favorites", navbar.Attrs("href", "/faves")),
	navbar.NewItem("Staff", navbar.Attrs("href", "/staff")),
	navbar.NewItem("Submit", navbar.Attrs("href", "/submit")),
)

func Route(ctx context.Context, s State) func(chi.Router) {
	return func(r chi.Router) {
		r.Use(middleware.StripSlashes)

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
		r.Get("/faves/{Nick}", s.GetFavesOld)
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

type Config struct {
	UserRequestDelay func() time.Duration
	UserUploadDelay  func() time.Duration
	MusicPath        func() string
	AkismetKey       func() string
	AkismetBlog      func() string
}

func NewConfig(cfg config.Config) Config {
	return Config{
		UserRequestDelay: config.Value(cfg, func(cfg config.Config) time.Duration {
			return time.Duration(cfg.Conf().UserRequestDelay)
		}),
		UserUploadDelay: config.Value(cfg, func(cfg config.Config) time.Duration {
			return time.Duration(cfg.Conf().UserUploadDelay)
		}),
		AkismetKey: config.Value(cfg, func(cfg config.Config) string {
			return cfg.Conf().Website.AkismetKey
		}),
		AkismetBlog: config.Value(cfg, func(cfg config.Config) string {
			return cfg.Conf().Website.AkismetBlog
		}),
		MusicPath: config.Value(cfg, func(cfg config.Config) string {
			return cfg.Conf().MusicPath
		}),
	}
}
