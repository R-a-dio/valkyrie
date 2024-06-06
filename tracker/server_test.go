package tracker

import (
	"context"
	"fmt"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListenerAddAndRemove(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg := config.TestConfig()

	dummy := NewServer(ctx, cfg)

	srv := httptest.NewServer(dummy.h)
	defer srv.Close()
	client := srv.Client()

	t.Run("join then leave", func(t *testing.T) {
		id := radio.ListenerClientID(500)

		// ========================
		// Do a normal join request
		resp, err := client.PostForm(srv.URL+"/listener_joined", url.Values{
			ICECAST_CLIENTID_FIELD_NAME: []string{id.String()},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		resp.Body.Close()

		// status should be OK
		require.Equal(t, http.StatusOK, resp.StatusCode)
		// and we should have the OK header that icecast needs
		require.Equal(t, "1", resp.Header.Get(ICECAST_AUTH_HEADER))

		// we also should have a listener in the recorder
		require.Eventually(t, func() bool {
			return assert.Equal(t, int64(1), dummy.recorder.ListenerAmount())
		}, eventuallyDelay, eventuallyTick)
		testListenerLength(t, dummy.recorder, 1)

		// =========================
		// Do a normal leave request
		resp, err = client.PostForm(srv.URL+"/listener_left", url.Values{
			ICECAST_CLIENTID_FIELD_NAME: []string{id.String()},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		resp.Body.Close()

		// status should be OK again
		require.Equal(t, http.StatusOK, resp.StatusCode)
		// and the listener should now be gone
		require.Eventually(t, func() bool {
			return assert.Equal(t, int64(0), dummy.recorder.ListenerAmount())
		}, eventuallyDelay, eventuallyTick)

		testListenerLength(t, dummy.recorder, 0)
	})

	for _, uri := range []string{"/listener_joined", "/listener_left"} {
		t.Run("empty client"+uri, func(t *testing.T) {
			// ========================================
			// Try an empty client ID, this should fail
			resp, err := client.PostForm(srv.URL+uri, url.Values{
				ICECAST_CLIENTID_FIELD_NAME: []string{},
			})
			require.NoError(t, err)
			require.NotNil(t, resp)
			resp.Body.Close()

			// status should still be OK
			require.Equal(t, http.StatusOK, resp.StatusCode)
			// but it should not have the OK header
			require.Zero(t, resp.Header.Get(ICECAST_AUTH_HEADER))
		})

		t.Run("non-integer client"+uri, func(t *testing.T) {
			// ========================================
			// Try a non-integer client ID, this should fail
			resp, err := client.PostForm(srv.URL+uri, url.Values{
				ICECAST_CLIENTID_FIELD_NAME: []string{"not an integer"},
			})
			require.NoError(t, err)
			require.NotNil(t, resp)
			resp.Body.Close()

			// status should still be OK
			require.Equal(t, http.StatusOK, resp.StatusCode)
			// but it should not have the OK header
			require.Zero(t, resp.Header.Get(ICECAST_AUTH_HEADER))
		})
	}
}

func BenchmarkListenerAdd(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg := config.TestConfig()

	recorder := NewRecorder(ctx, cfg)

	handler := ListenerAdd(ctx, recorder)

	values := url.Values{
		ICECAST_CLIENTID_FIELD_NAME: []string{radio.ListenerClientID(50).String()},
	}
	body := strings.NewReader(values.Encode())
	req := httptest.NewRequest(http.MethodPost, "/listener_joined", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	w.Body = nil

	for i := 0; i < b.N; i++ {
		handler.ServeHTTP(w, req)
	}
}

func TestPeriodicallyUpdateListeners(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg := config.TestConfig()

	done := make(chan struct{})

	dummy := new(Server)
	dummy.cfg = cfg
	dummy.recorder = NewRecorder(ctx, cfg)

	var last atomic.Int64
	var count int
	var closeOnce sync.Once

	manager := &mocks.ManagerServiceMock{
		UpdateListenersFunc: func(contextMoqParam context.Context, new int64) error {
			// we're done after 10 updates
			if count++; count > 10 {
				closeOnce.Do(func() {
					close(done)
				})
				return nil
			}
			// every 5 updates return an error
			if count%5 == 0 {
				return fmt.Errorf("that's an error")
			}

			// otherwise our new value should equal what we set it to previously
			if !assert.Equal(t, last.Load(), new) {
				closeOnce.Do(func() {
					close(done)
				})
				return nil
			}

			adjustment := rand.Int64()
			dummy.recorder.listenerAmount.Store(adjustment)
			last.Store(adjustment)

			return nil
		},
	}
	dummy.cfg.Manager = manager

	finished := make(chan struct{})
	go func() {
		defer close(finished)
		dummy.periodicallyUpdateListeners(ctx, time.Millisecond*10)
	}()

	// wait for the 10 updates
	<-done

	// cancel the context we gave the function, it should clean
	// itself up
	cancel()

	select {
	case <-finished:
	case <-time.After(eventuallyDelay):
		t.Error("failed to cleanup")
	}
}
