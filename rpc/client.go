package rpc

import (
	"context"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/util/eventstream"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	grpc "google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
	wrapperspb "google.golang.org/protobuf/types/known/wrapperspb"
)

var GrpcDial = grpc.NewClient

func PrepareConn(addr string) *grpc.ClientConn {
	if len(addr) == 0 {
		panic("invalid address passed to PrepareConn: empty string")
	}

	if addr[0] == ':' {
		addr = "localhost" + addr
	}

	conn, err := GrpcDial(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		panic("failed to setup grpc client: " + err.Error())
	}
	return conn
}

func NewListenerTrackerService(c *grpc.ClientConn) radio.ListenerTrackerService {
	return ListenerTrackerClientRPC{
		rpc: NewListenerTrackerClient(c),
	}
}

type ListenerTrackerClientRPC struct {
	rpc ListenerTrackerClient
}

var _ radio.ListenerTrackerService = ListenerTrackerClientRPC{}

func (lt ListenerTrackerClientRPC) ListClients(ctx context.Context) ([]radio.Listener, error) {
	resp, err := lt.rpc.ListClients(ctx, new(emptypb.Empty))
	if err != nil {
		return nil, err
	}

	listeners := make([]radio.Listener, len(resp.Entries))
	for i := range resp.Entries {
		listeners[i] = fromProtoListener(resp.Entries[i])
	}

	return listeners, nil
}

func (lt ListenerTrackerClientRPC) RemoveClient(ctx context.Context, id radio.ListenerClientID) error {
	_, err := lt.rpc.RemoveClient(ctx, &TrackerRemoveClientRequest{
		Id: uint64(id),
	})
	if err != nil {
		return err
	}
	return nil
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

func (a AnnouncerClientRPC) AnnounceUser(ctx context.Context, u *radio.User) error {
	ua := &UserAnnouncement{
		User: toProtoUser(u),
	}

	_, err := a.rpc.AnnounceUser(ctx, ua)
	return err
}

func (a AnnouncerClientRPC) AnnounceMurder(ctx context.Context, by radio.User, force bool) error {
	ma := &MurderAnnouncement{
		By:    toProtoUser(&by),
		Force: force,
	}

	_, err := a.rpc.AnnounceMurder(ctx, ma)
	return err
}

func NewProxyService(c *grpc.ClientConn) radio.ProxyService {
	return ProxyClientRPC{
		rpc: NewProxyClient(c),
	}
}

type ProxyClientRPC struct {
	rpc ProxyClient
}

func (p ProxyClientRPC) MetadataStream(ctx context.Context) (eventstream.Stream[radio.ProxyMetadataEvent], error) {
	c := func(ctx context.Context, e *emptypb.Empty, opts ...grpc.CallOption) (grpc.ServerStreamingClient[ProxyMetadataEvent], error) {
		return p.rpc.MetadataStream(ctx, e, opts...)
	}
	return streamFromProtobuf(ctx, c, new(emptypb.Empty), fromProtoProxyMetadataEvent)
}

func (p ProxyClientRPC) SourceStream(ctx context.Context) (eventstream.Stream[radio.ProxySourceEvent], error) {
	c := func(ctx context.Context, e *emptypb.Empty, opts ...grpc.CallOption) (grpc.ServerStreamingClient[ProxySourceEvent], error) {
		return p.rpc.SourceStream(ctx, e, opts...)
	}
	return streamFromProtobuf(ctx, c, new(emptypb.Empty), fromProtoProxySourceEvent)
}

func (p ProxyClientRPC) StatusStream(ctx context.Context, id radio.UserID) (eventstream.Stream[[]radio.ProxySource], error) {
	c := func(ctx context.Context, req *ProxyStatusRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[ProxyStatusEvent], error) {
		return p.rpc.StatusStream(ctx, req, opts...)
	}

	req := &ProxyStatusRequest{
		UserId: int32(id),
	}
	return streamFromProtobuf(ctx, c, req, fromProtoProxyStatusEvent)
}

func (p ProxyClientRPC) KickSource(ctx context.Context, id radio.SourceID) error {
	_, err := p.rpc.KickSource(ctx, toProtoSourceID(id))
	return err
}

func (p ProxyClientRPC) ListSources(ctx context.Context) ([]radio.ProxySource, error) {
	s, err := p.rpc.ListSources(ctx, new(emptypb.Empty))
	if err != nil {
		return nil, err
	}

	ss := make([]radio.ProxySource, len(s.Sources))
	for i, ps := range s.Sources {
		ss[i] = fromProtoProxySource(ps)
	}

	return ss, nil
}

func NewGuestService(c *grpc.ClientConn) radio.GuestService {
	return GuestClientRPC{
		rpc: NewGuestClient(c),
	}
}

type GuestClientRPC struct {
	rpc GuestClient
}

func (g GuestClientRPC) Create(ctx context.Context, nick string) (*radio.User, string, error) {
	u, err := g.rpc.Create(ctx, toProtoGuestUser(nick))
	if u != nil {
		return fromProtoUser(u.User), u.Password, err
	}
	return nil, "", err
}

func (g GuestClientRPC) Auth(ctx context.Context, nick string) (*radio.User, error) {
	u, err := g.rpc.Auth(ctx, toProtoGuestUser(nick))
	if u != nil {
		return fromProtoUser(u.User), err
	}
	return nil, err
}

func (g GuestClientRPC) Deauth(ctx context.Context, nick string) error {
	_, err := g.rpc.Deauth(ctx, toProtoGuestUser(nick))
	return err
}

func (g GuestClientRPC) CanDo(ctx context.Context, nick string, action radio.GuestAction) (bool, error) {
	b, err := g.rpc.CanDo(ctx, toProtoGuestCanDo(nick, action))
	return b.GetValue(), err
}

func (g GuestClientRPC) Do(ctx context.Context, nick string, action radio.GuestAction) (bool, error) {
	b, err := g.rpc.Do(ctx, toProtoGuestCanDo(nick, action))
	return b.GetValue(), err
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
func (m ManagerClientRPC) CurrentStatus(ctx context.Context) (eventstream.Stream[radio.Status], error) {
	c := func(ctx context.Context, e *emptypb.Empty, opts ...grpc.CallOption) (grpc.ServerStreamingClient[StatusResponse], error) {
		return m.rpc.CurrentStatus(ctx, e, opts...)
	}
	return streamFromProtobuf(ctx, c, new(emptypb.Empty), fromProtoStatus)
}

func (m ManagerClientRPC) CurrentUser(ctx context.Context) (eventstream.Stream[*radio.User], error) {
	c := func(ctx context.Context, e *emptypb.Empty, opts ...grpc.CallOption) (grpc.ServerStreamingClient[User], error) {
		return m.rpc.CurrentUser(ctx, e, opts...)
	}
	return streamFromProtobuf(ctx, c, new(emptypb.Empty), fromProtoUser)
}

// UpdateUser implements radio.ManagerService
func (m ManagerClientRPC) UpdateUser(ctx context.Context, u *radio.User) error {
	_, err := m.rpc.UpdateUser(ctx, toProtoUser(u))
	return err
}

func (m ManagerClientRPC) CurrentSong(ctx context.Context) (eventstream.Stream[*radio.SongUpdate], error) {
	c := func(ctx context.Context, e *emptypb.Empty, opts ...grpc.CallOption) (grpc.ServerStreamingClient[SongUpdate], error) {
		return m.rpc.CurrentSong(ctx, e, opts...)
	}
	return streamFromProtobuf(ctx, c, new(emptypb.Empty), fromProtoSongUpdate)
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
	c := func(ctx context.Context, e *emptypb.Empty, opts ...grpc.CallOption) (grpc.ServerStreamingClient[wrapperspb.StringValue], error) {
		return m.rpc.CurrentThread(ctx, e, opts...)
	}
	return streamFromProtobuf(ctx, c, new(emptypb.Empty), func(v *wrapperspb.StringValue) radio.Thread { return v.Value })
}

// UpdateListeners implements radio.ManagerService
func (m ManagerClientRPC) UpdateListeners(ctx context.Context, count radio.Listeners) error {
	_, err := m.rpc.UpdateListenerCount(ctx, wrapperspb.Int64(count))
	return err
}

func (m ManagerClientRPC) CurrentListeners(ctx context.Context) (eventstream.Stream[radio.Listeners], error) {
	c := func(ctx context.Context, e *emptypb.Empty, opts ...grpc.CallOption) (grpc.ServerStreamingClient[wrapperspb.Int64Value], error) {
		return m.rpc.CurrentListenerCount(ctx, e, opts...)
	}
	return streamFromProtobuf(ctx, c, new(emptypb.Empty), func(v *wrapperspb.Int64Value) radio.Listeners { return v.Value })
}

func (m ManagerClientRPC) UpdateFromStorage(ctx context.Context) error {
	_, err := m.rpc.UpdateFromStorage(ctx, new(emptypb.Empty))
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
func (s StreamerClientRPC) Queue(ctx context.Context) (radio.Queue, error) {
	resp, err := s.rpc.Queue(ctx, new(emptypb.Empty))
	if err != nil {
		return nil, err
	}

	var queue = make([]radio.QueueEntry, len(resp.Entries))
	for i := range resp.Entries {
		queue[i] = fromProtoQueueEntry(resp.Entries[i])
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

	entry := fromProtoQueueEntry(resp)
	return &entry, nil
}

func (q QueueClientRPC) ResetReserved(ctx context.Context) error {
	_, err := q.rpc.ResetReserved(ctx, new(emptypb.Empty))
	if err != nil {
		return err
	}
	return nil
}

// Remove implements radio.QueueService
func (q QueueClientRPC) Remove(ctx context.Context, id radio.QueueID) (bool, error) {
	resp, err := q.rpc.Remove(ctx, toProtoQueueID(id))
	if err != nil {
		return false, err
	}

	return resp.Value, nil
}

// Entries implements radio.QueueService
func (q QueueClientRPC) Entries(ctx context.Context) (radio.Queue, error) {
	resp, err := q.rpc.Entries(ctx, new(emptypb.Empty))
	if err != nil {
		return nil, err
	}

	var queue = make([]radio.QueueEntry, len(resp.Entries))
	for i, entry := range resp.Entries {
		queue[i] = fromProtoQueueEntry(entry)
	}
	return queue, nil
}

type pbCreator[P, A any] func(context.Context, A, ...grpc.CallOption) (grpc.ServerStreamingClient[P], error)

type grpcStream[P, T any] struct {
	s             grpc.ServerStreamingClient[P]
	conv          func(*P) T
	cancel        context.CancelFunc
	setSpanStatus func()
}

func (gs *grpcStream[P, T]) Next() (T, error) {
	p, err := gs.s.Recv()
	if err != nil {
		return *new(T), err
	}
	return gs.conv(p), nil
}

func (gs *grpcStream[P, T]) Close() error {
	// telemetry support, because our grpc instrumentation marks a context canceled
	// as an Error and we don't want that, so we set an explicit OK here if we get closed
	if gs.setSpanStatus != nil {
		gs.setSpanStatus()
	}
	// cancel the context we passed to stream creation
	gs.cancel()
	return nil
}

// streamFromProtobuf turns a grpc.ClientStream into an eventstream.Stream
//
// the types are:
//
//	A: the argument to the grpc stream creation function
//	P: the protobuf type the grpc stream returns
//	T: the radio type the eventstream.Stream should return
//
// the arguments are:
//
//	streamFn: the grpc function to create the grpc stream side
//	arg: the argument to streamFn
//	conv: a function that converts P into T
func streamFromProtobuf[A, P, T any](ctx context.Context, streamFn pbCreator[P, A], arg A, conv func(*P) T) (eventstream.Stream[T], error) {
	var gs grpcStream[P, T]
	var err error

	ctx, gs.cancel = context.WithCancel(ctx)
	gs.setSpanStatus = func() {
		trace.SpanFromContext(ctx).SetStatus(codes.Ok, "")
	}

	gs.s, err = streamFn(ctx, arg)
	if err != nil {
		gs.cancel()
		return nil, err
	}

	gs.conv = conv
	return &gs, nil
}
