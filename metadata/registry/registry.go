package registry

import (
	"context"
	"fmt"
	"maps"

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

type SearchResult struct {
	provider string
	radio.TrackMetadata
}

func (reg *registry) Search(ctx context.Context, s radio.Song) ([]*SearchResult, error) {
	const op errors.Op = "registry/Registry.Search"
	srs := []*SearchResult{}

	for name, provider := range reg.providers {
		res, err := provider.Find(ctx, provider.AuthString, s)
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

func NewRegistry(cfg config.Config) (*registry, error) {
	const op errors.Op = "registry/NewRegistry"
	reg := new(registry)

	pvds := make(map[string]*authedProvider)

	fmt.Println(cfg.Conf().Metadata)

	if len(cfg.Conf().Metadata) == 0 {
		return nil, errors.E(op, fmt.Errorf("no metadata providers found"))
	}

	for _, metadata := range cfg.Conf().Metadata {
		provider, ok := providers[metadata.Name]
		if !ok {
			return nil, errors.E(op, fmt.Errorf("metadata provider %q not found, expected any of %v", metadata.Name, maps.Keys(providers)))
		}
		ap := &authedProvider{AuthString(metadata.Auth), provider}

		pvds[metadata.Name] = ap
	}

	reg.providers = pvds
	return reg, nil
}

func (reg *registry) String() string {
	return fmt.Sprintf("registry providers: %v", reg.providers)
}
