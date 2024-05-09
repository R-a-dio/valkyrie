package admin

import (
	"html/template"
	"net/http"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/website/middleware"
	"github.com/gorilla/csrf"
)

type TrackerInput struct {
	middleware.Input
	CSRFTokenInput template.HTML

	Listeners []radio.Listener
}

func (TrackerInput) TemplateBundle() string {
	return "tracker"
}

func NewTrackerInput(lts radio.ListenerTrackerService, r *http.Request) (*TrackerInput, error) {
	const op errors.Op = "website/admin.NewTrackerInput"

	listeners, err := lts.ListClients(r.Context())
	if err != nil {
		return nil, errors.E(op, err)
	}

	input := &TrackerInput{
		Input:          middleware.InputFromRequest(r),
		CSRFTokenInput: csrf.TemplateField(r),
		Listeners:      listeners,
	}
	return input, nil
}

func (s *State) GetListeners(w http.ResponseWriter, r *http.Request) {
	input, err := NewTrackerInput(nil, r)
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

func (s *State) PostRemoveListener(w http.ResponseWriter, r *http.Request) {
	id, err := radio.ParseListenerClientID(r.FormValue("id"))
	if err != nil {
		s.errorHandler(w, r, err, "")
		return
	}

	err = radio.ListenerTrackerService(nil).RemoveClient(r.Context(), id)
	if err != nil {
		s.errorHandler(w, r, err, "")
		return
	}

	s.GetListeners(w, r)
}
