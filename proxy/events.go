package proxy

import (
	"context"
	"sync"
	"time"

	"github.com/R-a-dio/valkyrie/config"
	"github.com/rs/zerolog"
)

func NewEventHandler(ctx context.Context, cfg config.Config) *EventHandler {
	return &EventHandler{
		cfg:    cfg,
		logger: *zerolog.Ctx(ctx),
	}
}

type EventHandler struct {
	cfg    config.Config
	logger zerolog.Logger

	mu                 sync.Mutex
	lastLiveSourceSwap time.Time
}

func (eh *EventHandler) LiveSourceSwap(ctx context.Context, new *SourceClient) {
	// record when we were called since the goroutine might start running at
	// some other later time we use this to avoid logic races
	instant := time.Now()
	go func() {
		eh.mu.Lock()
		defer eh.mu.Unlock()

		if eh.lastLiveSourceSwap.After(instant) {
			// someone else already went live and was later, so eat
			// this event since it's out-dated
			return
		}

		err := eh.cfg.Manager.UpdateUser(ctx, &new.User)
		if err != nil {
			eh.logger.Error().Err(err).Msg("failed to update user")
			return
		}

		// update the last update time
		eh.lastLiveSourceSwap = instant
	}()
}
