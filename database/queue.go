package database

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/jmoiron/sqlx"
)

var (
	queryQueueSave = `INSERT INTO queue (trackid, time, ip, type, meta,
		length, id) VALUES (?, ?, ?, ?, ?, ?, ?);`
	queryQueueLoad = `SELECT trackid, ip AS uid,
		type AS isrequest FROM queue ORDER BY id ASC;`
	queryQueuePopulate = `SELECT tmp.id FROM (
			(SELECT id FROM tracks WHERE usable=1 ORDER BY 
			(UNIX_TIMESTAMP(lastplayed) + 1)*(UNIX_TIMESTAMP(lastrequested) + 1) 
			ASC LIMIT 100)
		UNION ALL 
			(SELECT id FROM tracks WHERE usable=1 ORDER BY LEAST(lastplayed,
				 lastrequested) ASC LIMIT 100)
		) AS tmp GROUP BY tmp.id HAVING count(*) >= 2;`
	queryQueueUpdateLastRequested = `UPDATE tracks SET 
		lastrequested=NOW() WHERE id=?`
	queryQueueDelete = `DELETE FROM queue;`
)

type QueueEntry struct {
	Track          radio.Song
	IsRequest      bool
	UserIdentifier string

	// fields not used by the database layer
	EstimatedPlayTime time.Time
}

// MarshalJSON implements json.Marshaler
func (q QueueEntry) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		IsRequest         bool
		UserIdentifier    string
		EstimatedPlayTime time.Time
		Metadata          string
		TrackID           radio.TrackID
	}{
		IsRequest:         q.IsRequest,
		UserIdentifier:    q.UserIdentifier,
		EstimatedPlayTime: q.EstimatedPlayTime,
		Metadata:          q.Track.Metadata,
		TrackID:           q.Track.TrackID,
	})
}

func QueueLoad(h Handler) ([]QueueEntry, error) {
	var databaseQueue = []struct {
		TrackID   radio.TrackID
		UID       sql.NullString
		IsRequest int
	}{}

	err := sqlx.Select(h, &databaseQueue, queryQueueLoad)
	if err != nil {
		fmt.Println("select")
		return nil, err
	}

	queue := make([]QueueEntry, 0, len(databaseQueue))
	for _, qi := range databaseQueue {
		t, err := GetTrack(h, qi.TrackID)
		if err != nil {
			fmt.Println("gettrack")
			return nil, err
		}

		queue = append(queue, QueueEntry{
			Track:          *t,
			IsRequest:      qi.IsRequest != 0,
			UserIdentifier: qi.UID.String,
		})
	}

	return queue, nil
}

func QueuePopulate(h Handler) ([]radio.TrackID, error) {
	var candidates = []radio.TrackID{}
	err := sqlx.Select(h, &candidates, queryQueuePopulate)
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, errors.New("empty candidate list")
	}

	return candidates, nil
}

func QueueUpdateTrack(h Handler, tid radio.TrackID) error {
	_, err := h.Exec(queryQueueUpdateLastRequested, tid)
	if err != nil {
		return err
	}
	return nil
}

func QueueSave(h Handler, queue []QueueEntry) error {
	_, err := h.Exec(queryQueueDelete)
	if err != nil {
		return err
	}

	for i, e := range queue {
		var isRequest = 0
		if e.IsRequest {
			isRequest = 1
		}

		_, err := h.Exec(queryQueueSave, e.Track.TrackID, e.EstimatedPlayTime,
			e.UserIdentifier, isRequest, e.Track.Metadata,
			e.Track.Length/time.Second, i+1)
		if err != nil {
			return err
		}
	}

	return nil
}
