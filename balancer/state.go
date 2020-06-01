package balancer

import (
	"context"
	"io/ioutil"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
)

type Balancer struct {
	config.Config
	Manager radio.ManagerService
	relays  radio.Relays

	serv      *http.Server
	listeners int
	min       float64
	current   atomic.Value
	mtime     time.Time
}

func health(c *http.Client, relay *radio.Relay, wg *sync.WaitGroup) {
	defer wg.Done()
	resp, err := c.Get(relay.Status)
	if err != nil {
		relay.Deactivate()
		return
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		relay.Deactivate()
		return
	}
	l, err := parsexml(body)
	if err != nil {
		relay.Deactivate()
		return
	}
	relay.Activate(l)
	return
}

func (br *Balancer) choose() {
	br.relays.Lock()
	defer br.relays.Unlock()
	for _, relay := range br.relays.M {
		if !relay.Online || relay.Noredir || relay.Disabled {
			continue
		}

		score := float64((relay.Listeners / relay.Max) - (relay.Weight / 1000))
		if score < br.min {
			br.min = score
			br.current.Store(relay.Stream)
			return
		}
	}
	br.current.Store(br.Config.Conf().Balancer.Fallback)
	return
}

func (br *Balancer) check() {
	client := &http.Client{
		Timeout: 3 * time.Second,
	}
	var wg sync.WaitGroup
	br.relays.Lock()
	defer br.relays.Unlock()
	for _, relay := range br.relays.M {
		if relay.Disabled {
			continue
		}
		wg.Add(1)
		go health(client, relay, &wg)
	}
	wg.Wait()
	return
}

func (br *Balancer) start(ctx context.Context) error {
	go func() {
		for {
			select {
			case <-time.After(10 * time.Second):
				br.check()
				br.choose()
			case <-ctx.Done():
				br.serv.Close()
			}
		}
	}()
	return br.serv.ListenAndServe()
}
