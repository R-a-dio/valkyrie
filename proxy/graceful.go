package proxy

import (
	"cmp"
	"context"
	"encoding/json"
	"net"
	"slices"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/Wessie/fdstore"
	"github.com/rs/zerolog"
)

func (srv *Server) storeSelf(ctx context.Context, store *fdstore.Store) error {
	state, err := json.Marshal(srv.proxy)
	if err != nil {
		return err
	}

	// store proxy state
	srv.listenerMu.Lock()
	_ = store.AddListener(srv.listener, "proxy", state)
	srv.listenerMu.Unlock()

	// store each mount in the proxy
	return srv.proxy.storeMounts(ctx, store)
}

func (pm *ProxyManager) storeMounts(ctx context.Context, store *fdstore.Store) error {
	pm.mountsMu.Lock()
	defer pm.mountsMu.Unlock()

	for _, mount := range pm.mounts {
		err := mount.storeSelf(ctx, store)
		if err != nil {
			zerolog.Ctx(ctx).Error().Err(err).Msg("mount store fail")
			continue
		}
	}
	return nil
}

func (m *Mount) storeSelf(ctx context.Context, store *fdstore.Store) error {
	state, err := json.Marshal(m)
	if err != nil {
		return err
	}
	_ = store.AddConn(m.Conn.Load(), "mount", state)

	return m.storeSources(ctx, store)
}

func (m *Mount) storeSources(ctx context.Context, store *fdstore.Store) error {
	m.SourcesMu.Lock()
	defer m.SourcesMu.Unlock()

	for _, source := range m.Sources {
		err := source.storeSelf(ctx, store)
		if err != nil {
			zerolog.Ctx(ctx).Error().Err(err).Msg("source store fail")
		}
	}
	return nil
}

type storedSource struct {
	ID          SourceID
	Priority    uint
	UserAgent   string
	ContentType string
	MountName   string
	Username    string
	Identifier  Identifier
	Metadata    *Metadata

	conn net.Conn
}

func (msc *MountSourceClient) storeSelf(ctx context.Context, store *fdstore.Store) error {
	ss := storedSource{
		ID:          msc.Source.ID,
		Priority:    msc.Priority,
		UserAgent:   msc.Source.UserAgent,
		ContentType: msc.Source.ContentType,
		MountName:   msc.Source.MountName,
		Username:    msc.Source.User.Username,
		Identifier:  msc.Source.Identifier,
		Metadata:    msc.Source.Metadata.Load(),
	}

	state, err := json.Marshal(ss)
	if err != nil {
		return err
	}

	return store.AddConn(msc.Source.conn, "source", state)
}

func (pm *ProxyManager) restoreMounts(ctx context.Context, store *fdstore.Store) error {
	mounts, err := store.RemoveConn("mount")
	if err != nil {
		return err
	}

	// since we run before any serving is happening this isn't technically
	// needed, but for the sake of correctness we lock all mutations below
	pm.mountsMu.Lock()

	for _, entry := range mounts {
		mount := new(Mount)

		err := json.Unmarshal(entry.Data, mount)
		if err != nil {
			// might still be able to recover other mounts so keep going
			zerolog.Ctx(ctx).Error().Err(err).Any("entry", entry).Msg("failed mount restore")
			entry.Conn.Close()
			continue
		}

		pm.mounts[mount.Name] = NewMount(
			ctx,
			pm.cfg,
			pm,
			pm.events,
			mount.Name,
			mount.ContentType,
			entry.Conn,
		)
	}

	pm.mountsMu.Unlock()
	return pm.restoreSources(ctx, pm.uss.User(ctx), store)
}

func (pm *ProxyManager) restoreSources(ctx context.Context, us radio.UserStorage, store *fdstore.Store) error {
	sources, err := store.RemoveConn("source")
	if err != nil {
		return err
	}

	var storedSources = make([]*storedSource, 0, len(sources))
	for _, entry := range sources {
		source := new(storedSource)

		err := json.Unmarshal(entry.Data, source)
		if err != nil {
			// might still be able to recover other sources if this fails
			zerolog.Ctx(ctx).Error().Err(err).Any("entry", entry).Msg("failed source restore")
			entry.Conn.Close()
			continue
		}
		source.conn = entry.Conn

		storedSources = append(storedSources, source)
	}

	// sort sources by their original priority so that every source should get
	// added in the correct ordering again
	slices.SortFunc(storedSources, func(a, b *storedSource) int {
		return cmp.Compare(a.Priority, b.Priority)
	})

	for _, source := range storedSources {
		// recover the full user data for this source
		user, err := us.Get(source.Username)
		if err != nil {
			zerolog.Ctx(ctx).Error().Err(err).Msg("failed to retrieve user")
			source.conn.Close()
			continue
		}

		new := NewSourceClient(
			source.ID,
			source.UserAgent,
			source.ContentType,
			source.MountName,
			source.conn,
			*user,
			source.Identifier,
			source.Metadata,
		)

		err = pm.AddSourceClient(new)
		if err != nil {
			zerolog.Ctx(ctx).Error().Err(err).Msg("")
			source.conn.Close()
			continue
		}
	}

	return nil
}
