package balancer

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

var x = []byte(
	`<?xml version="1.0" encoding="UTF-8"?>
<playlist xmlns="http://xspf.org/ns/0/" version="1">
  <title/>
  <creator/>
  <trackList>
    <track>
      <location>http://stream.r-a-d.io:8000/main.mp3</location>
      <title>test - test</title>
      <annotation>Stream Title: R/a/dio
Stream Description: Your weeaboo station
Content Type:audio/mpeg
Current Listeners: 1337
Peak Listeners: 144
Stream Genre: various</annotation>
    </track>
  </trackList>
</playlist>`)

func TestStateHealth(t *testing.T) {
	relaytests := []struct {
		name   string
		relay  http.HandlerFunc
		online bool
	}{
		{"valid", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintln(w, string(x))
		}, true},
		{"garbage", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, "garbage")
		}, false},
		{"timeout", func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(4 * time.Second) // the http.Client timeout should be 3 seconds
		}, false},
		{"almost", func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(2 * time.Second)
			fmt.Fprintln(w, string(x))
		}, true},
	}

	for _, tt := range relaytests {
		t.Run(tt.name, func(t *testing.T) {
			tt := tt
			t.Parallel()
			ts := httptest.NewServer(tt.relay)
			defer ts.Close()
			s, r, m := new(state), new(relay), make(map[string]*relay)
			r.Status, m[tt.name], s.relays.m = ts.URL, r, m
			s.check()
			o := s.relays.m[tt.name].Online
			if o != tt.online {
				t.Errorf("got %t, want %t", o, tt.online)
			}
		})
	}

	return
}
