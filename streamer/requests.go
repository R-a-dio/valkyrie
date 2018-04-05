package streamer

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/R-a-dio/valkyrie/database"
)

// RequestHandler returns a http.Handler that uses the state given to handle
// incoming track requests.
func RequestHandler(s *State) http.Handler {
	return &requestHandler{State: s}
}

func sendJSON(w io.Writer, errno int, s string) {
	fmt.Fprintf(w, `{"response": "%s", "errno": %d}`, s, errno)
}

type requestHandler struct {
	*State
	requestMutex sync.Mutex
}

// HandleRequest is the entry point to add requests to the streamer queue.
//
// We do not do authentication or authorization checks, this is left to the client. Request can be
// either a GET or POST with parameters `track` and `identifier`, where `track` is the track number
// to be requested, and `identifier` the unique identification used for the user (IP Address, hostname, etc)
func (h *requestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !h.Conf().RequestsEnabled {
		sendJSON(w, 1200, "error: requests are currently disabled")
		return
	}

	// Parse our request into usable form
	if err := r.ParseForm(); err != nil {
		sendJSON(w, 1210, "query error: invalid request")
		return
	}

	userID := r.Form.Get("identifier")
	if userID == "" {
		sendJSON(w, 1211,
			"query error: invalid parameter: identifier not valid")
		return
	}

	trackID, err := strconv.Atoi(r.Form.Get("track"))
	if err != nil {
		sendJSON(w, 1212, "query error: invalid parameter: track not a number")
		return
	}

	// once we start using database state, we need to avoid other requests
	// from reading it at the same time.
	h.requestMutex.Lock()
	defer h.requestMutex.Unlock()

	tx, err := h.db.Beginx()
	if err != nil {
		sendJSON(w, 1000,
			"database error: please return later, or complain in IRC")
		return
	}
	defer tx.Commit()

	// turn userID into a time of when this user last requested a song
	userLastRequest, err := database.UserRequestTime(tx, userID)
	if err != nil {
		fmt.Println("request: userrequesttime:", err)
		sendJSON(w, 1000,
			"database error: please return later, or complain in IRC")
		return
	}

	fmt.Println("ulastrequest:", userLastRequest)
	fmt.Println("delay:", userLastRequest.Add(h.Conf().UserRequestDelay))
	fmt.Println("now:", time.Now())
	// now we're going to check if the user is allowed to request
	if !userLastRequest.IsZero() &&
		userLastRequest.Add(h.Conf().UserRequestDelay).After(time.Now()) {
		sendJSON(w, 1201,
			"error: you need to wait longer before requesting again")
		return
	}

	// turn trackid into a usable DatabaseSong
	track, err := database.GetTrack(tx, database.TrackID(trackID))
	if err != nil {
		fmt.Println("request: gettrack:", err)
		sendJSON(w, 1000,
			"database error: please return later, or complain in IRC")
		return
	}
	// now we're going to check if the song can be requested
	if !track.Usable {
		sendJSON(w, 1202, "request error: this song can't be requested")
		return
	}
	// check if song timeout has expired
	if track.LastPlayed.Add(track.RequestDelay).After(time.Now()) {
		sendJSON(w, 1203,
			"error: you need to wait longer before requesting this song")
		return
	}

	// update the database to represent the request
	err = database.UpdateUserRequestTime(tx, userID, userLastRequest.IsZero())
	if err != nil {
		fmt.Println("request: updateuserrequesttime:", err)
		sendJSON(w, 1000,
			"database error: please return later, or complain in IRC")
		return
	}
	err = database.UpdateTrackRequestTime(tx, database.TrackID(trackID))
	if err != nil {
		fmt.Println("request: updatetrackrequesttime:", err)
		sendJSON(w, 1000,
			"database error: please return later, or complain in IRC")
		return
	}

	// send the song to the queue
	h.queue.AddRequest(track, userID)

	sendJSON(w, 0, "success: thank you for making your request!")
}
