package proxy

import (
	"context"
	"io"
	"net"
	"net/url"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/R-a-dio/valkyrie/config"
	"github.com/cenkalti/backoff"
	"github.com/rs/zerolog"
)

type Mount struct {
	// ConnFn is a function we can call to get a new conn to
	// the icecast target server
	ConnFn func() (net.Conn, error)
	// Name of the mountpoint
	Name string
	// Conn is the conn to the icecast server
	Conn net.Conn

	// Sources is the different sources of audio data, the mount
	// broadcasts the data of the first entry and voids the others
	Sources   []*SourceClient
	SourcesMu *sync.RWMutex

	ActiveSource *atomic.Pointer[SourceClient]
}

func NewMount(ctx context.Context, uri *url.URL) *Mount {
	var bo backoff.BackOff = config.NewConnectionBackoff()
	bo = backoff.WithContext(bo, ctx)

	return &Mount{
		ConnFn: func() (net.Conn, error) {
			var err error
			var conn net.Conn
			err = backoff.Retry(func() error {
				conn, err = NewSourceConn(ctx, uri)
				return err
			}, bo)
			if err != nil {
				return nil, err
			}

			return conn, nil
		},
		Name:         uri.Path,
		SourcesMu:    new(sync.RWMutex),
		ActiveSource: new(atomic.Pointer[SourceClient]),
	}
}

func (m *Mount) Write(b []byte) (n int, err error) {
	if m.Conn == nil {
		m.Conn, err = m.ConnFn()
		if err != nil {
			return 0, err
		}
	}

	return m.Conn.Write(b)
}

func (m *Mount) Close() error {
	if m.Conn != nil {
		return m.Conn.Close()
	}
	return nil
}

func (m *Mount) SendMetadata(ctx context.Context, meta *Metadata) {
	m.SourcesMu.RLock()
	// see if we have a source associated with this metadata
	for _, source := range m.Sources {
		if source.Identifier != meta.Identifier {
			continue
		}

		source.Metadata.Store(meta)
	}
	m.SourcesMu.RUnlock()
}

func (m *Mount) AddSource(ctx context.Context, source *SourceClient) {
	m.SourcesMu.Lock()
	m.Sources = append(m.Sources, source)
	m.SourcesMu.Unlock()
	// swap the source in as active client if there isn't already one active
	m.ActiveSource.CompareAndSwap(nil, source)
}

func (m *Mount) RemoveSource(ctx context.Context, toRemove *SourceClient) {
	m.SourcesMu.Lock()
	m.Sources = slices.DeleteFunc(m.Sources, func(source *SourceClient) bool {
		// match on their remote addr instead of the identifier because we technically
		// allow someone to connect twice and they would get identical identifiers
		return source.conn.RemoteAddr() == toRemove.conn.RemoteAddr()
	})
	var next *SourceClient
	if len(m.Sources) != 0 {
		next = m.Sources[0]
	}
	m.SourcesMu.Unlock()

	active := m.ActiveSource.Load()
	if active.conn.RemoteAddr() != toRemove.conn.RemoteAddr() {
		// the client we're removing isn't the active one so just return
		return
	}

	// client was the active one, swap active with the next source we grabbed earlier
	swapped := m.ActiveSource.CompareAndSwap(active, next)
	if swapped {
		// happy path we swapped and can return
		return
	}

	// we tried to swap in the next one but the active source got swapped under our nose
	// so we need to enter a slow path where we hold the SourcesMu lock until we get in
	m.SourcesMu.RLock()
	defer m.SourcesMu.RUnlock()

	for !swapped {
		var next *SourceClient
		if len(m.Sources) != 0 {
			next = m.Sources[0]
		}
		active = m.ActiveSource.Load()

		swapped = m.ActiveSource.CompareAndSwap(active, next)
	}
}

func (m *Mount) AddSourceLockless(ctx context.Context, source *SourceClient) {
	msc := &MountSourceClient{
		SourceClient: source,
		Into:         new(atomic.Pointer[MetaWriter]),
	}
	go msc.run(ctx)
}

func (m *Mount) runSourceClient(ctx context.Context, source *SourceClient) {

}

type MetaWriter interface {
	io.Writer
	SendMetadata(ctx context.Context, metadata *Metadata)
}

type MountSourceClient struct {
	*SourceClient
	Into *atomic.Pointer[MetaWriter]
}

func (msc *MountSourceClient) run(ctx context.Context) {
	const BUFFER_SIZE = 4096
	buf := make([]byte, BUFFER_SIZE)
	timeout := time.Second * 5
	logger := zerolog.Ctx(ctx).With().
		Str("address", msc.conn.RemoteAddr().String()).
		Str("mount", msc.MountName).
		Str("username", msc.User.Username).
		Logger()
	lastMetadata := time.Time{}

	for {
		// set a deadline so we don't keep bad clients around
		err := msc.conn.SetReadDeadline(time.Now().Add(timeout))
		if err != nil {
			// deadline failed to be set, not much we can do but log it and continue
			logger.Info().Msg("failed to set deadline")
		}
		// read some data from the source
		readn, err := msc.bufrw.Read(buf)
		if err != nil {
			logger.Error().Err(err).Msg("failed to read data")
			return
		}

		// see where we need to send it to
		sender := msc.Into.Load()
		if sender == nil {
			// nowhere to write to so just continue reading
			continue
		}

		writen, err := (*sender).Write(buf[:readn])
		if err != nil {
			logger.Error().Err(err).Msg("failed to write data")
			return
		}
		if readn != writen {
			// we didn't actually send all the data, there isn't much we can really do
			// here, but this is most likely a network failure and we will be exiting soon
			logger.Info().Msg("failed to write all data")
		}

		// then see if we have new metadata to send
		meta := msc.Metadata.Load()
		if meta != nil && meta.Time.After(lastMetadata) {
			(*sender).SendMetadata(ctx, meta)
		}
	}
}
