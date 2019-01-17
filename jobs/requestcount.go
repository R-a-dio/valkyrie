package jobs

import (
	"context"
	"log"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/database"

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

// ExecuteRequestCount drops the requestcount of all tracks by 1 if they have not been
// requested within the specified duration.
func ExecuteRequestCount(ctx context.Context, cfg config.Config) error {
	db, err := database.Connect(cfg)
	if err != nil {
		return err
	}

	h, err := database.HandleTx(ctx, db)
	if err != nil {
		return err
	}
	defer h.Rollback()

	var ids = []radio.TrackID{}
	err = sqlx.Select(h, &ids, selectRC, duration.Seconds())
	if err != nil {
		return err
	}

	_, err = h.Exec(updateRC, duration.Seconds())
	if err != nil {
		return err
	}
	err = h.Commit()
	if err != nil {
		return err
	}

	// TODO: update search index for the specified tracks

	log.Printf("requestcount: processed %d tracks\n", len(ids))
	return nil
}
