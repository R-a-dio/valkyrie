package proxy

import (
	"net"
	"testing"

	"github.com/R-a-dio/valkyrie/proxy/compat"
	"github.com/Wessie/fdstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetFileOnCompat(t *testing.T) {
	ln, err := compat.Listen(nil, "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	// check if we can get the fd from the listener we get from compat
	fd, err := ln.(fdstore.Filer).File()
	if assert.NoError(t, err) {
		assert.NotNil(t, fd)
		defer fd.Close()
	}

	// then check if we can get the fd from the net.Conn compat gives us
	done := make(chan struct{})
	go func() {
		defer close(done)

		conn, err := ln.Accept()
		assert.NoError(t, err)
		assert.NotNil(t, conn)

		fd, err = conn.(fdstore.Filer).File()
		if assert.NoError(t, err) {
			defer fd.Close()
			assert.NotNil(t, fd)
		}
	}()

	conn, err := net.Dial("tcp", ln.Addr().String())
	require.NoError(t, err)
	require.NotNil(t, conn)
	defer conn.Close()

	<-done
}
