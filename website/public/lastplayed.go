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

func (s State) getLastPlayed(w http.ResponseWriter, r *http.Request) error {
	input := struct {
		shared
		Songs []radio.Song
		Page  int
	}{
		shared: s.shared(r),
	}

	page, offset, err := getPageOffset(r, lastplayedSize)
	if err != nil {
		return err
	}

	songs, err := s.Storage.Song(r.Context()).LastPlayed(offset, lastplayedSize)
	if err != nil {
		return err
	}
	input.Songs = songs
	input.Page = page

	err = s.TemplateExecutor.ExecuteFull(theme, "lastplayed", w, input)
	if err != nil {
		return err
	}
	return nil
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
