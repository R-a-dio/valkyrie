package rpc

import (
	"context"
	"errors"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/golang/protobuf/ptypes/wrappers"
)

// NewAnnouncerClient returns a new AnnouncerClient with the client connecting to the given
// addr and using the client given.
func NewAnnouncerClient(addr string, c HTTPClient) radio.AnnounceService {
	return AnnouncerClient{
		twirp: NewAnnouncerProtobufClient(addr, c),
	}
}

// AnnouncerClient is a twirp client that implements radio.AnnounceService
type AnnouncerClient struct {
	twirp Announcer
}

var _ radio.AnnounceService = AnnouncerClient{}

// AnnounceSong implements radio.AnnounceService
func (a AnnouncerClient) AnnounceSong(ctx context.Context, s radio.Status) error {
	announcement := &SongAnnouncement{
		Song: toProtoSong(s.Song),
		Info: toProtoSongInfo(s.SongInfo),
		ListenerInfo: &ListenerInfo{
			Listeners: int64(s.Listeners),
		},
	}
	_, err := a.twirp.AnnounceSong(ctx, announcement)
	return err
}

// AnnounceRequest implements radio.AnnounceService
func (a AnnouncerClient) AnnounceRequest(ctx context.Context, s radio.Song) error {
	announcement := &SongRequestAnnouncement{
		Song: toProtoSong(s),
	}

	_, err := a.twirp.AnnounceRequest(ctx, announcement)
	return err
}

// NewManagerClient returns a new client implementing radio.ManagerService
func NewManagerClient(addr string, httpclient HTTPClient) radio.ManagerService {
	return ManagerClient{
		twirp: NewManagerProtobufClient(addr, httpclient),
	}
}

// ManagerClient is a twirp client that implements radio.ManagerService
type ManagerClient struct {
	twirp Manager
}

var _ radio.ManagerService = ManagerClient{}

// Status implements radio.ManagerService
func (m ManagerClient) Status(ctx context.Context) (*radio.Status, error) {
	s, err := m.twirp.Status(ctx, new(empty.Empty))
	if err != nil {
		return nil, err
	}

	return &radio.Status{
		User: radio.User{
			ID:       int(s.User.Id),
			Nickname: s.User.Nickname,
			IsRobot:  s.User.IsRobot,
		},
		Song:            fromProtoSong(s.Song),
		SongInfo:        fromProtoSongInfo(s.Info),
		Listeners:       int(s.ListenerInfo.Listeners),
		Thread:          s.Thread,
		RequestsEnabled: s.StreamerConfig.RequestsEnabled,
	}, nil
}

// UpdateUser implements radio.ManagerService
func (m ManagerClient) UpdateUser(ctx context.Context, u radio.User) error {
	_, err := m.twirp.SetUser(ctx, &User{
		Id:       int32(u.ID),
		Nickname: u.Nickname,
		IsRobot:  u.IsRobot,
	})
	return err
}

// UpdateSong implements radio.ManagerService
func (m ManagerClient) UpdateSong(ctx context.Context, s radio.Song, i radio.SongInfo) error {
	_, err := m.twirp.SetSong(ctx, &SongUpdate{
		Song: toProtoSong(s),
		Info: toProtoSongInfo(i),
	})
	return err
}

// UpdateThread implements radio.ManagerService
func (m ManagerClient) UpdateThread(ctx context.Context, thread string) error {
	_, err := m.twirp.SetThread(ctx, &wrappers.StringValue{
		Value: thread,
	})
	return err
}

// UpdateListeners implements radio.ManagerService
func (m ManagerClient) UpdateListeners(ctx context.Context, count int) error {
	_, err := m.twirp.SetListenerInfo(ctx, &ListenerInfo{
		Listeners: int64(count),
	})
	return err
}

// NewStreamerClient returns a new client implementing radio.StreamerService
func NewStreamerClient(addr string, httpclient HTTPClient) radio.StreamerService {
	return StreamerClient{
		twirp: NewStreamerProtobufClient(addr, httpclient),
	}
}

// StreamerClient is a twirp client that implements radio.StreamerService
type StreamerClient struct {
	twirp Streamer
}

var _ radio.StreamerService = StreamerClient{}

// Start implements radio.StreamerService
func (s StreamerClient) Start(ctx context.Context) error {
	_, err := s.twirp.Start(ctx, new(empty.Empty))
	return err
}

// Stop implements radio.StreamerService
func (s StreamerClient) Stop(ctx context.Context, force bool) error {
	_, err := s.twirp.Stop(ctx, &wrappers.BoolValue{
		Value: force,
	})
	return err
}

// RequestSong implements radio.StreamerService
func (s StreamerClient) RequestSong(ctx context.Context, song radio.Song, identifier string) error {
	if !song.HasTrack() {
		panic("request song called with non-database track")
	}

	resp, err := s.twirp.RequestSong(ctx, &SongRequest{
		UserIdentifier: identifier,
		Song:           toProtoSong(song),
	})
	if err != nil {
		return err
	}
	if !resp.Success {
		return errors.New(resp.Msg)
	}
	return nil
}

// Queue implements radio.StreamerService
func (s StreamerClient) Queue(ctx context.Context) ([]radio.QueueEntry, error) {
	resp, err := s.twirp.Queue(ctx, new(empty.Empty))
	if err != nil {
		return nil, err
	}

	var queue = make([]radio.QueueEntry, len(resp.Entries))
	for i := range resp.Entries {
		queue[i] = *fromProtoQueueEntry(resp.Entries[i])
	}

	return queue, nil
}

// NewQueueClient returns a new client implement radio.QueueService
func NewQueueClient(addr string, c HTTPClient) radio.QueueService {
	return QueueClient{
		twirp: NewQueueProtobufClient(addr, c),
	}
}

// QueueClient is a twirp client that implements radio.QueueService
type QueueClient struct {
	twirp Queue
}

var _ radio.QueueService = QueueClient{}

// AddRequest implements radio.QueueService
func (q QueueClient) AddRequest(ctx context.Context, s radio.Song, identifier string) error {
	_, err := q.twirp.AddRequest(ctx, &QueueEntry{
		Song:           toProtoSong(s),
		IsUserRequest:  true,
		UserIdentifier: identifier,
	})
	return err
}

// ReserveNext implements radio.QueueService
func (q QueueClient) ReserveNext(ctx context.Context) (*radio.QueueEntry, error) {
	resp, err := q.twirp.ReserveNext(ctx, new(empty.Empty))
	if err != nil {
		return nil, err
	}

	return fromProtoQueueEntry(resp), nil
}

// Remove implements radio.QueueService
func (q QueueClient) Remove(ctx context.Context, entry radio.QueueEntry) (bool, error) {
	resp, err := q.twirp.Remove(ctx, toProtoQueueEntry(entry))
	if err != nil {
		return false, err
	}

	return resp.Value, nil
}

// Entries implements radio.QueueService
func (q QueueClient) Entries(ctx context.Context) ([]radio.QueueEntry, error) {
	resp, err := q.twirp.Entries(ctx, new(empty.Empty))
	if err != nil {
		return nil, err
	}

	var queue = make([]radio.QueueEntry, len(resp.Entries))
	for _, entry := range resp.Entries {
		queue = append(queue, *fromProtoQueueEntry(entry))
	}
	return queue, nil
}
