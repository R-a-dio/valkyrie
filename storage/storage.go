package storage

import (
	"log"
	"strings"
	"sync"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/search"
)

// OpenFn is a function that returns a StorageService configured with the config given
type OpenFn func(config.Config) (radio.StorageService, error)

var providers = map[string]OpenFn{}
var instancesMu sync.Mutex
var instances = map[string]radio.StorageService{}

// Register registers an OpenFn under the name given, it is not safe to
// call Register from multiple goroutines
//
// Register will panic if the name already exists
func Register(name string, fn OpenFn) {
	if _, ok := providers[name]; ok {
		panic("storage already exists with name: " + name)
	}
	providers[name] = fn
}

const configPrefix = "index-"

// Open returns a radio.StorageService as configured by the config given
func Open(cfg config.Config) (radio.StorageService, error) {
	const op errors.Op = "storage/Open"

	// we optionally wrap the storage service into a special implementation of the
	// search package that updates the configured search engine whenever a document
	// that is in an index is updated, this is done by prepending the storage provider
	// name with `search-`
	var searchWrapped bool

	name := cfg.Conf().Providers.Storage
	if strings.HasPrefix(name, configPrefix) {
		searchWrapped = true
		name = name[len(configPrefix):]
	}

	instancesMu.Lock()
	defer instancesMu.Unlock()
	// see if there is already an instance available
	store, ok := instances[name]
	if ok {
		log.Printf("storage: re-using existing StorageService instance for %s", name)
		return store, nil
	}

	fn, ok := providers[name]
	if !ok {
		return nil, errors.E(op, errors.ProviderUnknown, errors.Info(name))
	}

	log.Printf("storage: creating new StorageService instance for %s", name)
	store, err := fn(cfg)
	if err != nil {
		return nil, errors.E(op, err)
	}

	// no search wrapping
	if !searchWrapped {
		return store, nil
	}

	ss, err := search.Open(cfg)
	if err != nil {
		return nil, errors.E(op, err)
	}

	store = search.WrapStorageService(ss, store)
	instances[name] = store
	return store, nil
}
