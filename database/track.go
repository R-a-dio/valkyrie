package database

import (
	"crypto/sha1"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
)

// NoTrack is the zero-value of Track, used when a track was not found
var NoTrack = Track{}

// NoSong is the zero-value of Song, used when no valid song was found
var NoSong = Song{}

// ErrTrackNotFound is returned if a track is not found in the database
var ErrTrackNotFound = errors.New("unknown track id")

// SongID is the songs identifier as found in all seen songs.
// See also TrackID.
type SongID uint64

// Scan implements sql.Scanner
func (s *SongID) Scan(src interface{}) error {
	if i, ok := src.(int64); ok {
		*s = SongID(i)
	}

	return nil
}

// SongHash is the songs identifier based on metadata.
type SongHash string

// NewSongHash generates a new SongHash for the metadata passed in
func NewSongHash(metadata string) SongHash {
	metadata = strings.TrimSpace(strings.ToLower(metadata))
	return SongHash(fmt.Sprintf("%x", sha1.Sum([]byte(metadata))))
}

// Value implements sql/driver.Valuer
func (s SongHash) Value() (driver.Value, error) {
	return string(s), nil
}

// Scan implements sql.Scanner
func (s *SongHash) Scan(src interface{}) error {
	*s = SongHash(string(src.([]byte)))
	return nil
}

// Song represents a song not in the streamers database
type Song struct {
	ID SongID
	// Hash is a hash based on the contents of Metadata
	Hash SongHash
	// Metadata is a combined string of `artist - title` format
	Metadata string
	// Length is the length of the song.
	// Length is an approximation if song is not in the streamers database
	Length time.Duration
	// LastPlayed is the last time this song played on stream
	LastPlayed time.Time
}

// PlayedCount returns the amount of times this song has been played
func (s Song) PlayedCount(h Handler) (int64, error) {
	var query = `SELECT count(*) FROM eplay WHERE isong=?;`
	var playedCount int64

	err := sqlx.Get(h, &playedCount, query, s.ID)
	return playedCount, errors.WithStack(err)
}

// FaveCount returns the amount of faves this song has
func (s Song) FaveCount(h Handler) (int64, error) {
	var query = `SELECT count(*) FROM efave WHERE isong=?;`
	var faveCount int64

	err := sqlx.Get(h, &faveCount, query, s.ID)
	return faveCount, errors.WithStack(err)
}

// CreateSong inserts a new row into the song database table and returns a Track
// containing the new data.
func CreateSong(h HandlerTx, metadata string) (Track, error) {
	// we only accept a tx handler because we potentially do multiple queries
	var query = `INSERT INTO esong (meta, hash, hash_link, len) VALUES (?, ?, ?, ?)`
	hash := NewSongHash(metadata)

	_, err := h.Exec(query, metadata, hash, hash, 0)
	if err != nil {
		return NoTrack, errors.WithStack(err)
	}

	return GetSongFromHash(h, hash)
}

// GetSongFromMetadata is a convenience function that calls NewSongHash
// and GetSongFromHash for you
func GetSongFromMetadata(h Handler, metadata string) (Track, error) {
	return GetSongFromHash(h, NewSongHash(metadata))
}

// GetSongFromHash retrieves a track using the hash given. It uses the esong table as
// primary join and will return ErrTrackNotFound if the hash only exists in the tracks
// table
func GetSongFromHash(h Handler, hash SongHash) (Track, error) {
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
		return NoTrack, err
	}

	return tmp.ToTrack(), nil
}

// TrackID is the songs identifier as found in the streamers database.
// A song in the streamers database has both a TrackID and a SongID, while
// a song not in the streamers database only has a SongID.
type TrackID uint64

// Track represents a song in the streamers database
type Track struct {
	TrackID TrackID
	Song

	Artist   string
	Title    string
	Album    string
	FilePath string
	Tags     string

	Acceptor   string
	LastEditor string

	Priority int
	Usable   bool

	LastRequested time.Time

	RequestCount int
	RequestDelay time.Duration
}

// EqualTo returns `o == t` in terms of Track equality
func (t Track) EqualTo(o Track) bool {
	return t.TrackID == o.TrackID
}

// Refresh returns a new Track with the latest database information
func (t Track) Refresh(h Handler) (Track, error) {
	return GetTrack(h, t.TrackID)
}

// databaseTrack is the type used to communicate with the database
type databaseTrack struct {
	// Hash is shared between tracks and esong
	Hash SongHash
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

func (dt databaseTrack) ToTrack() Track {
	metadata := dt.Metadata.String
	if dt.Track.String != "" && dt.Artist.String != "" {
		metadata = fmt.Sprintf("%s - %s", dt.Artist.String, dt.Track.String)
	} else if dt.Track.String != "" {
		metadata = dt.Track.String
	}

	return Track{
		Song: Song{
			ID:         SongID(dt.ID.Int64),
			Hash:       dt.Hash,
			Metadata:   metadata,
			Length:     time.Second * time.Duration(dt.Length.Int64),
			LastPlayed: dt.LastPlayed.Time,
		},

		TrackID:  TrackID(dt.TrackID.Int64),
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
		RequestDelay:  calculateRequestDelay(int(dt.RequestCount.Int64)),
	}
}

// AllTracks returns all tracks in the database
func AllTracks(h Handler) ([]Track, error) {
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

	var tracks = make([]Track, len(tmps))
	for i, tmp := range tmps {
		tracks[i] = tmp.ToTrack()
	}

	return tracks, nil
}

// GetTrack returns a track based on the id given.
// returns ErrTrackNotFound if the id does not exist.
func GetTrack(h Handler, id TrackID) (Track, error) {
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
		return NoTrack, err
	}

	return tmp.ToTrack(), nil
}

// UpdateTrackRequestTime updates the time the track given was last requested
// and increases the time between requests for the song.
//
// TODO: don't hardcode requestcount and priority increments?
func UpdateTrackRequestTime(h Handler, id TrackID) error {
	var query = `UPDATE tracks SET lastrequested=NOW(),
	requestcount=requestcount+2, priority=priority+1 WHERE id=?;`

	_, err := h.Exec(query, id)
	return errors.WithStack(err)
}

// UpdateTrackPlayTime updates the time the track given was last played
func UpdateTrackPlayTime(h Handler, id TrackID) error {
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
func ResolveMetadataBasic(h Handler, metadata string) (Track, error) {
	hash := NewSongHash(metadata)

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
		return NoTrack, err
	}

	// we have the metadata, because we take it as parameter, so overwrite
	// whatever resolve gives us, because it might be empty
	t := tmp.ToTrack()
	t.Metadata = metadata
	t.Hash = hash

	return t, nil
}

// InsertPlayedSong inserts a row into the eplay table with the arguments given
//
// ldiff can be nil to indicate no listener data was available
func InsertPlayedSong(h Handler, id SongID, ldiff *int64) error {
	var query = `INSERT INTO eplay (isong, ldiff) VALUES (?, ?);`

	_, err := h.Exec(query, id, ldiff)
	return errors.WithStack(err)
}

// UpdateSongLength updates the length for the ID given, length is rounded to seconds
func UpdateSongLength(h Handler, id SongID, length time.Duration) error {
	var query = "UPDATE esong SET len=? WHERE id=?;"

	len := int(length / time.Second)
	_, err := h.Exec(query, len, id)
	return errors.WithStack(err)
}
