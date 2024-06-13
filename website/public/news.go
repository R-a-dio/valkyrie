package public

import (
	"context"
	"crypto/sha1"
	"fmt"
	"html/template"
	"net/http"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/website/middleware"
	"github.com/R-a-dio/valkyrie/website/shared"
	"github.com/adtac/go-akismet/akismet"
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

type NewsInputPost struct {
	ID     radio.NewsPostID
	Title  string
	Header template.HTML
	User   radio.User

	CreatedAt time.Time
	UpdatedAt *time.Time
}

func (NewsInput) TemplateBundle() string {
	return "news"
}

func AsNewsInputPost(ctx context.Context, cache *shared.NewsCache, entries []radio.NewsPost) ([]NewsInputPost, error) {
	ctx, span := trace.SpanFromContext(ctx).TracerProvider().Tracer("markdown").Start(ctx, "markdown")
	defer span.End()

	posts := make([]NewsInputPost, 0, len(entries))
	for _, post := range entries {
		md, err := cache.RenderHeader(post)
		if err != nil {
			return nil, err
		}

		posts = append(posts, NewsInputPost{
			ID:        post.ID,
			Title:     post.Title,
			Header:    md.Output,
			User:      post.User,
			CreatedAt: post.CreatedAt,
			UpdatedAt: post.UpdatedAt,
		})
	}
	return posts, nil
}

func NewNewsInput(cache *shared.NewsCache, ns radio.NewsStorageService, r *http.Request) (*NewsInput, error) {
	const op errors.Op = "website/public.NewNewsInput"
	ctx := r.Context()

	page, offset, err := shared.PageAndOffset(r, newsPageSize)
	if err != nil {
		return nil, errors.E(op, err)
	}

	entries, err := ns.News(r.Context()).ListPublic(newsPageSize, offset)
	if err != nil {
		return nil, errors.E(op, err)
	}

	posts, err := AsNewsInputPost(ctx, cache, entries.Entries)
	if err != nil {
		return nil, errors.E(op, err)
	}

	return &NewsInput{
		Input:     middleware.InputFromContext(ctx),
		News:      posts,
		NewsTotal: entries.Total,
		Page: shared.NewPagination(page, shared.PageCount(int64(entries.Total), newsPageSize),
			r.URL,
		),
	}, nil
}

func (s State) GetNews(w http.ResponseWriter, r *http.Request) {
	input, err := NewNewsInput(s.News, s.Storage, r)
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

type NewsEntryPost struct {
	ID        radio.NewsPostID
	Title     string
	User      radio.User
	DeletedAt *time.Time
	CreatedAt time.Time
	UpdatedAt *time.Time
	Body      template.HTML

	Comments []NewsEntryComment
}

type NewsEntryInput struct {
	middleware.Input

	Post NewsEntryPost
}

type NewsEntryComment struct {
	ID         radio.NewsCommentID
	PostID     radio.NewsPostID
	Body       template.HTML
	Identifier string

	User      *radio.User
	DeletedAt *time.Time
	CreatedAt time.Time
	UpdatedAt *time.Time
}

func (NewsEntryInput) TemplateBundle() string {
	return "news-single"
}

func NewNewsEntryInput(cache *shared.NewsCache, ns radio.NewsStorage, r *http.Request) (*NewsEntryInput, error) {
	ctx := r.Context()

	id := chi.URLParamFromCtx(ctx, "NewsID")
	newsid, err := radio.ParseNewsPostID(id)
	if err != nil {
		return nil, err
	}

	post, err := ns.Get(newsid)
	if err != nil {
		return nil, err
	}

	ctx, span := trace.SpanFromContext(ctx).TracerProvider().Tracer("markdown").Start(ctx, "markdown")
	nm, err := cache.RenderBody(*post)
	if err != nil {
		span.End()
		return nil, err
	}
	span.End()

	raw, err := ns.Comments(post.ID)
	if err != nil {
		return nil, err
	}

	ctx, span = trace.SpanFromContext(ctx).TracerProvider().Tracer("markdown").Start(ctx, "markdown")
	comments := make([]NewsEntryComment, 0, len(raw))
	for _, comm := range raw {
		if comm.DeletedAt != nil {
			continue
		}

		nm, err := cache.RenderComment(comm)
		if err != nil {
			span.End()
			return nil, err
		}
		comments = append(comments, NewsEntryComment{
			ID:         comm.ID,
			PostID:     comm.PostID,
			Body:       nm.Output,
			Identifier: hashedIdentifier(comm.Identifier),
			User:       comm.User,
			DeletedAt:  comm.DeletedAt,
			CreatedAt:  comm.CreatedAt,
			UpdatedAt:  comm.UpdatedAt,
		})
	}
	span.End()

	return &NewsEntryInput{
		Input: middleware.InputFromContext(ctx),
		Post: NewsEntryPost{
			ID:        post.ID,
			Title:     post.Title,
			User:      post.User,
			DeletedAt: post.DeletedAt,
			CreatedAt: post.CreatedAt,
			UpdatedAt: post.UpdatedAt,
			Body:      nm.Output,
			Comments:  comments,
		},
	}, nil
}

func (s State) GetNewsEntry(w http.ResponseWriter, r *http.Request) {
	input, err := NewNewsEntryInput(s.News, s.Storage.News(r.Context()), r)
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
	const op errors.Op = "website/public.PostNewsEntry"

	comment, err := ParsePostNewsEntryForm(r)
	if err != nil {
		s.errorHandler(w, r, err)
		return
	}

	// check if we have a configured api key
	if key := s.Conf().Website.AkismetKey; key != "" {
		isSpam, err := akismet.Check(&akismet.Comment{
			Blog:           s.Conf().Website.AkismetBlog,
			UserIP:         r.RemoteAddr,
			UserAgent:      r.UserAgent(),
			CommentType:    "comment",
			CommentContent: comment.Body,
		}, key)
		if err != nil {
			s.errorHandler(w, r, err)
			return
		}
		if isSpam {
			s.errorHandler(w, r, errors.E(op, errors.Spam))
			return
		}
	}

	_, err = s.Storage.News(r.Context()).AddComment(*comment)
	if err != nil {
		s.errorHandler(w, r, err)
		return
	}
}

func ParsePostNewsEntryForm(r *http.Request) (*radio.NewsComment, error) {
	const op errors.Op = "website/public.ParsePostNewsEntryForm"

	ctx := r.Context()

	id := chi.URLParamFromCtx(ctx, "NewsID")
	newsid, err := radio.ParseNewsPostID(id)
	if err != nil {
		return nil, err
	}
	if newsid == 0 { // make sure we have an id
		return nil, errors.E(op, errors.InvalidForm)
	}

	comment := radio.NewsComment{
		ID:         0,
		PostID:     newsid,
		Body:       r.FormValue("comment"),
		Identifier: r.RemoteAddr,
		User:       middleware.UserFromContext(ctx),
		CreatedAt:  time.Now(),
	}

	if comment.User != nil {
		comment.UserID = &comment.User.ID
	}

	if len(comment.Body) > 500 { // comment too big
		return nil, errors.E(op, errors.InvalidForm)
	}

	return &comment, nil
}

func hashedIdentifier(identifier string) string {
	return fmt.Sprintf("%x", sha1.Sum([]byte(identifier)))[:4]
}
