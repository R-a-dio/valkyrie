package admin

import (
	"html/template"
	"net/http"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/website/middleware"
	"github.com/gorilla/csrf"
)

type ScheduleInput struct {
	middleware.Input
	CSRFTokenInput template.HTML

	Schedule []*radio.ScheduleEntry
}

func (ScheduleInput) TemplateBundle() string {
	return "schedule"
}

func NewScheduleInput(ss radio.ScheduleStorage, r *http.Request) (*ScheduleInput, error) {
	const op errors.Op = "website/admin.NewScheduleInput"
	schedule, err := ss.Latest()
	if err != nil {
		return nil, errors.E(op, err)
	}

	return &ScheduleInput{
		Input:          middleware.InputFromRequest(r),
		CSRFTokenInput: csrf.TemplateField(r),
		Schedule:       schedule,
	}, nil
}

func (s *State) GetSchedule(w http.ResponseWriter, r *http.Request) {
	input, err := NewScheduleInput(s.Storage.Schedule(r.Context()), r)
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
