package admin

import (
	"context"
	"net/http"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/website/middleware"
	"github.com/R-a-dio/valkyrie/website/shared"
	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel/trace"
)

const newsPageSize = 20

type NewsInput struct {
	middleware.Input

	News      []NewsInputPost
	NewsTotal int
	Page      *shared.Pagination
}

func (NewsInput) TemplateBundle() string {
	return "news"
}

type NewsInputPost struct {
	middleware.Input

	Raw radio.NewsPost

	Header shared.NewsMarkdown
	Body   shared.NewsMarkdown
}

func (NewsInputPost) TemplateBundle() string {
	return "news-single"
}

func AsNewsInputPost(ctx context.Context, cache *shared.NewsCache, entries []radio.NewsPost) ([]NewsInputPost, error) {
	const op errors.Op = "website/admin.AsNewsInputPost"
	ctx, span := trace.SpanFromContext(ctx).TracerProvider().Tracer("markdown").Start(ctx, "markdown")
	defer span.End()

	sharedInput := middleware.InputFromContext(ctx)
	posts := make([]NewsInputPost, 0, len(entries))
	for _, post := range entries {
		header, err := cache.RenderHeader(post)
		if err != nil {
			return nil, errors.E(op, err)
		}

		body, err := cache.RenderBody(post)
		if err != nil {
			return nil, errors.E(op, err)
		}

		posts = append(posts, NewsInputPost{
			Input:  sharedInput,
			Raw:    post,
			Header: header,
			Body:   body,
		})
	}
	return posts, nil
}

func NewNewsInput(cache *shared.NewsCache, ns radio.NewsStorage, r *http.Request) (*NewsInput, error) {
	ctx := r.Context()

	page, offset, err := shared.PageAndOffset(r, newsPageSize)
	if err != nil {
		return nil, err
	}

	entries, err := ns.List(newsPageSize, offset)
	if err != nil {
		return nil, err
	}

	posts, err := AsNewsInputPost(ctx, cache, entries.Entries)
	if err != nil {
		return nil, err
	}

	return &NewsInput{
		Input:     middleware.InputFromRequest(r),
		News:      posts,
		NewsTotal: entries.Total,
		Page: shared.NewPagination(
			page,
			shared.PageCount(int64(entries.Total), newsPageSize),
			r.URL,
		),
	}, nil
}

func (s *State) GetNews(w http.ResponseWriter, r *http.Request) {
	input, err := NewNewsInput(s.News, s.Storage.News(r.Context()), r)
	if err != nil {
		s.errorHandler(w, r, err, "")
		return
	}

	err = s.TemplateExecutor.Execute(w, r, input)
	if err != nil {
		s.errorHandler(w, r, err, "")
		return
	}
}

func NewNewsInputPost(cache *shared.NewsCache, ns radio.NewsStorage, r *http.Request) (*NewsInputPost, error) {
	const op errors.Op = "website/admin.NewNewsInputPost"
	ctx := r.Context()

	nid, err := radio.ParseNewsPostID(chi.URLParamFromCtx(ctx, "NewsID"))
	if err != nil {
		return nil, errors.E(op, err)
	}

	var input NewsInputPost

	post, err := ns.Get(nid)
	if err != nil {
		return nil, errors.E(op, err)
	}
	input.Raw = *post

	// errors in rendering are ignored so that faulty news post won't lock you
	// out of their admin panel. Can't fix the faulty post if you can't output
	// it.
	input.Header, err = cache.RenderHeader(*post)
	if err != nil {
		input.Header = cache.RenderError(err)
	}

	input.Body, err = cache.RenderBody(*post)
	if err != nil {
		input.Body = cache.RenderError(err)
	}

	input.Input = middleware.InputFromContext(ctx)
	return &input, nil
}

func (s *State) GetNewsEntry(w http.ResponseWriter, r *http.Request) {
	input, err := NewNewsInputPost(s.News, s.Storage.News(r.Context()), r)
	if err != nil {
		s.errorHandler(w, r, err, "")
		return
	}

	err = s.TemplateExecutor.Execute(w, r, input)
	if err != nil {
		s.errorHandler(w, r, err, "")
		return
	}
}

func (s *State) PostNewsEntry(w http.ResponseWriter, r *http.Request) {
	return
}

func (s *State) PostNewsRender(w http.ResponseWriter, r *http.Request) {
	post := radio.NewsPost{
		Header: r.FormValue("header"),
		Body:   r.FormValue("body"),
	}

	var templateName string
	var res shared.NewsMarkdown
	var err error

	part := r.FormValue("part")
	switch part {
	case "body":
		res, err = s.News.RenderBypass(post.Body)
		templateName = "body-render"
	case "header":
		res, err = s.News.RenderBypass(post.Header)
		templateName = "header-render"
	default:
		s.errorHandler(w, r, err, "invalid part argument")
		return
	}
	if err != nil {
		// TODO: give error output
		res, err = s.News.RenderBypass(`
		You have an error in your markdown: 
		` + err.Error())
		if err != nil {
			s.errorHandler(w, r, err, "failed to render error markdown")
			return
		}
	}

	input := SelectedNewsMarkdown{
		NewsMarkdown: res,
		Name:         templateName,
	}

	err = s.TemplateExecutor.Execute(w, r, input)
	if err != nil {
		s.errorHandler(w, r, err, "")
		return
	}
}

type SelectedNewsMarkdown struct {
	shared.NewsMarkdown
	Name string
}

func (SelectedNewsMarkdown) TemplateBundle() string {
	return "news-single"
}

func (snm SelectedNewsMarkdown) TemplateName() string {
	return snm.Name
}

func NewNewsPostFromRequest(post radio.NewsPost, r *http.Request) radio.NewsPost {
	post.Title = r.FormValue("title")
	post.Header = r.FormValue("header")
	post.Body = r.FormValue("body")
	post.User = *middleware.UserFromContext(r.Context())
	post.Private = r.FormValue("private") != ""

	switch r.FormValue("action") {
	case "delete":
		now := time.Now()
		post.DeletedAt = &now
	case "save":
		if post.ID == 0 {
			post.CreatedAt = time.Now()
			post.User = *middleware.UserFromContext(r.Context())
		} else {
			now := time.Now()
			post.UpdatedAt = &now
		}
	}

	return post
}
