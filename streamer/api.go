package streamer

import (
	"context"
	"sync"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/rpc"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
)

// NewGRPCServer returns a http server with RPC API handler and debug handlers
func NewGRPCServer(ctx context.Context, cfg config.Config, storage radio.StorageService,
	queue radio.QueueService, announce radio.AnnounceService,
	streamer *Streamer) (*grpc.Server, error) {

	s := &streamerService{
		cfgRequestsEnabled: config.Value(cfg, func(cfg config.Config) bool {
			return cfg.Conf().Streamer.RequestsEnabled
		}),
		cfgUserRequestDelay: config.Value(cfg, func(cfg config.Config) time.Duration {
			return time.Duration(cfg.Conf().UserRequestDelay)
		}),
		Storage:  storage,
		announce: announce,
		queue:    queue,
		streamer: streamer,
	}

	gs := rpc.NewGrpcServer(ctx)
	rpc.RegisterStreamerServer(gs, rpc.NewStreamer(s))
	rpc.RegisterQueueServer(gs, rpc.NewQueue(queue))

	return gs, nil
}

type streamerService struct {
	cfgRequestsEnabled  func() bool
	cfgUserRequestDelay func() time.Duration

	Storage radio.StorageService

	announce     radio.AnnounceService
	queue        radio.QueueService
	streamer     *Streamer
	requestMutex sync.Mutex
}

// Start implements radio.StreamerService
func (s *streamerService) Start(ctx context.Context) error {
	// don't use the passed ctx here as it will cancel once we return
	s.streamer.Start(context.WithoutCancel(ctx))
	return nil
}

// Stop implements radio.StreamerService
func (s *streamerService) Stop(ctx context.Context, who *radio.User, force bool) error {
	const op errors.Op = "streamer/streamerService.Stop"

	zerolog.Ctx(ctx).Info().Ctx(ctx).
		Str("who", who.Username).
		Bool("force", force).
		Msg("murder attempt")

	err := s.streamer.Stop(ctx, force)
	if err != nil {
		return errors.E(op, err)
	}

	err = s.announce.AnnounceMurder(ctx, who, force)
	if err != nil {
		zerolog.Ctx(ctx).Err(err).Ctx(ctx).Msg("failed to announce murder")
		// not a critical error, continue
	}
	return nil
}

// Queue implements radio.StreamerService
func (s *streamerService) Queue(ctx context.Context) (radio.Queue, error) {
	const op errors.Op = "streamer/streamerService.Queue"

	queue, err := s.queue.Entries(ctx)
	if err != nil {
		return nil, errors.E(op, err)
	}
	return queue, nil
}

func (s *streamerService) areWeStreaming() bool {
	latest := s.streamer.userValue.Latest()
	if latest == nil {
		return false
	}
	return s.streamer.userValue.Latest().ID == s.streamer.StreamUser.ID
}

// RequestSong implements radio.StreamerService
//
// We do not do authentication or authorization checks, this is left to the client. Request can be
// either a GET or POST with parameters `track` and `identifier`, where `track` is the track number
// to be requested, and `identifier` the unique identification used for the user (IP Address, hostname, etc)
func (s *streamerService) RequestSong(ctx context.Context, song radio.Song, identifier string) error {
	const op errors.Op = "streamer/streamerService.RequestSong"

	if !s.cfgRequestsEnabled() {
		return errors.E(op, errors.StreamerNoRequests)
	}

	// only accept requests if we are streaming
	if !s.areWeStreaming() {
		return errors.E(op, errors.StreamerNoRequests)
	}

	if identifier == "" {
		return errors.E(op, errors.InvalidArgument, errors.Info("identifier"))
	}

	if !song.HasTrack() || song.TrackID == 0 {
		return errors.E(op, errors.InvalidArgument, errors.Info("song"), song)
	}

	// once we start using database state, we need to avoid other requests
	// from reading it at the same time.
	s.requestMutex.Lock()
	defer s.requestMutex.Unlock()

	ts, tx, err := s.Storage.TrackTx(ctx, nil)
	if err != nil {
		return errors.E(op, errors.TransactionBegin, err, song)
	}
	defer tx.Rollback()

	rs, _, err := s.Storage.RequestTx(ctx, tx)
	if err != nil {
		return errors.E(op, errors.TransactionBegin, err, song)
	}

	// refresh our song information for if it's a bare song or something similar
	{
		songRefresh, err := ts.Get(song.TrackID)
		if err != nil {
			return errors.E(op, err, song)
		}
		song = *songRefresh
	}
	// find the last time this user requested a song
	userLastRequest, err := rs.LastRequest(identifier)
	if err != nil {
		return errors.E(op, err)
	}

	// check if the user is allowed to request
	waitTime, ok := radio.CalculateCooldown(s.cfgUserRequestDelay(), userLastRequest)
	if !ok {
		return errors.E(op, errors.UserCooldown, errors.Delay(waitTime), song)
	}

	// check if the track can be decoded by the streamer
	if !song.Usable {
		return errors.E(op, errors.SongUnusable, song)
	}

	// check if the track wasn't recently played or requested
	if !song.Requestable() {
		d := song.UntilRequestable()
		return errors.E(op, errors.SongCooldown, errors.Delay(d), song)
	}

	// update the database to represent the request
	err = rs.UpdateLastRequest(identifier)
	if err != nil {
		return errors.E(op, err)
	}
	err = ts.UpdateRequestInfo(song.TrackID)
	if err != nil {
		return errors.E(op, err, song)
	}

	if err = tx.Commit(); err != nil {
		return errors.E(op, errors.TransactionCommit, err)
	}

	// send the song to the queue
	err = s.queue.AddRequest(ctx, song, identifier)
	if err != nil {
		return errors.E(op, err, song)
	}

	err = s.announce.AnnounceRequest(ctx, song)
	if err != nil {
		// not a critical error, but log it anyway
		zerolog.Ctx(ctx).Error().Ctx(ctx).Err(err).Msg("failed to announce request")
	}
	return nil
}
