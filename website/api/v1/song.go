package v1

import (
	"fmt"
	"net/http"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/util"
	"github.com/R-a-dio/valkyrie/website/shared"
)

func (a *API) GetSong(w http.ResponseWriter, r *http.Request) {
	const op errors.Op = "website/api/v1/API.GetSong"

	query := r.URL.Query()

	tid, err := radio.ParseTrackID(query.Get("id"))
	if err != nil {
		a.errorHandler(w, r, errors.E(op, errors.InvalidForm, errors.Info("invalid or missing id")))
		return
	}

	key := query.Get("key")

	song, err := a.storage.Track(r.Context()).Get(tid)
	if err != nil {
		a.errorHandler(w, r, shared.ErrNotFound)
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

	util.AddContentDispositionSong(w, song.Metadata, song.FilePath)
	http.ServeContent(w, r, "", time.Now(), f)
}
