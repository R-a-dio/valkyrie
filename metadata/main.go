package metadata

import (
	"context"
	"fmt"

	radio "github.com/R-a-dio/valkyrie"
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

	var s radio.Song

	s.DatabaseTrack = new(radio.DatabaseTrack)

	s.DatabaseTrack.Title = "割れたリンゴ"

	res, err := reg.Search(ctx, s)
	if err != nil {
		return errors.E(op, err)
	}

	for _, r := range res {
		fmt.Printf("[%s] %s (%v)\n", r.ID, r.Title, r.Artists)
		for _, rel := range r.Releases {
			fmt.Printf("\t%s (%s)\n", rel.Name, rel.Date)
		}
	}

	return nil
}
