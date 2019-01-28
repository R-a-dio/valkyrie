package rpc

import (
	"context"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/golang/protobuf/ptypes/wrappers"
)

// NewAnnouncer returns a new shim around the service given
func NewAnnouncer(a radio.AnnounceService) Announcer {
	return AnnouncerShim{
		announcer: a,
	}
}

// AnnouncerShim implements Announcer
type AnnouncerShim struct {
	announcer radio.AnnounceService
}

// AnnounceSong implements Announcer
func (as AnnouncerShim) AnnounceSong(ctx context.Context, a *SongAnnouncement) (*empty.Empty, error) {
	err := as.announcer.AnnounceSong(ctx, radio.Status{
		Song:      fromProtoSong(a.Song),
		SongInfo:  fromProtoSongInfo(a.Info),
		Listeners: int(a.ListenerInfo.Listeners),
	})
	return new(empty.Empty), err
}

// AnnounceRequest implements Announcer
func (as AnnouncerShim) AnnounceRequest(ctx context.Context, ar *SongRequestAnnouncement) (*empty.Empty, error) {
	err := as.announcer.AnnounceRequest(ctx, fromProtoSong(ar.Song))
	return new(empty.Empty), err
}

// NewManager returns a new shim around the service given
func NewManager(m radio.ManagerService) Manager {
	return ManagerShim{
		manager: m,
	}
}

// ManagerShim implements Manager
type ManagerShim struct {
	manager radio.ManagerService
}

// Status implements Manager
func (m ManagerShim) Status(ctx context.Context, _ *empty.Empty) (*StatusResponse, error) {
	s, err := m.manager.Status(ctx)
	if err != nil {
		return nil, err
	}

	return &StatusResponse{
		User: toProtoUser(s.User),
		Song: toProtoSong(s.Song),
		Info: toProtoSongInfo(s.SongInfo),
		ListenerInfo: &ListenerInfo{
			Listeners: int64(s.Listeners),
		},
		Thread: s.Thread,
		StreamerConfig: &StreamerConfig{
			RequestsEnabled: s.RequestsEnabled,
		},
		StreamerName: s.StreamerName,
	}, nil
}

// SetUser implements Manager
func (m ManagerShim) SetUser(ctx context.Context, u *UserUpdate) (*empty.Empty, error) {
	err := m.manager.UpdateUser(ctx, u.StreamerName, fromProtoUser(u.User))
	return new(empty.Empty), err
}

// SetSong implements Manager
func (m ManagerShim) SetSong(ctx context.Context, su *SongUpdate) (*empty.Empty, error) {
	err := m.manager.UpdateSong(ctx, fromProtoSong(su.Song), fromProtoSongInfo(su.Info))
	return new(empty.Empty), err
}

// SetStreamerConfig implements Manager
func (m ManagerShim) SetStreamerConfig(ctx context.Context, c *StreamerConfig) (*empty.Empty, error) {
	// TODO: implement this
	return new(empty.Empty), nil
}

// SetThread implements Manager
func (m ManagerShim) SetThread(ctx context.Context, t *wrappers.StringValue) (*empty.Empty, error) {
	err := m.manager.UpdateThread(ctx, t.Value)
	return new(empty.Empty), err
}

// SetListenerInfo implements Manager
func (m ManagerShim) SetListenerInfo(ctx context.Context, i *ListenerInfo) (*empty.Empty, error) {
	err := m.manager.UpdateListeners(ctx, int(i.Listeners))
	return new(empty.Empty), err
}

// NewStreamer returns a new shim around the service given
func NewStreamer(s radio.StreamerService) Streamer {
	return StreamerShim{
		streamer: s,
	}
}

// StreamerShim implements Streamer
type StreamerShim struct {
	streamer radio.StreamerService
}

// Start implements Streamer
func (ss StreamerShim) Start(ctx context.Context, _ *empty.Empty) (*StreamerResponse, error) {
	err := ss.streamer.Start(ctx)
	resp := new(StreamerResponse)
	resp.UserError, err = toProtoUserError(err)
	return resp, err
}

// Stop implements Streamer
func (ss StreamerShim) Stop(ctx context.Context, force *wrappers.BoolValue) (*StreamerResponse, error) {
	err := ss.streamer.Stop(ctx, force.Value)
	resp := new(StreamerResponse)
	resp.UserError, err = toProtoUserError(err)
	return resp, err
}

// RequestSong implements Streamer
func (ss StreamerShim) RequestSong(ctx context.Context, req *SongRequest) (*RequestResponse, error) {
	err := ss.streamer.RequestSong(ctx, fromProtoSong(req.Song), req.UserIdentifier)
	if err == nil {
		// special response for HTTP JSON callers
		return &RequestResponse{
			Success: true,
			Msg:     "thank you for making your request!",
		}, nil
	}

	// otherwise check if we got our user error type
	uerr, ok := err.(radio.SongRequestError)
	if ok {
		return toProtoRequestResponse(uerr), nil
	}

	return new(RequestResponse), err
}

// Queue implements Streamer
func (ss StreamerShim) Queue(ctx context.Context, _ *empty.Empty) (*QueueInfo, error) {
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
func (ss StreamerShim) SetConfig(ctx context.Context, c *StreamerConfig) (*empty.Empty, error) {
	// TODO: implement this
	return new(empty.Empty), nil
}

// NewQueue returns a new shim around the service given
func NewQueue(q radio.QueueService) Queue {
	return QueueShim{
		queue: q,
	}
}

// QueueShim implements Queue
type QueueShim struct {
	queue radio.QueueService
}

// AddRequest implements Queue
func (q QueueShim) AddRequest(ctx context.Context, e *QueueEntry) (*empty.Empty, error) {
	err := q.queue.AddRequest(ctx, fromProtoSong(e.Song), e.UserIdentifier)
	if err != nil {
		return nil, err
	}
	return new(empty.Empty), nil
}

// ReserveNext implements Queue
func (q QueueShim) ReserveNext(ctx context.Context, _ *empty.Empty) (*QueueEntry, error) {
	e, err := q.queue.ReserveNext(ctx)
	if err != nil {
		return nil, err
	}

	return toProtoQueueEntry(*e), nil
}

// Remove implements Queue
func (q QueueShim) Remove(ctx context.Context, e *QueueEntry) (*wrappers.BoolValue, error) {
	ok, err := q.queue.Remove(ctx, *fromProtoQueueEntry(e))
	if err != nil {
		return nil, err
	}

	return &wrappers.BoolValue{
		Value: ok,
	}, nil
}

// Entries implements Queue
func (q QueueShim) Entries(ctx context.Context, _ *empty.Empty) (*QueueInfo, error) {
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
