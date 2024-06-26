package jobs

import (
	"context"

	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/search"
	"github.com/R-a-dio/valkyrie/storage"
)

func ExecuteIndexSearch(ctx context.Context, cfg config.Config) error {
	s, err := storage.Open(ctx, cfg)
	if err != nil {
		return err
	}

	ss, err := search.Open(ctx, cfg)
	if err != nil {
		return err
	}

	songs, err := s.Track(ctx).All()
	if err != nil {
		return err
	}

	return ss.Update(ctx, songs...)
}
