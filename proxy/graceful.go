package proxy

import (
	"context"
	"encoding/json"

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
	store.AddListener(srv.listener, "proxy", state)
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
	store.AddConn(m.Conn.Load(), "mount", state)

	return m.storeSources(ctx, store)
}

func (m *Mount) storeSources(ctx context.Context, store *fdstore.Store) error {
	m.SourcesMu.Lock()
	defer m.SourcesMu.Unlock()

	for _, source := range m.Sources {
		err := source.Source.storeSelf(ctx, store)
		if err != nil {
			zerolog.Ctx(ctx).Error().Err(err).Msg("source store fail")
		}
	}
	return nil
}

type storedSource struct {
	ID          SourceID
	UserAgent   string
	ContentType string
	MountName   string
	Username    string
	Identifier  Identifier
	Metadata    *Metadata
}

func (sc *SourceClient) storeSelf(ctx context.Context, store *fdstore.Store) error {
	ss := storedSource{
		ID:          sc.ID,
		UserAgent:   sc.UserAgent,
		ContentType: sc.ContentType,
		MountName:   sc.MountName,
		Username:    sc.User.Username,
		Identifier:  sc.Identifier,
		Metadata:    sc.Metadata.Load(),
	}

	state, err := json.Marshal(ss)
	if err != nil {
		return err
	}

	return store.AddConn(sc.conn, "source", state)
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
		mount.pm = pm

		err := mount.restoreSelf(ctx, store, entry)
		if err != nil {
			// might still be able to recover other mounts so keep going
			zerolog.Ctx(ctx).Error().Err(err).Any("entry", entry).Msg("failed mount restore")
			entry.Conn.Close()
			continue
		}

		pm.mounts[mount.Name] = mount
	}

	pm.mountsMu.Unlock()
	return pm.restoreSources(ctx, pm.uss.User(ctx), store)
}

func (m *Mount) restoreSelf(ctx context.Context, store *fdstore.Store, entry fdstore.ConnEntry) error {
	err := json.Unmarshal(entry.Data, m)
	if err != nil {
		return err
	}

	m.Conn.Store(entry.Conn)
	return nil
}

func (pm *ProxyManager) restoreSources(ctx context.Context, us radio.UserStorage, store *fdstore.Store) error {
	sources, err := store.RemoveConn("source")
	if err != nil {
		return err
	}

	for _, entry := range sources {
		source := new(SourceClient)

		err := source.restoreSelf(ctx, us, entry)
		if err != nil {
			// might still be able to recover other sources if this fails
			zerolog.Ctx(ctx).Error().Err(err).Any("entry", entry).Msg("failed source restore")
			entry.Conn.Close()
			continue
		}

		err = pm.AddSourceClient(source)
		if err != nil {
			zerolog.Ctx(ctx).Error().Err(err).Msg("")
			entry.Conn.Close()
			continue
		}
	}

	return nil
}

func (sc *SourceClient) restoreSelf(ctx context.Context, us radio.UserStorage, entry fdstore.ConnEntry) error {
	var ss storedSource

	err := json.Unmarshal(entry.Data, &ss)
	if err != nil {
		return err
	}

	user, err := us.Get(ss.Username)
	if err != nil {
		return err
	}

	source := NewSourceClient(
		ss.ID,
		ss.UserAgent,
		ss.ContentType,
		ss.MountName,
		entry.Conn,
		*user,
		ss.Identifier,
		ss.Metadata,
	)
	*sc = *source
	return nil
}
