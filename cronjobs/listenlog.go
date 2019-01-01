package cronjobs

import (
	"context"
	"net/http"

	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/database"
	"github.com/R-a-dio/valkyrie/rpc/manager"
)

const insertLog = `INSERT INTO listenlog (listeners, dj) VALUES (?, ?);`

// ListenLog fetches the listener count from the manager and inserts a line into
// the listenlog table.
func ListenLog(errCh chan<- error) config.StateStart {
	return func(s *config.State) (config.StateDefer, error) {
		go func() {
			m := manager.NewManagerProtobufClient(
				s.Conf().Streamer.Addr, http.DefaultClient)

			status, err := m.Status(context.TODO(), &manager.StatusRequest{})
			if err != nil {
				errCh <- err
				return
			}

			h := database.Handle(context.TODO(), s.DB)
			_, err = h.Exec(insertLog, status.ListenerInfo.Listeners,
				status.User.Id)
			if err != nil {
				errCh <- err
				return
			}

			errCh <- nil
		}()

		return nil, nil
	}
}
