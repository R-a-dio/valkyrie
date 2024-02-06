package public

import (
	"net/http"
)

type FavesInput struct {
	SharedInput
}

func (FavesInput) TemplateBundle() string {
	return "faves"
}

func NewFavesInput(r *http.Request) FavesInput {
	return FavesInput{
		SharedInput: NewSharedInput(r),
	}
}

func (s State) GetFaves(w http.ResponseWriter, r *http.Request) {
	input := NewFavesInput(r)

	err := s.Templates.Execute(w, r, input)
	if err != nil {
		s.errorHandler(w, r, err)
		return
	}
}

func (s State) PostFaves(w http.ResponseWriter, r *http.Request) {
	return
}
