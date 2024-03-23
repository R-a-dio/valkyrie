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

	metaMu    sync.Mutex
	metaStore map[Identifier]*Metadata
	mountsMu  sync.RWMutex
	mounts    map[string]*Mount
	cleanupMu sync.Mutex
	cleanup   map[string]*time.Timer
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

func (pm *ProxyManager) CreateMount(name, contentType string, conn net.Conn) *Mount {
	pm.mountsMu.Lock()
	defer pm.mountsMu.Unlock()
	// someone else might've created the mount while we were waiting on the
	// lock, so see if we now exist
	if mount, ok := pm.mounts[name]; ok {
		return mount
	}

	// otherwise we're responsible for creating it
	mount := NewMount(pm.ctx, pm.cfg, pm, name, contentType, conn)
	pm.mounts[name] = mount
	return mount
}

func (pm *ProxyManager) RemoveMount(mount *Mount) {
	pm.cleanupMu.Lock()
	defer pm.cleanupMu.Unlock()

	pm.cleanup[mount.Name] = time.AfterFunc(mountTimeout, func() {
		err := mount.Close()
		if err != nil {
			mount.logger.Error().Err(err).Msg("closing mount master connection")
		}
		pm.mountsMu.Lock()
		defer pm.mountsMu.Unlock()
		mount.logger.Info().Msg("removing mount")
		delete(pm.mounts, mount.Name)
	})
}

func (pm *ProxyManager) AddSourceClient(source *SourceClient) error {
	if source == nil {
		panic("nil source client in AddSourceClient")
	}

	logger := zerolog.Ctx(pm.ctx)

	mount, ok := pm.Mount(source.MountName)
	if !ok {
		// no mount exists yet, create one
		mount = pm.CreateMount(source.MountName, source.ContentType, nil)
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
	// because of a race condition from us calling Mount above and us adding this new
	// source, a RemoveMount could've triggered and deleted us from the mounts map.
	// So after adding a source check to make sure the mount still exists
	pm.mountsMu.Lock()
	defer pm.mountsMu.Unlock()
	if _, ok := pm.mounts[source.MountName]; !ok {
		pm.mounts[source.MountName] = mount
	}
	return nil
}

func (pm *ProxyManager) SendMetadata(ctx context.Context, metadata *Metadata) error {
	if metadata == nil {
		panic("nil metadata in SendMetadata")
	}

	mount, ok := pm.Mount(metadata.MountName)
	if !ok {
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

	mount.SendMetadata(ctx, metadata)
	return nil
}

func (pm *ProxyManager) Mount(name string) (*Mount, bool) {
	pm.mountsMu.RLock()
	defer pm.mountsMu.RUnlock()
	m, ok := pm.mounts[name]
	if !ok {
		return nil, false
	}

	// check if we were cleaning up and cancel it if so
	pm.cleanupMu.Lock()
	defer pm.cleanupMu.Unlock()
	if t := pm.cleanup[name]; t != nil {
		t.Stop()
	}
	return m, true
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
	pm.mountsMu.RLock()
	mounts := maps.Clone(pm.mounts)
	pm.mountsMu.RUnlock()

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
