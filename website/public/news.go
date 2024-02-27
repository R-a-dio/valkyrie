package public

import (
	"crypto/sha1"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/website/middleware"
	"github.com/R-a-dio/valkyrie/website/shared"
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
	iid, err := strconv.Atoi(id)
	if err != nil {
		return nil, err
	}
	newsid := radio.NewsPostID(iid)

	post, err := ns.Get(newsid)
	if err != nil {
		return nil, err
	}

	nm, err := cache.RenderBody(*post)
	if err != nil {
		return nil, err
	}

	raw, err := ns.Comments(post.ID)
	if err != nil {
		return nil, err
	}

	comments := make([]NewsEntryComment, 0, len(raw))
	for _, comm := range raw {
		if comm.DeletedAt != nil {
			continue
		}

		nm, err := cache.RenderComment(comm)
		if err != nil {
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
}

func hashedIdentifier(identifier string) string {
	return fmt.Sprintf("%x", sha1.Sum([]byte(identifier)))[:4]
}
