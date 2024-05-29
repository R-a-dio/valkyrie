package streamer

import (
	"context"
	"net"
	"syscall"

	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/storage"
	"github.com/R-a-dio/valkyrie/util"
	"github.com/Wessie/fdstore"
	"github.com/rs/zerolog"
)

// Execute starts a streamer instance and its RPC API server
func Execute(ctx context.Context, cfg config.Config) error {
	store, err := storage.Open(ctx, cfg)
	if err != nil {
		return err
	}

	zerolog.Ctx(ctx).Info().Msg("setting up queue")
	queue, err := NewQueueService(ctx, cfg, store)
	if err != nil {
		return err
	}

	fdstorage := fdstore.NewStoreListenFDs()

	zerolog.Ctx(ctx).Info().Msg("setting up streamer")
	streamer, err := NewStreamer(ctx, cfg, fdstorage, queue, store.User(ctx))
	if err != nil {
		return err
	}

	zerolog.Ctx(ctx).Info().Msg("starting grpc server")
	// setup a http server for our RPC API
	srv, err := NewGRPCServer(ctx, cfg, store, queue, cfg.IRC, streamer)
	if err != nil {
		return err
	}
	defer srv.Stop()

	ln, err := net.Listen("tcp", cfg.Conf().Streamer.RPCAddr.String())
	if err != nil {
		return err
	}
	zerolog.Ctx(ctx).Info().Str("address", ln.Addr().String()).Msg("started grpc server")

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Serve(ln)
	}()

	// wait for our context to be canceled or Serve to error out
	select {
	case <-util.Signal(syscall.SIGUSR2):
		zerolog.Ctx(ctx).Info().Msg("SIGUSR2 received")
		if err := streamer.handleRestart(ctx); err != nil {
			zerolog.Ctx(ctx).Error().Err(err).Msg("failed to handle restart")
		}
		if err := fdstorage.Send(); err != nil {
			zerolog.Ctx(ctx).Error().Err(err).Msg("failed to send store")
		}
		return nil
	case <-ctx.Done():
		return nil
	case err = <-errCh:
		streamer.Stop(ctx, true)
		return err
	}
}
