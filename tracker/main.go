package tracker

import (
	"context"
	"time"

	"github.com/R-a-dio/valkyrie/config"
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
				manager.UpdateListeners(ctx, recorder.ListenerAmount.Load())
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
