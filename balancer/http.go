package balancer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/R-a-dio/valkyrie/errors"
)

func (br *Balancer) getStatus() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var buf bytes.Buffer
		relays, err := br.storage.Relay(r.Context()).All()
		if err != nil {
			if errors.Is(errors.NoRelays, err) {
				http.Error(w, "no relays", 418)
				return
			}
		}
		err = json.NewEncoder(&buf).Encode(relays)
		if err != nil {
			log.Println(err)
			http.Error(w, "error encoding json", 500)
			return
		}
		buf.WriteString(fmt.Sprintf("%.5f", relays[0].Score()))
		io.Copy(w, &buf)
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
