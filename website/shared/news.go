package shared

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"html/template"
	"strings"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/util"
	"github.com/R-a-dio/valkyrie/util/pool"
	"github.com/R-a-dio/valkyrie/website/shared/markdown"
	"github.com/yuin/goldmark"
)

func NewNewsCache() *NewsCache {
	return &NewsCache{
		trusted: goldmark.New(
			markdown.RadioMarkdownOptions(false)...,
		//goldmark.WithRendererOptions(
		//html.WithUnsafe(), // TODO: see if we want to enable this
		//),
		),
		untrusted: goldmark.New(markdown.RadioMarkdownOptions(false)...),
		pool:      pool.NewResetPool(func() *bytes.Buffer { return new(bytes.Buffer) }),
		cache:     new(util.Map[newsCacheKey, NewsMarkdown]),
	}
}

type NewsMarkdown struct {
	GeneratedAt   time.Time
	HasHiddenHTML bool
	Source        string
	Output        template.HTML
}

type newsCacheKey [1 + 8*2]byte

func (key newsCacheKey) PostID() radio.NewsPostID {
	i := binary.NativeEndian.Uint64(key[1:])
	return radio.NewsPostID(i)
}

func (key newsCacheKey) CommentID() radio.NewsCommentID {
	i := binary.NativeEndian.Uint64(key[1+8:])
	return radio.NewsCommentID(i)
}

const (
	bodyKeyPrefix    = 'b'
	headerKeyPrefix  = 'h'
	commentKeyPrefix = 'c'
)

type NewsCache struct {
	trusted   goldmark.Markdown
	untrusted goldmark.Markdown

	pool  *pool.ResetPool[*bytes.Buffer]
	cache *util.Map[newsCacheKey, NewsMarkdown]
}

func (nc *NewsCache) render(md goldmark.Markdown, source string) (NewsMarkdown, error) {
	buf := nc.pool.Get()
	defer nc.pool.Put(buf)

	err := md.Convert([]byte(source), buf)
	if err != nil {
		return NewsMarkdown{}, err
	}
	output := buf.String()

	return NewsMarkdown{
		Source:        source,
		Output:        template.HTML(output),
		HasHiddenHTML: strings.Contains(output, "<!-- raw HTML omitted -->"),
		GeneratedAt:   time.Now(),
	}, nil
}

func (nc *NewsCache) loadOrRender(key newsCacheKey, md goldmark.Markdown, source string) (NewsMarkdown, error) {
	res, ok := nc.cache.Load(key)
	if ok {
		return res, nil
	}

	res, err := nc.render(md, source)
	if err != nil {
		return NewsMarkdown{}, err
	}

	nc.cache.Store(key, res)
	return res, nil
}

func (nc *NewsCache) RenderBody(post radio.NewsPost) (NewsMarkdown, error) {
	key := generateCacheKey(bodyKeyPrefix, post.ID, 0)

	return nc.loadOrRender(key, nc.trusted, post.Body)
}

func (nc *NewsCache) RenderHeader(post radio.NewsPost) (NewsMarkdown, error) {
	key := generateCacheKey(headerKeyPrefix, post.ID, 0)

	return nc.loadOrRender(key, nc.trusted, post.Header)
}

func (nc *NewsCache) RenderComment(comment radio.NewsComment) (NewsMarkdown, error) {
	key := generateCacheKey(commentKeyPrefix, comment.PostID, comment.ID)

	return nc.loadOrRender(key, nc.untrusted, comment.Body)
}

// RenderBypassCache renders a news post header and body as markdown and skips
// the cache mechanism
func (nc *NewsCache) RenderBypassCache(post radio.NewsPost) (header, body NewsMarkdown, err error) {
	header, err = nc.render(nc.trusted, post.Header)
	if err != nil {
		return header, body, err
	}

	body, err = nc.render(nc.trusted, post.Body)
	if err != nil {
		return header, body, err
	}

	return header, body, err
}

// RenderBypass renders string as markdown and skips the cache mechanism,
// source should come from a trusted input
func (nc *NewsCache) RenderBypass(source string) (NewsMarkdown, error) {
	return nc.render(nc.trusted, source)
}

const errorMarkdown = `
You have an error in your markdown, or something is broken:

%s
`

func (nc *NewsCache) RenderError(err error) NewsMarkdown {
	res, _ := nc.render(nc.untrusted, fmt.Sprintf(errorMarkdown, err.Error()))
	return res
}

// Empty clears the cache of the post given, this clears the Body and Header cache
func (nc *NewsCache) Empty(post radio.NewsPost) {
	nc.cache.Range(func(key newsCacheKey, value NewsMarkdown) bool {
		if key[0] != commentKeyPrefix && key.PostID() == post.ID {
			nc.cache.Delete(key)
		}
		return true
	})
}

// EmptyBefore removes any entries from the cache that had been generated
// before the time given
func (nc *NewsCache) EmptyBefore(t time.Time) {
	nc.cache.Range(func(key newsCacheKey, value NewsMarkdown) bool {
		if value.GeneratedAt.Before(t) {
			nc.cache.Delete(key)
		}
		return true
	})
}

func generateCacheKey(prefix byte, id radio.NewsPostID, cid radio.NewsCommentID) newsCacheKey {
	var key newsCacheKey

	key[0] = prefix
	binary.NativeEndian.PutUint64(key[1:], uint64(id))
	binary.NativeEndian.PutUint64(key[1+8:], uint64(cid))

	return key
}
