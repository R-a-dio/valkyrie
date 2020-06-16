package balancer

import (
	"context"
	"io/ioutil"
	"log"
	"net/http"
	"sync"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/balancer/current"
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
	c *current.Current

	// The amount of listeners from every relay.
	listeners int
}

// health checks the status of r using c, returning a copy of r.
func health(ctx context.Context, c *http.Client, r radio.Relay) radio.Relay {
	res := r
	res.Online, res.Listeners, res.Err = false, 0, ""

	req, err := http.NewRequestWithContext(ctx, "GET", r.Status, nil)
	if err != nil {
		res.Err = err.Error()
		return res
	}
	resp, err := c.Do(req)
	if err != nil {
		res.Err = err.Error()
		return res
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		res.Err = err.Error()
		return res
	}
	l, err := parsexml(body)
	if err != nil {
		res.Err = err.Error()
		return res
	}
	res.Online, res.Listeners = true, l
	return res
}

// checker receives relays on in and sends updated relays on out.
// the execution of checker can be halted by cancelling ctx.
func checker(ctx context.Context, in, out chan radio.Relay) {
	var wg sync.WaitGroup
	c := &http.Client{
		Timeout: 3 * time.Second,
	}
	for {
		select {
		case <-ctx.Done():
			return
		case relay, ok := <-in:
			if !ok {
				wg.Wait()
				close(out)
				return
			}
			wg.Add(1)
			go func(r radio.Relay) {
				defer wg.Done()
				out <- health(ctx, c, relay)
			}(relay)
		}
	}
}

// update checks all relays and sets the current relay to re-direct to.
// update also accumulates listeners from each relay.
func (br *Balancer) update(ctx context.Context) {
	relays, err := br.storage.Relay(ctx).All()
	if err != nil {
		if errors.Is(errors.NoRelays, err) {
			log.Println("balancer: no relays in database")
			return
		}
		log.Println("balancer: error getting relays:", err)
		return
	}
	// we already know that len(relays) != 0, so sends are non-blocking.
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

	// we assume the worst relay is the fallback, which has a score of zero.
	var winner radio.Relay
	winner.Stream = br.Conf().Balancer.Fallback
	br.listeners = 0
	for {
		select {
		case <-ctx.Done():
			return
		case relay, ok := <-out:
			if !ok {
				br.c.Set(winner.Stream)
				return
			}
			err := br.storage.Relay(ctx).Update(relay)
			if err != nil {
				log.Printf("balancer: error updating relay %s: %s\n", relay.Name, err)
				continue
			}

			if !relay.Online || relay.Disabled || relay.Noredir || relay.Max <= 0 {
				continue
			}

			br.listeners += relay.Listeners
			score := relay.Score()
			if score > winner.Score() {
				winner = relay
			}
		}
	}
}

func (br *Balancer) start(ctx context.Context) error {
	const op errors.Op = "balancer/start"

	go func() {
		for {
			select {
			case <-time.After(5 * time.Second):
				br.update(ctx)
				err := br.manager.UpdateListeners(ctx, br.listeners)
				if err != nil {
					log.Printf("balancer: error updating listeners: %s", err)
				}
			case <-ctx.Done():
				br.stop(ctx)
				return
			}
		}
	}()
	log.Println("balancer: listening on", br.serv.Addr)
	return errors.E(op, br.serv.ListenAndServe())
}

func (br *Balancer) stop(ctx context.Context) error {
	const op errors.Op = "balancer/stop"

	err := br.serv.Shutdown(ctx)
	if err != nil {
		return errors.E(op, err)
	}

	return nil
}
