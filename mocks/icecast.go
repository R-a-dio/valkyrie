package mocks

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

type icecastServerMock struct {
	*httptest.Server

	sources        atomic.Int32
	latestMetadata atomic.Value
}

func IcecastServerMock(t *testing.T) *icecastServerMock {
	var mock icecastServerMock

	mock.Server = httptest.NewServer(&mock)
	return &mock
}

func (ism *icecastServerMock) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)

	if r.Method == http.MethodPut {
		// we treat PUTs as source connections and just discard their data
		ism.sources.Add(1)
		defer ism.sources.Add(-1)
		_, _ = io.Copy(io.Discard, r.Body)
		return
	}

	// otherwise we expect it to be a GET to the /admin/metadata endpoint
	if r.Method == http.MethodGet && r.URL.Path == "/admin/metadata" {
		ism.latestMetadata.Store(r.FormValue("song"))
	}
}

func (ism *icecastServerMock) Sources() int32 {
	return ism.sources.Load()
}

func (ism *icecastServerMock) LatestMetadata() string {
	return ism.latestMetadata.Load().(string)
}
