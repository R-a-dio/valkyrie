package admin

import (
	"html/template"
	"net/http"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/website/middleware"
	"github.com/gorilla/csrf"
)

type ListenersInput struct {
	middleware.Input
	CSRFTokenInput template.HTML

	Listeners []radio.Listener
}

func (ListenersInput) TemplateBundle() string {
	return "listeners"
}

func NewListenersInput(ls radio.ListenerTrackerService, r *http.Request) (*ListenersInput, error) {
	const op errors.Op = "website/admin.NewListenersInput"

	listeners, err := ls.ListClients(r.Context())
	if err != nil {
		return nil, errors.E(op, err)
	}

	return &ListenersInput{
		Input:          middleware.InputFromRequest(r),
		CSRFTokenInput: csrf.TemplateField(r),
		Listeners:      listeners,
	}, nil
}

func (s *State) GetListeners(w http.ResponseWriter, r *http.Request) {
	input, err := NewListenersInput(nil, r)
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
