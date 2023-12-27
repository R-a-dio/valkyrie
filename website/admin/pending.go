package admin

import (
	"net/http"
)

type pendingInput struct {
	shared
}

func (a admin) GetPending(w http.ResponseWriter, r *http.Request) {
	var tmplInput = pendingInput{
		shared: a.shared(r),
	}
	a.templates.ExecuteFull("default", "admin-pending", w, tmplInput)
}
