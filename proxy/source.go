package proxy

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/proxy/compat"
	"github.com/R-a-dio/valkyrie/util/graceful"
	"github.com/R-a-dio/valkyrie/website/middleware"
	"github.com/rs/xid"
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
		hlog.FromRequest(r).Error().Msg("failed to get an identifier")
		return
	}

	user := middleware.UserFromContext(ctx)
	if user == nil {
		hlog.FromRequest(r).Error().Msg("failed to get an user")
		return
	}

	mountName := GetMountpoint(r)

	// get ready to hijack and proceed with data handling
	rc := http.NewResponseController(w)

	// set a response back that we're OK because most clients wait until
	// they get the header back before sending data
	w.WriteHeader(http.StatusOK)

	if err := rc.Flush(); err != nil {
		hlog.FromRequest(r).Error().Err(err).Msg("failed to flush OK header")
		return
	}

	// hijack the connection since we're now gonna be reading directly from conn
	conn, bufrw, err := rc.Hijack()
	if err != nil {
		hlog.FromRequest(r).Error().Err(err).Msg("failed to hijack source request")
		return
	}
	if err := bufrw.Flush(); err != nil {
		hlog.FromRequest(r).Error().Err(err).Msg("failed to flush bufrw")
		return
	}

	// now depending on what protocol the request was made with, it expects some extra
	// data to tell the client we're "done" sending anything
	if r.ProtoMajor == 1 && r.ProtoMinor == 0 {
		// HTTP/1.0 some clients expect an extra newline
		_, err = io.WriteString(conn, "\r\n")
		if err != nil {
			hlog.FromRequest(r).Error().Err(err).Msg("failed writing end of http request")
		}
	}
	if r.ProtoMajor == 1 && r.ProtoMinor == 1 {
		// HTTP/1.1 is chunked encoding and we need to send the end stream chunked chunk
		_, err = io.WriteString(conn, "0\r\n\r\n")
		if err != nil {
			hlog.FromRequest(r).Error().Err(err).Msg("failed writing end of http request")
		}
	}

	// reset any deadlines that were on the net.Conn, these will be reapplied later
	// by the function reading from it
	err = conn.SetDeadline(time.Time{})
	if err != nil {
		hlog.FromRequest(r).Error().Err(err).Msg("failed to set deadline")
		return
	}

	// TODO: clean this up
	conn = compat.DrainBuffer(bufrw, conn)

	client := NewSourceClient(
		NewSourceID(r),
		r.Header.Get("User-Agent"),
		r.Header.Get("Content-Type"),
		mountName,
		conn,
		*user,
		identifier,
		nil,
	)

	err = s.proxy.AddSourceClient(client)
	if err != nil {
		hlog.FromRequest(r).Error().Err(err).Msg("failed to add source client to proxy")
		return
	}
}

func NewSourceID(r *http.Request) SourceID {
	if id, ok := hlog.IDFromRequest(r); ok {
		return SourceID{id}
	}
	panic("NewSourceID called without hlog.RequestIDHandler middleware")
}

type SourceID struct {
	xid.ID
}

func NewSourceClient(id SourceID, ua, ct, mount string, conn net.Conn, user radio.User, identifier Identifier, metadata *Metadata) *SourceClient {
	meta := new(atomic.Pointer[Metadata])
	if metadata != nil {
		meta.Store(metadata)
	}

	return &SourceClient{
		ID:          id,
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
	ID SourceID
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

type wireSource struct {
	ID          SourceID
	UserAgent   string
	ContentType string
	MountName   string
	UserID      radio.UserID
	Identifier  Identifier
	Metadata    *Metadata
}

func (sc *SourceClient) writeSelf(dst *net.UnixConn) error {
	ws := wireSource{
		ID:          sc.ID,
		UserAgent:   sc.UserAgent,
		ContentType: sc.ContentType,
		MountName:   sc.MountName,
		UserID:      sc.User.ID,
		Identifier:  sc.Identifier,
		Metadata:    sc.Metadata.Load(),
	}

	fd, err := getFile(sc.conn)
	if err != nil {
		return fmt.Errorf("fd failure in source client: %w", err)
	}
	defer fd.Close()

	err = graceful.WriteJSONFile(dst, ws, fd)
	if err != nil {
		return err
	}

	return nil
}

func (sc *SourceClient) readSelf(ctx context.Context, cfg config.Config, src *net.UnixConn) error {
	var ws wireSource

	zerolog.Ctx(ctx).Info().Msg("resume: reading source client")
	file, err := graceful.ReadJSONFile(src, &ws)
	if err != nil {
		return err
	}
	defer file.Close()

	zerolog.Ctx(ctx).Info().Any("ws", ws).Msg("resume")

	conn, err := net.FileConn(file)
	if err != nil {
		return err
	}

	// TODO: implement this
	var user radio.User
	_ = user

	new := NewSourceClient(
		ws.ID,
		ws.UserAgent,
		ws.ContentType,
		ws.MountName,
		conn,
		user,
		ws.Identifier,
		ws.Metadata,
	)
	*sc = *new
	return nil
}
