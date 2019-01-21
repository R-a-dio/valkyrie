package database

import (
	"database/sql"
	"fmt"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
)

// ErrTrackNotFound is returned if a track is not found in the database
var ErrTrackNotFound = errors.New("unknown track id")

// SongPlayedCount returns the amount of times this song has been played
func SongPlayedCount(h Handler, s radio.Song) (int64, error) {
	var query = `SELECT count(*) FROM eplay WHERE isong=?;`
	var playedCount int64

	err := sqlx.Get(h, &playedCount, query, s.ID)
	return playedCount, errors.WithStack(err)
}

// SongFaveCount returns the amount of faves this song has
func SongFaveCount(h Handler, s radio.Song) (int64, error) {
	var query = `SELECT count(*) FROM efave WHERE isong=?;`
	var faveCount int64

	err := sqlx.Get(h, &faveCount, query, s.ID)
	return faveCount, errors.WithStack(err)
}

// CreateSong inserts a new row into the song database table and returns a Track
// containing the new data.
func CreateSong(h HandlerTx, metadata string) (*radio.Song, error) {
	// we only accept a tx handler because we potentially do multiple queries
	var query = `INSERT INTO esong (meta, hash, hash_link, len) VALUES (?, ?, ?, ?)`
	hash := radio.NewSongHash(metadata)

	_, err := h.Exec(query, metadata, hash, hash, 0)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return GetSongFromHash(h, hash)
}

// GetSongFromMetadata is a convenience function that calls NewSongHash
// and GetSongFromHash for you
func GetSongFromMetadata(h Handler, metadata string) (*radio.Song, error) {
	return GetSongFromHash(h, radio.NewSongHash(metadata))
}

// GetSongFromHash retrieves a track using the hash given. It uses the esong table as
// primary join and will return ErrTrackNotFound if the hash only exists in the tracks
// table
func GetSongFromHash(h Handler, hash radio.SongHash) (*radio.Song, error) {
	var tmp databaseTrack

	var query = `
	SELECT tracks.id AS trackid, esong.id AS id, esong.hash AS hash, esong.meta AS metadata,
	len AS length, eplay.dt AS lastplayed, artist, track, album, path,
	tags, accepter AS acceptor, lasteditor, priority, usable, lastrequested,
	requestcount FROM tracks RIGHT JOIN esong ON tracks.hash = esong.hash LEFT JOIN eplay ON
	esong.id = eplay.isong WHERE esong.hash=? ORDER BY eplay.dt DESC LIMIT 1;`

	err := sqlx.Get(h, &tmp, query, hash)
	if err != nil {
		if err == sql.ErrNoRows {
			err = ErrTrackNotFound
		} else {
			err = errors.WithStack(err)
		}
		return nil, err
	}

	return tmp.ToSong(), nil
}

// databaseTrack is the type used to communicate with the database
type databaseTrack struct {
	// Hash is shared between tracks and esong
	Hash radio.SongHash
	// LastPlayed is shared between tracks and eplay
	LastPlayed mysql.NullTime

	// esong fields
	ID       sql.NullInt64
	Length   sql.NullInt64
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

func (dt databaseTrack) ToSong() *radio.Song {
	metadata := dt.Metadata.String
	if dt.Track.String != "" && dt.Artist.String != "" {
		metadata = fmt.Sprintf("%s - %s", dt.Artist.String, dt.Track.String)
	} else if dt.Track.String != "" {
		metadata = dt.Track.String
	}

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

	return &radio.Song{
		ID:            radio.SongID(dt.ID.Int64),
		Hash:          dt.Hash,
		Metadata:      metadata,
		Length:        time.Second * time.Duration(dt.Length.Int64),
		LastPlayed:    dt.LastPlayed.Time,
		DatabaseTrack: track,
	}
}

// AllTracks returns all tracks in the database
func AllTracks(h Handler) ([]radio.Song, error) {
	var tmps = []databaseTrack{}

	var query = `
	SELECT tracks.id AS trackid, esong.id AS id, tracks.hash AS hash,
	len AS length, lastplayed, artist, track, album, path,
	tags, accepter AS acceptor, lasteditor, priority, usable, lastrequested,
	requestcount FROM tracks LEFT JOIN esong ON tracks.hash = esong.hash;`

	err := sqlx.Select(h, &tmps, query)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	var tracks = make([]radio.Song, len(tmps))
	for i, tmp := range tmps {
		tracks[i] = *(tmp.ToSong())
	}

	return tracks, nil
}

// GetTrack returns a track based on the id given.
// returns ErrTrackNotFound if the id does not exist.
func GetTrack(h Handler, id radio.TrackID) (*radio.Song, error) {
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

	err := sqlx.Get(h, &tmp, query, id)
	if err != nil {
		if err == sql.ErrNoRows {
			err = ErrTrackNotFound
		} else {
			err = errors.WithStack(err)
		}
		return nil, err
	}

	return tmp.ToSong(), nil
}

// UpdateTrackRequestTime updates the time the track given was last requested
// and increases the time between requests for the song.
//
// TODO: don't hardcode requestcount and priority increments?
func UpdateTrackRequestTime(h Handler, id radio.TrackID) error {
	var query = `UPDATE tracks SET lastrequested=NOW(),
	requestcount=requestcount+2, priority=priority+1 WHERE id=?;`

	_, err := h.Exec(query, id)
	return errors.WithStack(err)
}

// UpdateTrackPlayTime updates the time the track given was last played
func UpdateTrackPlayTime(h Handler, id radio.TrackID) error {
	var query = `UPDATE tracks SET lastplayed=NOW() WHERE id=?;`

	_, err := h.Exec(query, id)
	return errors.WithStack(err)
}

// ResolveMetadataBasic resolves the metadata to return the most basic
// information about it that we know. Currently this returns the following
// fields if they are found
//		esong.id
//		esong.len
// 		tracks.id
func ResolveMetadataBasic(h Handler, metadata string) (*radio.Song, error) {
	hash := radio.NewSongHash(metadata)

	var tmp databaseTrack

	var query = `
	SELECT tracks.id AS trackid, esong.id AS id, esong.len AS length
	FROM esong LEFT JOIN tracks ON esong.hash = tracks.hash WHERE esong.hash=?;`

	err := sqlx.Get(h, &tmp, query, hash)
	if err != nil {
		if err == sql.ErrNoRows {
			err = ErrTrackNotFound
		} else {
			err = errors.WithStack(err)
		}
		return nil, err
	}

	// we have the metadata, because we take it as parameter, so overwrite
	// whatever resolve gives us, because it might be empty
	t := tmp.ToSong()
	t.Metadata = metadata
	t.Hash = hash

	return t, nil
}

// InsertPlayedSong inserts a row into the eplay table with the arguments given
//
// ldiff can be nil to indicate no listener data was available
func InsertPlayedSong(h Handler, id radio.SongID, ldiff *int64) error {
	var query = `INSERT INTO eplay (isong, ldiff) VALUES (?, ?);`

	_, err := h.Exec(query, id, ldiff)
	return errors.WithStack(err)
}

// UpdateSongLength updates the length for the ID given, length is rounded to seconds
func UpdateSongLength(h Handler, id radio.SongID, length time.Duration) error {
	var query = "UPDATE esong SET len=? WHERE id=?;"

	len := int(length / time.Second)
	_, err := h.Exec(query, len, id)
	return errors.WithStack(err)
}

func GetLastPlayed(h Handler, offset, amount int) ([]radio.Song, error) {
	var query = `SELECT esong.id AS id, esong.meta AS metadata FROM esong
		RIGHT JOIN eplay ON esong.id = eplay.isong ORDER BY eplay.dt DESC LIMIT ? OFFSET ?;`

	var songs = make([]radio.Song, 0, amount)

	err := sqlx.Select(h, &songs, query, amount, offset)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return songs, nil
}

func FaveSong(h Handler, nick string, s radio.Song) (bool, error) {
	var query = `SELECT enick.id AS id, EXISTS(SELECT efave.id FROM efave WHERE
		inick=enick.id AND isong=?) AS hasfave FROM enick WHERE enick.nick=?;`

	var info = struct {
		ID      int64
		HasFave bool
	}{}

	err := sqlx.Get(h, &info, query, s.ID, nick)
	if err != nil {
		return false, err
	}

	if info.HasFave {
		return false, nil
	}

	if info.ID == 0 {
		query = `INSERT INTO enick (nick) VALUES (?)`
		res, err := h.Exec(query, nick)
		if err != nil {
			return false, err
		}

		info.ID, err = res.LastInsertId()
		if err != nil {
			panic("LastInsertId not supported")
		}
	}

	query = `INSERT INTO efave (inick, isong) VALUES (?, ?)`
	_, err = h.Exec(query, info.ID, s.ID)
	if err != nil {
		return false, err
	}

	return true, nil
}

func UnfaveSong(h Handler, nick string, s radio.Song) (bool, error) {
	var query = `DELETE efave FROM efave JOIN enick ON 
	enick.id = efave.inick WHERE enick.nick=? AND efave.isong=?;`

	res, err := h.Exec(query, nick, s.ID)
	if err != nil {
		return false, errors.WithStack(err)
	}

	n, err := res.RowsAffected()
	if err != nil {
		panic("RowsAffected not supported")
	}

	return n > 0, nil
}
