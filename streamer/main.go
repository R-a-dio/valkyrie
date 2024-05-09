package streamer

import (
	"context"
	"net"

	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/storage"
)

// Execute starts a streamer instance and its RPC API server
func Execute(ctx context.Context, cfg config.Config) error {
	store, err := storage.Open(ctx, cfg)
	if err != nil {
		return err
	}

	queue, err := NewQueueService(ctx, cfg, store)
	if err != nil {
		return err
	}

	streamer, err := NewStreamer(ctx, cfg, queue)
	if err != nil {
		return err
	}
	defer streamer.ForceStop(context.Background())

	// announcement service
	announce := cfg.IRC()

	// setup a http server for our RPC API
	srv, err := NewHTTPServer(cfg, store, queue, announce, streamer)
	if err != nil {
		return err
	}
	defer srv.Stop()

	ln, err := net.Listen("tcp", cfg.Conf().Streamer.ListenAddr)
	if err != nil {
		return err
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Serve(ln)
	}()

	// wait for our context to be canceled or Serve to error out
	select {
	case <-ctx.Done():
		return nil
	case err = <-errCh:
		return err
	}
}
