package v1

import (
	"net/http"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/util"
	"github.com/rs/zerolog/hlog"
	"github.com/spf13/afero"
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
		http.Error(w, "invalid id", http.StatusNotFound)
		return
	}

	if !a.songSecret.Equal(key, song.Hash[:]) {
		hlog.FromRequest(r).Error().Msg("wrong key for song download")
		http.Error(w, "invalid key", http.StatusUnauthorized)
		return
	}

	path := util.AbsolutePath(a.Config.Conf().MusicPath, song.FilePath)
	http.ServeFileFS(w, r, afero.NewIOFS(a.fs), path)
}
