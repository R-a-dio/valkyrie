package public

import (
	"net/http"
)

type NewsInput struct {
	SharedInput
}

func (NewsInput) TemplateBundle() string {
	return "news"
}

func NewNewsInput(r *http.Request) NewsInput {
	return NewsInput{
		SharedInput: NewSharedInput(r),
	}
}

func (s State) GetNews(w http.ResponseWriter, r *http.Request) {
	input := NewNewsInput(r)

	err := s.TemplateExecutor.Execute(w, r, input)
	if err != nil {
		s.errorHandler(w, r, err)
		return
	}
}

func (s State) PostNews(w http.ResponseWriter, r *http.Request) {
	return
}
