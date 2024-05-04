package admin

import (
	"html/template"
	"net/http"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/website/middleware"
	"github.com/gorilla/csrf"
)

type QueueInput struct {
	middleware.Input
	CSRFTokenInput template.HTML

	Queue []radio.QueueEntry
}

func (QueueInput) TemplateBundle() string {
	return "queue"
}

// TODO: make this use radio.QueueService
func NewQueueInput(qs radio.StreamerService, r *http.Request) (*QueueInput, error) {
	const op errors.Op = "website/admin.NewQueueInput"

	queue, err := qs.Queue(r.Context())
	if err != nil {
		return nil, errors.E(op, err)
	}

	input := &QueueInput{
		Input:          middleware.InputFromRequest(r),
		CSRFTokenInput: csrf.TemplateField(r),
		Queue:          queue,
	}
	return input, nil
}

func (s *State) GetQueue(w http.ResponseWriter, r *http.Request) {
	input, err := NewQueueInput(s.Conf().Streamer.Client(), r)
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
