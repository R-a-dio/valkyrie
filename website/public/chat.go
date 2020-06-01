package public

import (
	"log"
	"net/http"
)

func (s State) GetChat(w http.ResponseWriter, r *http.Request) {
	chatInput := struct {
		sharedInput
	}{}

	err := s.Templates[theme]["chat"].ExecuteDev(w, chatInput)
	if err != nil {
		log.Println(err)
		return
	}
}
