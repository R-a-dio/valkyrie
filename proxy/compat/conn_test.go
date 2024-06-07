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

	assert.False(t, isSingleOrNoReader(r))
	// first read is asking for more bytes than our first input provides, with
	// current MultiReader implementation this exhausts the first input and then
	// returns before starting on the next reader on the next Read call
	n, err := r.Read(p)
	require.NoError(t, err)
	assert.Equal(t, firstInput, p[:n])
	// our first reader isn't cleaned up until this next call, so we should still be
	// returning false for now
	assert.False(t, isSingleOrNoReader(r))
	n, err = r.Read(p)
	require.NoError(t, err)
	assert.Equal(t, secondInput[:len(p)], p[:n])
	// we've now read from our second reader so we should just have one reader left
	assert.True(t, isSingleOrNoReader(r))
	rest, err := io.ReadAll(r)
	require.NoError(t, err)
	assert.Equal(t, secondInput[len(p):], rest)
	// we've now exhausted both readers so we should have no readers left in the multireader
	assert.True(t, isSingleOrNoReader(r))
}

func TestIsSingleReaderNoPanic(t *testing.T) {
	assert.False(t, isSingleOrNoReader(nil))
	assert.False(t, isSingleOrNoReader(struct{ io.Reader }{}))
	assert.False(t, isSingleOrNoReader(strings.NewReader("")))
	assert.False(t, isSingleOrNoReader(bytes.NewReader([]byte(""))))
}

func TestMultiReader(t *testing.T) {
	ReadN := func(r io.Reader, n int) ([]byte, error) {
		return io.ReadAll(io.LimitReader(r, int64(n)))
	}
	_ = ReadN
	Concat := func(b1, b2, b3 []byte) []byte {
		tmp := append(b1, b2...)
		return append(tmp, b3...)
	}
	Readers := func(b1, b2, b3 []byte) (io.Reader, io.Reader, io.Reader) {
		return bytes.NewReader(b1), bytes.NewReader(b2), bytes.NewReader(b3)
	}

	parameters := gopter.DefaultTestParameters()

	run := func(t *testing.T, name string, prop gopter.Prop) {
		p := gopter.NewProperties(parameters)
		p.Property(name, prop)
		p.TestingRun(t)
	}

	run(t, "3 nested readers read everything", prop.ForAllNoShrink(
		func(b1, b2, b3 []byte) bool {
			final := Concat(b1, b2, b3)

			r1, r2, r3 := Readers(b1, b2, b3)

			// all of these are not multiReader so should return true
			assert.True(t, IsSingleReader(r1))
			assert.True(t, IsSingleReader(r2))
			assert.True(t, IsSingleReader(r3))

			// this is a multireader with two readers still so should be false
			mr1 := MultiReader(r2, r3)
			assert.False(t, IsSingleReader(mr1))

			// same as above
			mr2 := MultiReader(r1, mr1)
			assert.False(t, IsSingleReader(mr2))

			// now we read everything available
			data, err := io.ReadAll(mr2)
			assert.NoError(t, err)
			assert.Equal(t, final, data)

			assert.True(t, IsSingleReader(mr2))
			assert.Equal(t, UnwrapReader(mr2), r3)

			return true
		},
		gen.SliceOf(gen.UInt8()),
		gen.SliceOf(gen.UInt8()),
		gen.SliceOf(gen.UInt8()),
	))

	parameters.MinSize = 16
	run(t, "3 nested readers read 2", prop.ForAllNoShrink(
		func(b1, b2, b3 []byte) bool {
			r1, r2, r3 := Readers(b1, b2, b3)

			// all of these are not multiReader so should return true
			assert.True(t, IsSingleReader(r1))
			assert.True(t, IsSingleReader(r2))
			assert.True(t, IsSingleReader(r3))

			// this is a multireader with two readers still so should be false
			mr1 := MultiReader(r2, r3)
			assert.False(t, IsSingleReader(mr1))

			// same as above
			mr2 := MultiReader(r1, mr1)
			assert.False(t, IsSingleReader(mr2))

			// now we read the first two readers
			data, err := ReadN(mr2, len(b1)+len(b2))
			assert.NoError(t, err)
			assert.Equal(t, Concat(b1, b2, nil), data)
			// then a little bit of the third
			n := len(b3) / 2
			data, err = ReadN(mr2, n)

			assert.NoError(t, err)
			assert.Equal(t, b3[:n], data)

			assert.True(t, IsSingleReader(mr2))
			assert.Equal(t, UnwrapReader(mr2), r3)

			// and now we should be able to just read the rest from r3 directly
			data, err = io.ReadAll(r3)
			assert.NoError(t, err)
			assert.Equal(t, b3[n:], data)

			return true
		},
		gen.SliceOf(gen.UInt8()).SuchThat(func(b []byte) bool { return len(b) > 16 }),
		gen.SliceOf(gen.UInt8()).SuchThat(func(b []byte) bool { return len(b) > 16 }),
		gen.SliceOf(gen.UInt8()).SuchThat(func(b []byte) bool { return len(b) > 16 }),
	))
	run(t, "3 nested readers read first before creating second multi", prop.ForAllNoShrink(
		func(b1, b2, b3 []byte) bool {
			r1, r2, r3 := Readers(b1, b2, b3)

			// all of these are not multiReader so should return true
			assert.True(t, IsSingleReader(r1))
			assert.True(t, IsSingleReader(r2))
			assert.True(t, IsSingleReader(r3))

			// this is a multireader with two readers still so should be false
			mr1 := MultiReader(r2, r3)
			assert.False(t, IsSingleReader(mr1))

			// same as above
			mr2 := MultiReader(r1, mr1)
			assert.False(t, IsSingleReader(mr2))

			// now we read the first two readers
			data, err := ReadN(mr2, len(b1)+len(b2))
			assert.NoError(t, err)
			assert.Equal(t, Concat(b1, b2, nil), data)
			// then a little bit of the third
			n := len(b3) / 2
			data, err = ReadN(mr2, n)

			assert.NoError(t, err)
			assert.Equal(t, b3[:n], data)

			assert.True(t, IsSingleReader(mr2))
			assert.Equal(t, UnwrapReader(mr2), r3)

			// and now we should be able to just read the rest from r3 directly
			data, err = io.ReadAll(r3)
			assert.NoError(t, err)
			assert.Equal(t, b3[n:], data)

			return true
		},
		gen.SliceOf(gen.UInt8()).SuchThat(func(b []byte) bool { return len(b) > 16 }),
		gen.SliceOf(gen.UInt8()).SuchThat(func(b []byte) bool { return len(b) > 16 }),
		gen.SliceOf(gen.UInt8()).SuchThat(func(b []byte) bool { return len(b) > 16 }),
	))
}
