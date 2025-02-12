package metadata

import (
	"context"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/metadata/spotify"
)

type FindFn func(context.Context, radio.Song) (radio.TrackMetadata, error)

var providers = map[string]FindFn{}

func init() {
	Register("spotify", spotify.Find)
}

// Register registers a FindFn under the name given, it is not safe to
// call Register from multiple goroutines
//
// Register will panic if the name already exists
func Register(name string, fn FindFn) {
	if _, ok := providers[name]; ok {
		panic("finder already exists with name: " + name)
	}
	providers[name] = fn
}
