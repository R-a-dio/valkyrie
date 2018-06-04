package config

import (
	"time"

	"github.com/cenkalti/backoff"
)

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
func NewConnectionBackoff() backoff.BackOff {
	b := backoff.NewExponentialBackOff()
	b.InitialInterval = time.Millisecond * 250
	b.MaxInterval = ConnectionRetryMaxInterval
	b.MaxElapsedTime = ConnectionRetryMaxElapsedTime
	return b
}
