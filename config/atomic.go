package config

import (
	"fmt"
	"io"
	"sync/atomic"

	"github.com/BurntSushi/toml"
)

// Config is a type-safe wrapper around the config type
type Config struct {
	config *atomic.Value
}

// Load loads a configuration file from the reader given, it expects TOML as input
func Load(r io.Reader) (Config, error) {
	var c = defaultConfig
	m, err := toml.DecodeReader(r, &c)
	if err != nil {
		return Config{}, err
	}

	// print out keys that were found but don't have a destination
	// TODO: error when this happens?
	undec := m.Undecoded()
	if len(undec) > 0 {
		fmt.Println(undec)
	}

	var ac Config
	ac.StoreConf(c)

	return ac, nil
}

// Conf returns the configuration stored inside
//
// NOTE: Conf returns a shallow-copy of the config value stored inside; so do not edit
// 		 any slices or maps that might be inside
func (c Config) Conf() config {
	return c.config.Load().(config)
}

// StoreConf stores the configuration passed
func (c Config) StoreConf(new config) {
	c.config.Store(new)
}
