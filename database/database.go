package database

import (
	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/errors"
)

// OpenFn is a function that returns a StorageService configured with the config given
type OpenFn func(config.Config) (radio.StorageService, error)

var storages = map[string]OpenFn{}

// Register registers an OpenFn under the name given, it is not safe to
// call Register from multiple goroutines
//
// Register will panic if the name already exists
func Register(name string, fn OpenFn) {
	if _, ok := storages[name]; ok {
		panic("storage already exists with name: " + name)
	}
	storages[name] = fn
}

// Open returns a radio.StorageService as configured by the config given
func Open(cfg config.Config) (radio.StorageService, error) {
	const op errors.Op = "storage/Open"

	name := cfg.Conf().Storage.Name
	fn, ok := storages[name]
	if !ok {
		return nil, errors.E(op, errors.StorageUnknown, errors.Info(name))
	}
	return fn(cfg)
}
