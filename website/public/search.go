package public

import (
	"fmt"
	"html/template"
	"net/http"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/website/middleware"
	"github.com/R-a-dio/valkyrie/website/shared"
	"github.com/gorilla/csrf"
)

const searchPageSize = 20

type SearchInput struct {
	middleware.Input
	SearchSharedInput
	CSRFLegacyFix template.HTML
}

func (SearchInput) TemplateBundle() string {
	return "search"
}

func NewSearchInput(s radio.SearchService, rs radio.RequestStorage, r *http.Request, requestDelay time.Duration) (*SearchInput, error) {
	const op errors.Op = "website/public.NewSearchInput"

	sharedInput, err := NewSearchSharedInput(s, rs, r, requestDelay, searchPageSize)
	if err != nil {
		return nil, errors.E(op, err)
	}

	return &SearchInput{
		Input:             middleware.InputFromRequest(r),
		SearchSharedInput: *sharedInput,
		CSRFLegacyFix:     csrfLegacyFix(r),
	}, nil
}

type SearchSharedInput struct {
	CSRFTokenInput  template.HTML
	Query           string
	Songs           []radio.Song
	CanRequest      bool
	RequestCooldown time.Duration
	Page            *shared.Pagination

	// IsError indicates if the message given is an error
	IsError bool
	// Message to show at the top of the page
	Message string
}

func NewSearchSharedInput(s radio.SearchService, rs radio.RequestStorage, r *http.Request, requestDelay time.Duration, pageSize int64) (*SearchSharedInput, error) {
	const op errors.Op = "website/public.NewSearchSharedInput"
	ctx := r.Context()

	page, offset, err := getPageOffset(r, pageSize)
	if err != nil {
		return nil, errors.E(op, err)
	}

	var songs []radio.Song
	var totalHits int
	query := r.FormValue("q")
	if len(query) > 0 {
		searchResult, err := s.Search(ctx, query, searchPageSize, offset)
		if err != nil {
			//return nil, errors.E(op, err)
			songs = make([]radio.Song, 0)
			totalHits = 0
		} else {
			songs = searchResult.Songs
			totalHits = searchResult.TotalHits
		}
	}

	// RemoteAddr on the request should've already been scrubbed by some middleware to not
	// include any port numbers, trust in that and use the remote as-is
	identifier := r.RemoteAddr
	lastRequest, err := rs.LastRequest(identifier)
	if err != nil {
		return nil, errors.E(op, err)
	}

	cd, ok := radio.CalculateCooldown(requestDelay, lastRequest)

	// we also use this input if we're making a request, in which case our url
	// will be something other than /search that we can't use for the pagination
	// logic.
	r.URL.Path = "/search"

	return &SearchSharedInput{
		CSRFTokenInput:  csrf.TemplateField(r),
		Query:           query,
		Songs:           songs,
		CanRequest:      ok,
		RequestCooldown: cd,
		Page: shared.NewPagination(
			page, shared.PageCount(int64(totalHits), searchPageSize),
			r.URL,
		),
	}, nil
}

func (s *State) GetSearch(w http.ResponseWriter, r *http.Request) {
	input, err := NewSearchInput(
		s.Search,
		s.Storage.Request(r.Context()),
		r,
		s.Config.UserRequestDelay(),
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

func csrfLegacyFix(r *http.Request) template.HTML {
	return template.HTML(fmt.Sprintf(`
<!--
<form %s
-->
	`, csrf.TemplateField(r)))
}
