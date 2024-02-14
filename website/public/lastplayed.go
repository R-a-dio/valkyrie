package public

import (
	"net/http"
	"strconv"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/website/shared"
)

const (
	lastplayedSize = 20
)

type LastPlayedInput struct {
	shared.Input

	Songs []radio.Song
	Page  *shared.Pagination
}

func (LastPlayedInput) TemplateBundle() string {
	return "lastplayed"
}

func NewLastPlayedInput(f *shared.InputFactory, s radio.SongStorageService, r *http.Request) (*LastPlayedInput, error) {
	const op errors.Op = "website/public.NewLastPlayedInput"

	page, offset, err := getPageOffset(r, lastplayedSize)
	if err != nil {
		return nil, errors.E(op, err)
	}

	ss := s.Song(r.Context())
	songs, err := ss.LastPlayed(offset, lastplayedSize)
	if err != nil {
		return nil, errors.E(op, err)
	}

	total, err := ss.LastPlayedCount()
	if err != nil {
		return nil, errors.E(op, err)
	}

	return &LastPlayedInput{
		Input: f.New(r),
		Songs: songs,
		Page: shared.NewPagination(
			page, shared.PageCount(total, lastplayedSize),
			"/last-played?page=%d",
		),
	}, nil
}

func (s State) getLastPlayed(w http.ResponseWriter, r *http.Request) error {
	input, err := NewLastPlayedInput(s.Shared, s.Storage, r)
	if err != nil {
		return err
	}

	return s.Templates.Execute(w, r, input)
}

func (s State) GetLastPlayed(w http.ResponseWriter, r *http.Request) {
	err := s.getLastPlayed(w, r)
	if err != nil {
		s.errorHandler(w, r, err)
		return
	}
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
