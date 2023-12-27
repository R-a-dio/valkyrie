package public

import (
	"log"
	"net/http"
)

func (s State) GetNews(w http.ResponseWriter, r *http.Request) {
	newsInput := struct {
		shared
	}{
		shared: s.shared(r),
	}

	err := s.TemplateExecutor.ExecuteFull(theme, "news", w, newsInput)
	if err != nil {
		log.Println(err)
		return
	}
}

func (s State) PostNews(w http.ResponseWriter, r *http.Request) {
	return
}
