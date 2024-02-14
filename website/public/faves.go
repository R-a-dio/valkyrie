package public

import (
	"net/http"

	"github.com/R-a-dio/valkyrie/website/shared"
)

type FavesInput struct {
	shared.Input
}

func (FavesInput) TemplateBundle() string {
	return "faves"
}

func NewFavesInput(f *shared.InputFactory, r *http.Request) FavesInput {
	return FavesInput{
		Input: f.New(r),
	}
}

func (s State) GetFaves(w http.ResponseWriter, r *http.Request) {
	input := NewFavesInput(s.Shared, r)

	err := s.Templates.Execute(w, r, input)
	if err != nil {
		s.errorHandler(w, r, err)
		return
	}
}

func (s State) PostFaves(w http.ResponseWriter, r *http.Request) {
	return
}
