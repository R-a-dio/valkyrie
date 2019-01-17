package streamer

import (
	"context"
	"time"

	"github.com/twitchtv/twirp"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/database"
	pb "github.com/R-a-dio/valkyrie/rpc/streamer"
)

func requestResponse(success bool, msg string) (*pb.RequestResponse, error) {
	return &pb.RequestResponse{
		Success: success,
		Msg:     msg,
	}, nil
}

// HandleRequest is the entry point to add requests to the streamer queue.
//
// We do not do authentication or authorization checks, this is left to the client. Request can be
// either a GET or POST with parameters `track` and `identifier`, where `track` is the track number
// to be requested, and `identifier` the unique identification used for the user (IP Address, hostname, etc)
func (h *streamHandler) RequestTrack(ctx context.Context, r *pb.TrackRequest) (*pb.RequestResponse, error) {
	if !h.Conf().Streamer.RequestsEnabled {
		return requestResponse(false, "requests are currently disabled")
	}

	if r.Identifier == "" {
		return nil, twirp.RequiredArgumentError("identifier")
	}

	if r.Track == 0 {
		return nil, twirp.RequiredArgumentError("track")
	} else if r.Track < 0 {
		return nil, twirp.InvalidArgumentError("track", "negative number found")
	}

	// once we start using database state, we need to avoid other requests
	// from reading it at the same time.
	h.requestMutex.Lock()
	defer h.requestMutex.Unlock()

	tx, err := database.HandleTx(ctx, h.queue.DB)
	if err != nil {
		return nil, twirp.InternalErrorWith(err)
	}
	defer tx.Rollback()

	// find the last time this user requested a song
	userLastRequest, err := database.UserRequestTime(tx, r.Identifier)
	if err != nil {
		return nil, twirp.InternalErrorWith(err)
	}

	// check if the user is allowed to request
	withDelay := userLastRequest.Add(time.Duration(h.Conf().UserRequestDelay))
	if !userLastRequest.IsZero() && withDelay.After(time.Now()) {
		return requestResponse(false, "you need to wait longer before requesting again")
	}

	// find our track in the database
	track, err := database.GetTrack(tx, radio.TrackID(r.Track))
	if err != nil {
		if err == database.ErrTrackNotFound {
			return requestResponse(false, "unknown track")
		}
		return nil, twirp.InternalErrorWith(err)
	}

	// check if the track can be decoded by the streamer
	if !track.Usable {
		return requestResponse(false, "this song can't be requested")
	}
	// check if the track wasn't recently played or requested
	if time.Since(track.LastPlayed) < track.RequestDelay ||
		time.Since(track.LastRequested) < track.RequestDelay {
		return requestResponse(false,
			"you need to wait longer before requesting this song")
	}

	// update the database to represent the request
	err = database.UpdateUserRequestTime(tx, r.Identifier, userLastRequest.IsZero())
	if err != nil {
		return nil, twirp.InternalErrorWith(err)
	}
	err = database.UpdateTrackRequestTime(tx, radio.TrackID(r.Track))
	if err != nil {
		return nil, twirp.InternalErrorWith(err)
	}

	if err = tx.Commit(); err != nil {
		return nil, twirp.InternalErrorWith(err)
	}

	// send the song to the queue
	h.queue.AddRequest(*track, r.Identifier)
	return requestResponse(true, "thank you for making your request!")
}
