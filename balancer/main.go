package balancer

import (
	"context"
	"net/http"
	"time"

	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/storage"
)

// Execute executes the balancer with the context ctx and config cfg.
// Execution of the balancer can be halted by cancelling ctx.
func Execute(ctx context.Context, cfg config.Config) error {
	br, err := NewBalancer(ctx, cfg)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	return br.start(ctx)
}

// NewBalancer returns an initialized Balancer.
func NewBalancer(ctx context.Context, cfg config.Config) (*Balancer, error) {
	const op errors.Op = "balancer/NewBalancer"

	c := cfg.Conf()
	store, err := storage.Open(cfg)
	if err != nil {
		return nil, errors.E(op, err)
	}

	br := &Balancer{
		Config:  cfg,
		storage: store,
		manager: c.Manager.Client(),
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

	return br, nil
}
