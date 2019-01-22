package manager

import (
	"context"
	"log"
	"net/http"
	"net/http/pprof"
	"time"

	"github.com/twitchtv/twirp"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/database"
	"github.com/R-a-dio/valkyrie/rpc"
	"github.com/golang/protobuf/proto"
)

// NewHTTPServer sets up a net/http server ready to serve RPC requests
func NewHTTPServer(m *Manager) (*http.Server, error) {
	rpcServer := rpc.NewManagerServer(m, nil)
	mux := http.NewServeMux()
	// rpc server path
	mux.Handle(rpc.ManagerPathPrefix, rpcServer)

	// debug symbols
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	conf := m.Conf()
	server := &http.Server{Addr: conf.Manager.Addr, Handler: mux}
	return server, nil
}

// Status returns the current status of the radio
func (m *Manager) Status(ctx context.Context, _ *rpc.StatusRequest) (*rpc.StatusResponse, error) {
	var sr rpc.StatusResponse
	m.mu.Lock()
	proto.Merge(&sr, m.status)
	m.mu.Unlock()
	return &sr, nil
}

// SetUser sets information about the current streamer
func (m *Manager) SetUser(ctx context.Context, u *rpc.User) (*rpc.User, error) {
	var old *rpc.User
	m.mu.Lock()
	old, m.status.User = m.status.User, u
	m.mu.Unlock()
	return old, nil
}

// SetSong sets information about the currently playing song
func (m *Manager) SetSong(ctx context.Context, new *rpc.Song) (*rpc.Song, error) {
	switch { // required arguments check
	case new.Metadata == "":
		return nil, twirp.RequiredArgumentError("metadata")
	}

	tx, err := database.HandleTx(ctx, m.DB)
	if err != nil {
		return nil, twirp.InternalErrorWith(err)
	}
	defer tx.Rollback()

	// find information about the passed song from the database
	track, err := database.ResolveMetadataBasic(tx, new.Metadata)
	if err != nil && err != database.ErrTrackNotFound {
		return nil, twirp.InternalErrorWith(err)
	}

	// if we don't have this song in the database create a new entry for it
	if track == nil {
		track, err = database.CreateSong(tx, new.Metadata)
		if err != nil {
			return nil, twirp.InternalErrorWith(err)
		}
	}

	// we assume the song just started
	new.StartTime = uint64(time.Now().Unix())

	// now move our database knowledge into the status Song type
	new.Id = int32(track.ID)
	if track.DatabaseTrack != nil {
		new.TrackId = int32(track.TrackID)
	}
	new.EndTime = new.StartTime + uint64(track.Length/time.Second)

	var prev *rpc.Song
	var listenerCountDiff *int64

	// critical section to swap our new song with the previous one
	m.mu.Lock()

	prev, m.status.Song = m.status.Song, new

	// record listener count and calculate the difference between start/end of song
	currentListenerCount := m.status.ListenerInfo.Listeners
	// update and retrieve listener count of start of song
	var startListenerCount int64
	startListenerCount, m.songStartListenerCount = m.songStartListenerCount, currentListenerCount

	m.mu.Unlock()

	// only calculate a diff if we have more than 10 listeners
	if currentListenerCount > 10 && startListenerCount > 10 {
		diff := currentListenerCount - startListenerCount
		listenerCountDiff = &diff
	}

	log.Printf("manager: set song: \"%s\" (%s)\n", track.Metadata, track.Length)

	// finish up database work for the previous song
	err = m.handlePreviousSong(tx, prev, listenerCountDiff)
	if err == nil {
		tx.Commit()
	} else {
		tx.Rollback()
	}

	// announce the new song over a chat service
	err = m.client.announce.AnnounceSong(ctx, radio.Song{Metadata: new.Metadata})
	if err != nil {
		// this isn't a critical error, so we do not return it if it occurs
		log.Printf("manager: failed to announce song: %s", err)
	}

	return prev, nil
}

func (m *Manager) handlePreviousSong(tx database.HandlerTx, prev *rpc.Song, listenerDiff *int64) error {
	// protect against zero-d Song's
	if prev.Id == 0 {
		return nil
	}

	startTime := time.Unix(int64(prev.StartTime), 0)

	// insert an entry that the previous song just played
	err := database.InsertPlayedSong(tx, radio.SongID(prev.Id), listenerDiff)
	if err != nil {
		log.Printf("manager: unable to insert play history: %s", err)
		return err
	}

	if prev.StartTime == prev.EndTime {
		length := time.Since(startTime)

		return database.UpdateSongLength(tx, radio.SongID(prev.Id), length)
	}

	return nil
}

// SetBotConfig sets options related to the automated streamer
func (m *Manager) SetBotConfig(ctx context.Context, bc *rpc.BotConfig) (*rpc.BotConfig, error) {
	var old *rpc.BotConfig
	m.mu.Lock()
	old, m.status.BotConfig = m.status.BotConfig, bc
	m.mu.Unlock()
	return old, nil
}

// SetThread sets the current thread information on the front page and chats
func (m *Manager) SetThread(ctx context.Context, t *rpc.Thread) (*rpc.Thread, error) {
	var old *rpc.Thread
	m.mu.Lock()
	old, m.status.Thread = m.status.Thread, t
	m.mu.Unlock()
	return old, nil
}

// SetListenerInfo sets the listener info part of status
func (m *Manager) SetListenerInfo(ctx context.Context, li *rpc.ListenerInfo) (*rpc.ListenerInfo, error) {
	var old *rpc.ListenerInfo
	m.mu.Lock()
	old, m.status.ListenerInfo = m.status.ListenerInfo, li
	m.mu.Unlock()
	return old, nil
}
