package proxy

import (
	"context"
	"runtime/debug"
	"slices"
	"sync"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/util/eventstream"
	"github.com/rs/zerolog"
)

func NewEventHandler(ctx context.Context, cfg config.Config) *EventHandler {
	return &EventHandler{
		manager: cfg.Manager,
		primaryMountName: config.Value(cfg, func(c config.Config) string {
			return c.Conf().Proxy.PrimaryMountName
		}),
		logger:       *zerolog.Ctx(ctx),
		metaStream:   eventstream.NewEventStreamNoInit[radio.ProxyMetadataEvent](),
		sourceStream: eventstream.NewEventStreamNoInit[radio.ProxySourceEvent](),
		records:      make(map[string]eventRecords),
		status:       newEventProxyStatus(),
	}
}

type eventRecords struct {
	newLiveSource      time.Time
	liveMetadataUpdate time.Time
}

type EventHandler struct {
	logger           zerolog.Logger
	primaryMountName func() string
	manager          radio.ManagerService

	// streaming api support fields
	metaStream   *eventstream.EventStream[radio.ProxyMetadataEvent]
	sourceStream *eventstream.EventStream[radio.ProxySourceEvent]
	status       *eventProxyStatus

	// mu protects records
	mu sync.Mutex
	// map of MountName->eventRecords
	records map[string]eventRecords
}

func newEventProxyStatus() *eventProxyStatus {
	return &eventProxyStatus{
		orphans: make(map[radio.SourceID]eventProxyStatusOrphan),
		Users:   make(map[radio.UserID]*eventUser),
	}
}

type eventProxyStatus struct {
	sync.Mutex
	orphans map[radio.SourceID]eventProxyStatusOrphan
	Users   map[radio.UserID]*eventUser
}

type eventProxyStatusOrphan struct {
	Removed bool
	IsLive  bool
}

func (eps *eventProxyStatus) UpdateLive(ctx context.Context, sc *SourceClient) {
	if sc == nil {
		// got called with a nil, shouldn't happen
		return
	}
	eps.Lock()
	defer eps.Unlock()

	// update our user status
	eu := eps.Users[sc.User.ID]
	if eu == nil {
		o := eps.orphans[sc.ID]
		o.IsLive = true
		eps.orphans[sc.ID] = o
		return
	}
	for _, eus := range eu.Conns {
		if eus.ID != sc.ID {
			continue
		}

		eus.IsLive = true
	}
}

func (eps *eventProxyStatus) AddSource(ctx context.Context, sc *SourceClient) {
	if sc == nil {
		// got called with a nil, shouldn't happen
		return
	}
	eps.Lock()
	defer eps.Unlock()

	// check if this source has an orphan entry
	orphan, exists := eps.orphans[sc.ID]
	if exists {
		// if it does, we're going to consume it now so delete it
		delete(eps.orphans, sc.ID)
	}

	// check if said orphan indicated that we already got removed
	if orphan.Removed {
		// if it has we ignore it
		return
	}

	eu := eps.Users[sc.User.ID]
	if eu == nil {
		eu = &eventUser{
			User: sc.User,
		}
		eps.Users[sc.User.ID] = eu
	}
	eu.Conns = append(eu.Conns, &eventUserSource{
		SourceClient: sc,
		IsLive:       orphan.IsLive,
	})
}

func (eps *eventProxyStatus) RemoveSource(ctx context.Context, sc *SourceClient) {
	if sc == nil {
		// got called with a nil, shouldn't happen
		return
	}
	eps.Lock()
	defer eps.Unlock()

	eu := eps.Users[sc.User.ID]
	if eu == nil {
		// if the user doesn't exist either RemoveSource was called twice with the same source
		// (which shouldn't happen) or this Remove event happened before our matching Add and
		// that would leave us with an orphan later, mark this in the map so the Add can see it
		o := eps.orphans[sc.ID]
		o.Removed = true
		eps.orphans[sc.ID] = o
		return
	}
	// remove the sc
	eu.Conns = slices.DeleteFunc(eu.Conns, func(eus *eventUserSource) bool {
		return eus.ID == sc.ID
	})
	// if we have no conns left remove ourselves from the map
	if len(eu.Conns) == 0 {
		delete(eps.Users, sc.User.ID)
	}
}

type eventUser struct {
	User  radio.User
	Conns []*eventUserSource
}

type eventUserSource struct {
	*SourceClient
	IsLive bool
}

// eventNewLiveSource is called whenever the active live source is changed
func (eh *EventHandler) eventNewLiveSource(ctx context.Context, mountName string, new *SourceClient) {
	// record when we were called since the goroutine might start running at
	// some other later time we use this to avoid logic races
	instant := time.Now()
	go func() {
		defer recoverPanicLogger(ctx)
		// send source liveness event to any RPC listener
		if new != nil {
			eh.sourceStream.Send(radio.ProxySourceEvent{
				ID:        new.ID,
				MountName: mountName,
				User:      new.User,
				Event:     radio.SourceLive,
			})
		}

		// update the user in the manager
		eh.updateManagerUser(ctx, mountName, new, instant)

		// update our user status
		if new != nil {
			eh.status.UpdateLive(ctx, new)
		}
	}()
}

func (eh *EventHandler) updateManagerUser(ctx context.Context, mountName string, new *SourceClient, instant time.Time) {
	eh.mu.Lock()
	defer eh.mu.Unlock()

	record := eh.records[mountName]
	if record.newLiveSource.After(instant) {
		// someone else already went live and was later, so eat
		// this event since it's out-dated
		return
	}

	// update the manager if we're changing live source on the primary
	// mount, other mounts it doesn't care about
	if mountName == eh.primaryMountName() {
		var user *radio.User
		if new != nil && new.User.ID != 0 {
			user = &new.User
		}

		err := eh.manager.UpdateUser(ctx, user)
		if err != nil {
			eh.logger.Error().Err(err).Msg("failed to update user")
			return
		}
	}

	// update the record
	record.newLiveSource = instant
	eh.records[mountName] = record
}

// eventMetadataUpdate is called when a metadata update comes through.
func (eh *EventHandler) eventMetadataUpdate(ctx context.Context, new *Metadata) {
	instant := time.Now()
	go func() {
		defer recoverPanicLogger(ctx)
		_ = instant

		// send metadata to any RPC listeners
		if new != nil {
			eh.metaStream.Send(radio.ProxyMetadataEvent{
				MountName: new.MountName,
				Metadata:  new.Value,
				User:      new.User,
			})
		}
	}()
}

// eventLiveMetadataUpdate is a metadata update from a live source, to any mount.
// This is used to update the manager metadata if the mount is the primary mount,
// otherwise only used for display purposes to the admin panel
func (eh *EventHandler) eventLiveMetadataUpdate(ctx context.Context, mountName string, metadata string) {
	instant := time.Now()
	go func() {
		defer recoverPanicLogger(ctx)
		eh.mu.Lock()
		defer eh.mu.Unlock()

		record := eh.records[mountName]
		if record.liveMetadataUpdate.After(instant) {
			// someone else beat us to sending metadata, eat this outdated
			// event instead
			return
		}

		// update the manager if we're changing metadata for the primary mount
		if mountName == eh.primaryMountName() {
			err := eh.manager.UpdateSong(ctx, &radio.SongUpdate{
				Song: radio.NewSong(metadata),
				Info: radio.SongInfo{
					Start: instant,
				},
			})
			if err != nil {
				eh.logger.Error().Err(err).Msg("failed to update song")
				return
			}
		}

		// update the record
		record.liveMetadataUpdate = instant
		eh.records[mountName] = record
	}()
}

// eventSourceConnect is called when a source connects to any mountpoint
func (eh *EventHandler) eventSourceConnect(ctx context.Context, source *SourceClient) {
	go func() {
		defer recoverPanicLogger(ctx)
		// send source connect event to any RPC listener
		if source != nil {
			eh.sourceStream.Send(radio.ProxySourceEvent{
				ID:        source.ID,
				MountName: source.MountName,
				User:      source.User,
				Event:     radio.SourceConnect,
			})
		}

		// update our user status
		eh.status.AddSource(ctx, source)
	}()
}

// eventSourceDisconnect is called when a source disconnects from any mountpoint
func (eh *EventHandler) eventSourceDisconnect(ctx context.Context, source *SourceClient) {
	go func() {
		defer recoverPanicLogger(ctx)
		// send source disconnect event to any RPC listener
		if source != nil {
			eh.sourceStream.Send(radio.ProxySourceEvent{
				ID:        source.ID,
				MountName: source.MountName,
				User:      source.User,
				Event:     radio.SourceDisconnect,
			})
		}

		// update our user status
		eh.status.RemoveSource(ctx, source)
	}()
}

func recoverPanicLogger(ctx context.Context) {
	rvr := recover()
	if rvr == nil {
		return
	}
	if err, ok := rvr.(error); ok && err != nil {
		zerolog.Ctx(ctx).WithLevel(zerolog.PanicLevel).Str("stack", string(debug.Stack())).Err(err).Msg("panic in event handler")
		return
	}
	zerolog.Ctx(ctx).WithLevel(zerolog.PanicLevel).Str("stack", string(debug.Stack())).Any("recover", rvr).Msg("panic in event handler")
}
