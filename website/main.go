package website

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/search"
	"github.com/R-a-dio/valkyrie/storage"
	"github.com/R-a-dio/valkyrie/templates"
	"github.com/R-a-dio/valkyrie/util/daypass"
	"github.com/R-a-dio/valkyrie/website/admin"
	phpapi "github.com/R-a-dio/valkyrie/website/api/php"
	v1 "github.com/R-a-dio/valkyrie/website/api/v1"
	vmiddleware "github.com/R-a-dio/valkyrie/website/middleware"
	"github.com/R-a-dio/valkyrie/website/public"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/hlog"
	"github.com/spf13/afero"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

var NewRouter = func() chi.Router { return chi.NewRouter() }

func zerologLoggerFunc(r *http.Request, status, size int, duration time.Duration) {
	hlog.FromRequest(r).Info().
		Int("status_code", status).
		Int("response_size_bytes", size).
		Dur("elapsed_ms", duration).
		Msg("http request")
}

// Execute runs a website instance with the configuration given
func Execute(ctx context.Context, cfg config.Config) error {
	const op errors.Op = "website/Execute"
	logger := zerolog.Ctx(ctx)

	if cfg.Conf().Website.DJImagePath == "" {
		return errors.E(op, "Website.DJImagePath is not configured")
	}

	// database access
	storage, err := storage.Open(ctx, cfg)
	if err != nil {
		return errors.E(op, err)
	}
	// RPC clients
	streamer := cfg.Conf().Streamer.Client()
	manager := cfg.Conf().Manager.Client()
	// templates
	siteTemplates, err := templates.FromDirectory(cfg.Conf().TemplatePath)
	if err != nil {
		return errors.E(op, err)
	}
	executor := siteTemplates.Executor()
	// daypass generation
	dpass := daypass.New(ctx)
	// search service
	searchService, err := search.Open(ctx, cfg)
	if err != nil {
		return errors.E(op, err)
	}

	r := NewRouter()

	// TODO(wessie): check if nginx is setup to send the correct headers for real IP
	// passthrough, as it's required for request handling
	r.Use(middleware.RealIP)
	// setup zerolog details
	r.Use(
		hlog.NewHandler(*logger),
		hlog.RemoteAddrHandler("ip"),
		hlog.UserAgentHandler("user_agent"),
		hlog.RequestIDHandler("req_id", "Request-Id"), // TODO: check if we want to return the header
		hlog.URLHandler("url"),
		hlog.MethodHandler("method"),
		hlog.ProtoHandler("protocol"),
		hlog.CustomHeaderHandler("is_htmx", "Hx-Request"),
		hlog.AccessHandler(zerologLoggerFunc),
	)
	// recover from panics and clean our IP of a port number
	r.Use(removePortFromAddress)
	r.Use(middleware.Recoverer)
	// session handling
	sessionManager := vmiddleware.NewSessionManager(ctx, storage)
	r.Use(sessionManager.LoadAndSave)
	// user handling
	authentication := vmiddleware.NewAuthentication(storage, executor, sessionManager)
	r.Use(authentication.UserMiddleware)
	// shared input handling, stuff the base template needs
	r.Use(vmiddleware.InputMiddleware(cfg))
	// theme state management
	r.Use(templates.ThemeCtx(storage))

	// legacy urls that once pointed to our stream, redirect them to the new url
	r.Get("/main.mp3", RedirectLegacyStream)
	r.Get("/main", RedirectLegacyStream)
	r.Get("/stream.mp3", RedirectLegacyStream)
	r.Get("/stream", RedirectLegacyStream)
	r.Get("/R-a-dio", RedirectLegacyStream)

	// serve assets from the assets directory
	fs := http.FileServer(http.Dir(cfg.Conf().AssetsPath))
	r.Handle("/assets/*", http.StripPrefix("/assets/", fs))

	// version 0 of the api (the legacy PHP version)
	// it's mostly self-contained to the /api/* route, except for /request that
	// leaked out at some point
	logger.Info().Str("event", "init").Str("part", "api_v0").Msg("")
	v0, err := phpapi.NewAPI(ctx, cfg, storage, streamer, manager)
	if err != nil {
		return errors.E(op, err)
	}
	r.Route("/api", v0.Route)
	r.Route(`/request/{TrackID:[0-9]+}`, v0.RequestRoute)

	logger.Info().Str("event", "init").Str("part", "api_v1").Msg("")
	v1, err := v1.NewAPI(ctx, cfg, executor)
	if err != nil {
		return errors.E(op, err)
	}
	r.Route("/v1", v1.Route)

	// admin routes
	r.Get("/logout", authentication.LogoutHandler) // outside so it isn't login restricted
	r.Route("/admin", admin.Route(ctx, admin.NewState(
		ctx,
		cfg,
		dpass,
		storage,
		siteTemplates,
		executor,
		sessionManager,
		authentication,
		afero.NewOsFs(),
	)))

	// public routes
	r.Route("/", public.Route(ctx, public.NewState(
		ctx,
		cfg,
		dpass,
		executor,
		manager,
		streamer,
		storage,
		searchService,
	)))

	// setup the http server
	conf := cfg.Conf()
	server := &http.Server{
		Addr:    conf.Website.WebsiteAddr,
		Handler: r,
	}

	zerolog.Ctx(ctx).Info().Str("address", server.Addr).Msg("website started listening")
	ln, err := net.Listen("tcp", server.Addr)
	if err != nil {
		return err
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Serve(ln)
	}()

	select {
	case <-ctx.Done():
		return server.Close()
	case err = <-errCh:
		return err
	}
}

// RedirectLegacyStream redirects a request to the (new) icecast stream url
func RedirectLegacyStream(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Location", "//stream.r-a-d.io/main")
	w.WriteHeader(http.StatusMovedPermanently)
}

// removePortFromAddress is a middleware that should be behind RealIP to
// remove any potential port number that is present on r.RemoteAddr, since
// we use it to identify users and don't want the port to be used in those
// cases.
//
// This middleware will panic if the address is unparseable by net.SplitHostPort
func removePortFromAddress(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.RemoteAddr == "" {
			panic("removePortFromAddress: empty address")
		}
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			// constant used by the net package
			const missingPort = "missing port in address"
			aerr, ok := err.(*net.AddrError)
			if !ok || aerr.Err != missingPort {
				panic("removePortFromAddress: " + err.Error())
			}
		} else {
			// only set it if there is no error, if there was an error we will
			// either panic above, or there was no port involved and so we
			// don't need to touch the RemoteAddr
			r.RemoteAddr = host
		}
		next.ServeHTTP(w, r)
	})
}
