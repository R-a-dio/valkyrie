package rpc

import (
	"encoding/hex"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
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

func toProtoError(err error) ([]*Error, error) {
	var stack []*Error

	if err == nil {
		return stack, nil
	}

	for {
		e, ok := err.(*errors.Error)
		if !ok {
			stack = append(stack, &Error{
				Error: err.Error(),
			})
			break
		}

		stack = append(stack, &Error{
			Kind:    uint32(e.Kind),
			Op:      string(e.Op),
			SongId:  int32(e.SongID),
			TrackId: int32(e.TrackID),
			Delay:   dp(time.Duration(e.Delay)),
			Info:    string(e.Info),
		})

		if e.Err == nil {
			// no other nested errors
			break
		}
		// else work on the next one
		err = e.Err
	}

	return stack, nil
}

func fromProtoError(stack []*Error) error {
	if len(stack) == 0 {
		return nil
	}

	var err *errors.Error
	var prev *errors.Error
	var top error
	for i := 0; i < len(stack); i++ {
		e := stack[i]

		if e.Error != "" {
			err = &errors.Error{
				Err: errors.Errorf("%s", e.Error),
			}
		} else {
			err = &errors.Error{
				Kind:    errors.Kind(e.Kind),
				Op:      errors.Op(e.Op),
				SongID:  radio.SongID(e.SongId),
				TrackID: radio.TrackID(e.TrackId),
				Delay:   errors.Delay(d(e.Delay)),
				Info:    errors.Info(e.Info),
			}
		}

		if top == nil {
			top = err
		}

		if prev != nil {
			prev.Err = err
		}
		prev = err
	}

	return top
}
