package admin

import (
	"html/template"
	"net/http"
	"net/url"
	"slices"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/website/middleware"
	"github.com/gorilla/csrf"
)

type ScheduleInput struct {
	middleware.Input

	Schedule []ScheduleForm
}

func (ScheduleInput) TemplateBundle() string {
	return "schedule"
}

func scheduleGetUsers(us radio.UserStorage) ([]radio.User, error) {
	const op errors.Op = "website/admin.scheduleGetUsers"
	users, err := us.All()
	if err != nil {
		return nil, errors.E(op, err)
	}
	// filter users down to just djs
	users = slices.DeleteFunc(users, func(u radio.User) bool {
		return !u.UserPermissions.Has(radio.PermDJ)
	})

	return users, nil
}

func NewScheduleInput(ss radio.ScheduleStorage, us radio.UserStorage, r *http.Request) (*ScheduleInput, error) {
	const op errors.Op = "website/admin.NewScheduleInput"
	schedule, err := ss.Latest()
	if err != nil {
		return nil, errors.E(op, err)
	}

	for day, entry := range schedule {
		if entry == nil { // enter dummy entries for nils
			schedule[day] = &radio.ScheduleEntry{
				Weekday: radio.ScheduleDay(day),
			}
		}
	}

	users, err := scheduleGetUsers(us)
	if err != nil {
		return nil, errors.E(op, err)
	}

	shared := middleware.InputFromRequest(r)
	csrfToken := csrf.TemplateField(r)
	scheduleForms := make([]ScheduleForm, 7)
	for i, sch := range schedule {
		scheduleForms[i] = ScheduleForm{
			Input:          shared,
			CSRFTokenInput: csrfToken,
			Entry:          sch,
			AvailableDJs:   users,
		}
	}

	return &ScheduleInput{
		Input:    shared,
		Schedule: scheduleForms,
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

func (s *State) PostSchedule(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	err := r.ParseForm()
	if err != nil {
		s.errorHandler(w, r, err, "")
		return
	}

	form, err := NewScheduleForm(
		s.Storage.User(ctx),
		*middleware.UserFromContext(ctx),
		r,
	)
	if err != nil {
		s.errorHandler(w, r, err, "")
		return
	}

	if err := form.Validate(); err != nil {
		s.errorHandler(w, r, err, "")
		return
	}

	if form.Entry != nil {
		err := s.Storage.Schedule(ctx).Update(*form.Entry)
		if err != nil {
			s.errorHandler(w, r, err, "")
			return
		}
	}

	err = s.TemplateExecutor.Execute(w, r, form)
	if err != nil {
		s.errorHandler(w, r, err, "")
		return
	}
}

type ScheduleForm struct {
	middleware.Input
	CSRFTokenInput template.HTML

	Entry        *radio.ScheduleEntry
	AvailableDJs []radio.User
}

func (ScheduleForm) TemplateBundle() string {
	return "schedule"
}

func (ScheduleForm) TemplateName() string {
	return "form_schedule"
}

func NewScheduleForm(us radio.UserStorage, user radio.User, r *http.Request) (*ScheduleForm, error) {
	const op errors.Op = "website/admin.NewScheduleForm"

	values := r.PostForm

	var entry radio.ScheduleEntry

	if v := values.Get("weekday"); v != "" {
		entry.Weekday = radio.ParseScheduleDay(v)
	}
	if v := values.Get("notification"); v != "" {
		entry.Notification = true
	}
	entry.Text = values.Get("text")

	if v := values.Get("owner.id"); v != "" && v != "0" {
		ownerID, err := radio.ParseDJID(v)
		if err != nil {
			return nil, errors.E(op, err)
		}

		owner, err := us.GetByDJID(ownerID)
		if err != nil {
			return nil, errors.E(op, err)
		}
		entry.Owner = owner
	}
	entry.UpdatedAt = time.Now()
	entry.UpdatedBy = user

	users, err := scheduleGetUsers(us)
	if err != nil {
		return nil, errors.E(op, err)
	}

	return &ScheduleForm{
		Input:          middleware.InputFromRequest(r),
		CSRFTokenInput: csrf.TemplateField(r),
		Entry:          &entry,
		AvailableDJs:   users,
	}, nil
}

func (sf *ScheduleForm) Validate() error {
	const op errors.Op = "website/admin.ScheduleForm.Validate"

	if sf.Entry.Weekday == radio.UnknownDay {
		return errors.E(op, errors.InvalidArgument, errors.Info("unknown weekday"))
	}
	return nil
}

func (sf *ScheduleForm) ToValues() url.Values {
	values := url.Values{}
	if sf == nil || sf.Entry == nil {
		return values
	}

	if sf.Entry.Owner != nil {
		values.Set("owner.id", sf.Entry.Owner.DJ.ID.String())
	}
	values.Set("weekday", sf.Entry.Weekday.String())
	values.Set("text", sf.Entry.Text)
	if sf.Entry.Notification {
		values.Set("notification", "true")
	}
	return values
}
