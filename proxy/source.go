package proxy

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/website/middleware"
	"github.com/rs/xid"
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

	// now depending on what protocol the request was made with, it expects some extra
	// data to tell the client we're "done" sending anything
	if r.ProtoMajor == 1 && r.ProtoMinor == 0 {
		// HTTP/1.0 some clients expect an extra newline
		io.WriteString(conn, "\r\n")
	}
	if r.ProtoMajor == 1 && r.ProtoMinor == 1 {
		// HTTP/1.1 is chunked encoding and we need to send the end stream chunked chunk
		io.WriteString(conn, "0\r\n\r\n")
	}

	// reset any deadlines that were on the net.Conn, these will be reapplied later
	// by the function reading from it
	err = conn.SetDeadline(time.Time{})
	if err != nil {
		hlog.FromRequest(r).Error().Err(err).Msg("failed to set deadline")
		return
	}

	client := SourceClient{
		ID:          NewSourceID(r),
		UserAgent:   r.Header.Get("User-Agent"),
		ContentType: r.Header.Get("Content-Type"),
		conn:        conn,
		bufrw:       bufrw,
		MountName:   mountName,
		User:        *user,
		Identifier:  identifier,
		Metadata:    new(atomic.Pointer[Metadata]),
	}

	err = s.proxy.AddSourceClient(ctx, &client)
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

type SourceClient struct {
	ID SourceID
	// UserAgent is the User-Agent HTTP header passed by the client
	UserAgent string
	// ContentType is the Content-Type HTTP header passed by the client
	ContentType string
	// conn is the connection for this client, it can be a *compat.Conn
	conn net.Conn
	// bufrw is the bufio buffer we got back from net/http
	bufrw *bufio.ReadWriter
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

// StripBuffer returns an io.Reader that returns io.EOF after all buffered
// content in r is read.
func StripBuffer(r *bufio.Reader) io.Reader {
	return &stripper{r}
}

type stripper struct {
	r *bufio.Reader
}

func (s *stripper) Read(p []byte) (n int, err error) {
	bufN := s.r.Buffered()
	if bufN == 0 {
		return 0, io.EOF
	}
	if bufN < len(p) {
		p = p[:bufN]
	}

	return s.r.Read(p)
}
