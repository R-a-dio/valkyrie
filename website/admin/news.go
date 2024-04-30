package admin

import (
	"context"
	"html/template"
	"net/http"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/templates"
	"github.com/R-a-dio/valkyrie/website/middleware"
	"github.com/R-a-dio/valkyrie/website/shared"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/csrf"
	"go.opentelemetry.io/otel/trace"
)

const newsPageSize = 20
const chiNewsKey = "NewsID"

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
	CSRFTokenInput template.HTML

	IsNew bool
	Raw   radio.NewsPost

	Header shared.NewsMarkdown
	Body   shared.NewsMarkdown
}

func (NewsInputPost) TemplateBundle() string {
	return "news-single"
}

func AsNewsInputPost(ctx context.Context, cache *shared.NewsCache, r *http.Request, entries []radio.NewsPost) ([]NewsInputPost, error) {
	const op errors.Op = "website/admin.AsNewsInputPost"
	ctx, span := trace.SpanFromContext(ctx).TracerProvider().Tracer("markdown").Start(ctx, "markdown")
	defer span.End()

	sharedInput := middleware.InputFromContext(ctx)
	sharedCsrf := csrf.TemplateField(r)

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
			Input:          sharedInput,
			CSRFTokenInput: sharedCsrf,
			Raw:            post,
			Header:         header,
			Body:           body,
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

	posts, err := AsNewsInputPost(ctx, cache, r, entries.Entries)
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

func NewNewsInputPost(cache *shared.NewsCache, ns radio.NewsStorage, r *http.Request, nid radio.NewsPostID) (*NewsInputPost, error) {
	const op errors.Op = "website/admin.NewNewsInputPost"
	ctx := r.Context()
	var input NewsInputPost
	var err error

	id := chi.URLParamFromCtx(ctx, chiNewsKey)
	isNew := nid == 0 && id == "new"
	if nid == 0 && !isNew {
		nid, err = radio.ParseNewsPostID(id)
		if err != nil {
			return nil, errors.E(op, err)
		}
	}

	if !isNew {
		post, err := ns.Get(nid)
		if err != nil {
			return nil, errors.E(op, err)
		}
		input.Raw = *post
	}

	// errors in rendering are ignored so that faulty news post won't lock you
	// out of their admin panel. Can't fix the faulty post if you can't output
	// it.
	input.Header, err = cache.RenderHeader(input.Raw)
	if err != nil {
		input.Header = cache.RenderError(err)
	}

	input.Body, err = cache.RenderBody(input.Raw)
	if err != nil {
		input.Body = cache.RenderError(err)
	}

	input.Input = middleware.InputFromContext(ctx)
	input.CSRFTokenInput = csrf.TemplateField(r)
	input.IsNew = isNew
	return &input, nil
}

func (s *State) GetNewsEntry(w http.ResponseWriter, r *http.Request) {
	s.getNewsEntry(w, r, 0)
}

func (s *State) getNewsEntry(w http.ResponseWriter, r *http.Request, nid radio.NewsPostID) {
	input, err := NewNewsInputPost(s.News, s.Storage.News(r.Context()), r, nid)
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
	ctx := r.Context()
	id := chi.URLParamFromCtx(ctx, chiNewsKey)
	isNew := id == "new"

	var post radio.NewsPost
	var nid radio.NewsPostID
	var err error

	if !isNew {
		// not a new entry, so find the current data and then
		// update it with form values
		nid, err = radio.ParseNewsPostID(id)
		if err != nil {
			s.errorHandler(w, r, err, "")
			return
		}

		ppost, err := s.Storage.News(ctx).Get(nid)
		if err != nil {
			s.errorHandler(w, r, err, "")
			return
		}
		post = *ppost
	}

	post = NewNewsPostFromRequest(post, r)

	if isNew {
		nid, err = s.Storage.News(ctx).Create(post)
	} else if post.DeletedAt != nil {
		err = s.Storage.News(ctx).Delete(post.ID)
	} else {
		err = s.Storage.News(ctx).Update(post)
	}
	if err != nil {
		s.errorHandler(w, r, err, "")
		return
	}

	if isNew {
		// redirect to the newly created post with htmx
		w.Header().Set("Hx-Replace-Url", "/admin/news/"+nid.String())
	}

	s.getNewsEntry(w, r, nid)
}

func (s *State) PostNewsRender(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var post radio.NewsPost
	var res shared.NewsMarkdown
	var err error

	part := r.FormValue("part")
	switch part {
	case "body":
		res, err = s.News.RenderBypass(r.FormValue("body"))
	case "header":
		res, err = s.News.RenderBypass(r.FormValue("header"))
	case "title":
		// grab the id, but this might be the literal string "new" in
		// which case this will just error and give us a 0 back
		nid, _ := radio.ParseNewsPostID(r.FormValue("id"))
		if nid != 0 { // avoid a lookup if it's zero
			tmp, err := s.Storage.News(ctx).Get(nid)
			if err != nil && !errors.Is(errors.NewsUnknown, err) {
				s.errorHandler(w, r, err, "")
				return
			}
			post = *tmp
		}
		post.Title = r.FormValue("title")
	default:
		s.errorHandler(w, r, err, "invalid part argument")
		return
	}
	if err != nil {
		res = s.News.RenderError(err)
	}

	var input templates.TemplateSelectable

	switch part {
	case "body", "header":
		input = SelectedNewsMarkdown{
			NewsMarkdown: res,
			Name:         part + "-render",
		}
	case "title":
		if post.CreatedAt.IsZero() {
			post.CreatedAt = time.Now()
			post.User = *middleware.UserFromContext(ctx)
		}
		input = TitleNewsRender{
			NewsPost: post,
		}
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

type TitleNewsRender struct {
	radio.NewsPost
}

func (TitleNewsRender) TemplateBundle() string {
	return "news-single"
}

func (TitleNewsRender) TemplateName() string {
	return "title-render"
}

func NewNewsPostFromRequest(post radio.NewsPost, r *http.Request) radio.NewsPost {
	post.Title = r.FormValue("title")
	post.Header = r.FormValue("header")
	post.Body = r.FormValue("body")
	post.User = *middleware.UserFromContext(r.Context())
	post.Private = r.FormValue("private") != ""

	switch r.FormValue("action") {
	case "delete":
		if post.DeletedAt == nil {
			now := time.Now()
			post.DeletedAt = &now
		} else {
			// undelete if we're asked to delete a second time
			post.DeletedAt = nil
		}
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
