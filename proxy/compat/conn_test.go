package compat

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConnection(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	properties := gopter.NewProperties(parameters)
	properties.Property("handle ICE/1.0", prop.ForAll(
		func(path string, data []byte) bool {
			called := make(chan struct{})
			logBuf := new(bytes.Buffer)
			logger := zerolog.New(logBuf)

			srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				close(called)
			}))
			// replace listener with our wrapped one
			srv.Listener = &Listener{&logger, srv.Listener}
			srv.Start()

			// create a request that sends ICE/1.0
			var buf = new(bytes.Buffer)
			req := httptest.NewRequest("SOURCE", srv.URL, bytes.NewReader(data))

			// change the url path to test with
			req.URL.Path = "/" + path

			conn, err := net.Dial("tcp", req.URL.Host)
			require.NoError(t, err)

			require.NoError(t, req.Write(buf))
			iceReq := bytes.Replace(buf.Bytes(), []byte("HTTP/1.1"), iceLine, 1)

			n, err := conn.Write(iceReq)
			if assert.NoError(t, err) {
				assert.Equal(t, len(iceReq), n)
			}

			// wait for our request to go through, or 5 seconds whichever is first
			select {
			case <-called:
				assert.Contains(t, logBuf.String(), "ICE/1.0")
				return true
			case <-time.After(time.Second * 5):
				return false
			}
		},
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) < 900 }),
		gen.SliceOf(gen.UInt8()),
	))
	properties.Property("handle HTTP/1.1", prop.ForAll(
		func(path string, data []byte) bool {
			called := make(chan struct{})
			logBuf := new(bytes.Buffer)
			logger := zerolog.New(logBuf)

			srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got, err := io.ReadAll(r.Body)
				if assert.NoError(t, err) {
					assert.Equal(t, data, got)
				}
				close(called)
			}))
			srv.Listener = &Listener{&logger, srv.Listener}
			srv.Start()

			uri, err := url.Parse(srv.URL)
			require.NoError(t, err)
			uri.Path = "/" + path

			req, err := http.NewRequest("SOURCE", uri.String(), bytes.NewReader(data))
			require.NoError(t, err)

			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			require.NotNil(t, resp)

			// wait for our request to go through, or 5 seconds whichever is first
			select {
			case <-called:
				assert.Empty(t, logBuf.String())
				return true
			case <-time.After(time.Second * 5):
				return false
			}
		},
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) < 900 }),
		gen.SliceOf(gen.UInt8()),
	))
	properties.TestingRun(t)
}

func TestIsSingleReader(t *testing.T) {
	var p = make([]byte, 8)
	firstInput := []byte("hello")
	secondInput := []byte(" world and some extra text to keep it alive")

	r := io.MultiReader(bytes.NewReader(firstInput), bytes.NewReader(secondInput))

	assert.False(t, isSingleReader(r))
	// first read is asking for more bytes than our first input provides, with
	// current MultiReader implementation this exhausts the first input and then
	// returns before starting on the next reader on the next Read call
	n, err := r.Read(p)
	require.NoError(t, err)
	assert.Equal(t, firstInput, p[:n])
	// our first reader isn't cleaned up until this next call, so we should still be
	// returning false for now
	assert.False(t, isSingleReader(r))
	n, err = r.Read(p)
	require.NoError(t, err)
	assert.Equal(t, secondInput[:len(p)], p[:n])
	// we've now read from our second reader so we should just have one reader left
	assert.True(t, isSingleReader(r))
	rest, err := io.ReadAll(r)
	require.NoError(t, err)
	assert.Equal(t, secondInput[len(p):], rest)
	// we've now exhausted both readers so we should have no readers left in the multireader
	assert.False(t, isSingleReader(r))
}

func TestIsSingleReaderNoPanic(t *testing.T) {
	assert.False(t, isSingleReader(nil))
	assert.False(t, isSingleReader(struct{ io.Reader }{}))
	assert.False(t, isSingleReader(strings.NewReader("")))
	assert.False(t, isSingleReader(bytes.NewReader([]byte(""))))
}
