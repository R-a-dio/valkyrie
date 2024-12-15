package jobs

import (
	"context"

	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/storage"
	"github.com/R-a-dio/valkyrie/util"
)

// ExecuteListenerLog fetches the listener count from the manager and inserts a line into
// the listenlog table.
func ExecuteListenerLog(ctx context.Context, cfg config.Config) error {
	store, err := storage.Open(ctx, cfg)
	if err != nil {
		return err
	}
	defer store.Close()

	m := cfg.Manager

	status, err := util.OneOff(ctx, m.CurrentStatus)
	if err != nil {
		return err
	}

	return store.User(ctx).RecordListeners(status.Listeners, status.User)
}
