package config

import (
	"context"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/rpc"
	"github.com/R-a-dio/valkyrie/util/eventstream"
	"google.golang.org/grpc"
)

func newGuestService(cfg Config) radio.GuestService {
	return &guestService{
		Value(cfg, func(c Config) radio.GuestService {
			return rpc.NewGuestService(rpc.PrepareConn(cfg.Conf().Manager.RPCAddr.String()))
		}),
	}
}

type guestService struct {
	fn func() radio.GuestService
}

func (g *guestService) Create(ctx context.Context, nick string) (*radio.User, string, error) {
	return g.fn().Create(ctx, nick)
}

func (g *guestService) Auth(ctx context.Context, nick string) (*radio.User, error) {
	return g.fn().Auth(ctx, nick)
}

func (g *guestService) Deauth(ctx context.Context, nick string) error {
	return g.fn().Deauth(ctx, nick)
}

func (g *guestService) CanDo(ctx context.Context, nick string, action radio.GuestAction) (bool, error) {
	return g.fn().CanDo(ctx, nick, action)
}

var _ radio.GuestService = &guestService{}

func newManagerService(cfg Config) radio.ManagerService {
	return &managerService{
		Value(cfg, func(c Config) radio.ManagerService {
			return rpc.NewManagerService(rpc.PrepareConn(cfg.Conf().Manager.RPCAddr.String()))
		}),
	}
}

type managerService struct {
	fn func() radio.ManagerService
}

// CurrentListeners implements radio.ManagerService.
func (m *managerService) CurrentListeners(ctx context.Context) (eventstream.Stream[int64], error) {
	return m.fn().CurrentListeners(ctx)
}

// CurrentSong implements radio.ManagerService.
func (m *managerService) CurrentSong(ctx context.Context) (eventstream.Stream[*radio.SongUpdate], error) {
	return m.fn().CurrentSong(ctx)
}

// CurrentStatus implements radio.ManagerService.
func (m *managerService) CurrentStatus(ctx context.Context) (eventstream.Stream[radio.Status], error) {
	return m.fn().CurrentStatus(ctx)
}

// CurrentThread implements radio.ManagerService.
func (m *managerService) CurrentThread(ctx context.Context) (eventstream.Stream[string], error) {
	return m.fn().CurrentThread(ctx)
}

// CurrentUser implements radio.ManagerService.
func (m *managerService) CurrentUser(ctx context.Context) (eventstream.Stream[*radio.User], error) {
	return m.fn().CurrentUser(ctx)
}

// UpdateListeners implements radio.ManagerService.
func (m *managerService) UpdateListeners(ctx context.Context, i int64) error {
	return m.fn().UpdateListeners(ctx, i)
}

// UpdateSong implements radio.ManagerService.
func (m *managerService) UpdateSong(ctx context.Context, su *radio.SongUpdate) error {
	return m.fn().UpdateSong(ctx, su)
}

// UpdateThread implements radio.ManagerService.
func (m *managerService) UpdateThread(ctx context.Context, thread string) error {
	return m.fn().UpdateThread(ctx, thread)
}

// UpdateUser implements radio.ManagerService.
func (m *managerService) UpdateUser(ctx context.Context, u *radio.User) error {
	return m.fn().UpdateUser(ctx, u)
}

func (m *managerService) UpdateFromStorage(ctx context.Context) error {
	return m.fn().UpdateFromStorage(ctx)
}

var _ radio.ManagerService = &managerService{}

func newProxyService(cfg Config) radio.ProxyService {
	return &proxyService{
		Value(cfg, func(c Config) radio.ProxyService {
			return rpc.NewProxyService(rpc.PrepareConn(cfg.Conf().Proxy.RPCAddr.String()))
		}),
	}
}

type proxyService struct {
	fn func() radio.ProxyService
}

func (p *proxyService) MetadataStream(ctx context.Context) (eventstream.Stream[radio.ProxyMetadataEvent], error) {
	return p.fn().MetadataStream(ctx)
}

func (p *proxyService) SourceStream(ctx context.Context) (eventstream.Stream[radio.ProxySourceEvent], error) {
	return p.fn().SourceStream(ctx)
}

func (p *proxyService) StatusStream(ctx context.Context, id radio.UserID) (eventstream.Stream[[]radio.ProxySource], error) {
	return p.fn().StatusStream(ctx, id)
}

func (p *proxyService) KickSource(ctx context.Context, id radio.SourceID) error {
	return p.fn().KickSource(ctx, id)
}

func (p *proxyService) ListSources(ctx context.Context) ([]radio.ProxySource, error) {
	return p.fn().ListSources(ctx)
}

func newStreamerService(cfg Config, conn func() *grpc.ClientConn) radio.StreamerService {
	return &streamerService{
		Value(cfg, func(c Config) radio.StreamerService {
			return rpc.NewStreamerService(conn())
		}),
	}
}

type streamerService struct {
	fn func() radio.StreamerService
}

// Queue implements radio.StreamerService.
func (s *streamerService) Queue(ctx context.Context) (radio.Queue, error) {
	return s.fn().Queue(ctx)
}

// RequestSong implements radio.StreamerService.
func (s *streamerService) RequestSong(ctx context.Context, song radio.Song, identifier string) error {
	return s.fn().RequestSong(ctx, song, identifier)
}

// Start implements radio.StreamerService.
func (s *streamerService) Start(ctx context.Context) error {
	return s.fn().Start(ctx)
}

// Stop implements radio.StreamerService.
func (s *streamerService) Stop(ctx context.Context, force bool) error {
	return s.fn().Stop(ctx, force)
}

func newQueueService(cfg Config, conn func() *grpc.ClientConn) radio.QueueService {
	return &queueService{
		Value(cfg, func(c Config) radio.QueueService {
			return rpc.NewQueueService(conn())
		}),
	}
}

type queueService struct {
	fn func() radio.QueueService
}

// AddRequest implements radio.QueueService.
func (q *queueService) AddRequest(ctx context.Context, song radio.Song, identifier string) error {
	return q.fn().AddRequest(ctx, song, identifier)
}

// Entries implements radio.QueueService.
func (q *queueService) Entries(ctx context.Context) (radio.Queue, error) {
	return q.fn().Entries(ctx)
}

// Remove implements radio.QueueService.
func (q *queueService) Remove(ctx context.Context, id radio.QueueID) (bool, error) {
	return q.fn().Remove(ctx, id)
}

// ReserveNext implements radio.QueueService.
func (q *queueService) ReserveNext(ctx context.Context) (*radio.QueueEntry, error) {
	return q.fn().ReserveNext(ctx)
}

// ResetReserved implements radio.QueueService.
func (q *queueService) ResetReserved(ctx context.Context) error {
	return q.fn().ResetReserved(ctx)
}

func newTrackerService(cfg Config) radio.ListenerTrackerService {
	return &trackerService{
		Value(cfg, func(c Config) radio.ListenerTrackerService {
			return rpc.NewListenerTrackerService(rpc.PrepareConn(cfg.Conf().Tracker.RPCAddr.String()))
		}),
	}
}

type trackerService struct {
	fn func() radio.ListenerTrackerService
}

// ListClients implements radio.ListenerTrackerService.
func (t *trackerService) ListClients(ctx context.Context) ([]radio.Listener, error) {
	return t.fn().ListClients(ctx)
}

// RemoveClient implements radio.ListenerTrackerService.
func (t *trackerService) RemoveClient(ctx context.Context, id radio.ListenerClientID) error {
	return t.fn().RemoveClient(ctx, id)
}

func newIRCService(cfg Config) radio.AnnounceService {
	return &ircService{
		Value(cfg, func(c Config) radio.AnnounceService {
			return rpc.NewAnnouncerService(rpc.PrepareConn(cfg.Conf().IRC.RPCAddr.String()))
		}),
	}
}

type ircService struct {
	fn func() radio.AnnounceService
}

// AnnounceRequest implements radio.AnnounceService.
func (i *ircService) AnnounceRequest(ctx context.Context, song radio.Song) error {
	return i.fn().AnnounceRequest(ctx, song)
}

// AnnounceSong implements radio.AnnounceService.
func (i *ircService) AnnounceSong(ctx context.Context, status radio.Status) error {
	return i.fn().AnnounceSong(ctx, status)
}

// AnnounceUser implements radio.AnnounceService.
func (i *ircService) AnnounceUser(ctx context.Context, user *radio.User) error {
	return i.fn().AnnounceUser(ctx, user)
}
