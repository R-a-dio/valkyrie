package rpc

import (
	"context"
	"io"
	"runtime/debug"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/util/eventstream"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/recovery"
	"github.com/rs/zerolog"
	otelcodes "go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
	wrapperspb "google.golang.org/protobuf/types/known/wrapperspb"
)

func recoverFn(ctx context.Context, rvr any) error {
	err, ok := rvr.(error)
	if !ok {
		err = errors.New("panic in grpc server")
	}

	span := trace.SpanFromContext(ctx)
	span.SetStatus(otelcodes.Error, "panic in grpc server")
	span.RecordError(err, trace.WithStackTrace(true))

	zerolog.Ctx(ctx).WithLevel(zerolog.PanicLevel).Str("stack", string(debug.Stack())).Msg("panic in grpc server")
	return status.Errorf(codes.Unknown, "panic in server handler")
}

var NewGrpcServer = func(_ context.Context, opts ...grpc.ServerOption) *grpc.Server {
	// handle panics that happen inside of any grpc server handlers
	recoveryOpts := []recovery.Option{
		recovery.WithRecoveryHandlerContext(recoverFn),
	}
	// append them to the end, recovery handlers should typically be last in the chain so
	// that other middleware can operate on the recovered state instead of being directly
	// affected by any panic
	opts = append(opts,
		grpc.UnaryInterceptor(
			recovery.UnaryServerInterceptor(recoveryOpts...),
		),
		grpc.StreamInterceptor(
			recovery.StreamServerInterceptor(recoveryOpts...),
		),
	)
	return grpc.NewServer(opts...)
}

// NewAnnouncer returns a new shim around the service given
func NewAnnouncer(a radio.AnnounceService) AnnouncerServer {
	return AnnouncerShim{
		announcer: a,
	}
}

// AnnouncerShim implements Announcer
type AnnouncerShim struct {
	UnsafeAnnouncerServer
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

func (as AnnouncerShim) AnnounceUser(ctx context.Context, au *UserAnnouncement) (*emptypb.Empty, error) {
	err := as.announcer.AnnounceUser(ctx, fromProtoUser(au.User))
	return new(emptypb.Empty), err
}

func (as AnnouncerShim) AnnounceMurder(ctx context.Context, ma *MurderAnnouncement) (*emptypb.Empty, error) {
	err := as.announcer.AnnounceMurder(ctx, fromProtoUser(ma.By), ma.Force)
	return new(emptypb.Empty), err
}

func NewProxy(p radio.ProxyService) ProxyServer {
	return ProxyShim{
		proxy: p,
	}
}

type ProxyShim struct {
	UnsafeProxyServer
	proxy radio.ProxyService
}

func (ps ProxyShim) MetadataStream(_ *emptypb.Empty, s Proxy_MetadataStreamServer) error {
	return streamToProtobuf(s, ps.proxy.MetadataStream, toProtoProxyMetadataEvent)
}

func (ps ProxyShim) SourceStream(_ *emptypb.Empty, s Proxy_SourceStreamServer) error {
	return streamToProtobuf(s, ps.proxy.SourceStream, toProtoProxySourceEvent)
}

func (ps ProxyShim) StatusStream(req *ProxyStatusRequest, s Proxy_StatusStreamServer) error {
	return streamToProtobuf(s, func(ctx context.Context) (eventstream.Stream[[]radio.ProxySource], error) {
		return ps.proxy.StatusStream(ctx, radio.UserID(req.UserId))
	}, toProtoProxyStatusEvent)
}

func (ps ProxyShim) KickSource(ctx context.Context, id *SourceID) (*emptypb.Empty, error) {
	err := ps.proxy.KickSource(ctx, fromProtoSourceID(id))
	return new(emptypb.Empty), err
}

func (ps ProxyShim) ListSources(ctx context.Context, _ *emptypb.Empty) (*ProxyListResponse, error) {
	sl, err := ps.proxy.ListSources(ctx)
	if err != nil {
		return nil, err
	}

	sources := make([]*ProxySource, len(sl))
	for i, s := range sl {
		sources[i] = toProtoProxySource(s)
	}

	return &ProxyListResponse{
		Sources: sources,
	}, nil
}

func NewGuest(g radio.GuestService) GuestServer {
	return GuestShim{
		guest: g,
	}
}

type GuestShim struct {
	UnsafeGuestServer
	guest radio.GuestService
}

func (g GuestShim) Create(ctx context.Context, user *GuestUser) (*GuestCreateResponse, error) {
	u, pwd, err := g.guest.Create(ctx, fromProtoGuestUser(user))
	return &GuestCreateResponse{
		User:     toProtoUser(u),
		Password: pwd,
	}, err
}

func (g GuestShim) Auth(ctx context.Context, user *GuestUser) (*GuestAuthResponse, error) {
	u, err := g.guest.Auth(ctx, fromProtoGuestUser(user))
	return &GuestAuthResponse{
		User: toProtoUser(u),
	}, err
}

func (g GuestShim) Deauth(ctx context.Context, user *GuestUser) (*emptypb.Empty, error) {
	err := g.guest.Deauth(ctx, user.GetName())
	return new(emptypb.Empty), err
}

func (g GuestShim) CanDo(ctx context.Context, gcd *GuestCanDo) (*wrapperspb.BoolValue, error) {
	nick, action := fromProtoGuestCanDo(gcd)
	ok, err := g.guest.CanDo(ctx, nick, action)
	return wrapperspb.Bool(ok), err
}

func (g GuestShim) Do(ctx context.Context, gcd *GuestCanDo) (*wrapperspb.BoolValue, error) {
	nick, action := fromProtoGuestCanDo(gcd)
	ok, err := g.guest.Do(ctx, nick, action)
	return wrapperspb.Bool(ok), err
}

// NewManager returns a new shim around the service given
func NewManager(m radio.ManagerService) ManagerServer {
	return ManagerShim{
		manager: m,
	}
}

// ManagerShim implements Manager
type ManagerShim struct {
	UnsafeManagerServer
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

// streamToProtobuf turns an eventstream.Stream into an grpc.ServerStream
//
// the types are:
//
//	T: the radio type the eventstream.Stream returns
//	P: the protobuf type the grpc stream returns
//
// the arguments are:
//
//	s: the grpc ServerStream
//	streamFn: the function to make the eventstream.Stream
//	conv: a function that converts T into P
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

func (m ManagerShim) UpdateFromStorage(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	err := m.manager.UpdateFromStorage(ctx)
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
	UnsafeStreamerServer
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
func (ss StreamerShim) Stop(ctx context.Context, ssr *StreamerStopRequest) (*StreamerResponse, error) {
	err := ss.streamer.Stop(ctx, fromProtoUser(ssr.Who), ssr.Force)
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
	UnsafeQueueServer
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

func (q QueueShim) ResetReserved(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	return nil, q.queue.ResetReserved(ctx)
}

func NewListenerTracker(lt radio.ListenerTrackerService) ListenerTrackerServer {
	return ListenerTrackerShim{tracker: lt}
}

// ListenerTrackerShim implements ListenerTracker
type ListenerTrackerShim struct {
	UnsafeListenerTrackerServer
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

func (lt ListenerTrackerShim) RemoveClient(ctx context.Context, req *TrackerRemoveClientRequest) (*emptypb.Empty, error) {
	err := lt.tracker.RemoveClient(ctx, radio.ListenerClientID(req.Id))
	if err != nil {
		return nil, err
	}
	return new(emptypb.Empty), nil
}
