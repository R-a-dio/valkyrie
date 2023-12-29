package admin

import (
	"net/http"
)

type pendingInput struct {
	shared
}

func (s *State) GetPending(w http.ResponseWriter, r *http.Request) {
	var tmplInput = pendingInput{
		shared: s.shared(r),
	}
	s.TemplateExecutor.ExecuteFull("default", "admin-pending", w, tmplInput)
}

func (s *State) PostPending(w http.ResponseWriter, r *http.Request) {
	var tmplInput = pendingInput{
		shared: s.shared(r),
	}
	s.TemplateExecutor.ExecuteFull("default", "admin-pending", w, tmplInput)
}
