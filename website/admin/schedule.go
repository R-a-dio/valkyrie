package admin

import (
	"html/template"
	"net/http"
	"slices"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/website/middleware"
	"github.com/gorilla/csrf"
)

type ScheduleInput struct {
	middleware.Input
	CSRFTokenInput template.HTML

	Schedule     []*radio.ScheduleEntry
	AvailableDJs []radio.User
}

func (ScheduleInput) TemplateBundle() string {
	return "schedule"
}

func NewScheduleInput(ss radio.ScheduleStorage, us radio.UserStorage, r *http.Request) (*ScheduleInput, error) {
	const op errors.Op = "website/admin.NewScheduleInput"
	schedule, err := ss.Latest()
	if err != nil {
		return nil, errors.E(op, err)
	}

	users, err := us.All()
	if err != nil {
		return nil, errors.E(op, err)
	}
	// filter users down to just djs
	users = slices.DeleteFunc(users, func(u radio.User) bool {
		return u.UserPermissions.Has(radio.PermDJ)
	})

	return &ScheduleInput{
		Input:          middleware.InputFromRequest(r),
		CSRFTokenInput: csrf.TemplateField(r),
		Schedule:       schedule,
		AvailableDJs:   users,
	}, nil
}

func (s *State) GetSchedule(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	input, err := NewScheduleInput(
		s.Storage.Schedule(ctx),
		s.Storage.User(ctx),
		r,
	)
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
