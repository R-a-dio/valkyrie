package public

import (
	"log"
	"net/http"
)

func (s State) GetFaves(w http.ResponseWriter, r *http.Request) {
	favesInput := struct {
		sharedInput
	}{}

	err := s.TemplateExecutor.ExecuteFull(theme, "faves", w, favesInput)
	if err != nil {
		log.Println(err)
		return
	}
}

func (s State) PostFaves(w http.ResponseWriter, r *http.Request) {
	return
}
