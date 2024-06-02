package mariadb

import (
	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/jmoiron/sqlx"
)

// QueueStorage is a radio.QueueStorage backed by a sql database
type QueueStorage struct {
	handle handle
}

type queueSong struct {
	radio.QueueEntry

	// indicates what kind of entry this was
	IsRequest int
	// absolute position in the queue
	Position int
}

// Store stores the queue given under name in the database configured
//
// Implements radio.QueueStorage
func (qs QueueStorage) Store(name string, queue []radio.QueueEntry) error {
	const op errors.Op = "mariadb/QueueStorage.Store"
	handle, deferFn := qs.handle.span(op)
	defer deferFn()

	// prep the data before we even ask for a transaction
	var entries []queueSong

	for i, entry := range queue {
		if !entry.HasTrack() {
			return errors.E(op, errors.SongWithoutTrack, entry)
		}

		var isRequest = 0
		if entry.IsUserRequest {
			isRequest = 1
		}

		entries = append(entries, queueSong{
			QueueEntry: entry,
			Position:   1 + i,
			IsRequest:  isRequest,
		})
	}

	// then try and add all the songs to the queue
	handle, tx, err := requireTx(handle)
	if err != nil {
		return errors.E(op, err)
	}
	defer tx.Rollback()

	// empty the queue so we can repopulate it
	_, err = handle.Exec(`DELETE FROM queue`)
	if err != nil {
		return errors.E(op, err)
	}

	if len(entries) == 0 {
		// nothing to store, but NamedExec doesn't like that so we just don't
		// execute it
		return tx.Commit()
	}

	var query = `
	INSERT INTO
		queue (trackid, time, ip, type, meta, length, id, queue_id)
	VALUES (
		:trackid,
		:expectedstarttime,
		:useridentifier,
		:isrequest,
		:metadata,
		from_go_duration(:length),
		:position,
		:queueid
	);
	`
	_, err = sqlx.NamedExec(handle, query, entries)
	if err != nil {
		return errors.E(op, err)
	}

	return tx.Commit()
}

var queueLoadQuery = expand(`
SELECT
	queue.queue_id AS queueid,
	queue.trackid,
	queue.time AS expectedstarttime,
	queue.ip AS useridentifier,
	queue.type AS isrequest,
	queue.meta AS metadata,
	to_go_duration(queue.length) AS length,
	{lastplayedSelect},
	{maybeSongColumns},
	{maybeTrackColumns}
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
	handle, deferFn := qs.handle.span(op)
	defer deferFn()

	var queue []queueSong

	err := sqlx.Select(handle, &queue, queueLoadQuery)
	if err != nil {
		return nil, errors.E(op, err)
	}

	songs := make([]radio.QueueEntry, len(queue))
	for i, qSong := range queue {
		qSong.IsUserRequest = qSong.IsRequest == 1
		songs[i] = qSong.QueueEntry
	}

	return songs, nil
}
