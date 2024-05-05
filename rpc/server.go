package rpc

import (
	"context"
	"io"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/util/eventstream"
	grpc "google.golang.org/grpc"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
	wrapperspb "google.golang.org/protobuf/types/known/wrapperspb"
)

var NewGrpcServer = grpc.NewServer

// NewAnnouncer returns a new shim around the service given
func NewAnnouncer(a radio.AnnounceService) AnnouncerServer {
	return AnnouncerShim{
		announcer: a,
	}
}

// AnnouncerShim implements Announcer
type AnnouncerShim struct {
	UnimplementedAnnouncerServer
	announcer radio.AnnounceService
}

// AnnounceSong implements Announcer
func (as AnnouncerShim) AnnounceSong(ctx context.Context, a *SongAnnouncement) (*emptypb.Empty, error) {
	err := as.announcer.AnnounceSong(ctx, radio.Status{
		Song:      fromProtoSong(a.Song),
		SongInfo:  fromProtoSongInfo(a.Info),
		Listeners: a.ListenerInfo.Listeners,
	})
	return new(emptypb.Empty), err
}

// AnnounceRequest implements Announcer
func (as AnnouncerShim) AnnounceRequest(ctx context.Context, ar *SongRequestAnnouncement) (*emptypb.Empty, error) {
	err := as.announcer.AnnounceRequest(ctx, fromProtoSong(ar.Song))
	return new(emptypb.Empty), err
}

// NewManager returns a new shim around the service given
func NewManager(m radio.ManagerService) ManagerServer {
	return ManagerShim{
		manager: m,
	}
}

// ManagerShim implements Manager
type ManagerShim struct {
	UnimplementedManagerServer
	manager radio.ManagerService
}

// Status implements Manager
func (sm ManagerShim) CurrentStatus(_ *emptypb.Empty, s Manager_CurrentStatusServer) error {
	return streamToProtobuf(s, sm.manager.CurrentStatus, toProtoStatus)
}

type pbSender[P any] interface {
	Send(P) error
	grpc.ServerStream
}

func streamToProtobuf[T any, P any](s pbSender[P], streamFn func(context.Context) (eventstream.Stream[T], error), conv func(T) P) error {
	ctx, cancel := context.WithCancel(s.Context())
	defer cancel()

	stream, err := streamFn(ctx)
	if err != nil {
		if errors.IsE(err, context.Canceled) {
			return nil
		}
		return err
	}
	defer stream.Close()

	for {
		recv, err := stream.Next()
		if err != nil {
			if errors.IsE(err, io.EOF, context.Canceled) {
				return nil
			}
			return err
		}
		err = s.Send(conv(recv))
		if err != nil {
			if errors.IsE(err, io.EOF, context.Canceled) {
				return nil
			}
			return err
		}
	}
}

func (sm ManagerShim) CurrentSong(_ *emptypb.Empty, s Manager_CurrentSongServer) error {
	return streamToProtobuf(s, sm.manager.CurrentSong, toProtoSongUpdate)
}

func (m ManagerShim) UpdateSong(ctx context.Context, su *SongUpdate) (*emptypb.Empty, error) {
	err := m.manager.UpdateSong(ctx, fromProtoSongUpdate(su))
	return new(emptypb.Empty), err
}

func (sm ManagerShim) CurrentThread(_ *emptypb.Empty, s Manager_CurrentThreadServer) error {
	return streamToProtobuf(s, sm.manager.CurrentThread, wrapperspb.String)
}

func (m ManagerShim) UpdateThread(ctx context.Context, t *wrapperspb.StringValue) (*emptypb.Empty, error) {
	err := m.manager.UpdateThread(ctx, t.Value)
	return new(emptypb.Empty), err
}

func (sm ManagerShim) CurrentUser(_ *emptypb.Empty, s Manager_CurrentUserServer) error {
	return streamToProtobuf(s, sm.manager.CurrentUser, toProtoUser)
}

func (m ManagerShim) UpdateUser(ctx context.Context, u *User) (*emptypb.Empty, error) {
	err := m.manager.UpdateUser(ctx, fromProtoUser(u))
	return new(emptypb.Empty), err
}

func (sm ManagerShim) CurrentListenerCount(_ *emptypb.Empty, s Manager_CurrentListenerCountServer) error {
	return streamToProtobuf(s, sm.manager.CurrentListeners, wrapperspb.Int64)
}

func (m ManagerShim) UpdateListenerCount(ctx context.Context, i *wrapperspb.Int64Value) (*emptypb.Empty, error) {
	err := m.manager.UpdateListeners(ctx, i.Value)
	return new(emptypb.Empty), err
}

// NewStreamer returns a new shim around the service given
func NewStreamer(s radio.StreamerService) StreamerServer {
	return StreamerShim{
		streamer: s,
	}
}

// StreamerShim implements Streamer
type StreamerShim struct {
	UnimplementedStreamerServer
	streamer radio.StreamerService
}

// Start implements Streamer
func (ss StreamerShim) Start(ctx context.Context, _ *emptypb.Empty) (*StreamerResponse, error) {
	err := ss.streamer.Start(ctx)
	resp := new(StreamerResponse)
	resp.Error, err = toProtoError(err)
	return resp, err
}

// Stop implements Streamer
func (ss StreamerShim) Stop(ctx context.Context, force *wrapperspb.BoolValue) (*StreamerResponse, error) {
	err := ss.streamer.Stop(ctx, force.Value)
	resp := new(StreamerResponse)
	resp.Error, err = toProtoError(err)
	return resp, err
}

// RequestSong implements Streamer
func (ss StreamerShim) RequestSong(ctx context.Context, req *SongRequest) (*RequestResponse, error) {
	err := ss.streamer.RequestSong(ctx, fromProtoSong(req.Song), req.UserIdentifier)
	resp := new(RequestResponse)
	resp.Error, err = toProtoError(err)
	return resp, err
}

// Queue implements Streamer
func (ss StreamerShim) Queue(ctx context.Context, _ *emptypb.Empty) (*QueueInfo, error) {
	entries, err := ss.streamer.Queue(ctx)
	if err != nil {
		return nil, err
	}

	queue := make([]*QueueEntry, len(entries))
	for i := range entries {
		queue[i] = toProtoQueueEntry(entries[i])
	}
	return &QueueInfo{
		Name:    "default",
		Entries: queue,
	}, nil
}

// SetConfig implements Streamer
func (ss StreamerShim) SetConfig(ctx context.Context, c *StreamerConfig) (*emptypb.Empty, error) {
	// TODO: implement this
	return new(emptypb.Empty), nil
}

// NewQueue returns a new shim around the service given
func NewQueue(q radio.QueueService) QueueServer {
	return QueueShim{
		queue: q,
	}
}

// QueueShim implements Queue
type QueueShim struct {
	UnimplementedQueueServer
	queue radio.QueueService
}

// AddRequest implements Queue
func (q QueueShim) AddRequest(ctx context.Context, e *QueueEntry) (*emptypb.Empty, error) {
	err := q.queue.AddRequest(ctx, fromProtoSong(e.Song), e.UserIdentifier)
	if err != nil {
		return nil, err
	}
	return new(emptypb.Empty), nil
}

// ReserveNext implements Queue
func (q QueueShim) ReserveNext(ctx context.Context, _ *emptypb.Empty) (*QueueEntry, error) {
	e, err := q.queue.ReserveNext(ctx)
	if err != nil {
		return nil, err
	}

	return toProtoQueueEntry(*e), nil
}

// Remove implements Queue
func (q QueueShim) Remove(ctx context.Context, id *QueueID) (*wrapperspb.BoolValue, error) {
	ok, err := q.queue.Remove(ctx, fromProtoQueueID(id))
	if err != nil {
		return nil, err
	}

	return wrapperspb.Bool(ok), nil
}

// Entries implements Queue
func (q QueueShim) Entries(ctx context.Context, _ *emptypb.Empty) (*QueueInfo, error) {
	entries, err := q.queue.Entries(ctx)
	if err != nil {
		return nil, err
	}

	queue := make([]*QueueEntry, len(entries))
	for i := range entries {
		queue[i] = toProtoQueueEntry(entries[i])
	}
	return &QueueInfo{
		Name:    "default",
		Entries: queue,
	}, nil
}

func NewListenerTracker(lt radio.ListenerTrackerService) ListenerTrackerServer {
	return ListenerTrackerShim{tracker: lt}
}

// ListenerTrackerShim implements ListenerTracker
type ListenerTrackerShim struct {
	UnimplementedListenerTrackerServer
	tracker radio.ListenerTrackerService
}

func (lt ListenerTrackerShim) ListClients(ctx context.Context, _ *emptypb.Empty) (*Listeners, error) {
	entries, err := lt.tracker.ListClients(ctx)
	if err != nil {
		return nil, err
	}

	listeners := make([]*Listener, len(entries))
	for i := range entries {
		listeners[i] = toProtoListener(entries[i])
	}

	return &Listeners{
		Entries: listeners,
	}, nil
}

func (lt ListenerTrackerShim) RemoveClient(ctx context.Context) (*emptypb.Empty, error) {
	return nil, nil
}
