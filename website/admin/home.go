package admin

import (
	"net/http"

	"github.com/R-a-dio/valkyrie/util/daypass"
	"github.com/R-a-dio/valkyrie/website/middleware"
)

type HomeInput struct {
	middleware.Input
	Daypass daypass.DaypassInfo
}

func NewHomeInput(r *http.Request, dp *daypass.Daypass) HomeInput {
	return HomeInput{
		Input:   middleware.InputFromRequest(r),
		Daypass: dp.Info(),
	}
}

func (HomeInput) TemplateBundle() string {
	return "admin-home"
}

func (s *State) GetHome(w http.ResponseWriter, r *http.Request) {
	input := NewHomeInput(r, s.Daypass)

	s.TemplateExecutor.Execute(w, r, input)
}
