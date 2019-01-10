package jobs

import (
	"context"

	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/database"
	"github.com/R-a-dio/valkyrie/rpc/manager"
)

const insertLog = `INSERT INTO listenlog (listeners, dj) VALUES (?, ?);`

// ExecuteListenerLog fetches the listener count from the manager and inserts a line into
// the listenlog table.
func ExecuteListenerLog(ctx context.Context, cfg config.Config) error {
	db, err := database.Connect(cfg)
	if err != nil {
		return err
	}

	m := cfg.Conf().Manager.TwirpClient()

	status, err := m.Status(ctx, &manager.StatusRequest{})
	if err != nil {
		return err
	}

	h := database.Handle(ctx, db)
	_, err = h.Exec(insertLog, status.ListenerInfo.Listeners, status.User.Id)
	if err != nil {
		return err
	}

	return nil
}
