package admin

import (
	"net/http"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/website/middleware"
)

type shared struct {
	IsUser bool
	User   *radio.User
}

func (s *State) shared(r *http.Request) shared {
	user := middleware.UserFromContext(r.Context())
	return shared{
		IsUser: user != nil,
		User:   user,
	}
}

type homeInput struct {
	shared
}

func (s *State) GetHome(w http.ResponseWriter, r *http.Request) {
	var tmplInput = homeInput{
		shared: s.shared(r),
	}
	s.TemplateExecutor.ExecuteFull("default", "admin-home", w, tmplInput)
}
