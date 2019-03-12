package website

import (
	"context"
	"net"
	"net/http"

	"github.com/R-a-dio/valkyrie/config"
)

// Execute runs a website instance with the configuration given
func Execute(ctx context.Context, cfg config.Config) error {
	var mux http.Handler

	conf := cfg.Conf()
	server := &http.Server{
		Addr:    conf.Website.ListenAddr,
		Handler: mux,
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
