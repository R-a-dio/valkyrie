package jobs

import (
	"context"
	"strings"

	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/storage"
	"github.com/rs/zerolog"
)

func ExecuteTracksHash(ctx context.Context, cfg config.Config) error {
	store, err := storage.Open(ctx, cfg)
	if err != nil {
		return err
	}

	songs, err := store.Track(ctx).AllRaw()
	if err != nil {
		return err
	}

	for _, song := range songs {
		current := song.Hash

		// recalculate the hash
		song.Hydrate()

		// compare it to what we had previously
		if current == song.Hash {
			// no difference, continue with next
			continue
		}

		zerolog.Ctx(ctx).Info().
			Int64("trackid", int64(song.TrackID)).
			Str("metadata", song.Metadata).
			Msg("mismatched hash")

		// trim whitespace from artist and title, since those are the cause
		// of the mismatched hash
		song.Artist = strings.TrimSpace(song.Artist)
		song.Title = strings.TrimSpace(song.Title)
		// and then update the track
		err := store.Track(ctx).UpdateMetadata(song)
		if err != nil {
			zerolog.Ctx(ctx).Error().Ctx(ctx).Err(err).Msg("failed to update metadata")
		}
	}

	return nil
}
