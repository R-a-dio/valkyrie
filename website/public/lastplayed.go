package public

import (
	"net/http"
	"strconv"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/website/middleware"
	"github.com/R-a-dio/valkyrie/website/shared"
	"github.com/rs/zerolog/hlog"
)

const (
	lastplayedSize = 20
)

type LastPlayedInput struct {
	middleware.Input

	Songs []radio.Song
	Page  *shared.FromPagination[radio.LastPlayedKey]
}

func (LastPlayedInput) TemplateBundle() string {
	return "lastplayed"
}

func NewLastPlayedInput(s radio.SongStorageService, r *http.Request) (*LastPlayedInput, error) {
	const op errors.Op = "website/public.NewLastPlayedInput"

	key, page, err := getPageFrom(r)
	if err != nil {
		// this means someone is inputting weird stuff manually almost certainly
		// but log it anyway, we then proceed with the defaults that getPageFrom
		// returns for us
		hlog.FromRequest(r).Warn().Ctx(r.Context()).Msg("weird form, proceeding with defaults")
	}

	ss := s.Song(r.Context())
	songs, err := ss.LastPlayed(key, lastplayedSize)
	if err != nil {
		return nil, errors.E(op, err)
	}

	prev, next, err := ss.LastPlayedPagination(key, lastplayedSize, 5)
	if err != nil {
		return nil, errors.E(op, err)
	}

	pagination := shared.NewFromPagination(key, prev, next, r.URL).WithPage(page)

	return &LastPlayedInput{
		Input: middleware.InputFromRequest(r),
		Songs: songs,
		Page:  pagination,
	}, nil
}

func (s *State) getLastPlayed(w http.ResponseWriter, r *http.Request) error {
	input, err := NewLastPlayedInput(s.Storage, r)
	if err != nil {
		return err
	}

	return s.Templates.Execute(w, r, input)
}

func (s *State) GetLastPlayed(w http.ResponseWriter, r *http.Request) {
	err := s.getLastPlayed(w, r)
	if err != nil {
		s.errorHandler(w, r, err)
		return
	}
}

func getPageFrom(r *http.Request) (radio.LastPlayedKey, int, error) {
	var key = radio.LPKeyLast
	var page int = 1

	if rawPage := r.FormValue("page"); rawPage != "" {
		parsedPage, err := strconv.Atoi(rawPage)
		if err != nil {
			return key, page, errors.E(err, errors.InvalidForm)
		}
		page = parsedPage
	}

	rawFrom := r.FormValue("from")
	if rawFrom == "" {
		return key, page, nil
	}

	parsedFrom, err := strconv.ParseUint(rawFrom, 10, 32)
	if err != nil {
		return key, page, errors.E(err, errors.InvalidForm)
	}
	key = radio.LastPlayedKey(parsedFrom)

	return key, page, nil
}
func getPageOffset(r *http.Request, pageSize int64) (int64, int64, error) {
	var page int64 = 1
	{
		rawPage := r.FormValue("page")
		if rawPage == "" {
			return page, 0, nil
		}
		parsedPage, err := strconv.ParseInt(rawPage, 10, 0)
		if err != nil {
			return page, 0, errors.E(err, errors.InvalidForm)
		}
		page = parsedPage
	}
	var offset = (page - 1) * pageSize
	if offset < 0 {
		offset = 0
	}
	return page, offset, nil
}
