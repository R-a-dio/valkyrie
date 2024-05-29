//go:build !nostreamer
// +build !nostreamer

package jobs

import (
	"context"

	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/storage"
	"github.com/R-a-dio/valkyrie/streamer/audio"
	"github.com/R-a-dio/valkyrie/util"
	"github.com/rs/zerolog"
)

func ExecuteVerifier(ctx context.Context, cfg config.Config) error {
	logger := zerolog.Ctx(ctx)

	store, err := storage.Open(ctx, cfg)
	if err != nil {
		return err
	}

	ts := store.Track(ctx)

	songs, err := ts.Unusable()
	if err != nil {
		return err
	}

	root := cfg.Conf().MusicPath
	for _, song := range songs {
		filename := util.AbsolutePath(root, song.FilePath)
		err := decodeFile(ctx, filename)
		if err != nil {
			logger.Error().
				Err(err).
				Uint64("track_id", uint64(song.TrackID)).
				Str("filename", filename).
				Msg("failed to decode file")
			continue
		}

		err = ts.UpdateUsable(song, 1)
		if err != nil {
			logger.Error().Err(err).Uint64("track_id", uint64(song.TrackID)).Msg("failed to verify")
			continue
		}

		logger.Info().Uint64("track_id", uint64(song.TrackID)).Msg("success")
	}

	return nil
}

func decodeFile(ctx context.Context, filename string) error {
	buf, err := audio.DecodeFile(filename)
	if err != nil {
		return err
	}
	return buf.Wait(ctx)
}
