package database

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

// NoTrack is the zero-value of Track, used when a track was not found
var NoTrack = Track{}

// ErrTrackNotFound is returned if a track is not found in the database
var ErrTrackNotFound = errors.New("unknown track id")

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

func (t Track) Refresh(tx sqlx.Queryer) (Track, error) {
	return GetTrack(tx, t.TrackID)
}

type tmpTrack struct {
	NullLength sql.NullInt64
	Track
}

// AllTracks returns all tracks in the database
func AllTracks(tx sqlx.Queryer) ([]Track, error) {
	var tmps = []tmpTrack{}

	var query = `
	SELECT tracks.id AS trackid, esong.id AS id, tracks.hash AS hash,
	len AS nulllength, lastplayed, artist, track AS title, album, path AS filepath,
	tags, accepter AS acceptor, lasteditor, priority, usable, lastrequested,
	requestcount FROM tracks LEFT JOIN esong ON tracks.hash = esong.hash;`

	err := sqlx.Select(tx, &tmps, query)
	if err != nil {
		return nil, err
	}

	var tracks = make([]Track, len(tmps))
	for i, tmp := range tmps {
		tracks[i] = resolveTmpTrack(tmp)
	}

	return tracks, nil
}

// converts a tmpTrack into a full fledged Track
func resolveTmpTrack(tmp tmpTrack) Track {
	// handle the potential NULL fields now, and unwrap our Track
	t := tmp.Track
	t.Length = time.Second * time.Duration(tmp.NullLength.Int64)
	// handle fields that were not filled in by the query
	t.RequestDelay = calculateRequestDelay(t.RequestCount)
	t.Metadata = fmt.Sprintf("%s - %s", t.Artist, t.Title)
	return t
}

// GetTrack returns a *Track based on the trackid given.
// returns ErrTrackNotFound if the id does not exist.
func GetTrack(tx sqlx.Queryer, id TrackID) (Track, error) {
	// we create a temporary struct to handle NULL values returned by
	// the query, both Length and Song.ID can be NULL due to the LEFT JOIN
	// not necessarily having an entry in the `esong` table.
	// Song.ID is handled by the SongID type implementing sql.Scanner, but
	// we don't want a separate type for Length, so we're doing it separately.
	var tmp tmpTrack

	var query = `
	SELECT tracks.id AS trackid, esong.id AS id, tracks.hash AS hash,
	len AS nulllength, lastplayed, artist, track AS title, album, path AS filepath,
	tags, accepter AS acceptor, lasteditor, priority, usable, lastrequested,
	requestcount FROM tracks LEFT JOIN esong ON tracks.hash = esong.hash WHERE 
	tracks.id=?;`

	err := sqlx.Get(tx, &tmp, query, id)
	if err != nil {
		if err == sql.ErrNoRows {
			err = ErrTrackNotFound
		}
		return NoTrack, err
	}

	return resolveTmpTrack(tmp), nil
}

// UpdateTrackRequestTime updates the time the track given was last requested
// and increases the time between requests for the song.
//
// TODO: don't hardcode requestcount and priority increments?
func UpdateTrackRequestTime(tx sqlx.Execer, id TrackID) error {
	var query = `UPDATE tracks SET lastrequested=NOW(),
	requestcount=requestcount+2, priority=priority+1 WHERE id=?;`

	_, err := tx.Exec(query, id)
	return err
}

// UpdateTrackPlayTime updates the time the track given was last played
func UpdateTrackPlayTime(tx sqlx.Execer, id TrackID) error {
	var query = `UPDATE tracks SET lastplayed=NOW() WHERE id=?;`

	_, err := tx.Exec(query, id)
	return err
}
