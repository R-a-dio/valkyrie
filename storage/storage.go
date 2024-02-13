package storage

import (
	"context"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/migrations"
	"github.com/R-a-dio/valkyrie/search"
	"github.com/R-a-dio/valkyrie/storage/mariadb"
	"github.com/rs/zerolog"
)

// OpenFn is a function that returns a StorageService configured with the config given
type OpenFn func(context.Context, config.Config) (radio.StorageService, error)

var providers = map[string]OpenFn{}

func init() {
	Register("mariadb", mariadb.Connect)
}

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

// Open returns a radio.StorageService as configured by the config given
func Open(ctx context.Context, cfg config.Config) (radio.StorageService, error) {
	const op errors.Op = "storage/Open"

	name := cfg.Conf().Providers.Storage

	fn, ok := providers[name]
	if !ok {
		return nil, errors.E(op, errors.ProviderUnknown, errors.Info(name))
	}

	zerolog.Ctx(ctx).Info().Str("provider", name).Msg("creating new StorageService")
	store, err := fn(ctx, cfg)
	if err != nil {
		return nil, errors.E(op, err)
	}

	// check if the storage provider has migrations
	err = migrations.CheckVersion(ctx, cfg)
	if err != nil && !errors.Is(errors.NoMigrations, err) {
		return nil, errors.E(op, err)
	}

	// we optionally wrap the storage service into a special implementation of the
	// search package that updates the configured search engine whenever a document
	// that is in an index is updated
	if !search.NeedsWrap(cfg) {
		return store, nil
	}

	ss, err := search.Open(ctx, cfg)
	if err != nil {
		return nil, errors.E(op, err)
	}

	store = search.WrapStorageService(ss, store)
	return store, nil
}
