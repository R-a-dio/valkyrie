package metadata

import (
	"context"
	"fmt"

	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/errors"

	"github.com/R-a-dio/valkyrie/metadata/registry"
	_ "github.com/R-a-dio/valkyrie/metadata/registry/musicbrainz"
	// _ "github.com/R-a-dio/valkyrie/metadata/registry/spotify"
)

// Execute executes the metadata scraper with the context ctx and config cfg.
// Execution of the metadata scraper can be halted by cancelling ctx.
func Execute(ctx context.Context, cfg config.Config) error {
	const op errors.Op = "metadata/Execute"

	reg, err := registry.NewRegistry(cfg)
	if err != nil {
		return errors.E(op, err)
	}

	fmt.Println(reg)

	return nil
}
