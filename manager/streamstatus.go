package manager

import (
	"context"
	"log"
	"time"

	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/database"
	pb "github.com/R-a-dio/valkyrie/rpc/manager"
	"github.com/cenkalti/backoff"
)

type StreamStatus struct {
	*State

	// api is our entrypoint to the in-process status api
	api *api
}

func (ss *StreamStatus) Listen(ctx context.Context) error {
	var lnr *Listener
	var err error
	var lastMetadata string
	var newListener = func() error {
		l, err := NewListener(ctx, ss.Conf().Status.StreamURL)
		if err != nil {
			return err
		}
		lnr = l // move l to method scope
		return nil
	}

	var songCh = make(chan *pb.Song)
	var boff = config.NewConnectionBackoff()
	boff = backoff.WithContext(boff, ctx)

	for {
		if lnr == nil {
			err = backoff.Retry(newListener, boff)
			if err != nil {
				return err
			}
		}

		select {
		case <-ctx.Done():
			return nil
		case err = <-lnr.FatalErr:
			log.Println("metadata connection error:", err)
			lnr = nil
		case err = <-lnr.MetaErr:
			log.Println("metadata parsing error:", err)
		case meta := <-lnr.Meta:
			song := meta["StreamTitle"]
			if song == "" {
				continue
			}

			online := ss.isOnline(song)
			// nothing to do if we've identified to be on the fallback mount
			// or we've received the same metadata as before
			if !online || lastMetadata == song {
				lastMetadata = song
				continue
			}

			// TODO: handle goroutine cleanup
			go ss.resolveMetadata(songCh, song)
			lastMetadata = song
		case pbSong := <-songCh:
			ss.api.SetSong(ctx, pbSong)
		}
	}
}

// isOnline compares the string passed in to the configuration set in
// status.fallbacknames. returns false if a match is found.
//
// We assume that having a metadata equal to the fallback metadata means the
// main mount is offline
func (ss *StreamStatus) isOnline(meta string) bool {
	var online = true
	for _, fallback := range ss.Conf().Status.FallbackNames {
		online = meta != fallback && online
	}
	return online
}

func (ss *StreamStatus) resolveMetadata(ch chan *pb.Song, metadata string) {
	h := database.Handle(context.TODO(), ss.db)
	track, err := database.ResolveMetadataBasic(h, metadata)
	if err != nil && err != database.ErrTrackNotFound {
		// TODO: retry
		return
	}

	var s pb.Song
	var start = time.Now()

	s.Metadata = metadata
	s.StartTime = uint64(start.Unix())
	s.EndTime = uint64(start.Add(track.Length).Unix())
	s.Id = int32(track.ID)
	s.TrackId = int32(track.TrackID)

	select {
	// TODO: handle goroutine cleanup
	// case <-ctx.Done():
	case ch <- &s:
	}
}
