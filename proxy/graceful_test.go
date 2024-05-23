package proxy_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/migrations"
	"github.com/R-a-dio/valkyrie/proxy"
	"github.com/R-a-dio/valkyrie/storage"
	"github.com/R-a-dio/valkyrie/streamer/icecast"
	"github.com/R-a-dio/valkyrie/util"
	"github.com/R-a-dio/valkyrie/util/graceful"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/mariadb"
)

func TestGracefulRestart(t *testing.T) {
	t.SkipNow()

	logger := zerolog.New(zerolog.NewTestWriter(t))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = logger.WithContext(ctx)
	cfg := config.TestConfig()

	// setup a fake icecast server
	var sourceConnections atomic.Int64
	var latestMetadata util.TypedValue[string]
	icecastsrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Log(r)
		w.WriteHeader(http.StatusOK)
		if r.Method == http.MethodPut {
			sourceConnections.Add(1)
			// if it's a PUT it should be an audio stream so just discard everything
			io.Copy(io.Discard, r.Body)
			return
		}

		if r.Method == http.MethodGet && r.URL.Path == "/admin/metadata" {
			// metadata request
			latestMetadata.Store(r.FormValue("song"))
		}
	}))
	defer icecastsrv.Close()

	// setup database container to test in
	container, err := mariadb.RunContainer(ctx,
		testcontainers.WithImage("mariadb:latest"),
		mariadb.WithDatabase("test"),
		mariadb.WithUsername("root"),
		mariadb.WithPassword(""),
	)
	require.NoError(t, err, "failed setting up container")

	dsn, err := container.ConnectionString(ctx)
	require.NoError(t, err)

	// touch-up the config with our test container and other settings
	c := cfg.Conf()
	c.Database.DSN = dsn
	c.Proxy.ListenAddr = config.MustParseAddrPort("127.0.0.1:58643")
	c.Proxy.MasterServer = config.URL(icecastsrv.URL)
	cfg.StoreConf(c)

	// run migrations to give us tables
	migr, err := migrations.New(ctx, cfg)
	require.NoError(t, err)

	err = migr.Up()
	require.NoError(t, err)

	// then get ourselves a fake user we can use to login with
	store, err := storage.Open(ctx, cfg)
	require.NoError(t, err)

	passwd := "proxy-test-password"
	hashedPass, err := radio.GenerateHashFromPassword(passwd)
	require.NoError(t, err)

	user := radio.User{
		Username: "proxy-test",
		Password: hashedPass,
		UserPermissions: radio.UserPermissions{
			radio.PermActive: struct{}{},
			radio.PermDJ:     struct{}{},
		},
	}
	_, err = store.User(ctx).Create(user)
	require.NoError(t, err)

	// actual test run
	parentFinished := make(chan struct{})

	// setup a fake graceful state
	ctx, grace := graceful.TestGraceful(ctx, false)
	// then launch our "parent" process
	go func() {
		err := proxy.Execute(ctx, cfg)
		if err != nil {
			t.Error("failed parent execute:", err)
		}
		close(parentFinished)
	}()

	// start a fake source client to the server
	var clientConn net.Conn

	require.Eventually(t, func() bool {
		conn, err := icecast.Dial(ctx, "http://127.0.0.1:58643/test.mp3",
			icecast.ContentType("audio/mpeg"),
			icecast.Auth(user.Username, passwd),
		)
		if err == nil {
			clientConn = conn
			return true
		}
		t.Log(err)
		return false
	}, time.Second*10, 500*time.Millisecond)

	clientRunning := make(chan struct{})
	clientStarted := make(chan struct{})
	go func() {
		data := strings.Repeat("hello world", 4096)
		ticker := time.NewTicker(time.Second / 4)

		close(clientStarted)
		defer close(clientRunning)
		defer clientConn.Close()
		for {
			_, err := io.WriteString(clientConn, data)
			if err != nil {
				t.Error("client write error:", err)
			}

			select {
			case <-ticker.C:
			case <-ctx.Done():
				return
			}
		}
	}()

	// wait for the client to actually start running
	<-clientStarted

	// wait for the proxy to actually connect to the master server
	require.Eventually(t, func() bool {
		return sourceConnections.Load() == 1
	}, time.Second*10, time.Millisecond*100)

	// start the graceful process by simulating a SIGUSR2 coming in
	close(grace.Signal)

	// and launch our "child" process
	childFinished := make(chan struct{})
	go func() {
		defer close(childFinished)
		ctx := graceful.TestMarkChild(ctx)
		err := proxy.Execute(ctx, cfg)
		if err != nil {
			t.Error("failed child execute:", err)
		}
	}()

	select {
	case <-parentFinished:
		t.Log("parent finished")
	case <-time.After(time.Second * 10):
		t.Fatal("parent never finished")
	}

	// check if our client is still running
	select {
	case <-clientRunning:
		t.Fatal("client stopped running")
	case <-time.After(time.Second * 5):
		t.Log("client is running")
	}

	// then wait for our child to exit
	cancel()
	select {
	case <-childFinished:
		t.Log("child finished")
	case <-time.After(time.Second * 10):
		t.Fatal("child never finished")
	}

	// very bad, but let proxy goroutines have a chance of exiting
	time.Sleep(time.Second)
}
