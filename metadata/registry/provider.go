package registry

import (
	"context"

	radio "github.com/R-a-dio/valkyrie"
)

type AuthString string

type Provider interface {
	Find(context.Context, AuthString, radio.Song) (*FindResult, error)
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
