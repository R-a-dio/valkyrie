package proxy

import (
	"cmp"
	"context"
	"io"
	"net"
	"net/url"
	"slices"
	"sync"
	"time"

	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/streamer/icecast"
	"github.com/cenkalti/backoff"
	"github.com/rs/zerolog"
)

type Mount struct {
	// ContentType of this mount, this can only be set during creation and all
	// future clients afterwards will use the same content type
	ContentType string
	// ConnFn is a function we can call to get a new conn to
	// the icecast target server
	ConnFn func() (net.Conn, error)
	// MetadataFn is a function we can call to send metadata to
	// the icecast target server
	MetadataFn func(context.Context, string) error
	// Name of the mountpoint
	Name string
	// Conn is the conn to the icecast server
	Conn net.Conn

	// Sources is the different sources of audio data, the mount
	// broadcasts the data of the first entry and voids the others
	Sources   []*MountSourceClient
	SourcesMu *sync.RWMutex
}

func NewMount(ctx context.Context, uri *url.URL, ct string) *Mount {
	var bo backoff.BackOff = config.NewConnectionBackoff()
	bo = backoff.WithContext(bo, ctx)

	return &Mount{
		ConnFn: func() (net.Conn, error) {
			var err error
			var conn net.Conn
			err = backoff.RetryNotify(func() error {
				conn, err = icecast.DialURL(ctx, uri, icecast.ContentType(ct))
				return err
			}, bo, func(err error, d time.Duration) {
				zerolog.Ctx(ctx).Error().Err(err).Dur("backoff", d).Msg("failed connecting to master server")
			})
			if err != nil {
				return nil, err
			}

			return conn, nil
		},
		MetadataFn: icecast.MetadataURL(uri),
		Name:       uri.Path,
		SourcesMu:  new(sync.RWMutex),
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

// leastPriority returns the priority index that would put
// you at the lowest priority for next source consideration
func leastPriority(sources []*MountSourceClient) uint {
	if len(sources) == 0 {
		return 0
	}

	least := slices.MaxFunc(sources, func(a, b *MountSourceClient) int {
		return cmp.Compare(a.priority, b.priority)
	})

	return least.priority + 1
}

// mostPriority returns the source with the most priority
// (the lowest .priority value in the sources given). Returns nil if
// sources is empty.
func mostPriority(sources []*MountSourceClient) *MountSourceClient {
	if len(sources) == 0 {
		return nil
	}
	return slices.MinFunc(sources, func(a, b *MountSourceClient) int {
		return cmp.Compare(a.priority, b.priority)
	})
}

// MountSourceClient is a SourceClient with extra fields for mount-specific
// bookkeeping
type MountSourceClient struct {
	// source is the SourceClient we're handling, should not be mutated by
	// anything once the MountSourceClient is made
	source *SourceClient
	// priority is the priority for live-ness determination
	// lower is higher priority
	priority uint
	// live is an indicator of this being the currently live source
	live bool
	// mw is the writer this source is writing to
	mw *MountMetadataWriter

	logger zerolog.Logger
}

func (msc *MountSourceClient) GoLive(ctx context.Context, out MetadataWriter) {
	msc.live = true
	msc.mw.SetWriter(out)
	msc.mw.SetLive(true)
	msc.logger.Info().Msg("switching to live")
}

// SendMetadata finds the source associated with this metadata and updates
// their internal metadata. This does no transmission of metadata to the
// master server.
func (m *Mount) SendMetadata(ctx context.Context, meta *Metadata) {
	m.SourcesMu.RLock()
	// see if we have a source associated with this metadata
	for _, msc := range m.Sources {
		if msc.source.Identifier != meta.Identifier {
			continue
		}

		msc.source.Metadata.Store(meta)
	}
	m.SourcesMu.RUnlock()
}

func (m *Mount) AddSource(ctx context.Context, source *SourceClient) {
	mw := &MountMetadataWriter{
		mount: m,
	}

	msc := &MountSourceClient{
		source:   source,
		priority: 0,
		live:     false,
		mw:       mw,
		logger: zerolog.Ctx(ctx).With().
			Str("address", source.conn.RemoteAddr().String()).
			Str("mount", source.MountName).
			Str("username", source.User.Username).
			Logger(),
	}
	m.SourcesMu.Lock()
	defer m.SourcesMu.Unlock()

	// new sources always get assigned the least priority
	msc.priority = leastPriority(m.Sources)
	m.Sources = append(m.Sources, msc)
	go m.RunMountSourceClient(ctx, msc)
	if len(m.Sources) == 1 {
		msc.GoLive(ctx, m)
	}
}

func (m *Mount) RemoveSource(ctx context.Context, id SourceID) {
	m.SourcesMu.Lock()
	defer m.SourcesMu.Unlock()

	var removed *MountSourceClient

	m.Sources = slices.DeleteFunc(m.Sources, func(msc *MountSourceClient) bool {
		if msc.source.ID != id {
			return false
		}
		removed = msc
		return true
	})

	if removed == nil {
		// didn't remove anything
		return
	}

	removed.logger.Info().Msg("removing source client")

	// see if the source we removed is the live source
	if removed.live {
		// and swap to another source if possible
		m.liveSourceSwap(ctx)
	}
}

// liveSourceSwap moves the live-ness flag to the highest priority source
//
// liveSourceSwap should only be called with m.SourcesMu held in a write lock
func (m *Mount) liveSourceSwap(ctx context.Context) {
	next := mostPriority(m.Sources)
	if next != nil {
		next.GoLive(ctx, m)
	}
}

type MetadataWriter interface {
	io.Writer
	SendMetadata(ctx context.Context, metadata *Metadata)
}

func (m *Mount) RunMountSourceClient(ctx context.Context, msc *MountSourceClient) {
	const BUFFER_SIZE = 4096
	// remove ourselves from the mount if we exit
	defer m.RemoveSource(ctx, msc.source.ID)
	// and close our connection
	defer msc.source.conn.Close()

	buf := make([]byte, BUFFER_SIZE)
	// timeout before we cancel reading from the source
	timeout := time.Second * 20

	// the last time we send metadata
	lastMetadata := time.Time{}

	for {
		// set a deadline so we don't keep bad clients around
		err := msc.source.conn.SetReadDeadline(time.Now().Add(timeout))
		if err != nil {
			// deadline failed to be set, not much we can do but log it and continue
			msc.logger.Info().Msg("failed to set deadline")
		}
		// read some data from the source
		readn, err := msc.source.bufrw.Read(buf)
		if err != nil {
			if errors.IsE(err, io.EOF) {
				// client left us, exit cleanly
				return
			}
			msc.logger.Error().Err(err).Msg("failed to read data")
			return
		}

		writen, err := msc.mw.Write(buf[:readn])
		if err != nil {
			msc.logger.Error().Err(err).Msg("failed to write data")
			return
		}
		if readn != writen {
			// we didn't actually send all the data, there isn't much we can really do
			// here, but this is most likely a network failure and we will be exiting soon
			msc.logger.Info().Msg("failed to write all data")
		}

		// then see if we have new metadata to send
		meta := msc.source.Metadata.Load()
		if meta != nil && meta.Time.After(lastMetadata) {
			msc.mw.SendMetadata(ctx, meta)
			lastMetadata = time.Now()
		}
	}
}

type MountMetadataWriter struct {
	mu    sync.RWMutex
	mount *Mount
	live  bool
	out   io.Writer
}

func (mmw *MountMetadataWriter) SendMetadata(ctx context.Context, meta *Metadata) {
	mmw.mu.RLock()
	defer mmw.mu.RUnlock()

	// check if we're live
	if !mmw.live {
		zerolog.Ctx(ctx).Info().Str("metadata", meta.Value).Msg("skipping metadata, we're not live")
		return
	}

	zerolog.Ctx(ctx).Info().Str("metadata", meta.Value).Msg("sending metadata")
	err := mmw.mount.MetadataFn(ctx, meta.Value)
	if err != nil {
		zerolog.Ctx(ctx).Error().Err(err).Str("metadata", meta.Value).Msg("failed sending metadata")
	}
}

func (mmw *MountMetadataWriter) Write(p []byte) (n int, err error) {
	mmw.mu.RLock()
	defer mmw.mu.RUnlock()

	if mmw.out == nil {
		// nowhere to go with this data, just silently eat it
		return len(p), nil
	}

	return mmw.out.Write(p)
}

func (mmw *MountMetadataWriter) SetWriter(new io.Writer) {
	mmw.mu.Lock()
	mmw.out = new
	mmw.mu.Unlock()
}

func (mmw *MountMetadataWriter) SetLive(live bool) {
	mmw.mu.Lock()
	mmw.live = live
	mmw.mu.Unlock()
}
