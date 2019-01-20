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
func (a AnnounceService) AnnounceSong(s radio.Song) error {
	// TODO: implement song passing; current api server doesn't touch the argument so
	// we can get away with passing nothing
	_, err := a.twirp.AnnounceSong(context.TODO(), new(Song))
	return err
}

// AnnounceRequest implements radio.AnnounceService
func (a AnnounceService) AnnounceRequest(s radio.Song) error {
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
func (m managerService) Status() (radio.Status, error) {
	s, err := m.twirp.Status(context.TODO(), new(StatusRequest))
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
func (m managerService) UpdateUser(u radio.User) error {
	_, err := m.twirp.SetUser(context.TODO(), &User{
		Id:       int32(u.ID),
		Nickname: u.Nickname,
		IsRobot:  u.IsRobot,
	})
	return err
}

// UpdateSong implements radio.ManagerService
func (m managerService) UpdateSong(s radio.Song) error {
	_, err := m.twirp.SetSong(context.TODO(), &Song{Metadata: s.Metadata})
	return err
}

// UpdateThread implements radio.ManagerService
func (m managerService) UpdateThread(thread string) error {
	_, err := m.twirp.SetThread(context.TODO(), &Thread{Thread: thread})
	return err
}

// UpdateListeners implements radio.ManagerService
func (m managerService) UpdateListeners(count int) error {
	_, err := m.twirp.SetListenerInfo(context.TODO(), &ListenerInfo{Listeners: int64(count)})
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

func (s streamerService) Start() error {
	_, err := s.twirp.Start(context.TODO(), new(Null))
	return err
}

func (s streamerService) Stop(force bool) error {
	_, err := s.twirp.Stop(context.TODO(), &StopRequest{ForceStop: force})
	return err
}
