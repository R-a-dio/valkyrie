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

	currentSong string
	// api is our entrypoint to the in-process status api
	api *api
}

func (ss *StreamStatus) listen(ctx context.Context) error {
	l, err := NewListener(ctx, ss.Conf().Status.StreamURL)
	if err != nil {
		return err
	}
	defer l.Close()

	var cancel context.CancelFunc = func() {}
	var handleCtx context.Context

listen:
	for {
		select {
		case <-ctx.Done():
			err = nil
			break listen
		case err = <-l.Err:
			break listen
		case meta := <-l.Meta:
			song := meta["StreamTitle"]
			if song == "" {
				continue
			}

			// don't update status if we're running on the fallback
			if ss.isFallback(song) {
				ss.currentSong = song
				continue
			}

			// or when it's the same as the previous song
			if song == ss.currentSong {
				continue
			}

			cancel()
			handleCtx, cancel = context.WithTimeout(ctx, time.Second*15)
			go ss.handleSong(handleCtx, song)
		}
	}

	cancel()
	return err
}

func (ss *StreamStatus) handleSong(ctx context.Context, song string) {
	var track database.Track
	var start = time.Now()

	h := database.Handle(ctx, ss.db)
	err := h.Retry(nil, func(h database.Handler) (err error) {
		track, err = database.ResolveMetadataBasic(h, song)
		if err != nil && err != database.ErrTrackNotFound {
			return err
		}
		return nil
	})
	if err != nil {
		log.Println("status: failed database:", err)
	}

	s := &pb.Song{
		Metadata:  song,
		StartTime: uint64(start.Unix()),
		EndTime:   uint64(start.Add(track.Length).Unix()),
		Id:        int32(track.ID),
		TrackId:   int32(track.TrackID),
	}

	if _, err = ss.api.SetSong(ctx, s); err != nil {
		log.Println("status: failed to set song:", err)
	}
}

// isFallback checks if the meta passed in matches one of the known fallback
// mountpoint meta as defined with `fallbacknames` in configuration file
func (ss *StreamStatus) isFallback(meta string) bool {
	for _, fallback := range ss.Conf().Status.FallbackNames {
		if fallback == meta {
			return true
		}
	}
	return false
}
