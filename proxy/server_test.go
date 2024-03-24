package proxy

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/mocks"
	"github.com/R-a-dio/valkyrie/streamer/icecast"
	"github.com/R-a-dio/valkyrie/util"
	"github.com/R-a-dio/valkyrie/website/admin"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestUser(username, passwd string) *radio.User {
	bpasswd, _ := admin.GenerateHashFromPassword(passwd)

	return &radio.User{
		Username: username,
		Password: string(bpasswd),
		UserPermissions: radio.UserPermissions{
			radio.PermActive: struct{}{},
			radio.PermDJ:     struct{}{},
		},
	}
}

func dialIcecast(t *testing.T, ctx context.Context, u string, opts ...icecast.Option) net.Conn {
	conn, err := icecast.Dial(ctx, u, opts...)
	require.NoError(t, err)
	require.NotNil(t, conn)

	// write some random data to keep the connection alive
	go func() {
		for range time.Tick(time.Second / 4) {
			io.WriteString(conn, "hello")
		}
	}()

	return conn
}

func TestServer(t *testing.T) {
	ctx := context.Background()
	ctx = zerolog.New(os.Stdout).WithContext(ctx)

	cfg, err := config.LoadFile()
	require.NoError(t, err)

	username, pw := "test", "hackme"
	mountName := "/main.mp3"
	contentType := "audio/mpeg"

	manager := &mocks.ManagerServiceMock{}
	storage := &mocks.StorageServiceMock{
		UserFunc: func(contextMoqParam context.Context) radio.UserStorage {
			return &mocks.UserStorageMock{
				GetFunc: func(name string) (*radio.User, error) {
					return newTestUser(username, pw), nil
				},
			}
		},
	}

	var sourceConnections atomic.Int64
	var latestMetadata util.TypedValue[string]
	// we need a fake icecast server
	icecastsrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if r.Method == http.MethodPut {
			sourceConnections.Add(1)
			// if it's a PUT it should be an audio stream so just discard everything
			io.Copy(io.Discard, r.Body)
		}

		if r.Method == http.MethodGet && r.URL.Path == "/admin/metadata" {
			// metadata request
			latestMetadata.Store(r.FormValue("song"))
		}
	}))
	defer icecastsrv.Close()

	// configure the fake as the master server
	c := cfg.Conf()
	c.Proxy.MasterServer = icecastsrv.URL
	cfg.StoreConf(c)

	// and then our proxy server
	srv, err := NewServer(ctx, cfg, manager, storage)
	require.NoError(t, err)
	require.NotNil(t, srv)

	proxysrv := httptest.NewServer(srv.http.Handler)
	defer proxysrv.Close()

	// then we can dial into the proxy
	conn1Uri := proxysrv.URL + mountName
	conn1 := dialIcecast(t, ctx, conn1Uri,
		icecast.ContentType(contentType),
		icecast.Auth(username, pw),
	)

	conn2Uri := proxysrv.URL + mountName
	conn2 := dialIcecast(t, ctx, conn2Uri,
		icecast.ContentType(contentType),
		icecast.Auth(username, pw),
	)

	// now check if the mount exists
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		srv.proxy.mountsMu.Lock()
		defer srv.proxy.mountsMu.Unlock()

		mount := srv.proxy.mounts[mountName]

		assert.NotNil(c, mount)
		assert.Equal(c, mountName, mount.Name, "should have the same name")
		assert.Equal(c, contentType, mount.ContentType, "should have the same content-type")
		assert.Equal(c, 2, getSourcesLength(mount), "should have two sources")
	}, time.Second, time.Millisecond*50, "mount should exist")

	// see if we can send metadata
	metaFn, err := icecast.Metadata(conn1Uri, icecast.Auth(username, pw))
	if assert.NoError(t, err) {
		meta := "hello world"
		err = metaFn(ctx, meta)
		if assert.NoError(t, err) {
			assert.EventuallyWithT(t, func(c *assert.CollectT) {
				assert.Equal(c, meta, latestMetadata.Load())
			}, time.Second, time.Millisecond*50)
		}
	}

	// close the connections and see if stuff gets cleaned up
	conn1.Close()
	conn2.Close()

	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		srv.proxy.mountsMu.Lock()
		defer srv.proxy.mountsMu.Unlock()

		mount := srv.proxy.mounts[mountName]

		assert.Equal(c, 0, getSourcesLength(mount), "should have no sources")
	}, time.Second, time.Millisecond*50, "mount should be empty")

	assert.Eventually(t, func() bool {
		srv.proxy.mountsMu.Lock()
		defer srv.proxy.mountsMu.Unlock()

		mount := srv.proxy.mounts[mountName]
		return mount == nil
	}, mountTimeout*2, mountTimeout/20, "mount should've been cleaned up")

	assert.EqualValues(t, 1, sourceConnections.Load(), "should've only seen one icecast connection")
}
