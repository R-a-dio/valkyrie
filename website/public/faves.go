package public

import (
	"encoding/json"
	"html/template"
	"net/http"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/util"
	"github.com/R-a-dio/valkyrie/website/middleware"
	"github.com/R-a-dio/valkyrie/website/shared"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/csrf"
)

const favesPageSize = 100

type FavesInput struct {
	middleware.Input
	CSRFTokenInput  template.HTML
	Nickname        string
	CanRequest      bool
	RequestCooldown time.Duration
	DownloadURL     string
	Faves           []radio.Song
	FaveCount       int64
	Page            *shared.Pagination

	// IsError indicates if the message given is an error
	IsError bool
	// Message to show at the top of the page
	Message string
}

func (FavesInput) TemplateBundle() string {
	return "faves"
}

func NewFavesInput(ss radio.SongStorage, rs radio.RequestStorage, r *http.Request, requestDelay time.Duration) (*FavesInput, error) {
	const op errors.Op = "website/public.NewFavesInput"

	page, offset, err := getPageOffset(r, favesPageSize)
	if err != nil {
		return nil, errors.E(op, err)
	}

	// we support both ?nick=<nick> and /faves/<nick> so we need to see which one we have
	// try the old format first, and then the GET parameter
	nickname := chi.URLParam(r, "Nick")
	if nickname == "" {
		nickname = r.FormValue("nick")
	}

	var faves []radio.Song
	var faveCount int64
	if nickname != "" { // only ask for faves if we have a nickname
		faves, faveCount, err = ss.FavoritesOf(nickname, favesPageSize, offset)
		if err != nil {
			return nil, errors.E(op, err)
		}
	}

	// create download URL
	q := r.URL.Query()
	q.Set("dl", "true")
	dlUrl := *r.URL
	dlUrl.RawQuery = q.Encode()

	// we also use this input if we're making a song request, in which case our url
	// will be something other than /faves that we can't use for the pagination
	// logic.
	r.URL.Path = "/faves"

	// check if the user can request
	lastRequest, err := rs.LastRequest(r.RemoteAddr)
	if err != nil {
		return nil, errors.E(op, err)
	}
	cooldown, canRequest := radio.CalculateCooldown(requestDelay, lastRequest)

	return &FavesInput{
		Nickname:        nickname,
		CanRequest:      canRequest,
		RequestCooldown: cooldown,
		DownloadURL:     dlUrl.String(),
		Faves:           faves,
		FaveCount:       faveCount,
		Page: shared.NewPagination(
			page, shared.PageCount(int64(faveCount), favesPageSize),
			r.URL,
		),
		Input:          middleware.InputFromRequest(r),
		CSRFTokenInput: csrf.TemplateField(r),
	}, nil
}

// GetFavesOld handles the old URL format we used which is /faves/<nick> but supporting
// that everywhere is annoying so we just redirect to the new url instead
func (s *State) GetFavesOld(w http.ResponseWriter, r *http.Request) {
	nickname := chi.URLParam(r, "Nick")
	if nickname == "" {
		// no nickname shouldn't ever happen, but if it does we just give back the
		// normal faves page
		s.GetFaves(w, r)
		return
	}

	q := r.URL.Query()
	q.Set("nick", nickname)
	r.URL.RawQuery = q.Encode()
	r.URL.Path = "/faves"

	http.Redirect(w, r, r.URL.String(), http.StatusMovedPermanently)
}

func (s *State) GetFaves(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	input, err := NewFavesInput(
		s.Storage.Song(ctx),
		s.Storage.Request(ctx),
		r,
		s.Config.UserRequestDelay(),
	)
	if err != nil {
		s.errorHandler(w, r, err)
		return
	}

	// we have an old API that returns your faves as JSON if you use dl=true
	// so we need to support that for old users
	if r.FormValue("dl") != "" {
		w.Header().Set("Content-Type", "application/json")
		util.AddContentDisposition(w, input.Nickname+"_faves.json")
		err := json.NewEncoder(w).Encode(NewFaveDownload(input.Faves))
		if err != nil {
			s.errorHandler(w, r, err)
			return
		}
		return
	}

	err = s.Templates.Execute(w, r, input)
	if err != nil {
		s.errorHandler(w, r, err)
		return
	}
}

func (s *State) PostFaves(w http.ResponseWriter, r *http.Request) {
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

	entry.Metadata = song.Metadata
	return entry
}

func NewFaveDownload(songs []radio.Song) []FaveDownloadEntry {
	res := make([]FaveDownloadEntry, 0, len(songs))
	for _, song := range songs {
		res = append(res, NewFaveDownloadEntry(song))
	}
	return res
}
