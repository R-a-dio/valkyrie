package proxy

import (
	"context"

	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/proxy/compat"
	"github.com/R-a-dio/valkyrie/storage"
	"github.com/rs/zerolog"
)

func Execute(ctx context.Context, cfg config.Config) error {
	const op errors.Op = "proxy/Execute"
	logger := zerolog.Ctx(ctx)

	// setup dependencies
	storage, err := storage.Open(ctx, cfg)
	if err != nil {
		return errors.E(op, err)
	}
	m := cfg.Conf().Manager.Client()

	// get our configuration
	addr := cfg.Conf().Proxy.Address

	srv, err := NewServer(ctx, m, storage)
	if err != nil {
		return errors.E(op, err)
	}

	logger.Info().Str("address", addr).Msg("proxy started listening")
	ln, err := compat.Listen(logger, "tcp", addr)
	if err != nil {
		return errors.E(op, err)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Serve(ln)
	}()

	select {
	case <-ctx.Done():
		return srv.Close()
	case err = <-errCh:
		return err
	}
}
