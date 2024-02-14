package public

import (
	"net/http"

	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/website/shared"
)

type ScheduleInput struct {
	shared.Input
}

func NewScheduleInput(f *shared.InputFactory, r *http.Request) ScheduleInput {
	return ScheduleInput{
		Input: f.New(r),
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

	input := NewScheduleInput(s.Shared, r)

	return s.Templates.Execute(w, r, input)
}
