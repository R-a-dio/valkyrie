package public

import (
	"net/http"
	"strconv"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/website/middleware"
	"github.com/go-chi/chi/v5"
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

type NewsEntryInput struct {
	middleware.Input

	Entry radio.NewsPost
}

func (NewsEntryInput) TemplateBundle() string {
	return "news-single"
}

func NewNewsEntryInput(ns radio.NewsStorage, r *http.Request) (*NewsEntryInput, error) {
	ctx := r.Context()

	id := chi.URLParamFromCtx(ctx, "NewsID")
	iid, err := strconv.Atoi(id)
	if err != nil {
		return nil, err
	}
	newsid := radio.NewsPostID(iid)

	post, err := ns.Get(newsid)
	if err != nil {
		return nil, err
	}

	return &NewsEntryInput{
		Input: middleware.InputFromContext(ctx),
		Entry: *post,
	}, nil
}

func (s State) GetNewsEntry(w http.ResponseWriter, r *http.Request) {
	input, err := NewNewsEntryInput(s.Storage.News(r.Context()), r)
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

func (s State) PostNewsEntry(w http.ResponseWriter, r *http.Request) {
}
