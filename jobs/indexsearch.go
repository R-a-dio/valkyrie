package jobs

import (
	"context"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/search"
	"github.com/R-a-dio/valkyrie/storage"
	"github.com/rs/zerolog"
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

	var songs []radio.Song
	songs, err = s.Track(ctx).All()
	if err != nil {
		return err
	}

	zerolog.Ctx(ctx).Info().Ctx(ctx).Int("amount", len(songs)).Msg("start indexing songs")
	now := time.Now()
	err = ss.Update(ctx, songs...)
	zerolog.Ctx(ctx).Info().Ctx(ctx).Dur("took", time.Since(now)).Msg("finished indexing songs")
	if err != nil {
		return err
	}
	return nil
}
