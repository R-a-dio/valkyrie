package spotify

import (
	"context"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	metadata "github.com/R-a-dio/valkyrie/metadata/registry"
)

func init() {
	metadata.Register("spotify", &spotifyProvider{})
}

type spotifyProvider struct{}

func (sp *spotifyProvider) Find(ctx context.Context, cfg config.Config, s radio.Song) (radio.TrackMetadata, error) {
	panic("todo")
}
