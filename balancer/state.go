package balancer

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"sync/atomic"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/errors"
)

// Balancer represents the state of the load balancer.
type Balancer struct {
	config.Config
	manager radio.ManagerService
	storage radio.StorageService
	serv    *http.Server

	// The current stream to re-direct clients to.
	current atomic.Value
}

func (br *Balancer) getCurrent() string {
	return br.current.Load().(string)
}

// we know statically that current will only be of type string.
func (br *Balancer) setCurrent(str string) {
	br.current.Store(str)
}

// health checks the status of r using c, returning a copy of r.
func health(ctx context.Context, c *http.Client, r radio.Relay) radio.Relay {
	res := r
	res.Online, res.Listeners = false, 0
	req, err := http.NewRequestWithContext(ctx, "GET", r.Status, nil)
	if err != nil {
		return res
	}
	resp, err := c.Do(req)
	if err != nil {
		return res
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return res
	}
	resp.Body.Close()
	l, err := parsexml(body)
	if err != nil {
		return res
	}
	res.Online, res.Listeners = true, l
	return res
}

// checker receives relays on in and sends updated relays on out.
// the execution of checker can be halted by cancelling ctx.
func checker(ctx context.Context, in, out chan radio.Relay) {
	c := &http.Client{
		Timeout: 3 * time.Second,
	}
	for {
		select {
		case <-ctx.Done():
			return
		case relay, ok := <-in:
			if ok {
				fmt.Println("checking", relay.Name)
				out <- health(ctx, c, relay)
			} else { // we've received every value and the channel is closed
				close(out) // we're not sending anymore
				return
			}
		}
	}
}

// update checks all relays and sets the current relay to re-direct to.
func (br *Balancer) update(ctx context.Context) {
	relays, err := br.storage.Relay(ctx).All()
	if err != nil {
		if errors.Is(errors.NoRelays, err) {
			return // do nothing
		}
		log.Println("balancer: error getting relays:", err)
		return
	}
	// we already know that len(relays) != 0
	in := make(chan radio.Relay, len(relays))
	out := make(chan radio.Relay, len(relays))

	go checker(ctx, in, out)

	for _, relay := range relays {
		if relay.Disabled {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case in <- relay:
		}
	}
	close(in)

	var winner float64
	for {
		select {
		case <-ctx.Done():
			return
		case relay, ok := <-out:
			if ok {
				err := br.storage.Relay(ctx).Update(relay)
				if err != nil {
					log.Printf("balancer: error updating relay %s:%s\n", relay.Name, err)
					continue
				}
				if !relay.Online || relay.Disabled || relay.Noredir || relay.Max <= 0 {
					continue
				}
				score := relay.Score()
				if score > winner {
					winner = score
					br.setCurrent(relay.Stream)
				}
			} else {
				return
			}
		}
	}
}

func (br *Balancer) start(ctx context.Context) error {
	go func() {
		for {
			select {
			case <-time.After(5 * time.Second):
				br.update(ctx)
			case <-ctx.Done():
				br.stop()
				return
			}
		}
	}()
	log.Println("balancer: listening on", br.serv.Addr)
	return br.serv.ListenAndServe()
}

func (br *Balancer) stop() error {
	return br.serv.Close()
}
