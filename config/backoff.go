package config

import (
	"context"
	"time"

	"github.com/cenkalti/backoff/v4"
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
// for connection retrying
func NewConnectionBackoff(ctx context.Context) BackOff {
	b := backoff.NewExponentialBackOff(
		backoff.WithInitialInterval(initialInterval),
		backoff.WithMaxInterval(ConnectionRetryMaxInterval),
		backoff.WithMaxElapsedTime(ConnectionRetryMaxElapsedTime),
	)

	return _backoff{
		expo: b,
		ctx:  backoff.WithContext(b, ctx),
	}
}

type BackOff interface {
	backoff.BackOffContext
	GetElapsedTime() time.Duration
}

type _backoff struct {
	expo *backoff.ExponentialBackOff
	ctx  backoff.BackOffContext
}

// Context implements BackOff.
func (b _backoff) Context() context.Context {
	return b.ctx.Context()
}

// NextBackOff implements BackOff.
func (b _backoff) NextBackOff() time.Duration {
	return b.ctx.NextBackOff()
}

// Reset implements BackOff.
func (b _backoff) Reset() {
	b.ctx.Reset()
}

// GetElapsedTime returns the elapsed time since the
// last call to Reset
func (b _backoff) GetElapsedTime() time.Duration {
	return b.expo.GetElapsedTime()
}
