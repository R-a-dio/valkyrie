package public

import (
	"net/http"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/website/middleware"
	"github.com/rs/zerolog/hlog"
)

type QueueInput struct {
	middleware.Input

	Queue []radio.QueueEntry
}

func (QueueInput) TemplateBundle() string {
	return "queue"
}

func NewQueueInput(qs radio.QueueService, r *http.Request) (*QueueInput, error) {
	queue, err := qs.Entries(r.Context())
	if err != nil {
		hlog.FromRequest(r).Err(err).Ctx(r.Context()).Msg("failed to retrieve queue")
	}

	return &QueueInput{
		Input: middleware.InputFromRequest(r),
		Queue: queue,
	}, nil
}

func (s *State) getQueue(w http.ResponseWriter, r *http.Request) error {
	input, err := NewQueueInput(s.Queue, r)
	if err != nil {
		return err
	}

	return s.Templates.Execute(w, r, input)
}

func (s *State) GetQueue(w http.ResponseWriter, r *http.Request) {
	err := s.getQueue(w, r)
	if err != nil {
		s.errorHandler(w, r, err)
		return
	}
}
