package streamer

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewIcecastConn(t *testing.T) {
	e := make(chan string, 1)
	var statusCode int
	var testData []byte

	s := httptest.NewServer(http.HandlerFunc(
		func(rw http.ResponseWriter, r *http.Request) {
			if r.Method != "SOURCE" {
				e <- "method is not SOURCE"
				return
			}

			if r.Proto != "HTTP/1.0" {
				e <- "using wrong HTTP version"
				return
			}

			auth := r.Header.Get("Authorization")
			if auth == "" {
				e <- "no authorization send"
				return
			}

			// TODO: check auth values

			if r.Header.Get("Content-Type") == "" {
				e <- "no content-type set"
				return
			}

			rw.WriteHeader(statusCode)

			data := make([]byte, len(testData))
			n, err := io.ReadFull(r.Body, data)
			if err != nil {
				e <- err.Error()
				return
			}
			if n != len(data) {
				e <- "n not equal to data length"
				return
			}

			if !bytes.Equal(data, testData) {
				e <- "data not equal to testData"
				return
			}
		},
	))

	_ = s
}
