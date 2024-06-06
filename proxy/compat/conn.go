package compat

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"net"
	"os"
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
var httpLinePrefix = []byte("HTTP/1.")

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

// Wrap returns a *compat.Listener wrapping the listener given
func Wrap(logger *zerolog.Logger, ln net.Listener) net.Listener {
	return &Listener{logger, ln}
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

func (l *Listener) File() (*os.File, error) {
	switch ln := l.Listener.(type) {
	case *net.TCPListener:
		return ln.File()
	case *net.UnixListener:
		return ln.File()
	default:
		panic("unsupported listener")
	}
}

type Conn struct {
	net.Conn
	multi io.Reader

	logger *zerolog.Logger
}

func (c *Conn) Read(b []byte) (n int, err error) {
	// fast path, we've already read more than the max, just pass to
	// the connection directly
	if c.multi != nil {
		return c.multi.Read(b)
	}

	if len(b) == 0 {
		return 0, nil
	}

	n, err = c.Conn.Read(b)
	if err != nil {
		return n, err
	}
	old := b[:n]

	// the thing we're interested in is always on the first line
	if bytes.Contains(old, newLine) {
		// so if we found a newline, we can do our replacement and return
		new := bytes.Replace(old, iceLine, httpLine, 1)
		if len(new) > len(old) && c.logger != nil {
			// log when this happens so we know someone is using an old client
			c.logger.Info().Str("address", c.RemoteAddr().String()).Msg("ICE/1.0")
		}

		c.multi = MultiReader(bytes.NewReader(new), c.Conn)
		return c.multi.Read(b)
	}

	// we haven't found a newline yet, we now have two more conditions we can check to
	// see if we are done with replacing or still need to keep looking.
	// 		#1 is that we reached our maxReadSize
	// 		#2 is that we have encountered a HTTP/1.0 or HTTP/1.1
	if n > maxReadSize || bytes.Contains(old, httpLinePrefix) {
		// this we will assume is just a non-ice request so pass it along
		c.multi = c.Conn
		return n, nil
	}

	// otherwise, we don't have a newline, haven't read maxReadSize bytes yet and haven't
	// encountered a HTTP/1.0 or HTTP/1.1 yet; We could implement extra stuff here to
	// handle this case, but it should be virtually non-existant for proper behaving
	// clients. So just log that this occured and assume it's a non-ICE request
	c.logger.Error().Str("address", c.RemoteAddr().String()).Msg("rare compat")
	c.multi = c.Conn
	return n, nil
}

func (c *Conn) Close() error {
	return c.Conn.Close()
}

func (c *Conn) File() (*os.File, error) {
	f, ok := c.Conn.(interface{ File() (*os.File, error) })
	if !ok {
		return nil, errors.New("unsupported File call")
	}
	return f.File()
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

func DrainBuffer(rw *bufio.ReadWriter, conn net.Conn) net.Conn {
	return &Conn{
		Conn:  conn,
		multi: MultiReader(StripBuffer(rw.Reader), conn),
	}
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
