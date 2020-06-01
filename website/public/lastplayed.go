package public

import (
	"log"
	"net/http"
)

func (s State) GetLastPlayed(w http.ResponseWriter, r *http.Request) {
	lpInput := struct {
		sharedInput
	}{}

	err := s.Templates[theme]["lastplayed"].ExecuteDev(w, lpInput)
	if err != nil {
		log.Println(err)
		return
	}
}
