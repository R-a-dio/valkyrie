package website

import (
	"context"
	"log"
	"net"
	"net/http"

	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/storage"
	"github.com/R-a-dio/valkyrie/templates"
	"github.com/R-a-dio/valkyrie/website/admin"
	phpapi "github.com/R-a-dio/valkyrie/website/api/php"
	v1 "github.com/R-a-dio/valkyrie/website/api/v1"
	vmiddleware "github.com/R-a-dio/valkyrie/website/middleware"
	"github.com/R-a-dio/valkyrie/website/public"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// Execute runs a website instance with the configuration given
func Execute(ctx context.Context, cfg config.Config) error {
	const op errors.Op = "website/Execute"

	if cfg.Conf().Website.DJImagePath == "" {
		return errors.E(op, "Website.DJImagePath is not configured")
	}

	// database access
	storage, err := storage.Open(cfg)
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

	r := chi.NewRouter()
	// TODO(wessie): check if nginx is setup to send the correct headers for real IP
	// passthrough, as it's required for request handling
	r.Use(middleware.RealIP)
	r.Use(removePortFromAddress)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	// session handling
	sessionManager := vmiddleware.NewSessionManager(ctx, storage)
	r.Use(sessionManager.LoadAndSave)
	// user handling
	authentication := vmiddleware.NewAuthentication(storage, executor, sessionManager)
	r.Use(authentication.UserMiddleware)

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
	log.Println("starting v0 api")
	v0, err := phpapi.NewAPI(ctx, cfg, storage, streamer, manager)
	if err != nil {
		return errors.E(op, err)
	}
	r.Mount("/api", v0.Router())
	r.Route(`/request/{TrackID:[0-9]+}`, v0.RequestRoute)

	log.Println("starting v1 api")
	v1, err := v1.NewAPI(ctx, cfg)
	if err != nil {
		return errors.E(op, err)
	}
	r.Mount("/v1", v1.Router())

	// admin routes
	r.Get("/logout", authentication.LogoutHandler) // outside so it isn't login restricted
	r.Mount("/admin", admin.Router(ctx, admin.State{
		Config:           cfg,
		Storage:          storage,
		Templates:        siteTemplates,
		TemplateExecutor: executor,
		SessionManager:   sessionManager,
		Authentication:   authentication,
	}))

	// public routes
	r.Mount("/", public.Router(ctx, public.State{
		Config:           cfg,
		Templates:        siteTemplates,
		TemplateExecutor: siteTemplates.Executor(),
		Manager:          manager,
		Streamer:         streamer,
		Storage:          storage,
	}))

	// setup the http server
	conf := cfg.Conf()
	server := &http.Server{
		Addr:    conf.Website.WebsiteAddr,
		Handler: r,
	}

	log.Println("website listening on:", server.Addr)
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
