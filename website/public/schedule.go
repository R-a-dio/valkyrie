package public

import (
	"net/http"

	"github.com/R-a-dio/valkyrie/errors"
)

type ScheduleInput struct {
	SharedInput
}

func NewScheduleInput(r *http.Request) ScheduleInput {
	return ScheduleInput{
		SharedInput: NewSharedInput(r),
	}
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
	const op errors.Op = "website/public.getSchedule"

	input := NewScheduleInput(r)

	return s.TemplateExecutor.Execute(w, r, input)
}
