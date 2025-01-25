package v1

import (
	"fmt"
	"io"
	"net/http"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/streamer/audio"
	"github.com/R-a-dio/valkyrie/util"
	"github.com/R-a-dio/valkyrie/website/shared"
	"github.com/rs/zerolog"
)

func (a *API) GetSong(w http.ResponseWriter, r *http.Request) {
	const op errors.Op = "website/api/v1/API.GetSong"

	query := r.URL.Query()
	ctx := r.Context()

	tid, err := radio.ParseTrackID(query.Get("id"))
	if err != nil {
		a.errorHandler(w, r, errors.E(op, err, errors.InvalidForm, errors.Info("invalid or missing id")))
		return
	}

	key := query.Get("key")

	song, err := a.storage.Track(ctx).Get(tid)
	if err != nil {
		a.errorHandler(w, r, errors.E(op, shared.ErrNotFound, errors.Info("unknown id")))
		return
	}

	if !a.songSecret.Equal(key, song.Hash[:]) {
		a.errorHandler(w, r, errors.E(op, errors.InvalidForm, errors.Info("invalid or missing key")))
		return
	}

	path := util.AbsolutePath(a.Config.Conf().MusicPath, song.FilePath)

	f, err := a.fs.Open(path)
	if err != nil {
		a.errorHandler(w, r, fmt.Errorf("%w: %w", err, shared.ErrNotFound))
		return
	}
	defer f.Close()
	toSend := f

	wf, err := audio.WriteMetadata(ctx, f, *song)
	if err != nil {
		zerolog.Ctx(ctx).Error().Ctx(ctx).Err(err).Msg("failed to write metadata")
		// if writing metadata fails, just send the file as-is without the
		// metadata added in
		f.Seek(0, io.SeekStart)
	} else {
		defer wf.Close()
		toSend = wf
	}

	util.AddContentDispositionSong(w, song.Metadata, song.FilePath)
	http.ServeContent(w, r, "", time.Now(), toSend)
}
