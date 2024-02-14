package public

import (
	"fmt"
	"html/template"
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
	Page  *Pagination
}

func (LastPlayedInput) TemplateBundle() string {
	return "lastplayed"
}

func NewLastPlayedInput(s radio.SongStorageService, r *http.Request) (*LastPlayedInput, error) {
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
		SharedInput: NewSharedInput(r),
		Songs:       songs,
		Page: NewPagination(
			page, PageCount(total, lastplayedSize),
			"/last-played?page=%d",
		),
	}, nil
}

func (s State) getLastPlayed(w http.ResponseWriter, r *http.Request) error {
	input, err := NewLastPlayedInput(s.Storage, r)
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

func PageCount(total, size int64) int64 {
	full := total / size
	if leftover := total % size; leftover > 0 {
		return full + 1
	}
	return full
}

func NewPagination(current, total int64, urlFormat string) *Pagination {
	return &Pagination{
		Nr:     current,
		Total:  total,
		format: urlFormat,
	}
}

type Pagination struct {
	Nr     int64
	Total  int64
	format string
}

func (p *Pagination) URL() template.URL {
	return template.URL(fmt.Sprintf(p.format, p.Nr))
}

func (p *Pagination) createPage(page int64) *Pagination {
	if p == nil {
		return nil
	}
	if page > p.Total {
		return nil
	}
	if page < 1 {
		return nil
	}

	return &Pagination{
		Nr:     page,
		Total:  p.Total,
		format: p.format,
	}
}

func (p *Pagination) First() *Pagination {
	return p.createPage(1)
}

func (p *Pagination) Next(offset int64) *Pagination {
	return p.createPage(p.Nr + offset)
}

func (p *Pagination) Prev(offset int64) *Pagination {
	return p.createPage(p.Nr - offset)
}

func (p *Pagination) Last() *Pagination {
	return p.createPage(p.Total)
}
