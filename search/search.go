package search

import (
	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/errors"
)

// OpenFn is a function that returns a SearchService configured with the config given
type OpenFn func(config.Config) (radio.SearchService, error)

var providers = map[string]OpenFn{}

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
	fn, ok := providers[name]
	if !ok {
		return nil, errors.E(op, errors.ProviderUnknown, errors.Info(name))
	}
	return fn(cfg)
}
