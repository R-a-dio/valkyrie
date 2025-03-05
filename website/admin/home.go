package admin

import (
	"html/template"
	"net/http"
	"strings"

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

	CanSetHolidayTheme bool
	HolidayTheme       radio.ThemeName
}

func NewHomeInput(r *http.Request, dp secret.Secret, ht radio.ThemeName) HomeInput {
	input := middleware.InputFromRequest(r)
	return HomeInput{
		Input:              input,
		Daypass:            dp.Get(nil),
		HolidayTheme:       ht,
		CSRFTokenInput:     csrf.TemplateField(r),
		CanTemplateReload:  input.User.UserPermissions.Has(radio.PermAdmin),
		CanKillStreamer:    input.User.UserPermissions.Has(radio.PermAdmin),
		CanSetHolidayTheme: input.User.UserPermissions.Has(radio.PermAdmin),
	}
}

func (HomeInput) TemplateBundle() string {
	return "home"
}

func (s *State) GetHome(w http.ResponseWriter, r *http.Request) {
	input := NewHomeInput(r, s.Daypass, s.ThemeConfig.LoadHoliday())

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

	input := NewHomeInput(r, s.Daypass, s.ThemeConfig.LoadHoliday())
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

func (s *State) PostSetHolidayTheme(w http.ResponseWriter, r *http.Request) {
	theme := r.PostFormValue("theme")

	if theme == "" || strings.EqualFold(theme, "none") {
		s.ThemeConfig.ClearHoliday()
	} else {
		s.ThemeConfig.StoreHoliday(radio.ThemeName(theme))
	}

	input := NewHomeInput(r, s.Daypass, s.ThemeConfig.LoadHoliday())

	err := s.TemplateExecutor.Execute(w, r, input)
	if err != nil {
		s.errorHandler(w, r, err, "failed to set holiday theme")
		return
	}
}
