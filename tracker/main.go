package tracker

import (
	"context"
	"syscall"

	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/util"
	"github.com/Wessie/fdstore"
	"github.com/rs/zerolog"
)

func Execute(ctx context.Context, cfg config.Config) error {

	fds := fdstore.NewStoreListenFDs()

	srv := NewServer(ctx, cfg)

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start(ctx, fds)
	}()

	select {
	case <-ctx.Done():
		return srv.Close()
	case <-util.Signal(syscall.SIGUSR2):
		if err := srv.storeSelf(ctx, fds); err != nil {
			zerolog.Ctx(ctx).Error().Err(err).Msg("failed to store self")
		}
		if err := fds.Send(); err != nil {
			zerolog.Ctx(ctx).Error().Err(err).Msg("failed to send store")
		}
		return srv.Close()
	case err := <-errCh:
		return err
	}
}
