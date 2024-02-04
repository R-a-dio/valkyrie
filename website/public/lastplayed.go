package public

import (
	"net/http"
	"strconv"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
)

const (
	lastplayedSize = 20
)

type LastPlayedInput struct {
	SharedInput

	Songs []radio.Song
	Page  int
}

func (LastPlayedInput) TemplateBundle() string {
	return "lastplayed"
}

func NewLastPlayedInput(s radio.SongStorageService, r *http.Request) (*LastPlayedInput, error) {
	page, offset, err := getPageOffset(r, lastplayedSize)
	if err != nil {
		return nil, err
	}

	songs, err := s.Song(r.Context()).LastPlayed(offset, lastplayedSize)
	if err != nil {
		return nil, err
	}

	return &LastPlayedInput{
		SharedInput: NewSharedInput(r),
		Songs:       songs,
		Page:        page,
	}, nil
}

func (s State) getLastPlayed(w http.ResponseWriter, r *http.Request) error {
	input, err := NewLastPlayedInput(s.Storage, r)
	if err != nil {
		return err
	}

	return s.TemplateExecutor.Execute(w, r, input)
}

func (s State) GetLastPlayed(w http.ResponseWriter, r *http.Request) {
	err := s.getLastPlayed(w, r)
	if err != nil {
		s.errorHandler(w, r, err)
		return
	}
}

func getPageOffset(r *http.Request, pageSize int) (int, int, error) {
	var page = 1
	{
		rawPage := r.Form.Get("page")
		if rawPage == "" {
			return page, 0, nil
		}
		parsedPage, err := strconv.Atoi(rawPage)
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
