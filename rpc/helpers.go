package rpc

import (
	radio "github.com/R-a-dio/valkyrie"
	"github.com/golang/protobuf/ptypes"
)

func toProtoSong(s radio.Song) *Song {
	lp, _ := ptypes.TimestampProto(s.LastPlayed)
	return &Song{
		Id:         int32(s.ID),
		Metadata:   s.Metadata,
		LastPlayed: lp,
	}
}

func fromProtoSong(s *Song) radio.Song {
	lp, _ := ptypes.Timestamp(s.LastPlayed)
	return radio.Song{
		ID:         radio.SongID(s.Id),
		Metadata:   s.Metadata,
		LastPlayed: lp,
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
