package rpc

import (
	"context"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/util/eventstream"
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

func (m ManagerClientRPC) CurrentUser(ctx context.Context) (eventstream.Stream[radio.User], error) {
	c := func(ctx context.Context, e *emptypb.Empty, opts ...grpc.CallOption) (pbReceiver[*User], error) {
		return m.rpc.CurrentUser(ctx, e, opts...)
	}
	return streamFromProtobuf(ctx, c, fromProtoUser)
}

// UpdateUser implements radio.ManagerService
func (m ManagerClientRPC) UpdateUser(ctx context.Context, u radio.User) error {
	_, err := m.rpc.UpdateUser(ctx, toProtoUser(u))
	return err
}

func (m ManagerClientRPC) CurrentSong(ctx context.Context) (eventstream.Stream[*radio.SongUpdate], error) {
	c := func(ctx context.Context, e *emptypb.Empty, opts ...grpc.CallOption) (pbReceiver[*SongUpdate], error) {
		return m.rpc.CurrentSong(ctx, e, opts...)
	}
	return streamFromProtobuf(ctx, c, fromProtoSongUpdate)
}

// UpdateSong implements radio.ManagerService
func (m ManagerClientRPC) UpdateSong(ctx context.Context, u *radio.SongUpdate) error {
	_, err := m.rpc.UpdateSong(ctx, toProtoSongUpdate(u))
	return err
}

// UpdateThread implements radio.ManagerService
func (m ManagerClientRPC) UpdateThread(ctx context.Context, thread radio.Thread) error {
	_, err := m.rpc.UpdateThread(ctx, wrapperspb.String(thread))
	return err
}

func (m ManagerClientRPC) CurrentThread(ctx context.Context) (eventstream.Stream[radio.Thread], error) {
	c := func(ctx context.Context, e *emptypb.Empty, opts ...grpc.CallOption) (pbReceiver[*wrapperspb.StringValue], error) {
		return m.rpc.CurrentThread(ctx, e, opts...)
	}
	return streamFromProtobuf(ctx, c, func(v *wrapperspb.StringValue) radio.Thread { return v.Value })
}

// UpdateListeners implements radio.ManagerService
func (m ManagerClientRPC) UpdateListeners(ctx context.Context, count radio.Listeners) error {
	_, err := m.rpc.UpdateListenerCount(ctx, wrapperspb.Int64(count))
	return err
}

func (m ManagerClientRPC) CurrentListeners(ctx context.Context) (eventstream.Stream[radio.Listeners], error) {
	c := func(ctx context.Context, e *emptypb.Empty, opts ...grpc.CallOption) (pbReceiver[*wrapperspb.Int64Value], error) {
		return m.rpc.CurrentListenerCount(ctx, e, opts...)
	}
	return streamFromProtobuf(ctx, c, func(v *wrapperspb.Int64Value) radio.Listeners { return v.Value })
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

type pbCreator[P any] func(context.Context, *emptypb.Empty, ...grpc.CallOption) (pbReceiver[P], error)

type pbReceiver[P any] interface {
	Recv() (P, error)
	grpc.ClientStream
}

type grpcStream[P, T any] struct {
	s      pbReceiver[P]
	conv   func(P) T
	cancel context.CancelFunc
}

func (gs *grpcStream[P, T]) Next() (T, error) {
	p, err := gs.s.Recv()
	if err != nil {
		return *new(T), err
	}
	return gs.conv(p), nil
}

func (gs *grpcStream[P, T]) Close() error {
	gs.cancel()
	return nil
}

func streamFromProtobuf[P, T any](ctx context.Context, streamFn pbCreator[P], conv func(P) T) (eventstream.Stream[T], error) {
	var gs grpcStream[P, T]
	var err error

	ctx, gs.cancel = context.WithCancel(ctx)

	gs.s, err = streamFn(ctx, new(emptypb.Empty))
	if err != nil {
		return nil, err
	}

	gs.conv = conv
	return &gs, nil
}
