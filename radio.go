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
	User     User
	Song     Song
	SongInfo SongInfo
	// Listeners is the current amount of stream listeners
	Listeners       int
	Thread          string
	RequestsEnabled bool
}

// Copy makes a deep-copy of the status object
func (s Status) Copy() Status {
	c := s
	if s.Song.HasTrack() {
		track := *s.Song.DatabaseTrack
		c.Song.DatabaseTrack = &track
	}

	return s
}

type User struct {
	ID       int
	Nickname string
	IsRobot  bool
}

type SongInfo struct {
	// Start is the time at which the current song started playing
	Start time.Time
	// End is the expected time the current song stops playing
	End time.Time
}

type SearchService interface {
	Search(context.Context, string, int, int) ([]Song, error)
	UpdateIndex(context.Context, TrackID) error
}

type ManagerService interface {
	Status(context.Context) (*Status, error)

	UpdateUser(context.Context, User) error
	UpdateSong(context.Context, Song, SongInfo) error
	UpdateThread(ctx context.Context, thread string) error
	UpdateListeners(context.Context, int) error
}

type StreamerService interface {
	Start(context.Context) error
	Stop(ctx context.Context, force bool) error

	RequestSong(context.Context, Song, string) error
	Queue(context.Context) ([]QueueEntry, error)
}

// QueueEntry is a Song used in the QueueService
type QueueEntry struct {
	Song
	// IsUserRequest should be true if this song was added to the queue
	// by a third-party user
	IsUserRequest bool
	// UserIdentifier should be a way to identify the user that requested the song
	UserIdentifier string
	// ExpectedStartTime is the expected time this song will be played on stream
	ExpectedStartTime time.Time
}

func (qe *QueueEntry) EqualTo(qe2 QueueEntry) bool {
	return qe != nil &&
		qe.Song.EqualTo(qe2.Song) &&
		qe.UserIdentifier == qe2.UserIdentifier
}

type QueueStorage interface {
	Store(ctx context.Context, name string, queue []QueueEntry) error
	Load(ctx context.Context, name string) ([]QueueEntry, error)
}

type QueueService interface {
	// AddRequest requests the given song to be added to the queue, the string given
	// is an identifier of the user that requested it
	AddRequest(context.Context, Song, string) error
	// ReserveNext returns the next yet-to-be-reserved entry from the queue
	ReserveNext(context.Context) (*QueueEntry, error)
	// Remove removes the first occurence of the given entry from the queue
	Remove(context.Context, QueueEntry) (bool, error)
	// Entries returns all entries in the queue
	Entries(context.Context) ([]QueueEntry, error)
}

type AnnounceService interface {
	AnnounceSong(context.Context, Status) error
	AnnounceRequest(context.Context, Song) error
}

// SongID is a songs identifier
type SongID uint64

// Scan implements sql.Scanner
func (s *SongID) Scan(src interface{}) error {
	if src == nil {
		return nil
	}
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
	b, ok := src.([]byte)
	if src == nil || !ok {
		return nil
	}
	_, err := hex.Decode((*s)[:], b)
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

	// SyncTime is the time this Song was returned by the database layer
	SyncTime time.Time
}

// EqualTo returns s == d based on unique fields
func (s Song) EqualTo(d Song) bool {
	// check if we have a SongID in both
	if s.ID == 0 || d.ID == 0 {
		// if we don't, check for Tracks
		if !s.HasTrack() || !d.HasTrack() {
			// if we don't, we can't do an equality check
			return false
		}

		// check if we have a TrackID in both
		if s.TrackID == 0 || d.TrackID == 0 {
			return false
		}

		return s.TrackID == d.TrackID
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

var veryFarAway = time.Hour * 24 * 90

// UntilRequestable returns the time until this song can be requested again, returns 0
// if song.Requestable() == true
func (s *Song) UntilRequestable() time.Duration {
	if s.Requestable() {
		return 0
	}
	if s.RequestDelay == 0 {
		return veryFarAway
	}

	var furthest time.Time
	if s.LastPlayed.After(s.LastRequested) {
		furthest = s.LastPlayed
	} else {
		furthest = s.LastRequested
	}

	if furthest.IsZero() {
		return veryFarAway
	}

	furthest = furthest.Add(s.RequestDelay)
	return time.Until(furthest)
}

// HasTrack returns true if t != nil, can be used as Song.HasTrack to check if a track
// was allocated for the embedded field
func (t *DatabaseTrack) HasTrack() bool {
	return t != nil
}
