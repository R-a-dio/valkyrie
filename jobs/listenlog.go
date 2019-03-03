package jobs

import (
	"context"

	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/database"
)

// ExecuteListenerLog fetches the listener count from the manager and inserts a line into
// the listenlog table.
func ExecuteListenerLog(ctx context.Context, cfg config.Config) error {
	storage, err := database.Open(cfg)
	if err != nil {
		return err
	}

	m := cfg.Conf().Manager.Client()

	status, err := m.Status(ctx)
	if err != nil {
		return err
	}

	return storage.User(ctx).RecordListeners(status.Listeners, status.User)
}
