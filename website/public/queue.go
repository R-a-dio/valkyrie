package public

import (
	"log"
	"net/http"
)

func (s State) GetQueue(w http.ResponseWriter, r *http.Request) {
	queueInput := struct {
		sharedInput
	}{}

	err := s.TemplateExecutor.ExecuteFull(theme, "queue", w, queueInput)
	if err != nil {
		log.Println(err)
		return
	}
}
