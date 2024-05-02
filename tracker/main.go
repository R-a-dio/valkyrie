package tracker

import (
	"context"
	"time"

	"github.com/R-a-dio/valkyrie/config"
	"github.com/rs/zerolog"
)

func Execute(ctx context.Context, cfg config.Config) error {
	manager := cfg.Conf().Manager.Client()

	var recorder = NewRecorder()
	go func() {
		ticker := time.NewTicker(time.Second * 10)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				err := manager.UpdateListeners(ctx, recorder.ListenerAmount())
				if err != nil {
					zerolog.Ctx(ctx).Error().Err(err).Msg("failed update listeners")
				}
			}
		}
	}()

	srv := NewServer(ctx, ":9999", recorder)

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		return srv.Close()
	case err := <-errCh:
		return err
	}
}
