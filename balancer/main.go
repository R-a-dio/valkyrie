package balancer

import (
	"context"
	"net/http"
	"time"

	"github.com/R-a-dio/valkyrie/config"
)

func Execute(ctx context.Context, cfg config.Config) error {
	br := NewBalancer(ctx, cfg)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- br.start(ctx)
	}()
	select {
	case err := <-errCh:
		return err
	}
}

func NewBalancer(ctx context.Context, cfg config.Config) *Balancer {
	c := cfg.Conf()
	br := &Balancer{
		Config:  cfg,
		Manager: c.Manager.Client(),
	}

	br.current.Store(c.Balancer.Fallback)
	mux := http.NewServeMux()
	mux.HandleFunc("/", br.getIndex())
	mux.HandleFunc("/status", br.getStatus())
	mux.HandleFunc("/main", br.getMain())

	br.serv = &http.Server{
		Handler:      mux,
		Addr:         c.Balancer.Addr,
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}

	return br
}
