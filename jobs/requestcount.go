package jobs

import (
	"context"
	"log"
	"time"

	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/search"
	"github.com/R-a-dio/valkyrie/storage"
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
	store, err := storage.Open(cfg)
	if err != nil {
		return err
	}

	ts, tx, err := store.TrackTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	before := time.Now().Add(-duration)
	err = ts.DecrementRequestCount(before)
	if err != nil {
		return err
	}

	songs, err := ts.BeforeLastRequested(before)
	if err != nil {
		return err
	}

	err = tx.Commit()
	if err != nil {
		return err
	}

	// update search index
	search, err := search.NewElasticSearchService(ctx, cfg)
	if err != nil {
		return err
	}

	err = search.Update(ctx, songs...)
	if err != nil {
		return err
	}

	log.Printf("requestcount: processed %d tracks\n", len(songs))
	return nil
}
