package public

import (
	"net/http"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/website/middleware"
)

type NewsInput struct {
	middleware.Input

	News radio.NewsList
}

func (NewsInput) TemplateBundle() string {
	return "news"
}

func NewNewsInput(s radio.NewsStorageService, r *http.Request) (*NewsInput, error) {
	entries, err := s.News(r.Context()).ListPublic(20, 0)
	if err != nil {
		return nil, err
	}

	return &NewsInput{
		Input: middleware.InputFromRequest(r),
		News:  entries,
	}, nil
}

func (s State) GetNews(w http.ResponseWriter, r *http.Request) {
	input, err := NewNewsInput(s.Storage, r)
	if err != nil {
		s.errorHandler(w, r, err)
		return
	}

	err = s.Templates.Execute(w, r, input)
	if err != nil {
		s.errorHandler(w, r, err)
		return
	}
}

func (s State) GetNewsEntry(w http.ResponseWriter, r *http.Request) {
	s.errorHandler(w, r, nil)
	return
}

func (s State) PostNews(w http.ResponseWriter, r *http.Request) {
	return
}
