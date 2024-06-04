package jobs

import (
	"context"

	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/storage"
	"github.com/rs/zerolog"
)

func ExecuteTracksHash(ctx context.Context, cfg config.Config) error {
	store, err := storage.Open(ctx, cfg)
	if err != nil {
		return err
	}

	songs, err := store.Track(ctx).All()
	if err != nil {
		return err
	}

	for _, song := range songs {
		current := song.Hash

		// recalculate the hash
		song.Hydrate()

		// compare it to what we had previously
		if current != song.Hash {
			zerolog.Ctx(ctx).Info().
				Int64("trackid", int64(song.TrackID)).
				Str("metadata", song.Metadata).
				Msg("mismatched hash")
		}
	}

	return nil
}
