package streamer

import (
	"context"
	"fmt"
	"time"

	"github.com/twitchtv/twirp"

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
		return nil, twirp.InvalidArgumentError("track", "track can't be negative")
	}

	// once we start using database state, we need to avoid other requests
	// from reading it at the same time.
	h.requestMutex.Lock()
	defer h.requestMutex.Unlock()

	hl := database.Handle(ctx, h.db)
	hl, err := database.BeginTx(hl)
	if err != nil {
		return nil, twirp.InternalErrorWith(err)
	}
	defer hl.Rollback()

	// turn userID into a time of when this user last requested a song
	userLastRequest, err := database.UserRequestTime(hl, r.Identifier)
	if err != nil {
		return nil, twirp.InternalErrorWith(err)
	}

	fmt.Println("ulastrequest:", userLastRequest)
	fmt.Println("delay:", userLastRequest.Add(h.Conf().UserRequestDelay))
	fmt.Println("now:", time.Now())

	// now we're going to check if the user is allowed to request
	withDelay := userLastRequest.Add(h.Conf().UserRequestDelay)
	if !userLastRequest.IsZero() && withDelay.After(time.Now()) {
		return requestResponse(false, "you need to wait longer before requesting again")
	}

	// turn trackid into a usable DatabaseSong
	track, err := database.GetTrack(hl, database.TrackID(r.Track))
	if err != nil {
		return nil, twirp.InternalErrorWith(err)
	}
	// now we're going to check if the song can be requested
	if !track.Usable {
		return requestResponse(false, "this song can't be requested")
	}
	// check if song timeout has expired
	if track.LastPlayed.Add(track.RequestDelay).After(time.Now()) {
		return requestResponse(false,
			"you need to wait longer before requesting this song")
	}

	// update the database to represent the request
	err = database.UpdateUserRequestTime(hl, r.Identifier, userLastRequest.IsZero())
	if err != nil {
		return nil, twirp.InternalErrorWith(err)
	}
	err = database.UpdateTrackRequestTime(hl, database.TrackID(r.Track))
	if err != nil {
		return nil, twirp.InternalErrorWith(err)
	}

	err = hl.Commit()
	if err != nil {
		return nil, twirp.InternalErrorWith(err)
	}

	// send the song to the queue
	h.queue.AddRequest(track, r.Identifier)
	return requestResponse(true, "thank you for making your request!")
}
