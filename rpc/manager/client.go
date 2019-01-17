package manager

import (
	context "context"
	"time"

	radio "github.com/R-a-dio/valkyrie"
)

// NewClient returns a new client implementing radio.ManagerService
func NewClient(addr string, httpclient HTTPClient) radio.ManagerService {
	return Client{
		twirp: NewManagerProtobufClient(addr, httpclient),
	}
}

// NewWrapClient returns a new ManagerService using the manager given
func NewWrapClient(m Manager) radio.ManagerService {
	return Client{twirp: m}
}

// Client is a wrapper around a twirp Manager to implement radio.ManagerService
type Client struct {
	twirp Manager
}

// Status implements radio.ManagerService
func (c Client) Status() (radio.Status, error) {
	s, err := c.twirp.Status(context.TODO(), new(StatusRequest))
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
func (c Client) UpdateUser(u radio.User) error {
	_, err := c.twirp.SetUser(context.TODO(), &User{
		Id:       int32(u.ID),
		Nickname: u.Nickname,
		IsRobot:  u.IsRobot,
	})
	return err
}

// UpdateSong implements radio.ManagerService
func (c Client) UpdateSong(s radio.Song) error {
	_, err := c.twirp.SetSong(context.TODO(), &Song{Metadata: s.Metadata})
	return err
}

// UpdateThread implements radio.ManagerService
func (c Client) UpdateThread(thread string) error {
	_, err := c.twirp.SetThread(context.TODO(), &Thread{Thread: thread})
	return err
}

// UpdateListeners implements radio.ManagerService
func (c Client) UpdateListeners(count int) error {
	_, err := c.twirp.SetListenerInfo(context.TODO(), &ListenerInfo{Listeners: int64(count)})
	return err
}
