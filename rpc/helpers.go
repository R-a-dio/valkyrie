package rpc

import (
	"encoding/hex"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/golang/protobuf/ptypes"
)

func toProtoSong(s radio.Song) *Song {
	lp, _ := ptypes.TimestampProto(s.LastPlayed)
	song := &Song{
		Id:         int32(s.ID),
		Hash:       s.Hash.String(),
		Metadata:   s.Metadata,
		Length:     ptypes.DurationProto(s.Length),
		LastPlayed: lp,
	}

	if s.HasTrack() {
		lr, _ := ptypes.TimestampProto(s.LastRequested)
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
		song.LastRequested = lr
		song.RequestCount = int32(s.RequestCount)
		song.RequestDelay = ptypes.DurationProto(s.RequestDelay)
	}

	return song
}

func fromProtoSong(s *Song) radio.Song {
	lp, _ := ptypes.Timestamp(s.LastPlayed)
	length, _ := ptypes.Duration(s.Length)
	var hash radio.SongHash
	hex.Decode(hash[:], []byte(s.Hash))

	var track *radio.DatabaseTrack
	if s.TrackId != 0 {
		lr, _ := ptypes.Timestamp(s.LastRequested)
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
			LastRequested: lr,
			RequestCount:  int(s.RequestCount),
			RequestDelay:  delay,
		}
	}

	return radio.Song{
		ID:            radio.SongID(s.Id),
		Hash:          hash,
		Metadata:      s.Metadata,
		Length:        length,
		LastPlayed:    lp,
		DatabaseTrack: track,
	}
}

func toProtoSongInfo(i radio.SongInfo) *SongInfo {
	start, _ := ptypes.TimestampProto(i.Start)
	end, _ := ptypes.TimestampProto(i.End)
	return &SongInfo{
		StartTime: start,
		EndTime:   end,
	}
}

func fromProtoSongInfo(i *SongInfo) radio.SongInfo {
	start, _ := ptypes.Timestamp(i.StartTime)
	end, _ := ptypes.Timestamp(i.EndTime)
	return radio.SongInfo{
		Start: start,
		End:   end,
	}
}

func toProtoQueueEntry(entry radio.QueueEntry) *QueueEntry {
	est, _ := ptypes.TimestampProto(entry.ExpectedStartTime)
	return &QueueEntry{
		Song:              toProtoSong(entry.Song),
		IsUserRequest:     entry.IsUserRequest,
		UserIdentifier:    entry.UserIdentifier,
		ExpectedStartTime: est,
	}
}

func fromProtoQueueEntry(entry *QueueEntry) *radio.QueueEntry {
	if entry == nil || entry.Song == nil {
		return nil
	}

	est, _ := ptypes.Timestamp(entry.ExpectedStartTime)
	return &radio.QueueEntry{
		Song:              fromProtoSong(entry.Song),
		IsUserRequest:     entry.IsUserRequest,
		UserIdentifier:    entry.UserIdentifier,
		ExpectedStartTime: est,
	}
}

func toProtoUser(u radio.User) *User {
	return &User{
		Id:       int32(u.ID),
		Nickname: u.Nickname,
		IsRobot:  u.IsRobot,
	}
}

func fromProtoUser(u *User) radio.User {
	if u == nil {
		return radio.User{}
	}
	return radio.User{
		ID:       int(u.Id),
		Nickname: u.Nickname,
		IsRobot:  u.IsRobot,
	}
}

func fromProtoRequestResponse(r *RequestResponse) error {
	if r == nil || r.Success {
		return nil
	}

	ud, _ := ptypes.Duration(r.UserDelay)
	sd, _ := ptypes.Duration(r.SongDelay)
	return radio.SongRequestError{
		UserMessage: r.Msg,
		UserDelay:   ud,
		SongDelay:   sd,
	}
}

func toProtoRequestResponse(err radio.SongRequestError) *RequestResponse {
	return &RequestResponse{
		Msg:       err.UserMessage,
		UserDelay: ptypes.DurationProto(err.UserDelay),
		SongDelay: ptypes.DurationProto(err.SongDelay),
	}
}
