package cronjobs

import (
	"context"
	"log"
	"time"

	"github.com/R-a-dio/valkyrie/database"
	"github.com/R-a-dio/valkyrie/engine"

	"github.com/jmoiron/sqlx"
)

const duration = time.Hour * 24 * 11
const (
	selectRC = `SELECT id FROM tracks 
				WHERE UNIX_TIMESTAMP(NOW()) - UNIX_TIMESTAMP(lastrequested) > ?
				AND requestcount > 0;`
	updateRC = `UPDATE tracks SET requestcount=requestcount - 1 
				WHERE UNIX_TIMESTAMP(NOW()) - UNIX_TIMESTAMP(lastrequested) > ?
				AND requestcount > 0;`
)

// RequestCount drops the requestcount of all tracks by 1 if they have not been
// requested within the specified duration.
func RequestCount(errCh chan<- error) engine.StartFn {
	return func(e *engine.Engine) (engine.DeferFn, error) {
		go func() {
			h, err := database.HandleTx(context.TODO(), e.DB)
			if err != nil {
				errCh <- err
				return
			}
			defer h.Rollback()

			var ids = []database.TrackID{}
			err = sqlx.Select(h, &ids, selectRC, duration.Seconds())
			if err != nil {
				errCh <- err
				return
			}

			_, err = h.Exec(updateRC, duration.Seconds())
			if err != nil {
				errCh <- err
				return
			}
			err = h.Commit()
			if err != nil {
				errCh <- err
				return
			}

			// TODO: update search index for the specified tracks

			log.Printf("requestcount: processed %d tracks\n", len(ids))
			errCh <- nil
		}()

		return nil, nil
	}
}
