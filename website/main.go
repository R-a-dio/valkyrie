package website

import (
	"context"
	"net"
	"net/http"

	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/storage"
	"github.com/R-a-dio/valkyrie/templates"
	"github.com/R-a-dio/valkyrie/website/admin"
	phpapi "github.com/R-a-dio/valkyrie/website/api/php"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
)

// Execute runs a website instance with the configuration given
func Execute(ctx context.Context, cfg config.Config) error {
	const op errors.Op = "website/Execute"

	// database access
	storage, err := storage.Open(cfg)
	if err != nil {
		return errors.E(op, err, "failed to open storage")
	}
	// RPC clients
	streamer := cfg.Conf().Streamer.Client()
	manager := cfg.Conf().Manager.Client()
	// templates
	tmpl, err := templates.LoadTemplates(cfg.Conf().TemplatePath)
	if err != nil {
		return errors.E(op, err, "failed to load templates")
	}

	r := chi.NewRouter()
	// TODO(wessie): check if nginx is setup to send the correct headers for real IP
	// passthrough, as it's required for request handling
	r.Use(middleware.RealIP)
	r.Use(removePortFromAddress)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// legacy urls that once pointed to our stream, redirect them to the new url
	r.Get("/main.mp3", RedirectLegacyStream)
	r.Get("/main", RedirectLegacyStream)
	r.Get("/stream.mp3", RedirectLegacyStream)
	r.Get("/stream", RedirectLegacyStream)
	r.Get("/R-a-dio", RedirectLegacyStream)

	// version 0 of the api (the legacy PHP version)
	// it's mostly self-contained to the /api/* route, except for /request that
	// leaked out at some point
	v0, err := phpapi.NewAPI(ctx, cfg, storage, streamer, manager)
	if err != nil {
		return err
	}
	r.Mount("/api", v0.Router())
	r.Route(`/request/{TrackID:[0-9]+}`, v0.RequestRoute)

	// admin routes
	r.Mount("/admin", admin.Router(ctx, admin.State{
		Config:    cfg,
		Storage:   storage,
		Templates: tmpl,
	}))
	// other routes
	// other routes
	// other routes

	conf := cfg.Conf()
	server := &http.Server{
		Addr:    conf.Website.WebsiteAddr,
		Handler: r,
	}

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
