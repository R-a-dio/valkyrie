package proxy

import (
	"context"
	"net"
	"net/url"

	"github.com/R-a-dio/valkyrie/config"
	"github.com/cenkalti/backoff"
)

type Mount struct {
	ConnFn func() (net.Conn, error)
	Name   string
	Conn   net.Conn
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
		Name: uri.Path,
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
