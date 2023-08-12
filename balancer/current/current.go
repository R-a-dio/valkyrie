package current

import "sync/atomic"

// Current is a type-safe wrapper around an atomic.Value.
type Current struct {
	value atomic.Value
}

// NewCurrent returns an initialized Current with str.
func NewCurrent(str string) *Current {
	c := new(Current)
	c.value.Store(str)
	return c
}

// Get returns the underlying string value.
func (c *Current) Get() string {
	return c.value.Load().(string)
}

// Set sets the underlying string value.
func (c *Current) Set(str string) {
	c.value.Store(str)
}
