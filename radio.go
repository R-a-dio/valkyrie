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
	"github.com/rs/xid"
	"golang.org/x/crypto/bcrypt"
)

const (
	LimitArtistLength = 500
	LimitAlbumLength  = 200
	LimitTitleLength  = 200
	LimitReasonLength = 120
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

// IsRobot indicates if the user has the Robot flag
func IsRobot(user User) bool {
	return user.UserPermissions.HasExplicit(PermRobot)
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
	// StreamUser is the user that is currently streaming, will be nil
	// if no one is connected on the master mountpoint (even short drops)
	StreamUser *User
	// User is the user that is currently or was the last to broadcast
	// on the stream
	User User
	// Song is the song that is currently playing on the stream
	Song Song
	// SongInfo is extra information about the song that is currently playing
	SongInfo SongInfo
	// StreamerName is the name given to us by the user that is streaming
	StreamerName string
	// Listeners is the current amount of stream listeners
	Listeners Listeners
	// Thread is an URL to a third-party platform related to the current stream
	Thread string
}

func (s *Status) IsZero() bool {
	ok := s.User.ID == 0 &&
		s.User.DJ.ID == 0 &&
		s.Song.ID == 0 &&
		s.SongInfo == (SongInfo{}) &&
		s.StreamerName == "" &&
		s.Listeners == 0 &&
		s.Thread == "" &&
		(!s.Song.HasTrack() || s.Song.TrackID == 0)

	return ok
}

// UserID is an identifier corresponding to an user
type UserID uint32

func ParseUserID(s string) (UserID, error) {
	id, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		return 0, err
	}
	return UserID(id), nil
}

func (id UserID) String() string {
	return strconv.FormatUint(uint64(id), 10)
}

// UserPermission is a permission for user authorization
type UserPermission string

func (u UserPermission) String() string {
	return string(u)
}

type UserPermissions map[UserPermission]struct{}

// Has returns true if the permissions in UserPermission allow
// access to the permission given
func (up UserPermissions) Has(perm UserPermission) bool {
	if !up.has(PermActive) { // not an active user
		return false
	}

	return up.has(perm) || up.has(PermDev)
}

// HasExplicit returns true if the permission given is explicitly in the UserPermissions
func (up UserPermissions) HasExplicit(perm UserPermission) bool {
	return up.has(perm)
}

// HasEdit returns true if the UserPermissions is allowed to edit the given permission
func (up UserPermissions) HasEdit(perm UserPermission) bool {
	// devs can edit any permission
	if up.has(PermDev) {
		return true
	}
	// admins can edit every permission except for PermDev
	if up.has(PermAdmin) && perm != PermDev {
		return true
	}
	// no one else can edit permissions
	return false
}

func (up UserPermissions) has(perm UserPermission) bool {
	if up == nil {
		return false
	}
	_, ok := up[perm]
	return ok
}

// Scan implements sql.Scanner
//
// Done in a way that it expects all permissions to be a single string or []byte
// separated by a comma
func (upp *UserPermissions) Scan(src interface{}) error {
	if upp == nil {
		return fmt.Errorf("nil found in Scan")
	}

	*upp = make(UserPermissions)
	up := *upp

	switch perms := src.(type) {
	case []byte:
		for _, p := range bytes.Split(perms, []byte(",")) {
			up[UserPermission(bytes.TrimSpace(p))] = struct{}{}
		}
	case string:
		for _, p := range strings.Split(perms, ",") {
			up[UserPermission(strings.TrimSpace(p))] = struct{}{}
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
		PermStaff,
		PermDatabaseDelete,
		PermDatabaseEdit,
		PermDatabaseView,
		PermPendingEdit,
		PermPendingView,
		PermQueueEdit,
		PermRobot,
		PermScheduleEdit,
		PermListenerView,
		PermListenerKick,
		PermProxyKick,
		PermTelemetryView,
	}
}

// List of permissions, this should be kept in sync with the database version
const (
	PermActive         = "active"          // User is active
	PermNews           = "news"            // User has news creation/editing access
	PermDJ             = "dj"              // User has access to the icecast proxy
	PermDev            = "dev"             // User is a developer
	PermAdmin          = "admin"           // User is an administrator
	PermStaff          = "staff"           // User is staff, only for display purposes on staff page
	PermDatabaseDelete = "database_delete" // User can delete from the track database
	PermDatabaseEdit   = "database_edit"   // User can edit the track database
	PermDatabaseView   = "database_view"   // User can view the track database
	PermPendingEdit    = "pending_edit"    // User can edit the pending track queue
	PermPendingView    = "pending_view"    // User can view the pending track queue
	PermQueueEdit      = "queue_edit"      // User can edit the streamer queue
	PermRobot          = "robot"           // User is not human
	PermScheduleEdit   = "schedule_edit"   // User can edit the schedule
	PermListenerView   = "listener_view"   // User can view the listener list
	PermListenerKick   = "listener_kick"   // User can kick listeners
	PermProxyKick      = "proxy_kick"      // User can kick streamers"
	PermTelemetryView  = "telemetry_view"  // User can view telemetry backend
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

func (u *User) IsValid() bool {
	return u != nil && u.Username != ""
}

var bcryptCost = 14

func GenerateHashFromPassword(passwd string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(passwd), bcryptCost)
	if err != nil {
		return "", err
	}
	return string(h), nil
}

// DJID is an identifier corresponding to a dj
type DJID int32

func ParseDJID(s string) (DJID, error) {
	id, err := strconv.ParseInt(s, 10, 32)
	if err != nil {
		return 0, err
	}
	return DJID(id), nil
}

func (id DJID) String() string {
	return strconv.FormatInt(int64(id), 10)
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
type ThemeID uint32

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
}

type SearchService interface {
	Search(ctx context.Context, query string, limit int64, offset int64) (*SearchResult, error)
	Update(context.Context, ...Song) error
	Delete(context.Context, ...TrackID) error
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

// Listeners is a dedicated type for an amount of listeners
type Listeners = int64

// ListenerClientID is an identifier unique to each listener
type ListenerClientID uint64

// ParseListenerClientID parses a string as a ListenerClientID, input
// is expected to be like the output of ListenerClientID.String
func ParseListenerClientID(s string) (ListenerClientID, error) {
	id, err := strconv.ParseUint(s, 10, 64)
	return ListenerClientID(id), err
}

// String returns the ListenerClientID as a string
func (c ListenerClientID) String() string {
	return strconv.FormatUint(uint64(c), 10)
}

// Listener is a listener of the stream
type Listener struct {
	ID        ListenerClientID
	UserAgent string
	IP        string
	Start     time.Time
}

type ListenerTrackerService interface {
	// ListClients lists all listeners currently connected
	// to the stream
	ListClients(context.Context) ([]Listener, error)
	// RemoveClient kicks a listener from the stream
	RemoveClient(context.Context, ListenerClientID) error
}

type ManagerService interface {
	UpdateFromStorage(context.Context) error

	CurrentUser(context.Context) (eventstream.Stream[*User], error)
	UpdateUser(context.Context, *User) error
	CurrentSong(context.Context) (eventstream.Stream[*SongUpdate], error)
	UpdateSong(context.Context, *SongUpdate) error
	CurrentThread(context.Context) (eventstream.Stream[Thread], error)
	UpdateThread(context.Context, Thread) error
	CurrentListeners(context.Context) (eventstream.Stream[Listeners], error)
	UpdateListeners(context.Context, Listeners) error

	CurrentStatus(context.Context) (eventstream.Stream[Status], error)
}

type ProxyService interface {
	MetadataStream(context.Context) (eventstream.Stream[ProxyMetadataEvent], error)
	SourceStream(context.Context) (eventstream.Stream[ProxySourceEvent], error)
	KickSource(context.Context, SourceID) error
	ListSources(context.Context) ([]ProxySource, error)
}

type ProxySource struct {
	User      User
	ID        SourceID
	Start     time.Time
	MountName string
	IP        string
	UserAgent string
	Metadata  string
	Priority  uint32
}

type ProxyMetadataEvent struct {
	User      User
	MountName string
	Metadata  string
}

type ProxySourceEvent struct {
	ID        SourceID
	MountName string
	User      User
	Event     ProxySourceEventType
}

type ProxySourceEventType int

type SourceID struct {
	xid.ID
}

func ParseSourceID(s string) (SourceID, error) {
	id, err := xid.FromString(s)
	return SourceID{id}, err
}

const (
	SourceDisconnect ProxySourceEventType = iota
	SourceConnect
	SourceLive
)

type StreamerService interface {
	Start(context.Context) error
	Stop(ctx context.Context, force bool) error

	RequestSong(context.Context, Song, string) error
	Queue(context.Context) (Queue, error)
}

type Queue []QueueEntry

// Limit limits the queue size to the maxSize given or
// the whole queue if maxSize < len(queue)
func (q Queue) Limit(maxSize int) Queue {
	return q[:min(maxSize, len(q))]
}

// Length returns the length of the queue
func (q Queue) Length() time.Duration {
	if len(q) > 0 {
		last := q[len(q)-1]
		return time.Until(last.ExpectedStartTime) + last.Length
	}
	return 0
}

// RequestAmount returns the amount of QueueEntries that have
// IsUserRequest set to true
func (q Queue) RequestAmount() int {
	var n int
	for _, entry := range q {
		if entry.IsUserRequest {
			n++
		}
	}
	return n
}

func NewQueueID() QueueID {
	return QueueID{xid.New()}
}

type QueueID struct {
	xid.ID
}

func ParseQueueID(s string) (QueueID, error) {
	id, err := xid.FromString(s)
	return QueueID{id}, err
}

func (qid QueueID) String() string {
	return qid.ID.String()
}

// QueueEntry is a Song used in the QueueService
type QueueEntry struct {
	// QueueID is a unique identifier for this queue entry
	QueueID QueueID
	// Song that is queued
	Song
	// IsUserRequest should be true if this song was added to the queue
	// by a third-party user
	IsUserRequest bool
	// UserIdentifier should be a way to identify the user that requested the song
	UserIdentifier string
	// ExpectedStartTime is the expected time this song will be played on stream
	ExpectedStartTime time.Time
}

func (qe QueueEntry) Copy() QueueEntry {
	qe.Song = qe.Song.Copy()
	return qe
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
	return qe.QueueID == qe2.QueueID
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
	Remove(context.Context, QueueID) (bool, error)
	// Entries returns all entries in the queue
	Entries(context.Context) (Queue, error)
}

type AnnounceService interface {
	AnnounceSong(context.Context, Status) error
	AnnounceRequest(context.Context, Song) error
	AnnounceUser(context.Context, *User) error
}

// SongID is a songs identifier
type SongID uint32

func ParseSongID(s string) (SongID, error) {
	id, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		return 0, err
	}
	return SongID(id), nil
}

// Scan implements sql.Scanner
func (s *SongID) Scan(src any) error {
	// Scanner is only implemented so that null values are supported
	// without introducing an intermediate type
	if src == nil {
		return nil
	}

	var err error
	switch v := src.(type) {
	case int64:
		*s = SongID(v)
	case uint64: // mysql driver sometimes gives you this
		*s = SongID(v)
	case float64:
		*s = SongID(v)
	case []byte: // decimals
		*s, err = ParseSongID(string(v))
	case string:
		*s, err = ParseSongID(v)
	}

	return err
}

func (s SongID) String() string {
	return strconv.FormatUint(uint64(s), 10)
}

// SongHash is a sha1 hash
type SongHash [sha1.Size]byte

// ParseSongHash reverts SongHash.String
func ParseSongHash(s string) (SongHash, error) {
	var hash SongHash
	_, err := hex.Decode(hash[:], []byte(s))
	return hash, err
}

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
	if src == nil {
		return nil
	}

	var err error
	switch v := src.(type) {
	case []byte:
		_, err = hex.Decode((*s)[:], v)
	case string:
		_, err = hex.Decode((*s)[:], []byte(v))
	default:
		err = fmt.Errorf("unsupported type in SongHash.Scan: %t", src)
	}
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
	// HashLink is the same as Hash but points to another song that we share some data with
	HashLink SongHash
	// Metadata is simple metadata for this song in the format 'artist - title'
	Metadata string
	// Length is the length of the song
	Length time.Duration
	// LastPlayed is the last time this song played on stream
	LastPlayed time.Time
	// LastPlayedBy is the user that last played this song, can be nil
	LastPlayedBy *User
	// DatabaseTrack is only available if the song is in our streamer database
	*DatabaseTrack

	// SyncTime is the time this Song was returned by the database layer
	SyncTime time.Time
}

// EqualTo returns s == d based on unique fields
func (s Song) EqualTo(d Song) bool {
	// both songs we get are copies so we can hydrate them inside
	// and use the hash as unique identifier since it should handle
	// any special cases
	s.Hydrate()
	d.Hydrate()

	// equal hash means they're just the same metadata
	if s.Hash == d.Hash {
		return true
	}
	// equal hashlink to hash means the metadata was the same at some point
	if s.HashLink == d.Hash || s.Hash == d.HashLink {
		return true
	}
	// no tracks to compare, so these are then not equal
	if !s.HasTrack() || !d.HasTrack() {
		return false
	}
	// we do have a track, equal if the TrackID matches
	return s.TrackID == d.TrackID
}

// TrackID is a database track identifier
type TrackID uint32

func ParseTrackID(s string) (TrackID, error) {
	id, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		return 0, err
	}
	return TrackID(id), nil
}

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

// Copy copies the song and returns it
func (s Song) Copy() Song {
	if s.DatabaseTrack != nil {
		dt := *s.DatabaseTrack
		s.DatabaseTrack = &dt
	}
	if s.LastPlayedBy != nil {
		lpb := *s.LastPlayedBy
		s.LastPlayedBy = &lpb
	}
	return s
}

// Requestable returns whether this song can be requested by a user
func (s *Song) Requestable() bool {
	if s == nil || s.DatabaseTrack == nil {
		return false
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
	// trim any whitespace from the metadata
	s.Metadata = strings.TrimSpace(s.Metadata)
	// if our metadata is empty at this point, and we have a database track
	// we lookup the artist and title of the track to create our metadata
	if s.Metadata == "" && s.HasTrack() {
		s.Metadata = Metadata(s.Artist, s.Title)
	}

	if s.Metadata == "" {
		// no metadata to work with, to avoid a bogus hash creation down below
		// we just exit early and don't update anything
		return
	}

	// generate a hash from the metadata
	s.Hash = NewSongHash(s.Metadata)
	// and if our HashLink isn't set yet update that too
	if s.HashLink.IsZero() {
		s.HashLink = s.Hash
	}
}

func Metadata(artist, title string) string {
	artist = strings.TrimSpace(artist)
	title = strings.TrimSpace(title)

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
	ScheduleStorageService
	// Close closes the storage service and cleans up any resources
	Close() error
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

type LastPlayedKey uint32

const LPKeyLast = LastPlayedKey(math.MaxUint32)

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
	LastPlayed(key LastPlayedKey, amountPerPage int) ([]Song, error)
	// LastPlayedPagination looks up keys for adjacent pages of key
	LastPlayedPagination(key LastPlayedKey, amountPerPage, pageCount int) (prev, next []LastPlayedKey, err error)
	// LastPlayedCount returns the amount of plays recorded
	LastPlayedCount() (int64, error)
	// PlayedCount returns the amount of times the song has been played on stream
	PlayedCount(Song) (int64, error)
	// AddPlay adds a play to the song. streamer is the dj that played the song.
	// If present, ldiff is the difference in amount of listeners between
	// song-start and song-end.
	AddPlay(song Song, streamer User, ldiff *Listeners) error

	// FavoriteCount returns the amount of users that have added this song to
	// their favorite list
	FavoriteCount(Song) (int64, error)
	// Favorites returns all users that have this song on their favorite list
	Favorites(Song) ([]string, error)
	// FavoritesOf returns all songs that are on a users favorite list
	FavoritesOf(nick string, limit, offset int64) ([]Song, int64, error)
	// FavoritesOfDatabase returns all songs that are on a users favorite list
	// and also have a track database
	FavoritesOfDatabase(nick string) ([]Song, error)
	// AddFavorite adds the given song to nicks favorite list
	AddFavorite(song Song, nick string) (bool, error)
	// RemoveFavorite removes the given song from nicks favorite list
	RemoveFavorite(song Song, nick string) (bool, error)

	// UpdateLength updates the stored length of the song
	UpdateLength(Song, time.Duration) error
	// UpdateHashLink updates the HashLink of the song
	UpdateHashLink(old SongHash, new SongHash) error
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
	// AllRaw returns all tracks in storage, but without making sure all fields
	// are filled. This returns them as-is straight from storage
	AllRaw() ([]Song, error)
	// Delete removes a track from storage
	Delete(TrackID) error
	// Unusable returns all tracks that are deemed unusable by the streamer
	Unusable() ([]Song, error)
	// NeedReplacement returns the song that need a replacement
	NeedReplacement() ([]Song, error)
	// Insert inserts a new track, errors if ID or TrackID is set
	Insert(song Song) (TrackID, error)
	// Random returns limit amount of usable tracks
	Random(limit int) ([]Song, error)
	// RandomFavorite returns limit amount of tracks that are on the nicks favorite list
	RandomFavoriteOf(nick string, limit int) ([]Song, error)

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
	// Create creates a user
	Create(User) (UserID, error)
	// CreateDJ creates a DJ for the user given
	CreateDJ(User, DJ) (DJID, error)
	// Get returns the user matching the name given
	Get(name string) (*User, error)
	// GetByID returns the user associated with the UserID
	GetByID(UserID) (*User, error)
	// GetByDJID returns the user associated with the DJID
	GetByDJID(DJID) (*User, error)
	// Update updates the given user
	Update(User) (User, error)
	// LookupName matches the name given fuzzily to a user
	LookupName(name string) (*User, error)
	// ByNick returns an user that is associated with the nick given
	ByNick(nick string) (*User, error)
	// Permissions returns all available permissions
	Permissions() ([]UserPermission, error)
	// RecordListeners records a history of listener count
	RecordListeners(Listeners, User) error
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
	List(limit int64, offset int64) (NewsList, error)
	// ListPublic returns the same thing as List but with deleted and private
	// posts filtered out
	ListPublic(limit int64, offset int64) (NewsList, error)
	// AddComment adds a comment to a news post
	AddComment(NewsComment) (NewsCommentID, error)
	// Comments returns all comments associated with the news post given
	Comments(NewsPostID) ([]NewsComment, error)
	// CommentsPublic returns all comments that were not deleted
	CommentsPublic(NewsPostID) ([]NewsComment, error)
}

// NewsList contains multiple news posts and a total count of posts
type NewsList struct {
	Entries []NewsPost
	Total   int
}

// NewsPostID is an identifier for a news post
type NewsPostID uint32

func (id NewsPostID) String() string {
	return strconv.FormatUint(uint64(id), 10)
}

func ParseNewsPostID(s string) (NewsPostID, error) {
	id, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		return 0, err
	}
	return NewsPostID(id), nil
}

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
type NewsCommentID uint32

func (id NewsCommentID) String() string {
	return strconv.FormatUint(uint64(id), 10)
}

func ParseNewsCommentID(s string) (NewsCommentID, error) {
	id, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		return 0, err
	}
	return NewsCommentID(id), nil
}

// NewsComment is a single comment under a news post on the website
type NewsComment struct {
	ID         NewsCommentID
	PostID     NewsPostID
	Body       string
	Identifier string

	// Optional, only filled if an account-holder comments
	UserID    *UserID
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
	RecentAccepts        []PostPendingSong

	// Information about declined songs
	DeclinedTotal        int `db:"declined_total"`
	DeclinedLastTwoWeeks int `db:"declined_last_two_weeks"`
	DeclinedYou          int `db:"declined_you"`
	RecentDeclines       []PostPendingSong

	// Information about (You)
	LastSubmissionTime time.Time `db:"last_submission_time"`
}

// SubmissionID is the ID of a pending song
type SubmissionID uint32

func (id SubmissionID) String() string {
	return strconv.FormatUint(uint64(id), 10)
}

func ParseSubmissionID(s string) (SubmissionID, error) {
	id, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		return 0, err
	}
	return SubmissionID(id), nil
}

// SubmissionStatus is the status of a submitted song
type SubmissionStatus int

// Possible status for song submissions
const SubmissionInvalid SubmissionStatus = -1
const (
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
	ReplacementID *TrackID
	// Bitrate of the file
	Bitrate uint
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

func (p PendingSong) Metadata() string {
	return Metadata(p.Artist, p.Title)
}

type PostPendingID int32

func (id PostPendingID) String() string {
	return strconv.FormatInt(int64(id), 10)
}

func ParsePostPendingID(s string) (PostPendingID, error) {
	id, err := strconv.ParseInt(s, 10, 32)
	if err != nil {
		return 0, err
	}
	return PostPendingID(id), nil
}

type PostPendingSong struct {
	ID             PostPendingID
	AcceptedSong   *TrackID
	Metadata       string
	UserIdentifier string
	ReviewedAt     time.Time
	DeclineReason  *string
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

type ScheduleStorageService interface {
	Schedule(context.Context) ScheduleStorage
	ScheduleTx(context.Context, StorageTx) (ScheduleStorage, StorageTx, error)
}

type ScheduleStorage interface {
	// Latest returns the latest version of the schedule, one entry for
	// each day in order from Monday to Sunday. entry is nil if there is
	// no schedule for that day
	Latest() ([]*ScheduleEntry, error)
	// Update updates the schedule with the entry given
	Update(ScheduleEntry) error
	// History returns the previous versions of ScheduleEntry
	History(day ScheduleDay, limit, offset int64) ([]ScheduleEntry, error)
}

type ScheduleID uint32

type ScheduleDay uint8

const (
	Monday ScheduleDay = iota
	Tuesday
	Wednesday
	Thursday
	Friday
	Saturday
	Sunday
	UnknownDay ScheduleDay = 255
)

func ParseScheduleDay(s string) ScheduleDay {
	switch s {
	case "Monday":
		return Monday
	case "Tuesday":
		return Tuesday
	case "Wednesday":
		return Wednesday
	case "Thursday":
		return Thursday
	case "Friday":
		return Friday
	case "Saturday":
		return Saturday
	case "Sunday":
		return Sunday
	}
	return UnknownDay
}

func (day ScheduleDay) String() string {
	switch day {
	case Monday:
		return "Monday"
	case Tuesday:
		return "Tuesday"
	case Wednesday:
		return "Wednesday"
	case Thursday:
		return "Thursday"
	case Friday:
		return "Friday"
	case Saturday:
		return "Saturday"
	case Sunday:
		return "Sunday"
	}
	return "Unknown"
}

type ScheduleEntry struct {
	ID ScheduleID
	// Weekday is the day this entry is for
	Weekday ScheduleDay
	// Text is the actual body of the entry
	Text string
	// Owner is who "owns" this day for streaming rights
	Owner *User
	// UpdatedAt is when this was updated
	UpdatedAt time.Time
	// UpdatedBy is who updated this
	UpdatedBy User
	// Notification indicates if we should notify users of this entry
	Notification bool
}
