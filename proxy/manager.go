package proxy

import (
	"context"
	"net/url"

	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/rs/zerolog"
)

func createMountURL(master url.URL, mount string) *url.URL {
	master.Path = mount
	return &master
}

type ProxyManager struct {
	masterServerURL *url.URL

	newSource   chan *SourceClient
	newMetadata chan *Metadata

	metaStore map[Identifier]*Metadata

	Mounts map[string]*Mount
}

func NewProxyManager(cfg config.Config) (*ProxyManager, error) {
	const op errors.Op = "proxy.NewProxyManager"

	uri, err := url.Parse(cfg.Conf().Proxy.MasterServer)
	if err != nil {
		return nil, errors.E(op, err)
	}
	m := &ProxyManager{
		masterServerURL: uri,
		newSource:       make(chan *SourceClient),
		newMetadata:     make(chan *Metadata),
		metaStore:       make(map[Identifier]*Metadata),
		Mounts:          make(map[string]*Mount),
	}
	return m, nil
}

func (pm *ProxyManager) Run(ctx context.Context) {
	logger := zerolog.Ctx(ctx)

	for {
		select {
		case source := <-pm.newSource:
			// TODO: check if all source clients have the same content-type
			m, ok := pm.Mounts[source.MountName]
			if !ok {
				// no mount exists yet, create one
				logger.Info().Str("mount", source.MountName).Msg("create mount")
				m = NewMount(ctx, createMountURL(*pm.masterServerURL, source.MountName), source.ContentType)
				pm.Mounts[source.MountName] = m
			}

			logger.Info().
				Str("mount", source.MountName).
				Str("username", source.User.Username).
				Str("address", source.conn.RemoteAddr().String()).
				Msg("adding source client")

			// see if we have any metadata from before this source connected as
			// a source client, storing a nil here is fine
			source.Metadata.Store(pm.metaStore[source.Identifier])
			delete(pm.metaStore, source.Identifier)

			// add the source to the mount list
			m.AddSource(ctx, source)
		case metadata := <-pm.newMetadata:
			m, ok := pm.Mounts[metadata.MountName]
			if !ok {
				// metadata for a mount that doesn't exist, we store it temporarily
				// to see if a new source client will appear soon
				logger.Info().
					Str("mount", metadata.MountName).
					Str("username", metadata.User.Username).
					Str("address", metadata.Addr).
					Msg("storing metadata because mount does not exist")
				pm.metaStore[metadata.Identifier] = metadata
				continue
			}

			m.SendMetadata(ctx, metadata)
		case <-ctx.Done():
			return
		}
	}
}

func (pm *ProxyManager) AddSourceClient(ctx context.Context, c *SourceClient) error {
	if c == nil {
		panic("nil source client in AddSourceClient")
	}
	select {
	case pm.newSource <- c:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (pm *ProxyManager) SendMetadata(ctx context.Context, m *Metadata) error {
	if m == nil {
		panic("nil metadata in SendMetadata")
	}
	select {
	case pm.newMetadata <- m:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
