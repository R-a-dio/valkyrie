package mariadb

import (
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/jmoiron/sqlx"
)

// QueueStorage is a radio.QueueStorage backed by a sql database
type QueueStorage struct {
	handle handle
}

type queueSong struct {
	ExpectedStartTime time.Time
	UserIdentifier    string
	IsRequest         int
	radio.Song
}

// Store stores the queue given under name in the database configured
//
// Implements radio.QueueStorage
func (qs QueueStorage) Store(name string, queue []radio.QueueEntry) error {
	const op errors.Op = "mariadb/QueueStorage.Store"

	handle, tx, err := requireTx(qs.handle)
	if err != nil {
		return errors.E(op, err)
	}
	defer tx.Rollback()

	// empty the queue so we can repopulate it
	_, err = handle.Exec(`DELETE FROM queue`)
	if err != nil {
		return errors.E(op, err)
	}

	var query = `
	INSERT INTO
		queue (trackid, time, ip, type, meta, length, id)
	VALUES
		(?, ?, ?, ?, ?, from_go_duration(?), ?);
	`
	for i, entry := range queue {
		if !entry.HasTrack() {
			return errors.E(op, errors.SongWithoutTrack, entry)
		}

		var isRequest = 0
		if entry.IsUserRequest {
			isRequest = 1
		}

		_, err = handle.Exec(query,
			entry.TrackID,
			entry.ExpectedStartTime,
			entry.UserIdentifier,
			isRequest,
			entry.Metadata,
			entry.Length,
			i+1, // ordering id
		)
		if err != nil {
			return errors.E(op, err)
		}
	}

	return tx.Commit()
}

var queueLoadQuery = expand(`
SELECT
	queue.trackid,
	queue.time AS expectedstarttime,
	queue.ip AS useridentifier,
	queue.type AS isrequest,
	queue.meta AS metadata,
	to_go_duration(queue.length) AS length,
	{lastplayedSelect},
	{maybeSongColumns},
	{trackColumns}
FROM
	queue
LEFT JOIN
	tracks ON queue.trackid = tracks.id
LEFT JOIN
	esong ON tracks.hash = esong.hash
ORDER BY
	queue.id ASC;
`)

// Load loads the queue name given from the database configured
//
// Implements radio.QueueStorage
func (qs QueueStorage) Load(name string) ([]radio.QueueEntry, error) {
	const op errors.Op = "mariadb/QueueStorage.Load"

	var queue []queueSong

	err := sqlx.Select(qs.handle, &queue, queueLoadQuery)
	if err != nil {
		return nil, errors.E(op, err)
	}

	songs := make([]radio.QueueEntry, len(queue))
	for i, qSong := range queue {
		songs[i] = radio.QueueEntry{
			Song:              qSong.Song,
			IsUserRequest:     qSong.IsRequest == 1,
			UserIdentifier:    qSong.UserIdentifier,
			ExpectedStartTime: qSong.ExpectedStartTime,
		}
	}

	return songs, nil
}
