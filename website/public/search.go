package public

import (
	"net/http"

	"github.com/R-a-dio/valkyrie/website/middleware"
)

type SearchInput struct {
	middleware.Input
}

func NewSearchInput(r *http.Request) SearchInput {
	return SearchInput{
		Input: middleware.InputFromRequest(r),
	}
}

func (SearchInput) TemplateBundle() string {
	return "search"
}

func (s State) GetSearch(w http.ResponseWriter, r *http.Request) {
	input := NewSearchInput(r)

	err := s.Templates.Execute(w, r, input)
	if err != nil {
		s.errorHandler(w, r, err)
		return
	}
}
