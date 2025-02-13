package registry

import (
	"context"
	"fmt"
	"maps"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/errors"
)

type Provider interface {
	Find(context.Context, config.Config, radio.Song) (radio.TrackMetadata, error)
}

type registry struct {
	providers map[string]Provider

	cfg config.Config
}

type SearchResult struct {
	provider string
	radio.TrackMetadata
}

func (reg *registry) Search(ctx context.Context, s radio.Song) ([]*SearchResult, error) {
	const op errors.Op = "metadata/Registry.Search"
	srs := []*SearchResult{}

	for name, provider := range reg.providers {
		res, err := provider.Find(ctx, reg.cfg, s)
		if err != nil {
			return nil, errors.E()
		}

		sr := &SearchResult{
			provider:      name,
			TrackMetadata: res,
		}

		srs = append(srs, sr)
	}

	return srs, nil
}

var providers = map[string]Provider{}

// Register registers a Provider under the name given, it is not safe to
// call Register from multiple goroutines
//
// Register will panic if the name already exists
func Register(name string, pvd Provider) {
	if _, ok := providers[name]; ok {
		panic("provider already exists with name: " + name)
	}
	providers[name] = pvd
}

func NewRegistry(cfg config.Config) (*registry, error) {
	const op errors.Op = "metadata/NewRegistry"
	reg := new(registry)

	reg.providers = providers
	reg.cfg = cfg

	for _, provider := range cfg.Conf().Providers.Metadata {
		_, ok := providers[provider]
		if !ok {
			return nil, errors.E(op, fmt.Errorf("invalid provider %q, expected any of %v", provider, maps.Keys(providers)))
		}
	}

	return reg, nil
}

func (reg *registry) String() string {
	return fmt.Sprintf("registry providers: %v", reg.providers)
}
