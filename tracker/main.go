package tracker

import (
	"context"
	"syscall"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/util"
	"github.com/Wessie/fdstore"
	"github.com/rs/zerolog"
)

const (
	// UpdateListenersTickrate is the period between two UpdateListeners
	// calls done to the manager
	UpdateListenersTickrate = time.Second * 10
	// SyncListenersTickrate is the period between two sync operations
	SyncListenersTickrate = time.Minute * 10

	RemoveStaleTickrate = time.Hour * 24
	RemoveStalePeriod   = time.Minute * 5
)

func Execute(ctx context.Context, cfg config.Config) error {

	fds := fdstore.NewStoreListenFDs()

	// setup recorder
	var recorder = NewRecorder(ctx, cfg)

	// setup periodic task to update the manager of our listener count
	go PeriodicallyUpdateListeners(ctx, cfg.Manager, recorder, UpdateListenersTickrate)
	// setup periodic task to keep recorder state in sync with icecast
	go PeriodicallySyncListeners(ctx, cfg, recorder, SyncListenersTickrate)

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

func PeriodicallySyncListeners(ctx context.Context, cfg config.Config,
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
			err := periodicallySyncListeners(ctx, cfg, recorder)
			if err != nil {
				zerolog.Ctx(ctx).Error().Err(err).Msg("failed sync listeners")
			}
		}
	}
}

func periodicallySyncListeners(ctx context.Context, cfg config.Config, recorder *Recorder) error {
	const op errors.Op = "tracker/periodicallySyncListeners"

	recorder.syncing.Store(true)
	defer recorder.syncing.Store(false)

	ctx, cancel := context.WithTimeout(ctx, time.Second*5)
	defer cancel()

	list, err := GetIcecastListClients(ctx, cfg)
	if err != nil {
		return errors.E(op, err)
	}

	recorder.Sync(ctx, list)
	return nil
}
