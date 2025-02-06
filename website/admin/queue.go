package admin

import (
	"html/template"
	"net/http"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/website/middleware"
	"github.com/gorilla/csrf"
	"github.com/rs/zerolog/hlog"
)

type QueueInput struct {
	middleware.Input
	CSRFTokenInput template.HTML

	Queue []radio.QueueEntry
}

func (QueueInput) TemplateBundle() string {
	return "queue"
}

func NewQueueInput(qs radio.QueueService, r *http.Request) (*QueueInput, error) {
	const op errors.Op = "website/admin.NewQueueInput"

	queue, err := qs.Entries(r.Context())
	if err != nil {
		hlog.FromRequest(r).Err(errors.E(op, err)).Ctx(r.Context()).Msg("failed to retrieve queue")
	}

	input := &QueueInput{
		Input:          middleware.InputFromRequest(r),
		CSRFTokenInput: csrf.TemplateField(r),
		Queue:          queue,
	}
	return input, nil
}

func (s *State) GetQueue(w http.ResponseWriter, r *http.Request) {
	input, err := NewQueueInput(s.Queue, r)
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

func (s *State) PostQueueRemove(w http.ResponseWriter, r *http.Request) {
	id, err := radio.ParseQueueID(r.FormValue("id"))
	if err != nil {
		s.errorHandler(w, r, err, "")
		return
	}

	_, err = s.Queue.Remove(r.Context(), id)
	if err != nil {
		s.errorHandler(w, r, err, "")
		return
	}

	s.GetQueue(w, r)
}
