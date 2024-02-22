package public

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/website/middleware"
	"github.com/R-a-dio/valkyrie/website/shared"
	"github.com/go-chi/chi/v5"
)

const favesPageSize = 100

type FavesInput struct {
	middleware.Input
	Nickname    string
	DownloadURL string
	Faves       []radio.Song
	Page        *shared.Pagination
}

func (FavesInput) TemplateBundle() string {
	return "faves"
}

func NewFavesInput(ss radio.SongStorage, r *http.Request) (*FavesInput, error) {
	page, offset, err := getPageOffset(r, favesPageSize)
	if err != nil {
		return nil, err
	}

	// we support both ?nick=<nick> and /faves/<nick> so we need to see which one we have
	// try the old format first, and then the GET parameter
	nickname := chi.URLParam(r, "Nick")
	if nickname == "" {
		nickname = r.FormValue("nick")
	}
	nickname = html.EscapeString(nickname)

	faves, err := ss.FavoritesOf(nickname, favesPageSize, offset)
	if err != nil {
		return nil, err
	}

	q := r.URL.Query()
	q.Set("dl", "true")
	dlUrl := *r.URL
	dlUrl.RawQuery = q.Encode()

	return &FavesInput{
		Nickname:    nickname,
		DownloadURL: dlUrl.String(),
		Faves:       faves,
		Page: shared.NewPagination(
			page, shared.PageCount(int64(len(faves)), favesPageSize),
			r.URL,
		),
		Input: middleware.InputFromRequest(r),
	}, nil
}

func (s State) GetFaves(w http.ResponseWriter, r *http.Request) {
	input, err := NewFavesInput(s.Storage.Song(r.Context()), r)
	if err != nil {
		s.errorHandler(w, r, err)
		return
	}

	// we have an old API that returns your faves as JSON if you use dl=true
	// so we need to support that for old users
	if r.FormValue("dl") != "" {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s_faves.json", input.Nickname))
		err := json.NewEncoder(w).Encode(NewFaveDownload(input.Faves))
		if err != nil {
			s.errorHandler(w, r, err)
		}
		return
	}

	err = s.Templates.Execute(w, r, input)
	if err != nil {
		s.errorHandler(w, r, err)
		return
	}
}

func (s State) PostFaves(w http.ResponseWriter, r *http.Request) {
}

type FaveDownloadEntry struct {
	ID            *radio.TrackID `json:"tracks_id"`
	Metadata      string         `json:"meta"`
	LastRequested *int64         `json:"lastrequested"`
	LastPlayed    *int64         `json:"lastplayed"`
	RequestCount  *int           `json:"requestcount"`
}

func NewFaveDownloadEntry(song radio.Song) FaveDownloadEntry {
	var entry FaveDownloadEntry

	if song.HasTrack() {
		if song.TrackID > 0 {
			entry.ID = &song.TrackID
		}
		if !song.LastRequested.IsZero() {
			tmp := song.LastRequested.Unix()
			entry.LastRequested = &tmp
		}
		entry.RequestCount = &song.RequestCount
	}

	if !song.LastPlayed.IsZero() {
		tmp := song.LastPlayed.Unix()
		entry.LastPlayed = &tmp
	}

	return entry
}

func NewFaveDownload(songs []radio.Song) []FaveDownloadEntry {
	res := make([]FaveDownloadEntry, 0, len(songs))
	for _, song := range songs {
		res = append(res, NewFaveDownloadEntry(song))
	}
	return res
}
