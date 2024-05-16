package website

import (
	"net/http"
	"testing"
)

func TestRemovePortFromAddress(t *testing.T) {
	tests := []struct {
		hostport string
		host     string
	}{
		{"127.0.0.1:8000", "127.0.0.1"},
		{"127.0.0.1", "127.0.0.1"},
		{"localhost:65555", "localhost"},
		{"2001:0db8:85a3:0000:0000:8a2e:0370:7334", "2001:0db8:85a3:0000:0000:8a2e:0370:7334"},
		{"[2001:0db8:85a3:0000:0000:8a2e:0370:7334]:9000", "2001:0db8:85a3:0000:0000:8a2e:0370:7334"},
		{"", ""},
	}

	for _, test := range tests {
		// setup return value
		value := ""
		// setup our call check
		wasCalled := false
		// next handler so we can check the result
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			value = r.RemoteAddr
			wasCalled = true
		})

		removePortFromAddress(next).ServeHTTP(nil, &http.Request{
			RemoteAddr: test.hostport,
		})

		t.Logf("%s; wasCalled=%t,value=%s", test.hostport, wasCalled, value)
		if !wasCalled {
			t.Fatalf("removePortFromAddress did not call next middleware: %v", test)
		}

		if value != test.host {
			t.Errorf("removePortFromAddress did not return expected value: %v != %v",
				value, test.host)
		}
	}
}
