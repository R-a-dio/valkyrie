package proxy

import (
	"context"
	"io"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/proxy/compat"
	"github.com/R-a-dio/valkyrie/util"
	"github.com/R-a-dio/valkyrie/website/middleware"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/hlog"
)

func (s *Server) PutSource(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut && r.Method != "SOURCE" {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()

	identifier := IdentFromRequest(r)
	if identifier == 0 {
		hlog.FromRequest(r).Error().Ctx(ctx).Msg("failed to get an identifier")
		return
	}

	user := middleware.UserFromContext(ctx)
	if !user.IsValid() {
		hlog.FromRequest(r).Error().Ctx(ctx).Msg("failed to get an user")
		return
	}

	// check if the user has recently been kicked and has a timeout
	if !s.proxy.CheckAllowedToConnect(user.ID) {
		hlog.FromRequest(r).Error().Ctx(ctx).Str("username", user.Username).Any("user_id", user.ID).Msg("user is on timeout")
		time.Sleep(time.Second)
		return
	}

	mountName := GetMountpoint(r)

	// get ready to hijack and proceed with data handling
	rc := http.NewResponseController(w)

	// set a response back that we're OK because most clients wait until
	// they get the header back before sending data
	w.WriteHeader(http.StatusOK)

	if err := rc.Flush(); err != nil {
		hlog.FromRequest(r).Error().Ctx(ctx).Err(err).Msg("failed to flush OK header")
		return
	}

	// hijack the connection since we're now gonna be reading directly from conn
	conn, bufrw, err := rc.Hijack()
	if err != nil {
		hlog.FromRequest(r).Error().Ctx(ctx).Err(err).Msg("failed to hijack source request")
		return
	}
	if err := bufrw.Flush(); err != nil {
		hlog.FromRequest(r).Error().Ctx(ctx).Err(err).Msg("failed to flush bufrw")
		return
	}

	// now depending on what protocol the request was made with, it expects some extra
	// data to tell the client we're "done" sending anything
	if r.ProtoMajor == 1 && r.ProtoMinor == 0 {
		// HTTP/1.0 some clients expect an extra newline
		_, err = io.WriteString(conn, "\r\n")
		if err != nil {
			hlog.FromRequest(r).Error().Ctx(ctx).Err(err).Msg("failed writing end of http request")
		}
	}
	if r.ProtoMajor == 1 && r.ProtoMinor == 1 {
		// HTTP/1.1 is chunked encoding and we need to send the end stream chunked chunk
		_, err = io.WriteString(conn, "0\r\n\r\n")
		if err != nil {
			hlog.FromRequest(r).Error().Ctx(ctx).Err(err).Msg("failed writing end of http request")
		}
	}

	// reset any deadlines that were on the net.Conn, these will be reapplied later
	// by the function reading from it
	err = conn.SetDeadline(time.Time{})
	if err != nil {
		hlog.FromRequest(r).Error().Ctx(ctx).Err(err).Msg("failed to set deadline")
		return
	}

	// drain the bufio buffer we got from net/http, we want to use the raw conn
	conn = compat.DrainBuffer(bufrw, conn)

	client := NewSourceClient(
		NewSourceID(r, user.ID),
		r.Header.Get("User-Agent"),
		r.Header.Get("Content-Type"),
		mountName,
		conn,
		user,
		identifier,
		nil,
	)

	err = s.proxy.AddSourceClient(client)
	if err != nil {
		hlog.FromRequest(r).Error().Ctx(ctx).Err(err).Msg("failed to add source client to proxy")
		return
	}
}

func NewSourceID(r *http.Request, uid radio.UserID) radio.SourceID {
	if id, ok := hlog.IDFromRequest(r); ok {
		return radio.SourceID{ID: id, UserID: uid}
	}
	panic("NewSourceID called without hlog.RequestIDHandler middleware")
}

func NewSourceClient(id radio.SourceID, ua, ct, mount string, conn net.Conn, user radio.User, identifier Identifier, metadata *Metadata) *SourceClient {
	meta := new(atomic.Pointer[Metadata])
	if metadata != nil {
		meta.Store(metadata)
	}

	return &SourceClient{
		ID:          id,
		Start:       time.Now(),
		UserAgent:   ua,
		ContentType: ct,
		MountName:   mount,
		User:        user,
		Identifier:  identifier,
		conn:        conn,
		Metadata:    meta,
	}
}

type SourceClient struct {
	ID radio.SourceID
	// Start is the time the client connected at
	Start time.Time
	// UserAgent is the User-Agent HTTP header passed by the client
	UserAgent string
	// ContentType is the Content-Type HTTP header passed by the client
	ContentType string
	// conn is the connection for this client, it can be a *compat.Conn
	conn net.Conn
	// MountName is the mount this client is trying to stream to
	MountName string
	// User is the user that is trying to stream
	User radio.User
	// Identifier is an identifier that should be the same between two
	// different requests, but same mountpoint and user. This is to match-up
	// metadata information
	Identifier Identifier
	// Metadata is a pointer to the last Metadata received for this client
	Metadata *atomic.Pointer[Metadata]
}

// MountSourceClient is a SourceClient with extra fields for mount-specific
// bookkeeping
type MountSourceClient struct {
	// Source is the SourceClient we're handling, should not be mutated by
	// anything once the MountSourceClient is made
	Source *SourceClient
	// Mount is the Mount we're attached to
	Mount *Mount
	// Priority is the Priority for live-ness determination
	// lower is higher Priority
	Priority uint32

	live atomic.Bool
	out  util.TypedValue[io.Writer]

	logger zerolog.Logger
}

type MountInterface interface {
	RemoveSource(context.Context, radio.SourceID)
	SendMetadata(ctx context.Context, metadata string) error
}

func (msc *MountSourceClient) GoLive(ctx context.Context, out io.Writer) {
	msc.live.Store(true)
	msc.out.Store(out)
	msc.logger.Info().
		Str("req_id", msc.Source.ID.String()).
		Any("identifier", msc.Source.Identifier).
		Msg("switching to live")
}

func (msc *MountSourceClient) GetLive() bool {
	return msc.live.Load()
}

func (msc *MountSourceClient) runReadLoop(ctx context.Context) {
	const BUFFER_SIZE = 4096
	// remove ourselves from the mount if we exit
	defer msc.Mount.RemoveSource(ctx, msc.Source.ID)
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
			msc.logger.Info().Ctx(ctx).Msg("failed to set deadline")
		}
		// read some data from the source
		readn, err := msc.Source.conn.Read(buf)
		if err != nil {
			if errors.IsE(err, io.EOF) {
				// client left us, exit cleanly
				return
			}
			msc.logger.Error().Ctx(ctx).Err(err).Msg("failed to read data")
			return
		}

		// check if we have an output to write to, do nothing with the data if there isn't one
		if out := msc.out.Load(); out != nil {
			writen, err := out.Write(buf[:readn])
			if err != nil {
				msc.logger.Error().Ctx(ctx).Err(err).Msg("failed to write data")
				return
			}
			if readn != writen {
				// we didn't actually send all the data, there isn't much we can really do
				// here, but this is most likely a network failure and we will be exiting soon
				msc.logger.Info().Ctx(ctx).Msg("failed to write all data")
			}
		}

		// then see if we have new metadata to send
		meta := msc.Source.Metadata.Load()
		if meta != nil && meta.Time.After(lastMetadata) {
			if msc.live.Load() {
				msc.Mount.SendMetadata(ctx, meta.Value)
				lastMetadata = time.Now()
			} else {
				msc.logger.Info().Ctx(ctx).Str("metadata", meta.Value).Msg("skipping metadata, we're not live")
			}
		}
	}
}
