package shared

import (
	"fmt"
	"testing"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/arbitrary"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateCacheKey(t *testing.T) {
	a := arbitrary.DefaultArbitraries()

	p := gopter.NewProperties(nil)

	p.Property("generate body != header keys", a.ForAll(
		func(id radio.NewsPostID) bool {
			b := generateCacheKey(bodyKeyPrefix, id, 0)
			h := generateCacheKey(headerKeyPrefix, id, 0)
			return b != h
		},
	))
	p.Property("generate body != comment keys", a.ForAll(
		func(id radio.NewsPostID, cid radio.NewsCommentID) bool {
			b := generateCacheKey(bodyKeyPrefix, id, 0)
			c := generateCacheKey(commentKeyPrefix, id, cid)
			return b != c
		},
	))
	p.Property("generate header != comment keys", a.ForAll(
		func(id radio.NewsPostID, cid radio.NewsCommentID) bool {
			c := generateCacheKey(commentKeyPrefix, id, cid)
			h := generateCacheKey(headerKeyPrefix, id, 0)
			return c != h
		},
	))
	p.Property("generate body == body keys", a.ForAll(
		func(id radio.NewsPostID) bool {
			b1 := generateCacheKey(bodyKeyPrefix, id, 0)
			b2 := generateCacheKey(bodyKeyPrefix, id, 0)
			return b1 == b2
		},
	))
	p.Property("generate header == header keys", a.ForAll(
		func(id radio.NewsPostID) bool {
			h1 := generateCacheKey(headerKeyPrefix, id, 0)
			h2 := generateCacheKey(headerKeyPrefix, id, 0)
			return h1 == h2
		},
	))
	p.Property("generate comment == comment keys", a.ForAll(
		func(id radio.NewsPostID, cid radio.NewsCommentID) bool {
			c1 := generateCacheKey(commentKeyPrefix, id, cid)
			c2 := generateCacheKey(commentKeyPrefix, id, cid)
			return c1 == c2
		},
	))
	p.Property("extract body key PostID", a.ForAll(
		func(id radio.NewsPostID) bool {
			b := generateCacheKey(bodyKeyPrefix, id, 0)
			return b.PostID() == id && b.CommentID() == 0
		},
	))
	p.Property("extract header key PostID", a.ForAll(
		func(id radio.NewsPostID) bool {
			h := generateCacheKey(headerKeyPrefix, id, 0)
			return h.PostID() == id && h.CommentID() == 0
		},
	))
	p.Property("extract comment key PostID and CommentID", a.ForAll(
		func(id radio.NewsPostID, cid radio.NewsCommentID) bool {
			c := generateCacheKey(commentKeyPrefix, id, cid)
			return c.PostID() == id && c.CommentID() == cid
		},
	))

	p.TestingRun(t)
}

func TestNewsCacheHidden(t *testing.T) {
	cache := NewNewsCache()

	t.Run("RenderBody", func(t *testing.T) {
		post := radio.NewsPost{
			ID:   50,
			Body: "Hello <p>World</p>",
		}

		res, err := cache.RenderBody(post)
		if assert.NoError(t, err) {
			assert.True(t, res.HasHiddenHTML, "HasHiddenHTML should be true")
		}
	})
	t.Run("RenderHeader", func(t *testing.T) {
		post := radio.NewsPost{
			ID:     50,
			Header: "Hello <p>World</p>",
		}

		res, err := cache.RenderHeader(post)
		if assert.NoError(t, err) {
			assert.True(t, res.HasHiddenHTML, "HasHiddenHTML should be true")
		}
	})
	t.Run("RenderComment", func(t *testing.T) {
		comment := radio.NewsComment{
			ID:     10,
			PostID: 50,
			Body:   "Hello <p>World</p>",
		}

		res, err := cache.RenderComment(comment)
		if assert.NoError(t, err) {
			assert.True(t, res.HasHiddenHTML, "HasHiddenHTML should be true")
		}
	})
}

func TestNewsCacheCaching(t *testing.T) {
	cache := NewNewsCache()

	// a post with different data in body and header
	post := radio.NewsPost{
		ID:     50,
		Body:   "Hello World",
		Header: "A Header",
	}

	// render the body
	origBody, err := cache.RenderBody(post)
	if assert.NoError(t, err) {
		// the original and what the Source field gives us should be equal
		assert.Equal(t, post.Body, origBody.Source)
		// the original is plain text so should show up in the output
		assert.Contains(t, origBody.Output, post.Body)
	}

	// render the header
	origHeader, err := cache.RenderHeader(post)
	if assert.NoError(t, err) {
		// the original and what the Source field gives us should be equal
		assert.Equal(t, post.Header, origHeader.Source)
		// the original is plain text so should show up in the output
		assert.Contains(t, origHeader.Output, post.Header)
	}

	// now try and get the same data from the cache, we make sure of this
	// by only giving it the NewsPostID we used, so it can't render anything
	// new if the cache fails us
	emptyPost := radio.NewsPost{
		ID: 50,
	}

	// check the body
	cachedBody, err := cache.RenderBody(emptyPost)
	if assert.NoError(t, err) {
		// the one we got back from the original render
		// and this second cached call should be equal
		assert.Equal(t, origBody, cachedBody)
	}

	// check the header
	cachedHeader, err := cache.RenderHeader(emptyPost)
	if assert.NoError(t, err) {
		// the one we got back from the original render
		// and this second cached call should be equal
		assert.Equal(t, origHeader, cachedHeader)
	}
}

func TestNewsCacheEmptyBefore(t *testing.T) {
	nc := NewNewsCache()

	// generate a bunch of stuff to store in the cache, this tests
	// will do it through the Map directly because we can't control
	// the GeneratedAt field otherwise
	var epoch = time.Date(2000, time.February, 22, 12, 1, 9, 0, time.UTC)

	for i := radio.NewsPostID(0); i < 100; i++ {
		nm, err := nc.render(nc.trusted, fmt.Sprintf("This is post nr. %d", i))
		require.NoError(t, err)

		// fake the time for the generated markdown
		if i%2 == 0 { // half the entries get the epoch value
			nm.GeneratedAt = epoch
		} else { // the other half get the epoch with a day added to it
			nm.GeneratedAt = epoch.Add(time.Hour * 24)
		}

		bodyKey := generateCacheKey(bodyKeyPrefix, i, 0)
		headerKey := generateCacheKey(headerKeyPrefix, i, 0)
		commentKey := generateCacheKey(commentKeyPrefix, i, radio.NewsCommentID(i+100)*2)

		nc.cache.Store(bodyKey, nm)
		nc.cache.Store(headerKey, nm)
		nc.cache.Store(commentKey, nm)
	}

	var count int
	// now we need to make sure all of those entries are actually in the cache, we do
	// this with a simple count through a Range on the Map
	nc.cache.Range(func(key newsCacheKey, value NewsMarkdown) bool {
		count++
		return true
	})

	// we entered 100 entries, one for each body, header and comment so we should have
	// 300 entries
	require.Equal(t, 300, count)

	// now we're gonna empty all entries that are before the generated time we put in
	beforeTime := epoch.Add(time.Hour)
	nc.EmptyBefore(beforeTime)

	var newCount int
	// now we should have half the entries
	nc.cache.Range(func(key newsCacheKey, value NewsMarkdown) bool {
		newCount++
		// and all the values should have their time after the before time
		// we passed to EmptyBefore
		assert.True(t, value.GeneratedAt.After(beforeTime), "GeneratedAt of leftovers should be later than our cutoff time")
		return true
	})

	require.Equal(t, 150, newCount)

	// now see if empty works
	nc.Empty(radio.NewsPost{
		ID: 1,
	})

	comment1Key := generateCacheKey(commentKeyPrefix, 1, (1+100)*2)
	_, ok := nc.cache.Load(comment1Key)
	assert.True(t, ok, "Load for the comment key should be true")
}

func BenchmarkGenerateCacheKey(b *testing.B) {
	for n := 0; n < b.N; n++ {
		generateCacheKey(bodyKeyPrefix, 500, 0)
	}
}

func BenchmarkGenerateCacheKeyComment(b *testing.B) {
	for n := 0; n < b.N; n++ {
		generateCacheKey(bodyKeyPrefix, 500, 2000)
	}
}
