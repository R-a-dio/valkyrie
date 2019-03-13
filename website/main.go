package website

import (
	"context"
	"net"
	"net/http"

	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/storage"
	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
)

// Execute runs a website instance with the configuration given
func Execute(ctx context.Context, cfg config.Config) error {
	storage, err := storage.Open(cfg)
	if err != nil {
		return err
	}

	streamer := cfg.Conf().Streamer.Client()
	manager := cfg.Conf().Manager.Client()

	r := chi.NewRouter()
	// TODO(wessie): check if nginx is setup to send the correct headers for real IP
	// passthrough, as it's required for request handling
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// legacy urls that once pointed to our stream, redirect them to the new url
	r.Get("/main.mp3", RedirectLegacyStream)
	r.Get("/main", RedirectLegacyStream)
	r.Get("/stream.mp3", RedirectLegacyStream)
	r.Get("/stream", RedirectLegacyStream)
	r.Get("/R-a-dio", RedirectLegacyStream)
	// version 0 of the api (the legacy PHP version)
	api, err := NewAPIv0(ctx, cfg, storage, streamer, manager)
	if err != nil {
		return err
	}
	r.Route("/api", api.Route)

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
