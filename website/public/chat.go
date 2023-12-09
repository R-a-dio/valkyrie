package public

import (
	"log"
	"net/http"
)

func (s State) GetChat(w http.ResponseWriter, r *http.Request) {
	chatInput := struct {
		sharedInput
	}{}

	err := s.TemplateExecutor.ExecuteFull(theme, "chat", w, chatInput)
	if err != nil {
		log.Println(err)
		return
	}
}
