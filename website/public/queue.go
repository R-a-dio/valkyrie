package public

import (
	"net/http"

	radio "github.com/R-a-dio/valkyrie"
)

type QueueInput struct {
	SharedInput

	Queue []radio.QueueEntry
}

func (QueueInput) TemplateBundle() string {
	return "queue"
}

func NewQueueInput(s radio.StreamerService, r *http.Request) (*QueueInput, error) {
	queue, err := s.Queue(r.Context())
	if err != nil {
		return nil, err
	}

	return &QueueInput{
		SharedInput: NewSharedInput(r),
		Queue:       queue,
	}, nil
}

func (s State) getQueue(w http.ResponseWriter, r *http.Request) error {
	input, err := NewQueueInput(s.Streamer, r)
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
