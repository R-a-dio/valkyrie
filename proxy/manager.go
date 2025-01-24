package proxy

import (
	"context"
	"encoding/json"
	"maps"
	"sync"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/rs/zerolog"
)

type ProxyManager struct {
	ctx    context.Context
	cfg    config.Config
	events *EventHandler
	uss    radio.UserStorageService

	reloadConfig chan config.Config

	// metaMu protects metaStore
	metaMu    sync.Mutex
	metaStore map[Identifier]*Metadata
	// mountsMu protects mounts and cleanup
	mountsMu sync.Mutex
	mounts   map[string]*Mount
	cleanup  map[string]*time.Timer
}

func NewProxyManager(ctx context.Context, cfg config.Config, uss radio.UserStorageService, eh *EventHandler) (*ProxyManager, error) {
	m := &ProxyManager{
		ctx:          ctx,
		cfg:          cfg,
		events:       eh,
		uss:          uss,
		reloadConfig: make(chan config.Config),
		metaStore:    make(map[Identifier]*Metadata),
		mounts:       make(map[string]*Mount),
		cleanup:      make(map[string]*time.Timer),
	}
	return m, nil
}

func (pm *ProxyManager) RemoveMount(mount *Mount) {
	pm.mountsMu.Lock()
	defer pm.mountsMu.Unlock()

	t := pm.cleanup[mount.Name]
	if t != nil {
		// we're already trying to cleanup apparently, reset the timer
		t.Reset(mountTimeout)
		return
	}

	pm.cleanup[mount.Name] = time.AfterFunc(mountTimeout, func() {
		pm.mountsMu.Lock()
		defer pm.mountsMu.Unlock()

		// first see if we're still ment to be cleaning up, the cleanup can
		// be canceled by an AddSourceClient call between timer start and
		// timer fire, and we keep track of this by the entry in cleanup
		//
		// note that there is an obvious logic race here where the call to
		// AddSourceClient is waiting on mountsMu but we have already acquired
		// it, this does mean we've already passed our mountTimeout so just
		// proceed with cleaning up and the AddSourceClient can make a new mount
		if pm.cleanup[mount.Name] == nil {
			return
		}

		// otherwise we are still fine to cleanup and
		// remove the mount from our internal state
		delete(pm.mounts, mount.Name)

		// then let the mount cleanup whatever it needs to cleanup
		err := mount.Close()
		if err != nil {
			mount.logger.Error().Err(err).Msg("closing mount master connection")
		}
	})
}

func (pm *ProxyManager) AddSourceClient(source *SourceClient) error {
	if source == nil {
		panic("nil source client in AddSourceClient")
	}

	logger := zerolog.Ctx(pm.ctx)

	pm.mountsMu.Lock()
	defer pm.mountsMu.Unlock()

	mount, ok := pm.mounts[source.MountName]
	if !ok { // if it didn't exist create one
		mount = NewMount(pm.ctx, pm.cfg, pm, pm.events, source.MountName, source.ContentType, nil)
		pm.mounts[source.MountName] = mount
	}

	// check if our mount was empty and was waiting to clean itself up
	if t := pm.cleanup[mount.Name]; t != nil {
		// it's possible that we call Stop but the timer has already fired,
		// but since we're holding onto mountsMu the removal function can't run
		// yet and will cancel itself once it sees we've removed the entry in
		// the cleanup map.
		t.Stop()
		delete(pm.cleanup, mount.Name)
	}

	if mount.ContentType != source.ContentType {
		// log content-type mismatches, but keep going as if it didn't happen.
		// This shouldn't really occur unless someone is being silly
		logger.Warn().
			Str("mount-content-type", mount.ContentType).
			Str("source-content-type", source.ContentType).
			Msg("mismatching content-type")
	}

	logger.Info().
		Str("mount", source.MountName).
		Str("username", source.User.Username).
		Str("address", source.conn.RemoteAddr().String()).
		Msg("adding source client")

	// see if we have any metadata from before this source connected as
	// a source client
	if pre := pm.Metadata(source.Identifier); pre != nil {
		source.Metadata.Store(pre)
	}

	// add the source to the mount list
	mount.AddSource(pm.ctx, source)
	return nil
}

func (pm *ProxyManager) RemoveSourceClient(ctx context.Context, id radio.SourceID) error {
	pm.mountsMu.Lock()
	mounts := maps.Clone(pm.mounts)
	pm.mountsMu.Unlock()

	for _, mount := range mounts {
		mount.RemoveSource(ctx, id)
	}

	return nil
}

func (pm *ProxyManager) ListSources(ctx context.Context) ([]radio.ProxySource, error) {
	pm.mountsMu.Lock()
	defer pm.mountsMu.Unlock()

	var res []radio.ProxySource

	addSources := func(mount *Mount) {
		mount.SourcesMu.RLock()
		defer mount.SourcesMu.RUnlock()
		for _, source := range mount.Sources {
			res = append(res, ProxySourceFromSourceClient(source.Source, source.Priority, source.MW.GetLive()))
		}
	}

	// each mount has their respective mutex so we use an anonymous function to easily
	// handle the mutex locking and unlocking
	for _, mount := range pm.mounts {
		addSources(mount)
	}

	return res, nil
}

func ProxySourceFromSourceClient(sc *SourceClient, prio uint32, isLive bool) radio.ProxySource {
	var metadata string
	if tmp := sc.Metadata.Load(); tmp != nil {
		metadata = tmp.Value
	}

	return radio.ProxySource{
		ID:        sc.ID,
		User:      sc.User,
		Start:     sc.Start,
		MountName: sc.MountName,
		UserAgent: sc.UserAgent,
		Metadata:  metadata,
		IP:        sc.conn.RemoteAddr().String(),
		Priority:  prio,
		IsLive:    isLive,
	}
}

func (pm *ProxyManager) SendMetadata(ctx context.Context, metadata *Metadata) error {
	if metadata == nil {
		panic("nil metadata in SendMetadata")
	}
	// event handler
	defer pm.events.eventMetadataUpdate(ctx, metadata)

	pm.mountsMu.Lock()
	mount, ok := pm.mounts[metadata.MountName]
	pm.mountsMu.Unlock()
	if ok {
		mount.SendMetadata(ctx, metadata)
		return nil
	}

	// metadata for a mount that doesn't exist, we store it temporarily
	// to see if a new source client will appear soon
	zerolog.Ctx(ctx).Info().
		Str("mount", metadata.MountName).
		Str("username", metadata.User.Username).
		Str("address", metadata.Addr).
		Msg("storing metadata because mount does not exist")
	pm.metaMu.Lock()
	pm.metaStore[metadata.Identifier] = metadata
	pm.metaMu.Unlock()

	return nil
}

func (pm *ProxyManager) Metadata(identifier Identifier) *Metadata {
	pm.metaMu.Lock()
	defer pm.metaMu.Unlock()
	defer delete(pm.metaStore, identifier)
	return pm.metaStore[identifier]
}

type storedProxy struct {
	Metadata map[Identifier]*Metadata
}

func (pm *ProxyManager) MarshalJSON() ([]byte, error) {
	pm.metaMu.Lock()
	metadata := maps.Clone(pm.metaStore)
	pm.metaMu.Unlock()

	sp := storedProxy{
		Metadata: metadata,
	}

	return json.Marshal(sp)
}

func (pm *ProxyManager) UnmarshalJSON(b []byte) error {
	pm.metaMu.Lock()
	defer pm.metaMu.Unlock()

	var sp storedProxy
	err := json.Unmarshal(b, &sp)
	if err != nil {
		return err
	}

	pm.metaStore = sp.Metadata
	return nil
}
