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

// Balancer represents the state of the load balancer.
type Balancer struct {
	config.Config
	Manager radio.ManagerService
	relays  []*radio.Relay

	serv      *http.Server
	listeners int
	min       float64
	current   atomic.Value
	mtime     time.Time
}

func health(ctx context.Context, c *http.Client, relay *radio.Relay, wg *sync.WaitGroup) {
	relay.Lock()
	defer relay.Unlock()
	defer wg.Done()
	req, _ := http.NewRequestWithContext(ctx, "GET", relay.Status, nil)
	resp, err := c.Do(req)
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
	for _, relay := range br.relays {
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

func (br *Balancer) check(ctx context.Context) {
	client := &http.Client{
		Timeout: 3 * time.Second,
	}
	var wg sync.WaitGroup
	for _, relay := range br.relays {
		if relay.Disabled {
			continue
		}
		wg.Add(1)
		go health(ctx, client, relay, &wg)
	}
	wg.Wait()
	return
}

func (br *Balancer) start(ctx context.Context) error {
	go func() {
		for {
			select {
			case <-time.After(3 * time.Second):
				br.check(ctx)
				br.choose()
			case <-ctx.Done():
				br.stop()
			}
		}
	}()
	return br.serv.ListenAndServe()
}

func (br *Balancer) stop() error {
	return br.serv.Close()
}
