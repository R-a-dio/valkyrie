package radio

import (
	"bytes"
	"context"
	"crypto/sha1"
	"database/sql/driver"
	"encoding/hex"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/R-a-dio/valkyrie/util/eventstream"
	"golang.org/x/crypto/bcrypt"
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

// CalculateCooldown sees if the cooldown given has passed since `last` and returns
// the remaining time if any and a bool indicating if it has passed since then or
// not. It always returns true if `last` is zero.
func CalculateCooldown(delay time.Duration, last time.Time) (time.Duration, bool) {
	// zero time indicates never requested before
	if last.IsZero() {
		return 0, true
	}

	since := time.Since(last)
	if since > delay {
		return 0, true
	}

	return delay - since, false
}

type Status struct {
	// User is the user that is currently broadcasting on the stream
	User User
	// Song is the song that is currently playing on the stream
	Song Song
	// SongInfo is extra information about the song that is currently playing
	SongInfo SongInfo
	// StreamerName is the name given to us by the user that is streaming
	StreamerName string
	// Listeners is the current amount of stream listeners
	Listeners int
	// Thread is an URL to a third-party platform related to the current stream
	Thread string
	// RequestsEnabled tells you if requests to the automated streamer are enabled
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

// UserID is an identifier corresponding to an user
type UserID uint64

// UserPermission is a permission for user authorization
type UserPermission string

func (u UserPermission) String() string {
	return string(u)
}

type UserPermissions map[UserPermission]struct{}

// Has returns true if the permission given is in the UserPermissions
func (up UserPermissions) Has(perm UserPermission) bool {
	if up == nil { // nil map, has no permissions ever
		return false
	}
	_, ok := up[PermActive]
	if !ok { // not an active user, no permissions ever
		return false
	}

	_, ok = up[perm]
	if !ok { // don't have this perm but might be dev who has access to everything
		_, ok = up[PermDev]
		return ok
	}
	return ok
}

// Scan implements sql.Scanner
//
// Done in a way that it expects all permissions to be a single string or []byte
// separated by a comma
func (upp *UserPermissions) Scan(src interface{}) error {
	*upp = make(UserPermissions)
	up := *upp

	switch perms := src.(type) {
	case []byte:
		for _, p := range bytes.Split(perms, []byte(",")) {
			up[UserPermission(p)] = struct{}{}
		}
	case string:
		for _, p := range strings.Split(perms, ",") {
			up[UserPermission(p)] = struct{}{}
		}
	case nil: // no permissions, we made the map above though
	default:
		return fmt.Errorf("invalid argument passed to Scan")
	}

	return nil
}

func AllUserPermissions() []UserPermission {
	return []UserPermission{
		PermActive,
		PermNews,
		PermDJ,
		PermDev,
		PermAdmin,
		PermDatabaseDelete,
		PermDatabaseEdit,
		PermDatabaseView,
		PermPendingEdit,
		PermPendingView,
	}
}

// List of permissions, this should be kept in sync with the database version
const (
	PermActive         = "active"          // User is active
	PermNews           = "news"            // User has news creation/editing access
	PermDJ             = "dj"              // User has access to the icecast proxy
	PermDev            = "dev"             // User is a developer
	PermAdmin          = "admin"           // User is an administrator
	PermDatabaseDelete = "database_delete" // User can delete from the track database
	PermDatabaseEdit   = "database_edit"   // User can edit the track database
	PermDatabaseView   = "database_view"   // User can view the track database
	PermPendingEdit    = "pending_edit"    // User can edit the pending track queue
	PermPendingView    = "pending_view"    // User can view the pending track queue
)

// User is an user account in the database
type User struct {
	ID            UserID
	Username      string
	Password      string
	Email         string
	RememberToken string
	IP            string

	UpdatedAt *time.Time
	DeletedAt *time.Time
	CreatedAt time.Time

	DJ              DJ
	UserPermissions UserPermissions
}

func (u User) ComparePassword(passwd string) error {
	return bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(passwd))
}

// DJID is an identifier corresponding to a dj
type DJID uint64

func (id DJID) String() string {
	return strconv.Itoa(int(id))
}

// DJ is someone that has access to streaming
type DJ struct {
	ID    DJID
	Name  string
	Regex string

	Text  string
	Image string

	Visible  bool
	Priority int
	Role     string

	CSS   string
	Color string
	Theme Theme
}

// TrackState is the state of a Track in storage
type TrackState int

const (
	TrackStateUnverified TrackState = iota
	TrackStatePlayable
)

// ThemeID is the identifier of a website theme
type ThemeID uint64

// Theme is a website theme
type Theme struct {
	ID          ThemeID
	Name        string
	DisplayName string
	Author      string
}

type SongInfo struct {
	// Start is the time at which the current song started playing
	Start time.Time
	// End is the expected time the current song stops playing
	End time.Time
	// IsFallback indicates if the song currently playing is one marked as a
	// fallback song for when the icecast main stream is down
	IsFallback bool
}

type SearchService interface {
	Search(ctx context.Context, query string, limit int64, offset int64) (*SearchResult, error)
	Update(context.Context, ...Song) error
	Delete(context.Context, ...Song) error
}

type SearchResult struct {
	Songs     []Song
	TotalHits int
}

type SongUpdate struct {
	Song
	Info SongInfo
}

type Thread = string

type Listeners = int64

type ManagerService interface {
	CurrentUser(context.Context) (eventstream.Stream[User], error)
	UpdateUser(context.Context, User) error
	CurrentSong(context.Context) (eventstream.Stream[*SongUpdate], error)
	UpdateSong(context.Context, *SongUpdate) error
	CurrentThread(context.Context) (eventstream.Stream[Thread], error)
	UpdateThread(context.Context, Thread) error
	CurrentListeners(context.Context) (eventstream.Stream[Listeners], error)
	UpdateListeners(context.Context, Listeners) error

	Status(context.Context) (*Status, error)
}

func OneOff[T any](ctx context.Context, fn func(context.Context) (eventstream.Stream[T], error)) (T, error) {
	s, err := fn(ctx)
	if err != nil {
		return *new(T), err
	}
	defer s.Close()

	return s.Next()
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

func (qe QueueEntry) String() string {
	est := qe.ExpectedStartTime.Format("2006-01-02 15:04:05")
	if qe.IsUserRequest {
		return fmt.Sprintf("(%s)(R) %s", est, qe.Song.Metadata)
	} else {
		return fmt.Sprintf("(%s)(P) %s", est, qe.Song.Metadata)
	}
}

func (qe *QueueEntry) EqualTo(qe2 QueueEntry) bool {
	return qe != nil &&
		qe.Song.EqualTo(qe2.Song) &&
		qe.UserIdentifier == qe2.UserIdentifier
}

type QueueService interface {
	// AddRequest requests the given song to be added to the queue, the string given
	// is an identifier of the user that requested it
	AddRequest(context.Context, Song, string) error
	// ReserveNext returns the next yet-to-be-reserved entry from the queue
	ReserveNext(context.Context) (*QueueEntry, error)
	// ResetReserved resets the reserved status of all entries returned by ReserveNext
	// but not yet removed by Remove
	ResetReserved(context.Context) error
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

func (s SongID) String() string {
	return strconv.Itoa(int(s))
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

// MarshalJSON implements encoding/json.Marshaler
func (s SongHash) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`"%x"`, s[:])), nil
}

// UnmarshalJSON implements encoding/json.Unmarshaler
func (s *SongHash) UnmarshalJSON(b []byte) error {
	_, err := hex.Decode((*s)[:], b[1:len(b)-1])
	return err
}

var zeroSongHash SongHash

func (s *SongHash) IsZero() bool {
	return *s == zeroSongHash
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

func (t TrackID) String() string {
	return strconv.FormatUint(uint64(t), 10)
}

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

	Priority        int
	Usable          bool
	NeedReplacement bool

	LastRequested time.Time

	RequestCount int
}

// Requestable returns whether this song can be requested by a user
func (s *Song) Requestable() bool {
	if s == nil || s.DatabaseTrack == nil {
		panic("Requestable called with nil database track")
	}
	delay := s.RequestDelay()
	if delay == 0 {
		// unknown song delay
		return false
	}
	if time.Since(s.LastPlayed) < delay {
		return false
	}
	if time.Since(s.LastRequested) < delay {
		return false
	}

	return true
}

var veryFarAway = time.Hour * 24 * 90

func (s *Song) RequestDelay() time.Duration {
	if s == nil || s.DatabaseTrack == nil {
		return 0
	}
	return CalculateRequestDelay(s.RequestCount)
}

// UntilRequestable returns the time until this song can be requested again, returns 0
// if song.Requestable() == true
func (s *Song) UntilRequestable() time.Duration {
	if s.Requestable() {
		return 0
	}
	delay := s.RequestDelay()
	if delay == 0 {
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

	furthest = furthest.Add(delay)
	return time.Until(furthest)
}

// Hydrate tries to fill Song with data from other fields, mostly useful
// for if we have a DatabaseTrack but want to create the Song fields
func (s *Song) Hydrate() {
	s.Metadata = strings.TrimSpace(s.Metadata)
	if !s.HasTrack() {
		s.Hash = NewSongHash(s.Metadata)
		return
	}

	if s.Metadata == "" {
		s.Metadata = Metadata(s.Artist, s.Title)
	}

	s.Hash = NewSongHash(s.Metadata)
}

func Metadata(artist, title string) string {
	if artist != "" {
		return fmt.Sprintf("%s - %s", artist, title)
	}
	return title
}

func NewSong(metadata string, length ...time.Duration) Song {
	song := Song{
		Metadata: metadata,
	}
	if len(length) > 0 {
		song.Length = length[0]
	}
	song.Hydrate()
	return song
}

// HasTrack returns true if t != nil, can be used as Song.HasTrack to check if a track
// was allocated for the embedded field
func (t *DatabaseTrack) HasTrack() bool {
	return t != nil
}

type StorageTx interface {
	Commit() error
	Rollback() error
}

// StorageService is an interface containing all *StorageService interfaces
type StorageService interface {
	SessionStorageService
	RelayStorageService
	QueueStorageService
	SongStorageService
	TrackStorageService
	RequestStorageService
	UserStorageService
	StatusStorageService
	SubmissionStorageService
	NewsStorageService
}

// SessionStorageService is a service that supplies a SessionStorage
type SessionStorageService interface {
	Sessions(context.Context) SessionStorage
	SessionsTx(context.Context, StorageTx) (SessionStorage, StorageTx, error)
}

// SessionStorage stores Session's by a SessionToken
type SessionStorage interface {
	Delete(SessionToken) error
	Get(SessionToken) (Session, error)
	Save(Session) error
}

// SessionToken is the token associated with a singular session
type SessionToken string

// Session is a website user session
type Session struct {
	Token  SessionToken
	Expiry time.Time

	Data []byte
}

// QueueStorageService is a service able to supply a QueueStorage
type QueueStorageService interface {
	Queue(context.Context) QueueStorage
	QueueTx(context.Context, StorageTx) (QueueStorage, StorageTx, error)
}

// QueueStorage stores a queue
type QueueStorage interface {
	// Store stores the queue with the name given
	Store(name string, queue []QueueEntry) error
	// Load returns the queue associated with the name given
	Load(name string) ([]QueueEntry, error)
}

// SongStorageService is a service able to supply a SongStorage
type SongStorageService interface {
	Song(context.Context) SongStorage
	SongTx(context.Context, StorageTx) (SongStorage, StorageTx, error)
}

// SongStorage stores information about songs
//
// A song can be anything that plays on stream, unlike a track which is a specific
// kind of song that we have an audio file for and can be played by the automated streamer
type SongStorage interface {
	// Create creates a new song with the metadata given
	Create(Song) (*Song, error)
	// FromMetadata returns the song associated with the metadata given
	FromMetadata(metadata string) (*Song, error)
	// FromHash returns the song associated with the SongHash given
	FromHash(SongHash) (*Song, error)

	// LastPlayed returns songs that have recently played, up to amount given after
	// applying the offset
	LastPlayed(offset, amount int64) ([]Song, error)
	// LastPlayedCount returns the amount of plays recorded
	LastPlayedCount() (int64, error)
	// PlayedCount returns the amount of times the song has been played on stream
	PlayedCount(Song) (int64, error)
	// AddPlay adds a play to the song. If present, ldiff is the difference in amount
	// of listeners between song-start and song-end
	AddPlay(song Song, ldiff *int) error

	// FavoriteCount returns the amount of users that have added this song to
	// their favorite list
	FavoriteCount(Song) (int64, error)
	// Favorites returns all users that have this song on their favorite list
	Favorites(Song) ([]string, error)
	// FavoritesOf returns all songs that are on a users favorite list
	FavoritesOf(nick string) ([]Song, error)
	// AddFavorite adds the given song to nicks favorite list
	AddFavorite(song Song, nick string) (bool, error)
	// RemoveFavorite removes the given song from nicks favorite list
	RemoveFavorite(song Song, nick string) (bool, error)

	// UpdateLength updates the stored length of the song
	UpdateLength(Song, time.Duration) error
}

// TrackStorageService is a service able to supply a TrackStorage
type TrackStorageService interface {
	Track(context.Context) TrackStorage
	TrackTx(context.Context, StorageTx) (TrackStorage, StorageTx, error)
}

// TrackStorage stores information about tracks
//
// A track is a song that we have the audio file for and can thus be played by
// the automated streaming system
type TrackStorage interface {
	// Get returns a single track with the TrackID given
	Get(TrackID) (*Song, error)
	// All returns all tracks in storage
	All() ([]Song, error)
	// Unusable returns all tracks that are deemed unusable by the streamer
	Unusable() ([]Song, error)
	// Insert inserts a new track, errors if ID or TrackID is set
	Insert(song Song) (TrackID, error)
	// UpdateMetadata updates track metadata only (artist/title/album/tags/filepath/needreplacement)
	UpdateMetadata(song Song) error
	// UpdateUsable sets usable to the state given
	UpdateUsable(song Song, state TrackState) error

	// UpdateRequestInfo is called after a track has been requested, this should do any
	// necessary book-keeping related to that
	UpdateRequestInfo(TrackID) error
	// UpdateLastPlayed sets the last time the track was played to the current time
	UpdateLastPlayed(TrackID) error
	// UpdateLastRequested sets the last time the track was requested to the current time
	UpdateLastRequested(TrackID) error

	// BeforeLastRequested returns all tracks that have their LastRequested before the
	// time given
	BeforeLastRequested(before time.Time) ([]Song, error)
	// DecrementRequestCount decrements the RequestCount for all tracks that have
	// their LastRequested before the time given
	DecrementRequestCount(before time.Time) error

	// QueueCandidates returns tracks that are candidates to be queue'd by the
	// default queue implementation
	QueueCandidates() ([]TrackID, error)
}

// RequestStorageService is a service able to supply a RequestStorage
type RequestStorageService interface {
	Request(context.Context) RequestStorage
	RequestTx(context.Context, StorageTx) (RequestStorage, StorageTx, error)
}

// RequestStorage stores things related to automated streamer song requests
type RequestStorage interface {
	// LastRequest returns the time of when the identifier given last requested
	// a song from the streamer
	LastRequest(identifier string) (time.Time, error)
	// UpdateLastRequest updates the LastRequest time to the current time for the
	// identifier given
	UpdateLastRequest(identifier string) error
}

// UserStorageService is a service able to supply a UserStorage
type UserStorageService interface {
	User(context.Context) UserStorage
	UserTx(context.Context, StorageTx) (UserStorage, StorageTx, error)
}

// UserStorage stores things related to users with actual accounts on the website
type UserStorage interface {
	// All returns all users
	All() ([]User, error)
	// Get returns the user matching the name given
	Get(name string) (*User, error)
	// GetByDJID returns the user associated with the DJID
	GetByDJID(DJID) (*User, error)
	// UpdateUser updates the given user
	UpdateUser(User) (User, error)
	// LookupName matches the name given fuzzily to a user
	LookupName(name string) (*User, error)
	// ByNick returns an user that is associated with the nick given
	ByNick(nick string) (*User, error)
	// Permissions returns all available permissions
	Permissions() ([]UserPermission, error)
	// RecordListeners records a history of listener count
	RecordListeners(int, User) error
}

// StatusStorageService is a service able to supply a StatusStorage
type StatusStorageService interface {
	Status(context.Context) StatusStorage
}

// StatusStorage stores a Status structure
type StatusStorage interface {
	// Store stores the Status given
	Store(Status) error
	// Load returns the previously stored Status
	Load() (*Status, error)
}

// NewsStorageService is a service able to supply a NewsStorage
type NewsStorageService interface {
	News(context.Context) NewsStorage
	NewsTx(context.Context, StorageTx) (NewsStorage, StorageTx, error)
}

// NewsStorage stores website news and its comments
type NewsStorage interface {
	// Get returns the news post associated with the id given
	Get(NewsPostID) (*NewsPost, error)
	// Create creates a new news post
	//
	// Required fields to create a post are (title, header, body, user)
	Create(NewsPost) (NewsPostID, error)
	// Update updates the news post entry
	Update(NewsPost) error
	// Delete deletes a news post
	Delete(NewsPostID) error
	// List returns a list of news post starting at offset and returning up to
	// limit amount of posts, chronologically sorted by creation date
	List(limit int, offset int) (NewsList, error)
	// ListPublic returns the same thing as List but with deleted and private
	// posts filtered out
	ListPublic(limit int, offset int) (NewsList, error)
	// Comments returns all comments associated with the news post given
	Comments(NewsPostID) ([]NewsComment, error)
}

// NewsList contains multiple news posts and a total count of posts
type NewsList struct {
	Entries []NewsPost
	Total   int
}

// NewsPostID is an identifier for a news post
type NewsPostID int64

// NewsPost is a single news post created on the website
type NewsPost struct {
	ID     NewsPostID
	Title  string
	Header string
	Body   string

	User      User
	DeletedAt *time.Time
	CreatedAt time.Time
	UpdatedAt *time.Time
	Private   bool
}

// HasRequired tells if you all required fields in a news post are filled,
// returns the field name that is missing and a boolean
func (np NewsPost) HasRequired() (string, bool) {
	var field string
	switch {
	case np.Title == "":
		field = "title"
	case np.Header == "":
		field = "header"
	case np.Body == "":
		field = "body"
	case np.User.ID == 0:
		field = "user"
	}

	return field, field == ""
}

// NewsCommentID is an identifier for a news comment
type NewsCommentID int64

// NewsComment is a single comment under a news post on the website
type NewsComment struct {
	ID         NewsCommentID
	PostID     NewsPostID
	Body       string
	Identifier string

	// Optional, only filled if an account-holder comments
	User      *User
	DeletedAt *time.Time
	CreatedAt time.Time
	UpdatedAt *time.Time
}

// SubmissionStorageService is a service able to supply a SubmissionStorage
type SubmissionStorageService interface {
	Submissions(context.Context) SubmissionStorage
	SubmissionsTx(context.Context, StorageTx) (SubmissionStorage, StorageTx, error)
}

func CalculateSubmissionCooldown(t time.Time) time.Duration {
	if time.Since(t) > time.Hour {
		return 0
	}
	return (time.Hour) - time.Since(t)
}

// SubmissionStorage stores stuff related to the reviewing of submissions
// and associated information
type SubmissionStorage interface {
	// LastSubmissionTime returns the last known time of when the identifier
	// was used to upload a submission
	LastSubmissionTime(identifier string) (time.Time, error)
	// UpdateSubmissionTime updates the last submission time to the current time
	// for the identifier given
	UpdateSubmissionTime(identifier string) error
	// SubmissionStats returns the submission stats for the identifier given.
	SubmissionStats(identifier string) (SubmissionStats, error)

	// All returns all submissions
	All() ([]PendingSong, error)
	// InsertSubmission inserts a new pending song into the database
	InsertSubmission(PendingSong) error
	// GetSubmission returns a pending song by ID
	GetSubmission(SubmissionID) (*PendingSong, error)
	// RemoveSubmission removes a pending song by ID
	RemoveSubmission(SubmissionID) error

	// InsertPostPending inserts post-pending data
	InsertPostPending(PendingSong) error
}

type SubmissionStats struct {
	// Amount of submissions in the pending queue
	CurrentPending int `db:"current_pending"`
	// Information about accepted songs
	AcceptedTotal        int `db:"accepted_total"`
	AcceptedLastTwoWeeks int `db:"accepted_last_two_weeks"`
	AcceptedYou          int `db:"accepted_you"`
	RecentAccepts        []Song

	// Information about declined songs
	DeclinedTotal        int `db:"declined_total"`
	DeclinedLastTwoWeeks int `db:"declined_last_two_weeks"`
	DeclinedYou          int `db:"declined_you"`
	RecentDeclines       []PendingSong

	// Information about (You)
	LastSubmissionTime time.Time `db:"last_submission_time"`
}

// SubmissionID is the ID of a pending song
type SubmissionID uint

// SubmissionStatus is the status of a submitted song
type SubmissionStatus int

// Possible status for song submissions
const (
	SubmissionInvalid  SubmissionStatus = -1
	SubmissionDeclined SubmissionStatus = iota
	SubmissionAccepted
	SubmissionReplacement
	SubmissionAwaitingReview
)

// PendingSong is a song currently awaiting approval in the pending queue
type PendingSong struct {
	ID SubmissionID
	// Status of the song (accepted/declined/pending)
	Status SubmissionStatus
	// Artist of the song
	Artist string
	// Title of the song
	Title string
	// Album of the song
	Album string
	// Tags of the song
	Tags string
	// FilePath on disk
	FilePath string
	// Comment given by the uploader
	Comment string
	// Filename is the original filename from the uploader
	Filename string
	// UserIdentifier is the unique identifier for the uploader
	UserIdentifier string
	// SubmittedAt is the time of submission
	SubmittedAt time.Time
	// ReviewedAt tells you when the song was reviewed
	ReviewedAt time.Time
	// Duplicate indicates if this might be a duplicate
	Duplicate bool
	// ReplacementID is the TrackID that this upload will replace
	ReplacementID TrackID
	// Bitrate of the file
	Bitrate int
	// Length of the song
	Length time.Duration
	// Format of the song
	Format string
	// EncodingMode is the encoding mode used for the file
	EncodingMode string

	// Decline fields
	Reason string

	// Accepted fields
	GoodUpload   bool
	AcceptedSong *Song
}

// RelayStorage deals with the relays table.
type RelayStorage interface {
	Update(r Relay) error
	All() ([]Relay, error)
}

// RelayStorageService is a service able to supply a RelayStorage
type RelayStorageService interface {
	Relay(context.Context) RelayStorage
	RelayTx(context.Context, StorageTx) (RelayStorage, StorageTx, error)
}

// Relay is a stream relay for use by the load balancer.
type Relay struct {
	Name, Status, Stream, Err string
	Online, Disabled, Noredir bool
	Listeners, Max            int
}

// Score takes in a relay and returns its score. Score ranges from 0 to 1, where 1 is perfect.
// Score punishes a relay for having a high ratio of listeners to its max.
func (r Relay) Score() float64 {
	// Avoid a division by zero panic.
	if r.Max <= 0 {
		return 0
	}
	return 1.0 - float64(2.0*r.Listeners)/float64(r.Listeners+r.Max)
}
