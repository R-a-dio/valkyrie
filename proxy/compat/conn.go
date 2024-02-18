package compat

import (
	"bytes"
	"io"
	"net"
	"reflect"

	"github.com/rs/zerolog"
)

type Listener struct {
	logger *zerolog.Logger
	net.Listener
}

const maxReadSize = 1024

var newLine = []byte{'\n'}
var iceLine = []byte("ICE/1.0")
var httpLine = []byte("HTTP/1.0")

var _ net.Listener = new(Listener)
var _ net.Conn = new(Conn)

// Listen returns a *compat.Listener wrapping the listener returned
// by a call to net.Listen(network, address)
func Listen(logger *zerolog.Logger, network, address string) (net.Listener, error) {
	l, err := net.Listen(network, address)
	if err != nil {
		return nil, err
	}
	return &Listener{logger, l}, nil
}

// Accept accepts the next connection but returns a net.Conn that has been
// wrapped to scan for ICE/1.0 in the HTTP request line and replaces it
// with a HTTP/1.0 instance.
func (l *Listener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}

	return &Conn{Conn: conn, logger: l.logger}, nil
}

func NewConn(r io.Reader, conn net.Conn) *Conn {
	return &Conn{
		r:    r,
		Conn: conn,
	}
}

type Conn struct {
	r io.Reader
	net.Conn
	logger *zerolog.Logger
}

func (c *Conn) Read(b []byte) (n int, err error) {
	// if we already have a multireader present use that
	if c.r != nil {
		return c.r.Read(b)
	}

	n, err = c.Conn.Read(b)
	if err != nil {
		return n, err
	}

	// fast path, we already read more than our maximum allowed
	// or have a newline already, do our replacement and continue
	old := b[:n]
	if n > maxReadSize || bytes.Contains(old, newLine) {
		new := bytes.Replace(old, iceLine, httpLine, 1)
		if len(new) > len(old) && c.logger != nil {
			c.logger.Info().Str("address", c.RemoteAddr().String()).Msg("ICE/1.0")
		}
		c.r = io.MultiReader(bytes.NewReader(new), c.Conn)
	}

	return c.r.Read(b)
}

func (c *Conn) Close() error {
	return c.Conn.Close()
}

func ToNetConn(conn net.Conn) net.Conn {
	if c, ok := conn.(*Conn); ok && c.CanUseConn() {
		return c.Conn
	}
	return conn
}

func (c *Conn) CanUseConn() bool {
	return isSingleReader(c.r)
}

func isSingleReader(r io.Reader) bool {
	v := reflect.Indirect(reflect.ValueOf(r))
	if !v.IsValid() || v.NumField() != 1 {
		return false
	}
	fv := v.Field(0)
	if fv.Type().Kind() != reflect.Slice {
		return false
	}
	return fv.Len() == 1
}
