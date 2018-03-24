package database

import (
	"time"
)

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
