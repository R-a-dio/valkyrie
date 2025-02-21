package tracker

import (
	"context"

	"github.com/R-a-dio/valkyrie/cmd"
	"github.com/R-a-dio/valkyrie/config"
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
	case <-cmd.USR2Signal(ctx):
		if err := srv.Shutdown(ctx); err != nil {
			zerolog.Ctx(ctx).Error().Ctx(ctx).Err(err).Msg("failed to close server")
		}
		if err := srv.storeSelf(ctx, fds); err != nil {
			zerolog.Ctx(ctx).Error().Ctx(ctx).Err(err).Msg("failed to store self")
		}
		if err := fds.Send(); err != nil {
			zerolog.Ctx(ctx).Error().Ctx(ctx).Err(err).Msg("failed to send store")
		}
		return nil
	case err := <-errCh:
		return err
	}
}
