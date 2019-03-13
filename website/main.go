package website

import (
	"context"
	"net"
	"net/http"

	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/storage"
	"github.com/gorilla/mux"
)

// Execute runs a website instance with the configuration given
func Execute(ctx context.Context, cfg config.Config) error {
	storage, err := storage.Open(cfg)
	if err != nil {
		return err
	}

	streamer := cfg.Conf().Streamer.Client()
	manager := cfg.Conf().Manager.Client()

	api, err := NewAPIv0(ctx, storage, streamer, manager)
	if err != nil {
		return err
	}

	r := mux.NewRouter()
	r.Handle("/api", api.Route(r.PathPrefix("/api").Subrouter()))

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
