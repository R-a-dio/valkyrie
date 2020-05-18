package php

import (
	"net/http"
	"testing"
)

func TestGetIdentifier(t *testing.T) {
	var tests = []struct {
		addr       string
		identifier string
	}{
		{"localhost:9000", "localhost"},
		{"127.0.0.1:8888", "127.0.0.1"},
		{"8.8.8.8", "8.8.8.8"},
	}

	for _, test := range tests {
		var r http.Request
		r.RemoteAddr = test.addr

		identifier := getIdentifier(&r)
		if identifier != test.identifier {
			t.Errorf("unexpected identifier: wanted %v got %v", identifier, test.identifier)
		}
	}
}
