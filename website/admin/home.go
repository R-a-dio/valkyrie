package admin

import (
	"net/http"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/util/daypass"
	"github.com/R-a-dio/valkyrie/website/middleware"
)

func NewSharedInput(r *http.Request) SharedInput {
	user := middleware.UserFromContext(r.Context())
	return SharedInput{
		IsUser: user != nil,
		User:   user,
	}
}

type SharedInput struct {
	IsUser bool
	User   *radio.User
}

func (SharedInput) TemplateName() string {
	return "full-page"
}

type HomeInput struct {
	SharedInput
	Daypass daypass.DaypassInfo
}

func NewHomeInput(r *http.Request, dp *daypass.Daypass) HomeInput {
	return HomeInput{
		SharedInput: NewSharedInput(r),
		Daypass:     dp.Info(),
	}
}

func (HomeInput) TemplateBundle() string {
	return "admin-home"
}

func (s *State) GetHome(w http.ResponseWriter, r *http.Request) {
	input := NewHomeInput(r, s.Daypass)

	s.TemplateExecutor.Execute(w, r, input)
}
