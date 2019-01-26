package rpc

import (
	"encoding/hex"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/golang/protobuf/ptypes"
)

func toProtoSong(s radio.Song) *Song {
	lp, _ := ptypes.TimestampProto(s.LastPlayed)
	lr, _ := ptypes.TimestampProto(s.LastRequested)
	return &Song{
		Id:         int32(s.ID),
		Hash:       s.Hash.String(),
		Metadata:   s.Metadata,
		Length:     ptypes.DurationProto(s.Length),
		LastPlayed: lp,
		// track fields
		TrackId:       int32(s.TrackID),
		Artist:        s.Artist,
		Title:         s.Title,
		Album:         s.Album,
		FilePath:      s.FilePath,
		Tags:          s.Tags,
		Acceptor:      s.Acceptor,
		LastEditor:    s.LastEditor,
		Priority:      int32(s.Priority),
		Usable:        s.Usable,
		LastRequested: lr,
		RequestCount:  int32(s.RequestCount),
		RequestDelay:  ptypes.DurationProto(s.RequestDelay),
	}
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
