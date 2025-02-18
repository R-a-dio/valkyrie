package spotify

import (
	"context"
	"net/http"

	radio "github.com/R-a-dio/valkyrie"
	metadata "github.com/R-a-dio/valkyrie/metadata/registry"
)

const NAME = "spotify"

func init() {
	metadata.Register(NAME, &spotifyProvider{})
}

type spotifyProvider struct {
	c *http.Client
}

func (sp *spotifyProvider) Find(ctx context.Context, auth metadata.AuthString, s radio.Song) (*metadata.FindResult, error) {
	panic("todo")
}
