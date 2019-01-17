package radio

import (
	"crypto/sha1"
	"database/sql/driver"
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
	Status() (Status, error)

	UpdateUser(User) error
	UpdateSong(Song) error
	UpdateThread(thread string) error
	UpdateListeners(int) error
}

type StreamerService interface {
}

type QueueService interface {
	// AddSong adds the song given to the queue
	AddSong(Song) error
	// Peek returns the song that is queued after the song given
	Peek(Song) (Song, error)
	// Pop pops off the song given if it's the top-most song, otherwise ignores it
	Pop(Song) error
	// Remove removes the song given from the queue;  Remove should only remove
	// the first occurence of the song
	Remove(Song) error
}

type ChatService interface {
	AnnounceSong(Song) error
	AnnounceRequest(Song) error
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

func (s Song) EqualTo(d Song) bool {
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
