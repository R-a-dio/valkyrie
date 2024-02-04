package public

import (
	"net/http"

	radio "github.com/R-a-dio/valkyrie"
)

func (s State) getQueue(w http.ResponseWriter, r *http.Request) error {
	input := struct {
		shared
		Queue []radio.QueueEntry
	}{
		shared: s.shared(r),
	}

	queue, err := s.Streamer.Queue(r.Context())
	if err != nil {
		return err
	}
	input.Queue = queue

	err = s.TemplateExecutor.ExecuteFull(theme, "queue", w, input)
	if err != nil {
		return err
	}
	return nil
}

func (s State) GetQueue(w http.ResponseWriter, r *http.Request) {
	err := s.getQueue(w, r)
	if err != nil {
		s.errorHandler(w, r, err)
		return
	}
}
