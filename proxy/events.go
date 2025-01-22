package proxy

import (
	"context"
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

	// mu protects records
	mu sync.Mutex
	// map of MountName->eventRecords
	records map[string]eventRecords
}

// "live" got swapped (any mount)
func (eh *EventHandler) eventNewLiveSource(ctx context.Context, mountName string, new *SourceClient) {
	// record when we were called since the goroutine might start running at
	// some other later time we use this to avoid logic races
	instant := time.Now()
	go func() {
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
	}()
}

// eventMetadataUpdate is any metadata update send by any source, to any mount.
// We use this information mostly for display purposes to the admin panel
func (eh *EventHandler) eventMetadataUpdate(ctx context.Context, new *Metadata) {
	instant := time.Now()
	go func() {
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

// source connected (any mount)
func (eh *EventHandler) eventSourceConnect(ctx context.Context, source *SourceClient) {
	go func() {
		// send source connect event to any RPC listener
		if source != nil {
			eh.sourceStream.Send(radio.ProxySourceEvent{
				ID:        source.ID,
				MountName: source.MountName,
				User:      source.User,
				Event:     radio.SourceConnect,
			})
		}
	}()
}

func (eh *EventHandler) eventSourceDisconnect(ctx context.Context, source *SourceClient) {
	go func() {
		// send source disconnect event to any RPC listener
		if source != nil {
			eh.sourceStream.Send(radio.ProxySourceEvent{
				ID:        source.ID,
				MountName: source.MountName,
				User:      source.User,
				Event:     radio.SourceDisconnect,
			})
		}
	}()
}
