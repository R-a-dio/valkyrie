package admin

import (
	"html/template"
	"net/http"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/util/secret"
	"github.com/R-a-dio/valkyrie/website/middleware"
	"github.com/gorilla/csrf"
)

type HomeInput struct {
	middleware.Input
	Daypass        string
	CSRFTokenInput template.HTML

	CanKillStreamer bool

	CanTemplateReload bool
	TemplateReload    TemplateReloadInput
}

func NewHomeInput(r *http.Request, dp secret.Secret) HomeInput {
	input := middleware.InputFromRequest(r)
	return HomeInput{
		Input:             input,
		Daypass:           dp.Get(nil),
		CSRFTokenInput:    csrf.TemplateField(r),
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

func (s *State) PostReloadTemplates(w http.ResponseWriter, r *http.Request) {
	err := s.Templates.Reload()

	input := NewHomeInput(r, s.Daypass)
	input.TemplateReload = TemplateReloadInput{
		Reloaded: err == nil,
		Error:    err,
	}

	err = s.TemplateExecutor.Execute(w, r, input)
	if err != nil {
		s.errorHandler(w, r, err, "failed to reload templates")
		return
	}
}
