package public

import (
	"net/http"
)

type StaffInput struct {
	SharedInput
}

func NewStaffInput(r *http.Request) StaffInput {
	return StaffInput{
		SharedInput: NewSharedInput(r),
	}
}

func (StaffInput) TemplateBundle() string {
	return "staff"
}

func (s State) GetStaff(w http.ResponseWriter, r *http.Request) {
	input := NewStaffInput(r)

	err := s.Templates.Execute(w, r, input)
	if err != nil {
		s.errorHandler(w, r, err)
		return
	}
}
