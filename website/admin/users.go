package admin

import (
	"cmp"
	"net/http"
	"slices"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/website/middleware"
)

type UsersInput struct {
	middleware.Input

	Users []radio.User
}

func (UsersInput) TemplateBundle() string {
	return "users"
}

type UsersForm struct {
	radio.User
}

func NewUsersInput(us radio.UserStorage, r *http.Request) (*UsersInput, error) {
	const op errors.Op = "website/admin.NewUsersInput"

	// get all the users
	users, err := us.All()
	if err != nil {
		return nil, errors.E(op, err)
	}
	// sort users by their id
	slices.SortFunc(users, func(a, b radio.User) int {
		return cmp.Compare(a.ID, b.ID)
	})
	// construct the input
	input := &UsersInput{
		Input: middleware.InputFromRequest(r),
		Users: users,
	}

	return input, nil
}

func (s State) GetUsersList(w http.ResponseWriter, r *http.Request) {
	input, err := NewUsersInput(s.Storage.User(r.Context()), r)
	if err != nil {
		s.errorHandler(w, r, err, "input creation failure")
		return
	}

	err = s.TemplateExecutor.Execute(w, r, input)
	if err != nil {
		s.errorHandler(w, r, err, "template failure")
		return
	}
}
