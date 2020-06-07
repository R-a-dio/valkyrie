package balancer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

func (br *Balancer) getStatus() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		// start the JSON array
		buf.WriteString("[")
		for i, relay := range br.relays {
			relay.RLock()
			err := enc.Encode(relay)
			if err != nil {
				http.Error(w, err.Error(), http.StatusTeapot)
				relay.RUnlock()
				return
			}
			relay.RUnlock()
			if i != len(br.relays)-1 {
				buf.WriteString(",")
			}
		}
		// end the JSON array and flush
		buf.WriteString("]")
		io.Copy(w, &buf)
		return
	}
}

func (br *Balancer) getIndex() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, time.Now())
		return
	}
}

func (br *Balancer) getMain() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, br.current.Load().(string), http.StatusFound)
		return
	}
}
