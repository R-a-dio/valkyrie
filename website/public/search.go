package public

import (
	"net/http"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/website/middleware"
	"github.com/R-a-dio/valkyrie/website/shared"
)

const searchPageSize = 20

type SearchInput struct {
	middleware.Input

	Query string
	Songs []radio.Song
	Page  *shared.Pagination
}

func NewSearchInput(s radio.SearchService, r *http.Request) (*SearchInput, error) {
	const op errors.Op = "website/public.NewSearchInput"
	ctx := r.Context()

	page, offset, err := getPageOffset(r, searchPageSize)
	if err != nil {
		return nil, errors.E(op, err)
	}

	query := r.FormValue("q")
	result, err := s.Search(ctx, query, searchPageSize, offset)
	if err != nil {
		return nil, err
	}

	return &SearchInput{
		Input: middleware.InputFromRequest(r),
		Query: query,
		Songs: result.Songs,
		Page: shared.NewPagination(
			page, shared.PageCount(int64(result.TotalHits), searchPageSize),
			r.URL,
		),
	}, nil
}

func (SearchInput) TemplateBundle() string {
	return "search"
}

func (s State) GetSearch(w http.ResponseWriter, r *http.Request) {
	input, err := NewSearchInput(s.Search, r)
	if err != nil {
		s.errorHandler(w, r, err)
		return
	}

	err = s.Templates.Execute(w, r, input)
	if err != nil {
		s.errorHandler(w, r, err)
		return
	}
}
