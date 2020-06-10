package balancer

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/R-a-dio/valkyrie/errors"
)

func (br *Balancer) getStatus() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		relays, err := br.storage.Relay(r.Context()).All()
		if err != nil {
			if errors.Is(errors.NoRelays, err) {
				http.Error(w, "no relays", 418)
				return
			}
		}
		err = json.NewEncoder(w).Encode(relays)
		if err != nil {
			log.Println(err)
			http.Error(w, "error encoding json", 500)
			return
		}
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
		http.Redirect(w, r, br.getCurrent(), http.StatusFound)
		return
	}
}
