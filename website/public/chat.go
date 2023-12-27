package public

import (
	"log"
	"net/http"
)

func (s State) GetChat(w http.ResponseWriter, r *http.Request) {
	chatInput := struct {
		shared
	}{
		shared: s.shared(r),
	}

	err := s.TemplateExecutor.ExecuteFull(theme, "chat", w, chatInput)
	if err != nil {
		log.Println(err)
		return
	}
}
