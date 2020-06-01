// +build !nostreamer

package jobs

import (
	"context"
	"log"
	"path/filepath"

	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/storage"
	"github.com/R-a-dio/valkyrie/streamer/audio"
)

func ExecuteVerifier(ctx context.Context, cfg config.Config) error {
	store, err := storage.Open(cfg)
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
		filename := filepath.Join(root, song.FilePath)
		err := decodeFile(filename)
		if err != nil {
			if err, ok := err.(*audio.DecodeError); ok {
				log.Printf("verify: failed to decode file: (%d) %s: %s\n %s",
					song.TrackID, filename, err.Err, err.ExtraInfo)
			} else {
				log.Printf("verify: failed to decode file: (%d) %s: %s",
					song.TrackID, filename, err)
			}
			continue
		}

		err = ts.UpdateUsable(song, 1)
		if err != nil {
			log.Printf("verify: failed to mark as usable: (%d): %s", song.TrackID, err)
			continue
		}

		log.Printf("verify: success: (%d) %s", song.TrackID, song.Metadata)
	}

	return nil
}

func decodeFile(filename string) error {
	buf, err := audio.DecodeFile(filename)
	if err != nil {
		return err
	}
	return buf.Wait()
}
