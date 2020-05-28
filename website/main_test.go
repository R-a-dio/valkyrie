package website

import (
	"net/http"
	"testing"
)

func TestRemovePortFromAddress(t *testing.T) {
	attempts := []struct {
		hostport      string
		host          string
		panicExpected bool
	}{
		{"127.0.0.1:8000", "127.0.0.1", false},
		{"127.0.0.1", "127.0.0.1", false},
		{"localhost:65555", "localhost", false},
		{"", "", true},
	}

	for _, attempt := range attempts {
		// setup return value
		value := ""
		// setup our call check
		wasCalled := false
		panicked := false
		// next handler so we can check the result
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			value = r.RemoteAddr
			wasCalled = true
		})

		// catch panics
		func() {
			defer func() {
				if err := recover(); err != nil {
					panicked = true
				}
			}()
			removePortFromAddress(next).ServeHTTP(nil, &http.Request{
				RemoteAddr: attempt.hostport,
			})
		}()

		t.Logf("%s; panicked=%t,wasCalled=%t,value=%s", attempt.hostport, panicked, wasCalled, value)
		if attempt.panicExpected {
			if !panicked {
				t.Fatalf("removePortFromAddress panicked unexpectedly on: %v", attempt)
			}
			continue
		}
		if !wasCalled {
			t.Fatalf("removePortFromAddress did not call next middleware: %v", attempt)
		}

		if value != attempt.host {
			t.Errorf("removePortFromAddress did not return expected value: %v != %v",
				value, attempt.host)
		}
	}
}
