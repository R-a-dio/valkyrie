package database

import (
	"context"
	"fmt"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/jmoiron/sqlx"
)

// NewQueueStorage creates a new QueueStorage that is backed by the database
func NewQueueStorage(db *sqlx.DB) radio.QueueStorage {
	return QueueStorage{db}
}

// QueueStorage is a radio.QueueStorage backed by a sql database
type QueueStorage struct {
	db *sqlx.DB
}

type queueSong struct {
	ExpectedStartTime time.Time
	UserIdentifier    string
	IsRequest         int
	databaseTrack
}

// Store stores the queue given under name in the database configured
//
// Implements radio.QueueStorage
func (qs QueueStorage) Store(ctx context.Context, name string, queue []radio.QueueEntry) error {
	tx, err := HandleTx(ctx, qs.db)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// empty the queue so we can repopulate it
	_, err = tx.Exec(`DELETE FROM queue`)
	if err != nil {
		return err
	}

	var query = `
	INSERT INTO
		queue (trackid, time, ip, type, meta, length, id)
	VALUES
		(?, ?, ?, ?, ?, ?, ?);
	`
	for i, entry := range queue {
		if !entry.HasTrack() {
			return fmt.Errorf("queue storage: song with no track found in queue: %v", entry)
		}

		var isRequest = 0
		if entry.IsUserRequest {
			isRequest = 1
		}

		_, err = tx.Exec(query,
			entry.TrackID,
			entry.ExpectedStartTime,
			entry.UserIdentifier,
			isRequest,
			entry.Metadata,
			entry.Length.Seconds(),
			i+1, // ordering id
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// Load loads the queue name given from the database configured
//
// Implements radio.QueueStorage
func (qs QueueStorage) Load(ctx context.Context, name string) ([]radio.QueueEntry, error) {
	tx, err := HandleTx(ctx, qs.db)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var query = `
	SELECT
		queue.trackid,
		queue.time AS expectedstarttime,
		queue.ip AS useridentifier,
		queue.type AS isrequest,
		queue.meta AS metadata,
		queue.length, 
		(
			SELECT
				dt
			FROM eplay
			WHERE 
				eplay.isong = esong.id
			ORDER BY dt DESC
			LIMIT 1
		) AS lastplayed,
		esong.id AS id,
		esong.hash AS hash,
		tracks.artist,
		tracks.track,
		tracks.album,
		tracks.path,
		tracks.tags,
		tracks.accepter AS acceptor,
		tracks.lasteditor,
		tracks.priority,
		tracks.usable,
		tracks.lastrequested,
		tracks.requestcount
	FROM queue 
		LEFT JOIN tracks ON queue.trackid = tracks.id
		LEFT JOIN esong ON tracks.hash = esong.hash
	ORDER BY 
		queue.id ASC;
	`

	var queue []queueSong

	err = sqlx.Select(tx, &queue, query)
	if err != nil {
		return nil, err
	}

	songs := make([]radio.QueueEntry, len(queue))
	for i, qSong := range queue {
		songs[i] = radio.QueueEntry{
			Song:              qSong.ToSong(),
			IsUserRequest:     qSong.IsRequest == 1,
			UserIdentifier:    qSong.UserIdentifier,
			ExpectedStartTime: qSong.ExpectedStartTime,
		}
	}

	return songs, tx.Commit()
}

func QueuePopulate(h Handler) ([]radio.TrackID, error) {
	var query = `
		SELECT
			id
		FROM
			tracks
		WHERE
			usable=1
		ORDER BY (
			UNIX_TIMESTAMP(lastplayed) + 1)*(UNIX_TIMESTAMP(lastrequested) + 1) 
		ASC LIMIT 100;
	`
	var candidates = []radio.TrackID{}
	err := sqlx.Select(h, &candidates, query)
	if err != nil {
		return nil, err
	}

	return candidates, nil
}

func QueueUpdateTrack(h Handler, id radio.TrackID) error {
	var query = `
	UPDATE
		tracks
	SET 
		lastrequested=NOW()
	WHERE
		id=?;
	`

	_, err := h.Exec(query, id)
	if err != nil {
		return err
	}
	return nil
}
