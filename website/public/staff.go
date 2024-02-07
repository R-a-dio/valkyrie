package public

import (
	"net/http"

	radio "github.com/R-a-dio/valkyrie"
)

type StaffInput struct {
	SharedInput

	Users []radio.User
}

func (si StaffInput) Roles() []string {
	return []string{"staff", "dev", "djs"}
}

func NewStaffInput(us radio.UserStorageService, r *http.Request) (*StaffInput, error) {
	users, err := us.User(r.Context()).All()
	if err != nil {
		return nil, err
	}

	return &StaffInput{
		SharedInput: NewSharedInput(r),
		Users:       users,
	}, nil
}

func (StaffInput) TemplateBundle() string {
	return "staff"
}

func (s State) GetStaff(w http.ResponseWriter, r *http.Request) {
	input, err := NewStaffInput(s.Storage, r)
	if err != nil {
		s.errorHandler(w, r, err)
		return
	}

	err = s.Templates.Execute(w, r, input)
	if err != nil {
		s.errorHandler(w, r, err)
		return
	}
}
