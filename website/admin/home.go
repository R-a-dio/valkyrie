package admin

import (
	"net/http"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/util/secret"
	"github.com/R-a-dio/valkyrie/website/middleware"
)

type HomeInput struct {
	middleware.Input
	Daypass string

	CanKillStreamer bool

	CanTemplateReload bool
	TemplateReload    TemplateReloadInput
}

func NewHomeInput(r *http.Request, dp secret.Secret) HomeInput {
	input := middleware.InputFromRequest(r)
	return HomeInput{
		Input:             input,
		Daypass:           dp.Get(nil),
		CanTemplateReload: input.User.UserPermissions.Has(radio.PermAdmin),
		CanKillStreamer:   input.User.UserPermissions.Has(radio.PermDJ),
	}
}

func (HomeInput) TemplateBundle() string {
	return "home"
}

func (s *State) GetHome(w http.ResponseWriter, r *http.Request) {
	input := NewHomeInput(r, s.Daypass)

	err := s.TemplateExecutor.Execute(w, r, input)
	if err != nil {
		s.errorHandler(w, r, err, "input creation failure")
		return
	}
}

type TemplateReloadInput struct {
	Reloaded bool
	Error    error
}

func (TemplateReloadInput) TemplateBundle() string {
	return "home"
}

func (TemplateReloadInput) TemplateName() string {
	return "template-reload"
}

func (s *State) PostReloadTemplates(w http.ResponseWriter, r *http.Request) {
	err := s.Templates.Reload()
	err = s.TemplateExecutor.Execute(w, r, TemplateReloadInput{
		Reloaded: err == nil,
		Error:    err,
	})
	if err != nil {
		s.errorHandler(w, r, err, "failed to reload templates")
	}
}
