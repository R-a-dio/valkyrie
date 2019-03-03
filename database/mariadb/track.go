package mariadb

import (
	"database/sql"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
)

// databaseTrack is the type used to communicate with the database
type databaseTrack struct {
	// Hash is shared between tracks and esong
	Hash radio.SongHash
	// LastPlayed is shared between tracks and eplay
	LastPlayed mysql.NullTime

	// esong fields
	ID       sql.NullInt64
	Length   sql.NullFloat64
	Metadata sql.NullString

	// tracks fields
	TrackID       sql.NullInt64
	Artist        sql.NullString
	Track         sql.NullString
	Album         sql.NullString
	Path          sql.NullString
	Tags          sql.NullString
	Priority      sql.NullInt64
	LastRequested mysql.NullTime
	Usable        sql.NullInt64
	Acceptor      sql.NullString
	LastEditor    sql.NullString

	RequestCount    sql.NullInt64
	NeedReplacement sql.NullInt64
}

func (dt databaseTrack) ToSong() radio.Song {
	var track *radio.DatabaseTrack
	if dt.TrackID.Valid {
		track = &radio.DatabaseTrack{
			TrackID:  radio.TrackID(dt.TrackID.Int64),
			Artist:   dt.Artist.String,
			Title:    dt.Track.String,
			Album:    dt.Album.String,
			FilePath: dt.Path.String,
			Tags:     dt.Tags.String,

			Acceptor:   dt.Acceptor.String,
			LastEditor: dt.LastEditor.String,

			Priority: int(dt.Priority.Int64),
			Usable:   dt.Usable.Int64 == 1,

			LastRequested: dt.LastRequested.Time,
			RequestCount:  int(dt.RequestCount.Int64),
			RequestDelay:  radio.CalculateRequestDelay(int(dt.RequestCount.Int64)),
		}
	}

	song := radio.Song{
		ID:            radio.SongID(dt.ID.Int64),
		Hash:          dt.Hash,
		Metadata:      dt.Metadata.String,
		Length:        time.Duration(float64(time.Second) * dt.Length.Float64),
		LastPlayed:    dt.LastPlayed.Time,
		DatabaseTrack: track,
		SyncTime:      time.Now(),
	}
	song.FillMetadata()
	return song
}

func (dt databaseTrack) ToSongPtr() *radio.Song {
	song := dt.ToSong()
	return &song
}

// SongStorage implements radio.SongStorage
type SongStorage struct {
	handle handle
}

// TrackStorage implements radio.TrackStorage
type TrackStorage struct {
	handle handle
}

// Get implements radio.TrackStorage
func (ts TrackStorage) Get(id radio.TrackID) (*radio.Song, error) {
	const op errors.Op = "mariadb/TrackStorage.Get"

	// we create a temporary struct to handle NULL values returned by
	// the query, both Length and Song.ID can be NULL due to the LEFT JOIN
	// not necessarily having an entry in the `esong` table.
	// Song.ID is handled by the SongID type implementing sql.Scanner, but
	// we don't want a separate type for Length, so we're doing it separately.
	var tmp databaseTrack

	var query = `
	SELECT tracks.id AS trackid, esong.id AS id, tracks.hash AS hash,
	len AS length, lastplayed, artist, track, album, path,
	tags, accepter AS acceptor, lasteditor, priority, usable, lastrequested,
	requestcount FROM tracks LEFT JOIN esong ON tracks.hash = esong.hash WHERE 
	tracks.id=?;`

	err := sqlx.Get(ts.handle, &tmp, query, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.E(op, errors.SongUnknown)
		}
		return nil, errors.E(op, err)
	}

	return tmp.ToSongPtr(), nil
}

// All implements radio.TrackStorage
func (ts TrackStorage) All() ([]radio.Song, error) {
	const op errors.Op = "mariadb/TrackStorage.All"

	var tmps = []databaseTrack{}

	var query = `
	SELECT tracks.id AS trackid, esong.id AS id, tracks.hash AS hash,
	len AS length, lastplayed, artist, track, album, path,
	tags, accepter AS acceptor, lasteditor, priority, usable, lastrequested,
	requestcount FROM tracks LEFT JOIN esong ON tracks.hash = esong.hash;`

	err := sqlx.Select(ts.handle, &tmps, query)
	if err != nil {
		return nil, errors.E(op, err)
	}

	var tracks = make([]radio.Song, len(tmps))
	for i, tmp := range tmps {
		tracks[i] = tmp.ToSong()
	}

	return tracks, nil
}

// Unusable implements radio.TrackStorage
func (ts TrackStorage) Unusable() ([]radio.Song, error) {
	const op errors.Op = "mariadb/TrackStorage.Unusable"

	var tmps = []databaseTrack{}

	var query = `
	SELECT 
		esong.id AS id,
		esong.hash AS hash,
		esong.meta AS metadata,
		esong.len AS length,
		tracks.id AS trackid,
		tracks.lastplayed,
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
	FROM
		tracks
	LEFT JOIN 
		esong
	ON
		tracks.hash = esong.hash
	WHERE
		tracks.usable != 1;
	`

	err := sqlx.Select(ts.handle, &tmps, query)
	if err != nil {
		return nil, errors.E(op, err)
	}

	var tracks = make([]radio.Song, len(tmps))
	for i, tmp := range tmps {
		tracks[i] = tmp.ToSong()
	}

	return tracks, nil
}

// UpdateRequestInfo updates the time the track given was last requested
// and increases the time between requests for the song.
//
// implements radio.TrackStorage
func (ts TrackStorage) UpdateRequestInfo(id radio.TrackID) error {
	const op errors.Op = "mariadb/TrackStorage.UpdateRequestInfo"

	// TODO(wessie): don't hardcode requestcount and priority
	var query = `UPDATE tracks SET lastrequested=NOW(),
	requestcount=requestcount+2, priority=priority+1 WHERE id=?;`

	_, err := ts.handle.Exec(query, id)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

// UpdateLastPlayed implements radio.TrackStorage
func (ts TrackStorage) UpdateLastPlayed(id radio.TrackID) error {
	const op errors.Op = "mariadb/TrackStorage.UpdateLastPlayed"

	var query = `UPDATE tracks SET lastplayed=NOW() WHERE id=?;`

	_, err := ts.handle.Exec(query, id)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

// UpdateLastRequested implements radio.TrackStorage
func (ts TrackStorage) UpdateLastRequested(id radio.TrackID) error {
	const op errors.Op = "mariadb/TrackStorage.UpdateLastRequested"

	var query = `
	UPDATE
		tracks
	SET 
		lastrequested=NOW()
	WHERE
		id=?;
	`

	_, err := ts.handle.Exec(query, id)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}
