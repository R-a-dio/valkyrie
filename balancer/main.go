package balancer

import (
	"context"
	"net/http"
	"time"

	"github.com/R-a-dio/valkyrie/config"
)

// Execute executes the balancer with the context ctx and config cfg.
// Execution of the balancer can be halted by cancelling ctx.
func Execute(ctx context.Context, cfg config.Config) error {
	br := NewBalancer(ctx, cfg)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	return br.start(ctx)
}

// NewBalancer returns an initialized Balancer.
func NewBalancer(ctx context.Context, cfg config.Config) *Balancer {
	c := cfg.Conf()
	br := &Balancer{
		Config:  cfg,
		Manager: c.Manager.Client(),
	}

	br.relays = c.Balancer.Relays

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
