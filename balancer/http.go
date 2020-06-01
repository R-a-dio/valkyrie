package balancer

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

func (br *Balancer) getStatus() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		br.relays.Lock()
		defer br.relays.Unlock()
		b, err := json.Marshal(br.relays.M)
		if err != nil {
			http.Error(w, "error marshalling relay map", http.StatusTeapot)
			return
		}
		fmt.Fprintln(w, string(b))
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
