package radio

import (
	"context"
	"crypto/sha1"
	"database/sql/driver"
	"encoding/hex"
	"fmt"
	"math"
	"strings"
	"time"
)

// CalculateRequestDelay returns the delay between two requests of a song
func CalculateRequestDelay(requestCount int) time.Duration {
	if requestCount > 30 {
		requestCount = 30
	}

	var dur float64
	if requestCount >= 0 && requestCount <= 7 {
		dur = -11057*math.Pow(float64(requestCount), 2) +
			172954*float64(requestCount) + 81720
	} else {
		dur = 599955*math.Exp(0.0372*float64(requestCount)) + 0.5
	}

	return time.Duration(time.Duration(dur/2) * time.Second)
}

type Status struct {
	User            User
	Song            Song
	StreamInfo      StreamInfo
	Thread          string
	RequestsEnabled bool
}

type User struct {
	ID       int
	Nickname string
	IsRobot  bool
}

type StreamInfo struct {
	// Listeners is the current amount of stream listeners
	Listeners int
	// SongStart is the time at which the current song started playing
	SongStart time.Time
	// SongEnd is the expected time the current song stops playing
	SongEnd time.Time
}

type ManagerService interface {
	Status(context.Context) (Status, error)

	UpdateUser(context.Context, User) error
	UpdateSong(context.Context, Song) error
	UpdateThread(ctx context.Context, thread string) error
	UpdateListeners(context.Context, int) error
}

type StreamerService interface {
	Start(context.Context) error
	Stop(ctx context.Context, force bool) error
}

// QueueSong is a Song used in the QueueService
type QueueSong struct {
	Song

	// IsUserRequest should be true if this song was added to the queue
	// by a third-party user
	IsUserRequest bool
	// UserIdentifier should be a way to identify the user that requested the song
	UserIdentifier string
	// ExpectedStartTime is the expected time this song will be played on stream
	ExpectedStartTime time.Time
}

type QueueStorage interface {
	Store(ctx context.Context, name string, queue []QueueSong) error
	Load(ctx context.Context, name string) ([]QueueSong, error)
}

type QueueService interface {
	// Append adds the song given to the queue
	Append(context.Context, QueueSong) error
	// Peek returns the song that is queued after the song given
	Peek(context.Context, QueueSong) (QueueSong, error)
	// Pop pops off the song given if it's the top-most song, otherwise ignores it
	Pop(context.Context, QueueSong) error
	// Remove removes the song given from the queue;  Remove should only remove
	// the first occurence of the song
	Remove(context.Context, QueueSong) error
	// All returns all songs in the queue
	All(context.Context) ([]QueueSong, error)
}

type AnnounceService interface {
	AnnounceSong(context.Context, Song, StreamInfo) error
	AnnounceRequest(context.Context, Song) error
}

// SongID is a songs identifier
type SongID uint64

// Scan implements sql.Scanner
func (s *SongID) Scan(src interface{}) error {
	if i, ok := src.(int64); ok {
		*s = SongID(i)
	}

	return nil
}

// SongHash is a sha1 hash
type SongHash [sha1.Size]byte

// NewSongHash generates a new SongHash for the metadata passed in
func NewSongHash(metadata string) SongHash {
	metadata = strings.TrimSpace(strings.ToLower(metadata))
	return SongHash(sha1.Sum([]byte(metadata)))
}

// Value implements sql/driver.Valuer
func (s SongHash) Value() (driver.Value, error) {
	return s.String(), nil
}

// Scan implements sql.Scanner
func (s *SongHash) Scan(src interface{}) error {
	_, err := hex.Decode((*s)[:], src.([]byte))
	return err
}

// String returns a hexadecimal representation of the song hash
func (s SongHash) String() string {
	return fmt.Sprintf("%x", s[:])
}

// Song is a song we've seen played on the stream
type Song struct {
	ID SongID
	// Hash is a sha1 of the contents of Metadata
	Hash SongHash
	// Metadata is simple metadata for this song in the format 'artist - title'
	Metadata string
	// Length is the length of the song
	Length time.Duration
	// LastPlayed is the last time this song played on stream
	LastPlayed time.Time
	// DatabaseTrack is only available if the song is in our streamer database
	*DatabaseTrack
}

// EqualTo returns s == d based on unique fields
func (s Song) EqualTo(d Song) bool {
	if s.ID == 0 || d.ID == 0 {
		// zero means uninitialized and should never be equal
		return false
	}

	return s.ID == d.ID
}

// TrackID is a database track identifier
type TrackID uint64

// DatabaseTrack is a song we have the actual audio file for and is available to the
// automated streamer
type DatabaseTrack struct {
	TrackID TrackID

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

// Requestable returns whether this song can be requested by a user
func (s *Song) Requestable() bool {
	if s == nil || s.DatabaseTrack == nil {
		panic("Requestable called with nil database track")
	}
	if s.RequestDelay == 0 {
		// no delay set, so we can't really do a proper comparison below
		return false
	}
	if time.Since(s.LastPlayed) < s.RequestDelay {
		return false
	}
	if time.Since(s.LastRequested) < s.RequestDelay {
		return false
	}

	return true
}

func (t *DatabaseTrack) HasTrack() bool {
	return t != nil
}
