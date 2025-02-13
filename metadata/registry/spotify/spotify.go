package spotify

import (
	"context"

	radio "github.com/R-a-dio/valkyrie"
	metadata "github.com/R-a-dio/valkyrie/metadata/registry"
)

func init() {
	metadata.Register("spotify", &spotifyProvider{})
}

type spotifyProvider struct{}

func (sp *spotifyProvider) Find(ctx context.Context, auth metadata.AuthString, s radio.Song) (radio.TrackMetadata, error) {
	panic("todo")
}
