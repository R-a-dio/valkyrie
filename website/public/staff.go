package public

import (
	"log"
	"net/http"
)

func (s State) GetStaff(w http.ResponseWriter, r *http.Request) {
	staffInput := struct {
		sharedInput
	}{}

	err := s.Templates[theme]["staff"].ExecuteDev(w, staffInput)
	if err != nil {
		log.Println(err)
		return
	}
}
