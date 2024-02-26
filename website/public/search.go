package public

import (
	"net/http"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/website/middleware"
	"github.com/R-a-dio/valkyrie/website/shared"
)

const searchPageSize = 20

type SearchInput struct {
	middleware.Input

	Query           string
	Songs           []radio.Song
	CanRequest      bool
	RequestCooldown time.Duration
	Page            *shared.Pagination
}

func NewSearchInput(s radio.SearchService, rs radio.RequestStorage, r *http.Request, requestDelay time.Duration) (*SearchInput, error) {
	const op errors.Op = "website/public.NewSearchInput"
	ctx := r.Context()

	page, offset, err := getPageOffset(r, searchPageSize)
	if err != nil {
		return nil, errors.E(op, err)
	}

	query := r.FormValue("q")
	searchResult, err := s.Search(ctx, query, searchPageSize, offset)
	if err != nil {
		return nil, errors.E(op, err)
	}

	// TODO(wessie): check if this is the right identifier
	identifier := r.RemoteAddr
	lastRequest, err := rs.LastRequest(identifier)
	if err != nil {
		return nil, errors.E(op, err)
	}

	cd, ok := radio.CalculateCooldown(requestDelay, lastRequest)

	return &SearchInput{
		Input:           middleware.InputFromRequest(r),
		Query:           query,
		Songs:           searchResult.Songs,
		CanRequest:      ok,
		RequestCooldown: cd,
		Page: shared.NewPagination(
			page, shared.PageCount(int64(searchResult.TotalHits), searchPageSize),
			r.URL,
		),
	}, nil
}

func (SearchInput) TemplateBundle() string {
	return "search"
}

func (s State) GetSearch(w http.ResponseWriter, r *http.Request) {
	input, err := NewSearchInput(
		s.Search,
		s.Storage.Request(r.Context()),
		r,
		time.Duration(s.Conf().UserRequestDelay),
	)
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
