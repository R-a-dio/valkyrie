package streamer

import (
	"context"
	"log"
	"net/http"
	"net/http/pprof"
	"sync"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/database"
	"github.com/R-a-dio/valkyrie/rpc"
	"github.com/jmoiron/sqlx"
)

// NewHTTPServer returns a http server with RPC API handler and debug handlers
func NewHTTPServer(cfg config.Config, db *sqlx.DB,
	queue radio.QueueService, announce radio.AnnounceService,
	streamer *Streamer) (*http.Server, error) {

	s := &streamerService{
		Config:   cfg,
		DB:       db,
		announce: announce,
		queue:    queue,
		streamer: streamer,
	}

	rpcServer := rpc.NewStreamerServer(rpc.NewStreamer(s), nil)
	mux := http.NewServeMux()
	// rpc server path
	mux.Handle(rpc.StreamerPathPrefix, rpcServer)

	// debug symbols
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	conf := cfg.Conf()
	server := &http.Server{Addr: conf.Streamer.ListenAddr, Handler: mux}

	return server, nil
}

type streamerService struct {
	config.Config
	DB *sqlx.DB

	announce     radio.AnnounceService
	queue        radio.QueueService
	streamer     *Streamer
	requestMutex sync.Mutex
}

// Start implements radio.StreamerService
func (s *streamerService) Start(_ context.Context) error {
	// don't use the passed ctx here as it will cancel once we return
	s.streamer.Start(context.Background())
	return nil
}

// Stop implements radio.StreamerService
func (s *streamerService) Stop(ctx context.Context, force bool) error {
	if force {
		return s.streamer.ForceStop(ctx)
	}
	return s.streamer.Stop(ctx)
}

// Queue implements radio.StreamerService
func (s *streamerService) Queue(ctx context.Context) ([]radio.QueueEntry, error) {
	return s.queue.Entries(ctx)
}

// RequestSong implements radio.StreamerService
//
// We do not do authentication or authorization checks, this is left to the client. Request can be
// either a GET or POST with parameters `track` and `identifier`, where `track` is the track number
// to be requested, and `identifier` the unique identification used for the user (IP Address, hostname, etc)
func (s *streamerService) RequestSong(ctx context.Context, song radio.Song, identifier string) error {
	if !s.Conf().Streamer.RequestsEnabled {
		return radio.ErrRequestsDisabled
	}

	if identifier == "" {
		log.Printf("request: empty identifier given")
		return radio.ErrInvalidRequest
	}

	if !song.HasTrack() && song.ID == 0 {
		log.Println("request: empty song given")
		return radio.ErrInvalidRequest
	}

	// once we start using database state, we need to avoid other requests
	// from reading it at the same time.
	s.requestMutex.Lock()
	defer s.requestMutex.Unlock()

	tx, err := database.HandleTx(ctx, s.DB)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// refresh our song information for if it's a bare song or something similar
	{
		songRefresh, err := database.GetTrack(tx, song.TrackID)
		if err != nil {
			if err == database.ErrTrackNotFound {
				return radio.ErrUnknownSong
			}
			return err
		}
		song = *songRefresh
	}
	// find the last time this user requested a song
	userLastRequest, err := database.UserRequestTime(tx, identifier)
	if err != nil {
		return err
	}

	// check if the user is allowed to request
	withDelay := userLastRequest.Add(time.Duration(s.Conf().UserRequestDelay))
	if !userLastRequest.IsZero() && withDelay.After(time.Now()) {
		err := radio.ErrUserCooldown
		err.UserDelay = time.Until(withDelay)
		return err
	}

	// check if the track can be decoded by the streamer
	if !song.Usable {
		return radio.ErrUnusableSong
	}
	// check if the track wasn't recently played or requested
	if !song.Requestable() {
		err := radio.ErrSongCooldown
		err.SongDelay = song.UntilRequestable()
		return err
	}

	// update the database to represent the request
	err = database.UpdateUserRequestTime(tx, identifier, userLastRequest.IsZero())
	if err != nil {
		return err
	}
	err = database.UpdateTrackRequestInfo(tx, song.TrackID)
	if err != nil {
		return err
	}

	if err = tx.Commit(); err != nil {
		return err
	}

	// send the song to the queue
	err = s.queue.AddRequest(ctx, song, identifier)
	if err != nil {
		return err
	}

	return s.announce.AnnounceRequest(ctx, song)
}
