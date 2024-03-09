package proxy

import (
	"cmp"
	"context"
	"io"
	"net"
	"slices"
	"sync"
	"time"

	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/streamer/icecast"
	"github.com/R-a-dio/valkyrie/util"
	"github.com/cenkalti/backoff"
	"github.com/rs/zerolog"
)

type Mount struct {
	logger zerolog.Logger
	// ContentType of this mount, this can only be set during creation and all
	// future clients afterwards will use the same content type
	ContentType string
	// MetadataFn is a function we can call to send metadata to
	// the icecast target server
	MetadataFn func(context.Context, string) error
	// Name of the mountpoint
	Name string

	// ConnFn is a function we can call to get a new conn to
	// the icecast target server
	ConnFn func() (net.Conn, error)
	// Conn is the conn to the icecast server
	Conn util.TypedValue[net.Conn]

	// Sources is the different sources of audio data, the mount
	// broadcasts the data of the first entry and voids the others
	Sources   []*MountSourceClient
	SourcesMu *sync.RWMutex
}

func NewMount(ctx context.Context, name string, urlFn MountURLFn, ct string) *Mount {
	var bo backoff.BackOff = config.NewConnectionBackoff()
	bo = backoff.WithContext(bo, ctx)
	logger := zerolog.Ctx(ctx).With().Str("mount", name).Logger()

	return &Mount{
		logger: logger,
		ConnFn: func() (net.Conn, error) {
			var err error
			var conn net.Conn
			err = backoff.Retry(func() error {
				uri, err := urlFn()
				if err != nil {
					logger.Error().Err(err).Msg("failed to parse master server url")
					return err
				}

				logger.Info().Msg("dialing icecast")
				conn, err = icecast.DialURL(ctx, uri, icecast.ContentType(ct))
				if err != nil {
					logger.Error().Err(err).Msg("failed connected to master server")
					return err
				}
				return nil
			}, bo)
			if err != nil {
				return nil, err
			}

			return conn, nil
		},
		MetadataFn: func(ctx context.Context, s string) error {
			uri, err := urlFn()
			if err != nil {
				return err
			}
			return icecast.MetadataURL(uri)(ctx, s)
		},
		Name:      name,
		SourcesMu: new(sync.RWMutex),
	}
}

func (m *Mount) Write(b []byte) (n int, err error) {
	conn := m.Conn.Load()
retry:
	if conn == nil {
		conn, err = m.ConnFn()
		if err != nil {
			return 0, err
		}
		m.Conn.Store(conn)
	}

	n, err = conn.Write(b)
	if err != nil {
		m.logger.Error().Err(err).Msg("failed to write to master")
		// reset our connection
		conn.Close()
		conn = nil
		goto retry
	}
	return n, err
}

func (m *Mount) Close() error {
	conn := m.Conn.Load()
	if conn != nil {
		return conn.Close()
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
		return cmp.Compare(a.Priority, b.Priority)
	})

	return least.Priority + 1
}

// mostPriority returns the source with the most priority
// (the lowest .priority value in the sources given). Returns nil if
// sources is empty.
func mostPriority(sources []*MountSourceClient) *MountSourceClient {
	if len(sources) == 0 {
		return nil
	}
	return slices.MinFunc(sources, func(a, b *MountSourceClient) int {
		return cmp.Compare(a.Priority, b.Priority)
	})
}

// MountSourceClient is a SourceClient with extra fields for mount-specific
// bookkeeping
type MountSourceClient struct {
	// Source is the SourceClient we're handling, should not be mutated by
	// anything once the MountSourceClient is made
	Source *SourceClient
	// Priority is the Priority for live-ness determination
	// lower is higher Priority
	Priority uint
	// Live is an indicator of this being the currently Live source
	Live bool
	// MW is the writer this source is writing to
	MW *MountMetadataWriter

	logger zerolog.Logger
}

func (msc *MountSourceClient) GoLive(ctx context.Context, out MetadataWriter) {
	msc.Live = true
	msc.MW.SetWriter(out)
	msc.MW.SetLive(ctx, true)
	msc.logger.Info().Msg("switching to live")
}

// SendMetadata finds the source associated with this metadata and updates
// their internal metadata. This does no transmission of metadata to the
// master server.
func (m *Mount) SendMetadata(ctx context.Context, meta *Metadata) {
	m.SourcesMu.RLock()
	// see if we have a source associated with this metadata
	for _, msc := range m.Sources {
		if msc.Source.Identifier != meta.Identifier {
			continue
		}

		msc.Source.Metadata.Store(meta)
	}
	m.SourcesMu.RUnlock()
}

func (m *Mount) AddSource(ctx context.Context, source *SourceClient) {
	mw := &MountMetadataWriter{
		mount: m,
	}

	msc := &MountSourceClient{
		Source:   source,
		Priority: 0,
		Live:     false,
		MW:       mw,
		logger: zerolog.Ctx(ctx).With().
			Str("address", source.conn.RemoteAddr().String()).
			Str("mount", source.MountName).
			Str("username", source.User.Username).
			Logger(),
	}
	m.SourcesMu.Lock()
	defer m.SourcesMu.Unlock()

	// new sources always get assigned the least priority
	msc.Priority = leastPriority(m.Sources)
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
		if msc.Source.ID != id {
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
	if removed.Live {
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
	defer m.RemoveSource(ctx, msc.Source.ID)
	// and close our connection
	defer msc.Source.conn.Close()

	buf := make([]byte, BUFFER_SIZE)
	// timeout before we cancel reading from the source
	timeout := time.Second * 20

	// the last time we send metadata
	lastMetadata := time.Time{}

	for {
		// set a deadline so we don't keep bad clients around
		err := msc.Source.conn.SetReadDeadline(time.Now().Add(timeout))
		if err != nil {
			// deadline failed to be set, not much we can do but log it and continue
			msc.logger.Info().Msg("failed to set deadline")
		}
		// read some data from the source
		readn, err := msc.Source.bufrw.Read(buf)
		if err != nil {
			if errors.IsE(err, io.EOF) {
				// client left us, exit cleanly
				return
			}
			msc.logger.Error().Err(err).Msg("failed to read data")
			return
		}

		writen, err := msc.MW.Write(buf[:readn])
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
		meta := msc.Source.Metadata.Load()
		if meta != nil && meta.Time.After(lastMetadata) {
			msc.MW.SendMetadata(ctx, meta)
			lastMetadata = time.Now()
		}
	}
}

type MountMetadataWriter struct {
	mu sync.RWMutex
	// metadata is the last metadata we send (or tried to send)
	Metadata string
	// mount is the Mount that we are associated with
	mount *Mount
	// live indicates if we are the live writer, actually writing to the master
	Live bool
	// out is the writer we write into
	Out io.Writer
}

func (mmw *MountMetadataWriter) SendMetadata(ctx context.Context, meta *Metadata) {
	mmw.mu.Lock()
	mmw.Metadata = meta.Value
	mmw.mu.Unlock()

	mmw.sendMetadata(ctx)
}

func (mmw *MountMetadataWriter) sendMetadata(ctx context.Context) {
	mmw.mu.RLock()
	defer mmw.mu.RUnlock()

	// check if we're live
	if !mmw.Live {
		zerolog.Ctx(ctx).Info().Str("metadata", mmw.Metadata).Msg("skipping metadata, we're not live")
		return
	}

	zerolog.Ctx(ctx).Info().Str("metadata", mmw.Metadata).Msg("sending metadata")
	err := mmw.mount.MetadataFn(ctx, mmw.Metadata)
	if err != nil {
		zerolog.Ctx(ctx).Error().Err(err).Str("metadata", mmw.Metadata).Msg("failed sending metadata")
	}
}

func (mmw *MountMetadataWriter) Write(p []byte) (n int, err error) {
	mmw.mu.RLock()
	defer mmw.mu.RUnlock()

	if mmw.Out == nil {
		// nowhere to go with this data, just silently eat it
		return len(p), nil
	}

	return mmw.Out.Write(p)
}

func (mmw *MountMetadataWriter) SetWriter(new io.Writer) {
	mmw.mu.Lock()
	mmw.Out = new
	mmw.mu.Unlock()
}

func (mmw *MountMetadataWriter) SetLive(ctx context.Context, live bool) {
	mmw.mu.Lock()
	mmw.Live = live
	mmw.mu.Unlock()
	if live {
		mmw.sendMetadata(ctx)
	}
}
