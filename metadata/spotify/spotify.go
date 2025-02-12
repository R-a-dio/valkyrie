package spotify

import (
	"context"

	radio "github.com/R-a-dio/valkyrie"
)

type spotifyProvider struct {
}

func (sp *spotifyProvider) Find(ctx context.Context, s radio.Song) radio.TrackMetadata {
	panic("todo")
}
