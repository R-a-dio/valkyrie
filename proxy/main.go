package proxy

import (
	"context"
	"syscall"

	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/storage"
	"github.com/R-a-dio/valkyrie/util"
	"github.com/Wessie/fdstore"
)

func Execute(ctx context.Context, cfg config.Config) error {
	const op errors.Op = "proxy/Execute"

	// setup dependencies
	storage, err := storage.Open(ctx, cfg)
	if err != nil {
		return errors.E(op, err)
	}
	m := cfg.Manager

	srv, err := NewServer(ctx, cfg, m, storage)
	if err != nil {
		return errors.E(op, err)
	}

	fdstorage := fdstore.NewStore(fdstore.ListenFDs())

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start(ctx, fdstorage)
	}()

	select {
	case <-ctx.Done():
		return srv.Close()
	case <-util.Signal(syscall.SIGUSR2):
		srv.storeSelf(ctx, fdstorage)
		util.TrySendStore(ctx, fdstorage)
		return srv.Close()
	case err = <-errCh:
		return err
	}
}
