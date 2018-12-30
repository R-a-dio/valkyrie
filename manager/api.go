package manager

import (
	"context"
	"log"
	"net/http"
	"net/http/pprof"
	"time"

	"github.com/twitchtv/twirp"

	"github.com/R-a-dio/valkyrie/database"
	pb "github.com/R-a-dio/valkyrie/rpc/manager"
	"github.com/golang/protobuf/proto"
)

// NewHTTPServer sets up a net/http server ready to serve RPC requests
func NewHTTPServer(m *Manager) (*http.Server, error) {
	rpcServer := pb.NewManagerServer(m, nil)
	mux := http.NewServeMux()
	// rpc server path
	mux.Handle(pb.ManagerPathPrefix, rpcServer)

	// debug symbols
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	conf := m.Conf()
	server := &http.Server{Addr: conf.Streamer.Addr, Handler: mux}
	return server, nil
}

// Status returns the current status of the radio
func (m *Manager) Status(ctx context.Context, _ *pb.StatusRequest) (*pb.StatusResponse, error) {
	var sr pb.StatusResponse
	m.mu.Lock()
	proto.Merge(&sr, m.status)
	m.mu.Unlock()
	return &sr, nil
}

// SetUser sets information about the current streamer
func (m *Manager) SetUser(ctx context.Context, u *pb.User) (*pb.User, error) {
	var old *pb.User
	m.mu.Lock()
	old, m.status.User = m.status.User, u
	m.mu.Unlock()
	return old, nil
}

// SetSong sets information about the currently playing song
func (m *Manager) SetSong(ctx context.Context, s *pb.Song) (*pb.Song, error) {
	switch { // required arguments check
	case s.Metadata == "":
		return nil, twirp.RequiredArgumentError("metadata")
	case s.StartTime == 0:
		return nil, twirp.RequiredArgumentError("start_time")
	}

	tx, err := database.HandleTx(ctx, m.DB)
	if err != nil {
		return nil, twirp.InternalErrorWith(err)
	}
	defer tx.Rollback()

	track, err := database.ResolveMetadataBasic(tx, s.Metadata)
	if err != nil && err != database.ErrTrackNotFound {
		return nil, twirp.InternalErrorWith(err)
	}

	if track.ID == 0 {
		// this is a new song we haven't seen before, so create a new entry
		track, err = database.CreateSong(tx, s.Metadata)
		if err != nil {
			return nil, twirp.InternalErrorWith(err)
		}
	}

	if err = tx.Commit(); err != nil {
		return nil, twirp.InternalErrorWith(err)
	}

	s.Id = int32(track.ID)
	s.TrackId = int32(track.TrackID)
	s.EndTime = s.StartTime + uint64(track.Length/time.Second)

	var old *pb.Song
	m.mu.Lock()
	old, m.status.Song = m.status.Song, s
	m.mu.Unlock()

	log.Printf("manager: set song: \"%s\" (%s)\n", track.Metadata, track.Length)
	// announce the new song on irc
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
		m.client.irc.AnnounceSong(ctx, s)
		cancel()
	}()

	return old, nil
}

// SetBotConfig sets options related to the automated streamer
func (m *Manager) SetBotConfig(ctx context.Context, bc *pb.BotConfig) (*pb.BotConfig, error) {
	var old *pb.BotConfig
	m.mu.Lock()
	old, m.status.BotConfig = m.status.BotConfig, bc
	m.mu.Unlock()
	return old, nil
}

// SetThread sets the current thread information on the front page and chats
func (m *Manager) SetThread(ctx context.Context, t *pb.Thread) (*pb.Thread, error) {
	var old *pb.Thread
	m.mu.Lock()
	old, m.status.Thread = m.status.Thread, t
	m.mu.Unlock()
	return old, nil
}

// SetListenerInfo sets the listener info part of status
func (m *Manager) SetListenerInfo(ctx context.Context, li *pb.ListenerInfo) (*pb.ListenerInfo, error) {
	var old *pb.ListenerInfo
	m.mu.Lock()
	old, m.status.ListenerInfo = m.status.ListenerInfo, li
	m.mu.Unlock()
	return old, nil
}
