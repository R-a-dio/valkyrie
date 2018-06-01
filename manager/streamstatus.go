package manager

import (
	"context"
	"log"
	"time"

	"github.com/R-a-dio/valkyrie/database"
	pb "github.com/R-a-dio/valkyrie/rpc/manager"
)

type StreamStatus struct {
	*State

	api pb.Manager
}

func (ss *StreamStatus) Listen(ctx context.Context) error {
	var l *Listener
	var err error
	var lastMetadata string

	for {
		if l == nil {
			l, err = NewListener(ctx, ss.Conf().Status.StreamURL)
			if err != nil {
				return err
			}
		}

		select {
		case <-ctx.Done():
			return nil
		case err = <-l.FatalErr:
			log.Println("metadata connection error:", err)
			l = nil
		case err = <-l.MetaErr:
			log.Println("metadata parsing error:", err)
		case meta := <-l.Meta:
			song := meta["StreamTitle"]
			if song == "" {
				continue
			}

			var online = true
			// look if the metadata we received is one of the known fallback
			// names; when the main mount goes down we also get moved to the
			// fallback so try to detect that
			for _, fallback := range ss.Conf().Status.FallbackNames {
				if song == fallback {
					online = false
					break
				}
			}

			// TODO: add new esong entries to the database as needed
			if online && lastMetadata != song {
				pbSong, err := ss.createProtoSong(song)
				if err != nil {
					// TODO: figure out what to do on error
				} else {
					ss.api.SetSong(ctx, pbSong)
				}
			}

			lastMetadata = song
		}
	}
}

func (ss *StreamStatus) createProtoSong(metadata string) (*pb.Song, error) {
	var s pb.Song
	var start = time.Now()

	track, err := database.ResolveMetadataBasic(ss.db, metadata)
	if err != nil {
		return nil, err
	}

	s.Metadata = track.Metadata
	s.StartTime = uint64(start.Unix())
	s.EndTime = uint64(start.Add(track.Length).Unix())
	s.Id = int32(track.ID)
	s.TrackId = int32(track.TrackID)
	return &s, nil
}
