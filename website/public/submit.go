package public

import (
	"log"
	"net/http"
)

func (s State) GetSubmit(w http.ResponseWriter, r *http.Request) {
	submitInput := struct {
		sharedInput
	}{}

	err := s.TemplateExecutor.ExecuteFull(theme, "submit", w, submitInput)
	if err != nil {
		log.Println(err)
		return
	}
}

func (s State) PostSubmit(w http.ResponseWriter, r *http.Request) {
	return
}
