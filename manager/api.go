package manager

import (
	"context"
	"log"
	"net/http"
	"net/http/pprof"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/database"
	"github.com/R-a-dio/valkyrie/rpc"
)

// NewHTTPServer sets up a net/http server ready to serve RPC requests
func NewHTTPServer(m *Manager) (*http.Server, error) {
	rpcServer := rpc.NewManagerServer(rpc.NewManager(m), nil)
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
func (m *Manager) Status(ctx context.Context) (*radio.Status, error) {
	m.mu.Lock()
	status := m.status.Copy()
	m.mu.Unlock()
	return &status, nil
}

// UpdateUser sets information about the current streamer
func (m *Manager) UpdateUser(ctx context.Context, u radio.User) error {
	defer m.updateStreamStatus()
	m.mu.Lock()
	m.status.User = u
	m.mu.Unlock()
	return nil
}

// UpdateSong sets information about the currently playing song
func (m *Manager) UpdateSong(ctx context.Context, new radio.Song, info radio.SongInfo) error {
	defer m.updateStreamStatus()
	tx, err := database.HandleTx(ctx, m.DB)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// we assume that the song we received has very little or no data except for the
	// Metadata field. So we try and find more info from that
	song, err := database.GetSongFromMetadata(tx, new.Metadata)
	if err != nil && err != database.ErrTrackNotFound {
		return err
	}

	// if we don't have this song in the database create a new entry for it
	if song == nil {
		song, err = database.CreateSong(tx, new.Metadata)
		if err != nil {
			return err
		}
	}

	// calculate start and end time only if they're zero
	if info.Start.IsZero() {
		// we assume the song just started if it wasn't set
		info.Start = time.Now()
	}
	if info.End.IsZero() && song.Length > 0 {
		info.End = info.Start.Add(song.Length)
	}

	var prev radio.Song
	var prevInfo radio.SongInfo
	var listenerCountDiff *int

	// critical section to swap our new song with the previous one
	m.mu.Lock()

	prev, m.status.Song = m.status.Song, *song
	prevInfo, m.status.SongInfo = m.status.SongInfo, info

	// record listener count and calculate the difference between start/end of song
	currentListenerCount := m.status.Listeners
	// update and retrieve listener count of start of song
	var startListenerCount int
	startListenerCount, m.songStartListenerCount = m.songStartListenerCount, currentListenerCount

	// make a copy of our current status to send to the announcer
	announceStatus := m.status.Copy()
	m.mu.Unlock()

	// only calculate a diff if we have more than 10 listeners
	if currentListenerCount > 10 && startListenerCount > 10 {
		diff := currentListenerCount - startListenerCount
		listenerCountDiff = &diff
	}

	log.Printf("manager: set song: \"%s\" (%s)\n", song.Metadata, song.Length)

	// finish up database work for the previous song
	err = m.handlePreviousSong(tx, prev, prevInfo, listenerCountDiff)
	if err == nil {
		tx.Commit()
	} else {
		tx.Rollback()
	}

	// announce the new song over a chat service
	err = m.client.announce.AnnounceSong(ctx, announceStatus)
	if err != nil {
		// this isn't a critical error, so we do not return it if it occurs
		log.Printf("manager: failed to announce song: %s", err)
	}

	return nil
}

func (m *Manager) handlePreviousSong(tx database.HandlerTx, song radio.Song, info radio.SongInfo, listenerDiff *int) error {
	// protect against zero-d Song's
	if song.ID == 0 {
		return nil
	}

	// insert an entry that the previous song just played
	err := database.InsertPlayedSong(tx, song.ID, listenerDiff)
	if err != nil {
		log.Printf("manager: unable to insert play history: %s", err)
		return err
	}

	if song.Length == 0 {
		length := time.Since(info.Start)

		return database.UpdateSongLength(tx, song.ID, length)
	}

	return nil
}

// UpdateThread sets the current thread information on the front page and chats
func (m *Manager) UpdateThread(ctx context.Context, thread string) error {
	defer m.updateStreamStatus()
	m.mu.Lock()
	m.status.Thread = thread
	m.mu.Unlock()
	return nil
}

// UpdateListeners sets the listener count
func (m *Manager) UpdateListeners(ctx context.Context, listeners int) error {
	defer m.updateStreamStatus()
	m.mu.Lock()
	m.status.Listeners = listeners
	m.mu.Unlock()
	return nil
}
