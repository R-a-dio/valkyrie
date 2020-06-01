package public

import (
	"log"
	"net/http"
)

func (s State) GetSubmit(w http.ResponseWriter, r *http.Request) {
	submitInput := struct {
		sharedInput
	}{}

	err := s.Templates[theme]["submit"].ExecuteDev(w, submitInput)
	if err != nil {
		log.Println(err)
		return
	}
}
