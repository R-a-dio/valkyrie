package admin

import (
	"net/http"

	"github.com/R-a-dio/valkyrie/util/secret"
	"github.com/R-a-dio/valkyrie/website/middleware"
)

type HomeInput struct {
	middleware.Input
	Daypass string
}

func NewHomeInput(r *http.Request, dp secret.Secret) HomeInput {
	return HomeInput{
		Input:   middleware.InputFromRequest(r),
		Daypass: dp.Get(nil),
	}
}

func (HomeInput) TemplateBundle() string {
	return "home"
}

func (s *State) GetHome(w http.ResponseWriter, r *http.Request) {
	input := NewHomeInput(r, s.Daypass)

	err := s.TemplateExecutor.Execute(w, r, input)
	if err != nil {
		s.errorHandler(w, r, err)
	}
}
