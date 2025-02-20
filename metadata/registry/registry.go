package registry

import (
	"context"
	"fmt"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/errors"
)

type authedProvider struct {
	AuthString
	Provider
}

type registry struct {
	providers map[string]*authedProvider
}

type FindResult struct {
	Provider string
	Info
}

type Info struct {
	ID       string
	Title    string
	Artists  []string
	Releases []*Release
	URLs     []string
}

type Release struct {
	ID   string
	Name string
	Art  map[string][]byte
	Date string
}

type Art struct {
	Format string
	Data   []byte
}

func (reg *registry) Search(ctx context.Context, s radio.Song) ([]*FindResult, error) {
	const op errors.Op = "registry/Registry.Search"
	srs := []*FindResult{}

	for _, provider := range reg.providers {
		sr, err := provider.Find(ctx, provider.AuthString, s)
		if err != nil {
			return nil, errors.E()
		}

		srs = append(srs, sr...)
	}

	return srs, nil
}

func NewRegistry(cfg config.Config) (*registry, error) {
	const op errors.Op = "registry/NewRegistry"
	reg := new(registry)

	pvds := make(map[string]*authedProvider)

	if len(cfg.Conf().Metadata) == 0 {
		return nil, errors.E(op, fmt.Errorf("no metadata providers found"))
	}

	for _, metadata := range cfg.Conf().Metadata {
		provider, ok := providers[metadata.Name]
		if !ok {
			return nil, errors.E(op, fmt.Errorf("metadata provider %q not found, expected any of: %v", metadata.Name, mapKeys(providers)))
		}
		ap := &authedProvider{AuthString(metadata.Auth), provider}

		pvds[metadata.Name] = ap
	}

	reg.providers = pvds
	return reg, nil
}

func mapKeys[K comparable, V any](m map[K]V) []K {
	r := []K{}
	for k := range m {
		r = append(r, k)
	}

	return r
}

func (reg *registry) String() string {
	return fmt.Sprintf("providers: %v", mapKeys(reg.providers))
}
