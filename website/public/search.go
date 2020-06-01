package public

import (
	"log"
	"net/http"
)

func (s State) GetSearch(w http.ResponseWriter, r *http.Request) {
	searchInput := struct {
		sharedInput
	}{}

	err := s.Templates[theme]["search"].ExecuteDev(w, searchInput)
	if err != nil {
		log.Println(err)
		return
	}
}
