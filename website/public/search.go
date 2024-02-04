package public

import (
	"net/http"
)

type SearchInput struct {
	SharedInput
}

func NewSearchInput(r *http.Request) SearchInput {
	return SearchInput{
		SharedInput: NewSharedInput(r),
	}
}

func (SearchInput) TemplateBundle() string {
	return "search"
}

func (s State) GetSearch(w http.ResponseWriter, r *http.Request) {
	input := NewSearchInput(r)

	err := s.TemplateExecutor.Execute(w, r, input)
	if err != nil {
		s.errorHandler(w, r, err)
		return
	}
}
