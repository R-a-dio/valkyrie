package search

import (
	"log"
	"sync"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/errors"
)

// OpenFn is a function that returns a SearchService configured with the config given
type OpenFn func(config.Config) (radio.SearchService, error)

var providers = map[string]OpenFn{}
var instancesMu sync.Mutex
var instances = map[string]radio.SearchService{}

// Register registers an OpenFn under the name given, it is not safe to
// call Register from multiple goroutines
//
// Register will panic if the name already exists
func Register(name string, fn OpenFn) {
	if _, ok := providers[name]; ok {
		panic("search provider already exists with name: " + name)
	}
	providers[name] = fn
}

// Open returns a radio.SearchService as configured by the config given
func Open(cfg config.Config) (radio.SearchService, error) {
	const op errors.Op = "search/Open"

	name := cfg.Conf().Providers.Search
	instancesMu.Lock()
	defer instancesMu.Unlock()

	// see if there is already an instance available
	ss, ok := instances[name]
	if ok {
		log.Printf("search: re-using existing SearchService instance for %s", name)
		return ss, nil
	}

	// otherwise create a new one
	fn, ok := providers[name]
	if !ok {
		return nil, errors.E(op, errors.ProviderUnknown, errors.Info(name))
	}

	log.Printf("search: creating new SearchService instance for %s", name)
	ss, err := fn(cfg)
	if err != nil {
		return nil, errors.E(op, err)
	}

	instances[name] = ss
	return ss, nil
}
