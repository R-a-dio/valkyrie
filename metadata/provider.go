package metadata

import (
	"context"

	radio "github.com/R-a-dio/valkyrie"
)

type Provider interface {
	Find(context.Context, radio.Song) radio.TrackMetadata
}
