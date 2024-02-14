package public

import (
	"net/http"

	"github.com/R-a-dio/valkyrie/website/shared"
)

type SearchInput struct {
	shared.Input
}

func NewSearchInput(f *shared.InputFactory, r *http.Request) SearchInput {
	return SearchInput{
		Input: f.New(r),
	}
}

func (SearchInput) TemplateBundle() string {
	return "search"
}

func (s State) GetSearch(w http.ResponseWriter, r *http.Request) {
	input := NewSearchInput(s.Shared, r)

	err := s.Templates.Execute(w, r, input)
	if err != nil {
		s.errorHandler(w, r, err)
		return
	}
}
