package tracker

import (
	"context"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/rs/zerolog"
)

const (
	UpdateListenersTickrate    = time.Second * 10
	RemoveStalePendingTickrate = time.Hour * 24
	RemoveStalePendingPeriod   = time.Minute * 5
)

func Execute(ctx context.Context, cfg config.Config) error {
	manager := cfg.Conf().Manager.Client()

	var recorder = NewRecorder(ctx)

	go PeriodicallyUpdateListeners(ctx, manager, recorder, UpdateListenersTickrate)

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

func PeriodicallyUpdateListeners(ctx context.Context,
	manager radio.ManagerService,
	recorder *Recorder,
	tickrate time.Duration,
) {
	ticker := time.NewTicker(tickrate)
	defer ticker.Stop()

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
}
