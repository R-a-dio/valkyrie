package search

import (
	"context"
	"sync"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/rs/zerolog"
)

// OpenFn is a function that returns a SearchService configured with the config given
type OpenFn func(context.Context, config.Config) (radio.SearchService, error)

var providers = map[string]provider{}
var instancesMu sync.Mutex
var instances = map[string]radio.SearchService{}

type provider struct {
	fn        OpenFn
	needsWrap bool
}

// Register registers an OpenFn under the name given, it is not safe to
// call Register from multiple goroutines
//
// Register will panic if the name already exists
func Register(name string, needsWrap bool, fn OpenFn) {
	if _, ok := providers[name]; ok {
		panic("search provider already exists with name: " + name)
	}
	providers[name] = provider{fn, needsWrap}
}

// Open returns a radio.SearchService as configured by the config given
func Open(ctx context.Context, cfg config.Config) (radio.SearchService, error) {
	const op errors.Op = "search/Open"

	name := cfg.Conf().Providers.Search
	instancesMu.Lock()
	defer instancesMu.Unlock()

	// see if there is already an instance available
	ss, ok := instances[name]
	if ok {
		zerolog.Ctx(ctx).Info().Str("provider", name).Msg("re-using existing SearchService")
		return ss, nil
	}

	// otherwise create a new one
	p, ok := providers[name]
	if !ok {
		return nil, errors.E(op, errors.ProviderUnknown, errors.Info(name))
	}

	zerolog.Ctx(ctx).Info().Str("provider", name).Msg("creating new SearchService")
	ss, err := p.fn(ctx, cfg)
	if err != nil {
		return nil, errors.E(op, err)
	}

	instances[name] = ss
	return ss, nil
}

func NeedsWrap(cfg config.Config) bool {
	p, ok := providers[cfg.Conf().Providers.Search]
	if !ok {
		return false
	}
	return p.needsWrap
}
