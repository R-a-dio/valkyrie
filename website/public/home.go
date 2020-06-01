package public

import (
	"log"
	"net/http"
)

func (s State) GetHome(w http.ResponseWriter, r *http.Request) {
	homeInput := struct {
		sharedInput
	}{}

	err := s.Templates[theme]["home"].ExecuteDev(w, homeInput)
	if err != nil {
		log.Println(err)
		return
	}
}
