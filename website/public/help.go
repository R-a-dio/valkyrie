package public

import (
	"net/http"

	"github.com/R-a-dio/valkyrie/website/middleware"
)

type HelpInput struct {
	middleware.Input
}

func (HelpInput) TemplateBundle() string {
	return "help"
}

func NewHelpInput(r *http.Request) HelpInput {
	return HelpInput{
		Input: middleware.InputFromRequest(r),
	}
}

func (s State) GetHelp(w http.ResponseWriter, r *http.Request) {
	input := NewHelpInput(r)

	err := s.Templates.Execute(w, r, input)
	if err != nil {
		s.errorHandler(w, r, err)
		return
	}
}
