package metadata

import (
	"context"

	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/errors"

	_ "github.com/R-a-dio/valkyrie/metadata/registry/musicbrainz"
	// _ "github.com/R-a-dio/valkyrie/metadata/registry/spotify"
)

// Execute executes the metadata scraper with the context ctx and config cfg.
// Execution of the metadata scraper can be halted by cancelling ctx.
func Execute(ctx context.Context, cfg config.Config) error {
	const op errors.Op = "metadata/Execute"

	_ = cfg.Conf().Providers.Metadata

	return nil
}
