package rpc

import (
	"context"
	"time"

	radio "github.com/R-a-dio/valkyrie"
)

// NewAnnounceService returns a new AnnounceService
func NewAnnounceService(addr string, httpclient HTTPClient) radio.AnnounceService {
	return AnnounceService{
		twirp: NewBotProtobufClient(addr, httpclient),
	}
}

// AnnounceService wraps irc.Bot to implement radio.AnnouncerService
type AnnounceService struct {
	twirp Bot
}

// AnnounceSong implements radio.AnnounceService
func (a AnnounceService) AnnounceSong(ctx context.Context, s radio.Song) error {
	song := &Song{
		Id:       int32(s.ID),
		Metadata: s.Metadata,
	}

	if s.DatabaseTrack != nil {
		song.TrackId = int32(s.TrackID)
	}

	_, err := a.twirp.AnnounceSong(ctx, song)
	return err
}

// AnnounceRequest implements radio.AnnounceService
func (a AnnounceService) AnnounceRequest(ctx context.Context, s radio.Song) error {
	return nil
}

// NewManagerService returns a new client implementing radio.ManagerService
func NewManagerService(addr string, httpclient HTTPClient) radio.ManagerService {
	return managerService{
		twirp: NewManagerProtobufClient(addr, httpclient),
	}
}

// NewManagerServiceWrap returns a new ManagerService using the manager given
func NewManagerServiceWrap(m Manager) radio.ManagerService {
	return managerService{twirp: m}
}

// managerService is a wrapper around a twirp Manager to implement radio.ManagerService
type managerService struct {
	twirp Manager
}

// Status implements radio.ManagerService
func (m managerService) Status(ctx context.Context) (radio.Status, error) {
	s, err := m.twirp.Status(ctx, new(StatusRequest))
	if err != nil {
		return radio.Status{}, nil
	}

	return radio.Status{
		User: radio.User{
			ID:       int(s.User.Id),
			Nickname: s.User.Nickname,
			IsRobot:  s.User.IsRobot,
		},
		Song: radio.Song{
			ID:       radio.SongID(s.Song.Id),
			Metadata: s.Song.Metadata,
		},
		StreamInfo: radio.StreamInfo{
			Listeners: int(s.ListenerInfo.Listeners),
			SongStart: time.Unix(int64(s.Song.StartTime), 0),
			SongEnd:   time.Unix(int64(s.Song.EndTime), 0),
		},
		Thread:          s.Thread.Thread,
		RequestsEnabled: s.BotConfig.RequestsEnabled,
	}, nil
}

// UpdateUser implements radio.ManagerService
func (m managerService) UpdateUser(ctx context.Context, u radio.User) error {
	_, err := m.twirp.SetUser(ctx, &User{
		Id:       int32(u.ID),
		Nickname: u.Nickname,
		IsRobot:  u.IsRobot,
	})
	return err
}

// UpdateSong implements radio.ManagerService
func (m managerService) UpdateSong(ctx context.Context, s radio.Song) error {
	_, err := m.twirp.SetSong(ctx, &Song{Metadata: s.Metadata})
	return err
}

// UpdateThread implements radio.ManagerService
func (m managerService) UpdateThread(ctx context.Context, thread string) error {
	_, err := m.twirp.SetThread(ctx, &Thread{Thread: thread})
	return err
}

// UpdateListeners implements radio.ManagerService
func (m managerService) UpdateListeners(ctx context.Context, count int) error {
	_, err := m.twirp.SetListenerInfo(ctx, &ListenerInfo{Listeners: int64(count)})
	return err
}

// NewStreamerService returns a new StreamerService implemented with a twirp client
func NewStreamerService(addr string, httpclient HTTPClient) radio.StreamerService {
	return streamerService{
		twirp: NewStreamerProtobufClient(addr, httpclient),
	}
}

type streamerService struct {
	twirp Streamer
}

func (s streamerService) Start(ctx context.Context) error {
	_, err := s.twirp.Start(ctx, new(Null))
	return err
}

func (s streamerService) Stop(ctx context.Context, force bool) error {
	_, err := s.twirp.Stop(ctx, &StopRequest{ForceStop: force})
	return err
}
