package admin

import (
	"net/http"

	"github.com/R-a-dio/valkyrie/util/daypass"
	"github.com/R-a-dio/valkyrie/website/shared"
)

type HomeInput struct {
	shared.Input
	Daypass daypass.DaypassInfo
}

func NewHomeInput(f *shared.InputFactory, r *http.Request, dp *daypass.Daypass) HomeInput {
	return HomeInput{
		Input:   f.New(r),
		Daypass: dp.Info(),
	}
}

func (HomeInput) TemplateBundle() string {
	return "admin-home"
}

func (s *State) GetHome(w http.ResponseWriter, r *http.Request) {
	input := NewHomeInput(s.Shared, r, s.Daypass)

	s.TemplateExecutor.Execute(w, r, input)
}
