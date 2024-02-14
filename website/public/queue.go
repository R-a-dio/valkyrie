package public

import (
	"net/http"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/website/shared"
)

type QueueInput struct {
	shared.Input

	Queue []radio.QueueEntry
}

func (QueueInput) TemplateBundle() string {
	return "queue"
}

func NewQueueInput(f *shared.InputFactory, s radio.StreamerService, r *http.Request) (*QueueInput, error) {
	queue, err := s.Queue(r.Context())
	if err != nil {
		return nil, err
	}

	return &QueueInput{
		Input: f.New(r),
		Queue: queue,
	}, nil
}

func (s State) getQueue(w http.ResponseWriter, r *http.Request) error {
	input, err := NewQueueInput(s.Shared, s.Streamer, r)
	if err != nil {
		return err
	}

	return s.Templates.Execute(w, r, input)
}

func (s State) GetQueue(w http.ResponseWriter, r *http.Request) {
	err := s.getQueue(w, r)
	if err != nil {
		s.errorHandler(w, r, err)
		return
	}
}
