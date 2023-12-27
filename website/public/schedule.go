package public

import (
	"log"
	"net/http"

	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/website/middleware"
)

func (s State) GetSchedule(w http.ResponseWriter, r *http.Request) {
	err := s.getSchedule(w, r)
	if err != nil {
		log.Println(err)
	}
}

func (s State) getSchedule(w http.ResponseWriter, r *http.Request) error {
	const op errors.Op = "website/public.getSchedule"
	ctx := r.Context()

	tmplInput := struct {
		shared
	}{
		shared: s.shared(r),
	}

	theme := middleware.GetTheme(ctx)
	err := s.TemplateExecutor.ExecuteFull(theme, "schedule", w, tmplInput)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}
