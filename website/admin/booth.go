package admin

import (
	"html/template"
	"net/http"

	"github.com/R-a-dio/valkyrie/website/middleware"
	"github.com/gorilla/csrf"
)

type BoothInput struct {
	middleware.Input
	CSRFTokenInput template.HTML
}

func (BoothInput) TemplateBundle() string {
	return "booth"
}

func NewBoothInput(r *http.Request) (*BoothInput, error) {
	return &BoothInput{
		Input:          middleware.InputFromRequest(r),
		CSRFTokenInput: csrf.TemplateField(r),
	}, nil
}

func (s *State) GetBooth(w http.ResponseWriter, r *http.Request) {
	input, err := NewBoothInput(r)
	if err != nil {
		s.errorHandler(w, r, err, "")
		return
	}

	err = s.TemplateExecutor.Execute(w, r, input)
	if err != nil {
		s.errorHandler(w, r, err, "")
		return
	}
}
