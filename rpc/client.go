package rpc

import (
	"context"

	radio "github.com/R-a-dio/valkyrie"
	grpc "google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
	wrapperspb "google.golang.org/protobuf/types/known/wrapperspb"
)

func PrepareConn(addr string) *grpc.ClientConn {
	if len(addr) == 0 {
		panic("invalid address passed to PrepareConn: empty string")
	}

	if addr[0] == ':' {
		addr = "localhost" + addr
	}

	conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		panic("failed to setup grpc client: " + err.Error())
	}
	return conn
}

// NewAnnouncerService returns a new client implementing radio.AnnounceService
func NewAnnouncerService(c *grpc.ClientConn) radio.AnnounceService {
	return AnnouncerClientRPC{
		rpc: NewAnnouncerClient(c),
	}
}

// AnnouncerClient is a grpc client that implements radio.AnnounceService
type AnnouncerClientRPC struct {
	rpc AnnouncerClient
}

var _ radio.AnnounceService = AnnouncerClientRPC{}

// AnnounceSong implements radio.AnnounceService
func (a AnnouncerClientRPC) AnnounceSong(ctx context.Context, s radio.Status) error {
	announcement := &SongAnnouncement{
		Song: toProtoSong(s.Song),
		Info: toProtoSongInfo(s.SongInfo),
		ListenerInfo: &ListenerInfo{
			Listeners: int64(s.Listeners),
		},
	}
	_, err := a.rpc.AnnounceSong(ctx, announcement)
	return err
}

// AnnounceRequest implements radio.AnnounceService
func (a AnnouncerClientRPC) AnnounceRequest(ctx context.Context, s radio.Song) error {
	announcement := &SongRequestAnnouncement{
		Song: toProtoSong(s),
	}

	_, err := a.rpc.AnnounceRequest(ctx, announcement)
	return err
}

// NewManagerService returns a new client implementing radio.ManagerService
func NewManagerService(c *grpc.ClientConn) radio.ManagerService {
	return ManagerClientRPC{
		rpc: NewManagerClient(c),
	}
}

// ManagerClient is a grpc client that implements radio.ManagerService
type ManagerClientRPC struct {
	rpc ManagerClient
}

var _ radio.ManagerService = ManagerClientRPC{}

// Status implements radio.ManagerService
func (m ManagerClientRPC) Status(ctx context.Context) (*radio.Status, error) {
	s, err := m.rpc.Status(ctx, new(emptypb.Empty))
	if err != nil {
		return nil, err
	}

	return &radio.Status{
		User:            fromProtoUser(s.User),
		Song:            fromProtoSong(s.Song),
		SongInfo:        fromProtoSongInfo(s.Info),
		Listeners:       int(s.ListenerInfo.Listeners),
		Thread:          s.Thread,
		RequestsEnabled: s.StreamerConfig.RequestsEnabled,
		StreamerName:    s.StreamerName,
	}, nil
}

// UpdateUser implements radio.ManagerService
func (m ManagerClientRPC) UpdateUser(ctx context.Context, n string, u radio.User) error {
	_, err := m.rpc.SetUser(ctx, &UserUpdate{
		User:         toProtoUser(u),
		StreamerName: n,
	})
	return err
}

// UpdateSong implements radio.ManagerService
func (m ManagerClientRPC) UpdateSong(ctx context.Context, s radio.Song, i radio.SongInfo) error {
	_, err := m.rpc.SetSong(ctx, &SongUpdate{
		Song: toProtoSong(s),
		Info: toProtoSongInfo(i),
	})
	return err
}

// UpdateThread implements radio.ManagerService
func (m ManagerClientRPC) UpdateThread(ctx context.Context, thread string) error {
	_, err := m.rpc.SetThread(ctx, wrapperspb.String(thread))
	return err
}

// UpdateListeners implements radio.ManagerService
func (m ManagerClientRPC) UpdateListeners(ctx context.Context, count int) error {
	_, err := m.rpc.SetListenerInfo(ctx, &ListenerInfo{
		Listeners: int64(count),
	})
	return err
}

// NewStreamerService returns a new client implementing radio.StreamerService
func NewStreamerService(c *grpc.ClientConn) radio.StreamerService {
	return StreamerClientRPC{
		rpc: NewStreamerClient(c),
	}
}

// StreamerClient is a grpc client that implements radio.StreamerService
type StreamerClientRPC struct {
	rpc StreamerClient
}

var _ radio.StreamerService = StreamerClientRPC{}

// Start implements radio.StreamerService
func (s StreamerClientRPC) Start(ctx context.Context) error {
	resp, err := s.rpc.Start(ctx, new(emptypb.Empty))
	if err != nil {
		return err
	}
	return fromProtoError(resp.Error)
}

// Stop implements radio.StreamerService
func (s StreamerClientRPC) Stop(ctx context.Context, force bool) error {
	resp, err := s.rpc.Stop(ctx, wrapperspb.Bool(force))
	if err != nil {
		return err
	}
	return fromProtoError(resp.Error)
}

// RequestSong implements radio.StreamerService
func (s StreamerClientRPC) RequestSong(ctx context.Context, song radio.Song, identifier string) error {
	if !song.HasTrack() {
		panic("request song called with non-database track")
	}

	resp, err := s.rpc.RequestSong(ctx, &SongRequest{
		UserIdentifier: identifier,
		Song:           toProtoSong(song),
	})
	if err != nil {
		return err
	}

	return fromProtoError(resp.Error)
}

// Queue implements radio.StreamerService
func (s StreamerClientRPC) Queue(ctx context.Context) ([]radio.QueueEntry, error) {
	resp, err := s.rpc.Queue(ctx, new(emptypb.Empty))
	if err != nil {
		return nil, err
	}

	var queue = make([]radio.QueueEntry, len(resp.Entries))
	for i := range resp.Entries {
		queue[i] = *fromProtoQueueEntry(resp.Entries[i])
	}

	return queue, nil
}

// NewQueueService returns a new client implement radio.QueueService
func NewQueueService(c *grpc.ClientConn) radio.QueueService {
	return QueueClientRPC{
		rpc: NewQueueClient(c),
	}
}

// QueueClient is a grpc client that implements radio.QueueService
type QueueClientRPC struct {
	rpc QueueClient
}

var _ radio.QueueService = QueueClientRPC{}

// AddRequest implements radio.QueueService
func (q QueueClientRPC) AddRequest(ctx context.Context, s radio.Song, identifier string) error {
	_, err := q.rpc.AddRequest(ctx, &QueueEntry{
		Song:           toProtoSong(s),
		IsUserRequest:  true,
		UserIdentifier: identifier,
	})
	return err
}

// ReserveNext implements radio.QueueService
func (q QueueClientRPC) ReserveNext(ctx context.Context) (*radio.QueueEntry, error) {
	resp, err := q.rpc.ReserveNext(ctx, new(emptypb.Empty))
	if err != nil {
		return nil, err
	}

	return fromProtoQueueEntry(resp), nil
}

func (q QueueClientRPC) ResetReserved(ctx context.Context) error {
	// TODO: implement this
	return nil
}

// Remove implements radio.QueueService
func (q QueueClientRPC) Remove(ctx context.Context, entry radio.QueueEntry) (bool, error) {
	resp, err := q.rpc.Remove(ctx, toProtoQueueEntry(entry))
	if err != nil {
		return false, err
	}

	return resp.Value, nil
}

// Entries implements radio.QueueService
func (q QueueClientRPC) Entries(ctx context.Context) ([]radio.QueueEntry, error) {
	resp, err := q.rpc.Entries(ctx, new(emptypb.Empty))
	if err != nil {
		return nil, err
	}

	var queue = make([]radio.QueueEntry, len(resp.Entries))
	for _, entry := range resp.Entries {
		queue = append(queue, *fromProtoQueueEntry(entry))
	}
	return queue, nil
}
