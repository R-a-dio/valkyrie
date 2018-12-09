package database

import (
	"crypto/sha1"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
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
	return SongHash(fmt.Sprintf("%x", sha1.Sum([]byte(metadata))))
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

// CreateSong inserts a new row into the song database table and returns a Track
// containing the new data.
func CreateSong(h HandlerTx, metadata string) (Track, error) {
	// we only accept a tx handler because we potentially do multiple queries
	var query = `
	INSERT INTO esong (meta, hash, hash_link) VALUES (?, ?, ?)`
	hash := NewSongHash(metadata)

	_, err := h.Exec(query, metadata, hash, hash)
	if err != nil {
		return NoTrack, err
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
	var tmp tmpTrack

	var query = `
	SELECT tracks.id AS trackid, esong.id AS id, esong.hash AS hash,
	len AS nulllength, eplay.dt AS lastplayed, artist, track AS title, album, path AS filepath,
	tags, accepter AS acceptor, lasteditor, priority, usable, lastrequested,
	requestcount FROM tracks RIGHT JOIN esong ON tracks.hash = esong.hash JOIN eplay ON
	esong.id = eplay.isong WHERE esong.hash=? ORDER BY eplay.dt LIMIT 1;`

	err := sqlx.Get(h, &tmp, query, hash)
	if err != nil {
		if err == sql.ErrNoRows {
			err = ErrTrackNotFound
		}
		return NoTrack, err
	}

	return resolveTmpTrack(tmp), nil
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

type tmpTrack struct {
	NullLength sql.NullInt64
	Track
}

// AllTracks returns all tracks in the database
func AllTracks(h Handler) ([]Track, error) {
	var tmps = []tmpTrack{}

	var query = `
	SELECT tracks.id AS trackid, esong.id AS id, tracks.hash AS hash,
	len AS nulllength, lastplayed, artist, track AS title, album, path AS filepath,
	tags, accepter AS acceptor, lasteditor, priority, usable, lastrequested,
	requestcount FROM tracks LEFT JOIN esong ON tracks.hash = esong.hash;`

	err := sqlx.Select(h, &tmps, query)
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

// GetTrack returns a track based on the id given.
// returns ErrTrackNotFound if the id does not exist.
func GetTrack(h Handler, id TrackID) (Track, error) {
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

	err := sqlx.Get(h, &tmp, query, id)
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
func UpdateTrackRequestTime(h Handler, id TrackID) error {
	var query = `UPDATE tracks SET lastrequested=NOW(),
	requestcount=requestcount+2, priority=priority+1 WHERE id=?;`

	_, err := h.Exec(query, id)
	return err
}

// UpdateTrackPlayTime updates the time the track given was last played
func UpdateTrackPlayTime(h Handler, id TrackID) error {
	var query = `UPDATE tracks SET lastplayed=NOW() WHERE id=?;`

	_, err := h.Exec(query, id)
	return err
}

// ResolveMetadataBasic resolves the metadata to return the most basic
// information about it that we know. Currently this returns the following
// fields if they are found
//		esong.id
//		esong.len
// 		tracks.id
func ResolveMetadataBasic(h Handler, metadata string) (Track, error) {
	hash := NewSongHash(metadata)

	var tmp tmpTrack

	var query = `
	SELECT tracks.id AS track_id, esong.id AS song_id, esong.len AS length
	FROM esong LEFT JOIN tracks ON esong.hash = tracks.hash WHERE esong.hash=?;`

	err := sqlx.Get(h, &tmp, query, hash)
	if err != nil {
		if err == sql.ErrNoRows {
			err = ErrTrackNotFound
		}
		return NoTrack, err
	}

	// we have the metadata, because we take it as parameter, so overwrite
	// whatever resolve gives us, because it might be empty
	t := resolveTmpTrack(tmp)
	t.Metadata = metadata

	return t, nil
}
