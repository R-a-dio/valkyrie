package public

import (
	"net/http"

	"github.com/R-a-dio/valkyrie/website/middleware"
)

type FavesInput struct {
	middleware.Input
}

func (FavesInput) TemplateBundle() string {
	return "faves"
}

func NewFavesInput(r *http.Request) FavesInput {
	return FavesInput{
		Input: middleware.InputFromRequest(r),
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
