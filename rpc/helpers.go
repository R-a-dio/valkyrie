package rpc

import (
	"encoding/hex"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/golang/protobuf/ptypes"
	duration "github.com/golang/protobuf/ptypes/duration"
	timestamp "github.com/golang/protobuf/ptypes/timestamp"
)

func t(x *timestamp.Timestamp) time.Time {
	if x == nil {
		return time.Time{}
	}
	y, _ := ptypes.Timestamp(x)
	return y
}

func tp(x time.Time) *timestamp.Timestamp {
	y, _ := ptypes.TimestampProto(x)
	return y
}

func d(x *duration.Duration) time.Duration {
	if x == nil {
		return 0
	}
	y, _ := ptypes.Duration(x)
	return y
}

func dp(x time.Duration) *duration.Duration {
	return ptypes.DurationProto(x)
}

func toProtoSong(s radio.Song) *Song {
	song := &Song{
		Id:         int32(s.ID),
		Hash:       s.Hash.String(),
		Metadata:   s.Metadata,
		Length:     dp(s.Length),
		LastPlayed: tp(s.LastPlayed),
		SyncTime:   tp(s.SyncTime),
	}

	if s.HasTrack() {
		// track fields
		song.TrackId = int32(s.TrackID)
		song.Artist = s.Artist
		song.Title = s.Title
		song.Album = s.Album
		song.FilePath = s.FilePath
		song.Tags = s.Tags
		song.Acceptor = s.Acceptor
		song.LastEditor = s.LastEditor
		song.Priority = int32(s.Priority)
		song.Usable = s.Usable
		song.LastRequested = tp(s.LastRequested)
		song.RequestCount = int32(s.RequestCount)
		song.RequestDelay = dp(s.RequestDelay)
	}

	return song
}

func fromProtoSong(s *Song) radio.Song {
	var hash radio.SongHash
	hex.Decode(hash[:], []byte(s.Hash))

	var track *radio.DatabaseTrack
	if s.TrackId != 0 {
		delay, _ := ptypes.Duration(s.RequestDelay)
		track = &radio.DatabaseTrack{
			TrackID:       radio.TrackID(s.TrackId),
			Artist:        s.Artist,
			Title:         s.Title,
			Album:         s.Album,
			FilePath:      s.FilePath,
			Tags:          s.Tags,
			Acceptor:      s.Acceptor,
			LastEditor:    s.LastEditor,
			Priority:      int(s.Priority),
			Usable:        s.Usable,
			LastRequested: t(s.LastRequested),
			RequestCount:  int(s.RequestCount),
			RequestDelay:  delay,
		}
	}

	return radio.Song{
		ID:            radio.SongID(s.Id),
		Hash:          hash,
		Metadata:      s.Metadata,
		Length:        d(s.Length),
		LastPlayed:    t(s.LastPlayed),
		DatabaseTrack: track,
		SyncTime:      t(s.SyncTime),
	}
}

func toProtoSongInfo(i radio.SongInfo) *SongInfo {
	return &SongInfo{
		StartTime:  tp(i.Start),
		EndTime:    tp(i.End),
		IsFallback: i.IsFallback,
	}
}

func fromProtoSongInfo(i *SongInfo) radio.SongInfo {
	return radio.SongInfo{
		Start:      t(i.StartTime),
		End:        t(i.EndTime),
		IsFallback: i.IsFallback,
	}
}

func toProtoQueueEntry(entry radio.QueueEntry) *QueueEntry {
	return &QueueEntry{
		Song:              toProtoSong(entry.Song),
		IsUserRequest:     entry.IsUserRequest,
		UserIdentifier:    entry.UserIdentifier,
		ExpectedStartTime: tp(entry.ExpectedStartTime),
	}
}

func fromProtoQueueEntry(entry *QueueEntry) *radio.QueueEntry {
	if entry == nil || entry.Song == nil {
		return nil
	}

	return &radio.QueueEntry{
		Song:              fromProtoSong(entry.Song),
		IsUserRequest:     entry.IsUserRequest,
		UserIdentifier:    entry.UserIdentifier,
		ExpectedStartTime: t(entry.ExpectedStartTime),
	}
}

func toProtoUser(u radio.User) *User {
	// TODO: implement this fully
	return &User{
		Id:       int32(u.ID),
		Username: u.Username,
		Ip:       u.IP,
		Dj:       toProtoDJ(u.DJ),
	}
}

func fromProtoUser(u *User) radio.User {
	if u == nil {
		return radio.User{}
	}
	return radio.User{
		ID:            radio.UserID(u.Id),
		Username:      u.Username,
		Password:      u.Password,
		Email:         u.Email,
		RememberToken: u.RememberToken,
		IP:            u.Ip,
		UpdatedAt:     t(u.UpdatedAt),
		DeletedAt:     t(u.DeletedAt),
		CreatedAt:     t(u.CreatedAt),
		DJ:            fromProtoDJ(u.Dj),
	}
}

func toProtoDJ(d radio.DJ) *DJ {
	// TODO: implement this fully
	return &DJ{
		Id: int32(d.ID),
	}
}

func fromProtoDJ(d *DJ) radio.DJ {
	if d == nil {
		return radio.DJ{}
	}
	// TODO: implement this fully
	return radio.DJ{
		ID: radio.DJID(d.Id),
	}
}

func fromProtoRequestResponse(r *RequestResponse) error {
	if r == nil || r.Success {
		return nil
	}

	return radio.SongRequestError{
		UserMessage: r.Msg,
		UserDelay:   d(r.UserDelay),
		SongDelay:   d(r.SongDelay),
	}
}

func toProtoRequestResponse(err radio.SongRequestError) *RequestResponse {
	return &RequestResponse{
		Msg:       err.UserMessage,
		UserDelay: dp(err.UserDelay),
		SongDelay: dp(err.SongDelay),
	}
}

func toProtoUserError(err error) (*UserError, error) {
	if err == nil {
		return nil, nil
	}
	uerr, ok := err.(radio.UserError)
	if !ok {
		return nil, err
	}
	return &UserError{
		Public:    uerr.Public(),
		UserError: uerr.UserError(),
		Error:     uerr.Error(),
	}, nil
}

func fromProtoUserError(err *UserError) error {
	if err == nil {
		return nil
	}
	return userError{
		userError: err.UserError,
		errorMsg:  err.Error,
		public:    err.Public,
	}
}

type userError struct {
	userError string
	errorMsg  string
	public    bool
}

func (err userError) Error() string {
	return err.errorMsg
}

func (err userError) UserError() string {
	return err.userError
}

func (err userError) Public() bool {
	return err.public
}
