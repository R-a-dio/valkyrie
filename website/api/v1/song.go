package v1

import (
	"net/http"
	"os"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/util"
	"github.com/rs/zerolog/hlog"
)

func (a *API) GetSong(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	tid, err := radio.ParseTrackID(query.Get("id"))
	if err != nil {
		hlog.FromRequest(r).Error().Err(err)
		http.Error(w, "invalid or missing id", http.StatusBadRequest)
		return
	}

	key := query.Get("key")

	song, err := a.storage.Track(r.Context()).Get(tid)
	if err != nil {
		hlog.FromRequest(r).Error().Err(err)
		http.Error(w, "unknown id", http.StatusNotFound)
		return
	}

	if !a.songSecret.Equal(key, song.Hash[:]) {
		hlog.FromRequest(r).Error().Msg("wrong key for song download")
		http.Error(w, "invalid key", http.StatusUnauthorized)
		return
	}

	path := util.AbsolutePath(a.Config.Conf().MusicPath, song.FilePath)

	f, err := a.fs.Open(path)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.IsE(err, os.ErrNotExist) {
			status = http.StatusNotFound
		}
		hlog.FromRequest(r).Error().Err(err)
		http.Error(w, http.StatusText(status), status)
		return
	}
	defer f.Close()

	util.AddContentDispositionSong(w, song.Metadata, song.FilePath)
	http.ServeContent(w, r, "", time.Now(), f)
}
