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

type Conn struct {
	net.Conn
	multi io.Reader

	logger *zerolog.Logger
}

func (c *Conn) Read(b []byte) (n int, err error) {
	// if we already have a multireader present use that
	if c.multi != nil {
		return c.multi.Read(b)
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

		c.multi = MultiReader(bytes.NewReader(new), c.Conn)
	}

	return c.multi.Read(b)
}

func (c *Conn) Close() error {
	return c.Conn.Close()
}

func isSingleOrNoReader(r io.Reader) bool {
	v := reflect.Indirect(reflect.ValueOf(r))
	if !v.IsValid() || v.NumField() != 1 {
		return false
	}
	fv := v.Field(0)
	if fv.Type().Kind() != reflect.Slice {
		return false
	}
	return fv.Len() <= 1
}

func MultiReader(r1, r2 io.Reader) io.Reader {
	mr, ok := r2.(*multiReader)
	if !ok {
		// not a previous multiReader so just attach them together
		return &multiReader{
			first:  r1,
			second: r2,
			Reader: io.MultiReader(r1, r2),
		}
	}

	// if we only have one reader left we can cut out the extra multireader
	if isSingleOrNoReader(mr.Reader) {
		return &multiReader{
			first:  r1,
			second: mr.second,
			Reader: io.MultiReader(r1, mr.second),
		}
	}

	// otherwise just attach them together
	return &multiReader{
		first:  r1,
		second: r2,
		Reader: io.MultiReader(r1, r2),
	}
}

type multiReader struct {
	first  io.Reader
	second io.Reader
	io.Reader
}

func IsSingleReader(r io.Reader) bool {
	mr, ok := r.(*multiReader)
	if !ok {
		return true
	}

	// we've exhausted the first reader
	if isSingleOrNoReader(mr.Reader) {
		// recursively check if the last reader is a multiReader
		return IsSingleReader(mr.second)
	}

	return false
}

func UnwrapReader(r io.Reader) io.Reader {
	for {
		mr, ok := r.(*multiReader)
		if !ok {
			return r
		}

		r = mr.second
	}
}
