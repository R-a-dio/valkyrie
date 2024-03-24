package public

import (
	"net/http"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/website/middleware"
)

type ScheduleInput struct {
	middleware.Input

	Schedule []radio.ScheduleEntry
}

func NewScheduleInput(ss radio.ScheduleStorageService, r *http.Request) (*ScheduleInput, error) {
	schedule, err := ss.Schedule(r.Context()).Latest()
	if err != nil {
		return nil, err
	}

	return &ScheduleInput{
		Input:    middleware.InputFromRequest(r),
		Schedule: schedule,
	}, nil
}

func (ScheduleInput) TemplateBundle() string {
	return "schedule"
}

func (s State) GetSchedule(w http.ResponseWriter, r *http.Request) {
	err := s.getSchedule(w, r)
	if err != nil {
		s.errorHandler(w, r, err)
		return
	}
}

func (s State) getSchedule(w http.ResponseWriter, r *http.Request) error {
	input, err := NewScheduleInput(s.Storage, r)
	if err != nil {
		s.errorHandler(w, r, err)
		return err
	}

	return s.Templates.Execute(w, r, input)
}
