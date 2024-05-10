package proxy

import (
	"context"
	"maps"
	"net"
	"sync"
	"time"

	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/util/graceful"
	"github.com/rs/zerolog"
	xmaps "golang.org/x/exp/maps"
)

type ProxyManager struct {
	ctx          context.Context
	cfg          config.Config
	reloadConfig chan config.Config

	// metaMu protects metaStore
	metaMu    sync.Mutex
	metaStore map[Identifier]*Metadata
	// mountsMu protects mounts and cleanup
	mountsMu sync.Mutex
	mounts   map[string]*Mount
	cleanup  map[string]*time.Timer
}

func NewProxyManager(ctx context.Context, cfg config.Config) (*ProxyManager, error) {
	m := &ProxyManager{
		ctx:          ctx,
		cfg:          cfg,
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
		mount = NewMount(pm.ctx, pm.cfg, pm, source.MountName, source.ContentType, nil)
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
	// a source client, storing a nil here is fine
	source.Metadata.Store(pm.Metadata(source.Identifier))

	// add the source to the mount list
	mount.AddSource(pm.ctx, source)
	return nil
}

func (pm *ProxyManager) SendMetadata(ctx context.Context, metadata *Metadata) error {
	if metadata == nil {
		panic("nil metadata in SendMetadata")
	}

	pm.mountsMu.Lock()
	mount, ok := pm.mounts[metadata.MountName]
	if ok {
		pm.mountsMu.Unlock()
		mount.SendMetadata(ctx, metadata)
		return nil
	}
	defer pm.mountsMu.Unlock()

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

type wireProxy struct {
	Mounts   []string
	Metadata map[Identifier]*Metadata
}

func (pm *ProxyManager) writeSelf(dst *net.UnixConn) error {
	pm.mountsMu.Lock()
	mounts := maps.Clone(pm.mounts)
	pm.mountsMu.Unlock()

	pm.metaMu.Lock()
	metadata := maps.Clone(pm.metaStore)
	pm.metaMu.Unlock()

	wp := wireProxy{
		Mounts:   xmaps.Keys(mounts),
		Metadata: metadata,
	}

	err := graceful.WriteJSON(dst, wp)
	if err != nil {
		return err
	}

	for _, mount := range mounts {
		err := mount.writeSelf(dst)
		if err != nil {
			return err
		}
	}
	return nil
}

func (pm *ProxyManager) readSelf(ctx context.Context, cfg config.Config, src *net.UnixConn) error {
	var wp wireProxy

	zerolog.Ctx(ctx).Info().Msg("resume: reading proxy manager data")
	err := graceful.ReadJSON(src, &wp)
	if err != nil {
		return err
	}

	zerolog.Ctx(ctx).Info().Any("wire", wp).Msg("resume")

	pm.metaMu.Lock()
	xmaps.Copy(pm.metaStore, wp.Metadata)
	pm.metaMu.Unlock()

	mounts := make(map[string]*Mount)
	for range wp.Mounts {
		mount := new(Mount)

		err = mount.readSelf(ctx, cfg, src)
		if err != nil {
			return err
		}

		mounts[mount.Name] = mount
	}

	pm.mountsMu.Lock()
	xmaps.Copy(pm.mounts, mounts)
	pm.mountsMu.Unlock()

	return nil
}
