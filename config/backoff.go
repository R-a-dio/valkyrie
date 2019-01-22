package config

import (
	"time"

	"github.com/cenkalti/backoff"
)

const initialInterval = time.Millisecond * 250

const (
	// ConnectionRetryMaxInterval indicates the maximum interval to retry icecast
	// connections
	ConnectionRetryMaxInterval = time.Second * 2
	// ConnectionRetryMaxElapsedTime indicates how long to try retry before
	// erroring out completely. Set to 0 means it never errors out
	ConnectionRetryMaxElapsedTime = time.Second * 0
)

// NewConnectionBackoff returns a new backoff set to the intended configuration
// for local connection retrying, for connections going to non-local addresses
// don't use this
func NewConnectionBackoff() *backoff.ExponentialBackOff {
	b := &backoff.ExponentialBackOff{
		RandomizationFactor: backoff.DefaultRandomizationFactor,
		Multiplier:          backoff.DefaultMultiplier,
		Clock:               backoff.SystemClock,
		// fields we use non-default
		InitialInterval: initialInterval,
		MaxInterval:     ConnectionRetryMaxInterval,
		MaxElapsedTime:  ConnectionRetryMaxElapsedTime,
	}
	b.Reset()
	return b
}

const (
	// DatabaseRetryMaxInterval indicates the maximum interval between database
	// call retries after an error occurs
	DatabaseRetryMaxInterval = time.Second * 5
	// DatabaseRetryMaxElapsedTime indicates how long to try again before
	// erroring out. Set to 0 means it never errors out
	DatabaseRetryMaxElapsedTime = time.Second * 0
)

// NewDatabaseBackoff returns a new backoff set to the intended configuration
// for database retrying
func NewDatabaseBackoff() backoff.BackOff {
	b := &backoff.ExponentialBackOff{
		RandomizationFactor: backoff.DefaultRandomizationFactor,
		Multiplier:          backoff.DefaultMultiplier,
		Clock:               backoff.SystemClock,
		// fields we use non-default
		InitialInterval: initialInterval,
		MaxInterval:     DatabaseRetryMaxInterval,
		MaxElapsedTime:  DatabaseRetryMaxElapsedTime,
	}
	b.Reset()
	return b
}
