package tracker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListenerAddAndRemove(t *testing.T) {
	ctx := context.Background()

	recorder := NewRecorder()
	dummy := NewServer(ctx, "", recorder)

	srv := httptest.NewServer(dummy.Handler)
	defer srv.Close()
	client := srv.Client()

	t.Run("join then leave", func(t *testing.T) {
		id := ClientID(500)

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
			return assert.Equal(t, int64(1), recorder.ListenerAmount())
		}, eventuallyDelay, eventuallyTick)
		testListenerLength(t, recorder, 1)

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
			return assert.Equal(t, int64(0), recorder.ListenerAmount())
		}, eventuallyDelay, eventuallyTick)

		testListenerLength(t, recorder, 0)
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
	recorder := NewRecorder()
	ctx := context.Background()

	handler := ListenerAdd(ctx, recorder)

	values := url.Values{
		ICECAST_CLIENTID_FIELD_NAME: []string{ClientID(50).String()},
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
