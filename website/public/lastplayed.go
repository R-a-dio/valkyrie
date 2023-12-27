package public

import (
	"log"
	"net/http"
)

func (s State) GetLastPlayed(w http.ResponseWriter, r *http.Request) {
	lpInput := struct {
		shared
	}{
		shared: s.shared(r),
	}

	err := s.TemplateExecutor.ExecuteFull(theme, "lastplayed", w, lpInput)
	if err != nil {
		log.Println(err)
		return
	}
}
