package shared

import (
	"math"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFromPagination(t *testing.T) {
	uri, err := url.Parse("http://example.org/?from=test")
	require.NoError(t, err)

	now := uint(80)
	prev := []uint{
		now - 5,
		now - 4,
		now - 3,
		now - 2,
		now - 1,
	}
	next := []uint{
		now + 1,
		now + 2,
		now + 3,
		now + 4,
		now + 5,
	}

	p := NewFromPagination(now, prev, next, uri)

	t.Run("InitialIndex", func(t *testing.T) {
		assert.Equal(t, now, p.boundaries[p.index])
	})
	t.Run("Current/URL", func(t *testing.T) {

	})
	t.Run("Prev", func(t *testing.T) {
		p := p.Prev(1)
		assert.Equal(t, prev[4], p.boundaries[p.index])
		p = p.Prev(1)
		assert.Equal(t, prev[3], p.boundaries[p.index])
		p = p.Prev(1)
		assert.Equal(t, prev[2], p.boundaries[p.index])
		p = p.Prev(1)
		assert.Equal(t, prev[1], p.boundaries[p.index])
		p = p.Prev(1)
		assert.Equal(t, prev[0], p.boundaries[p.index])
		p = p.Prev(1) // prev too far should nil
		assert.Nil(t, p)
		p = p.Prev(1) // prev on nil should nil
		assert.Nil(t, p)
	})

	t.Run("Next", func(t *testing.T) {
		p := p.Next(1)
		assert.Equal(t, next[0], p.boundaries[p.index])
		p = p.Next(1)
		assert.Equal(t, next[1], p.boundaries[p.index])
		p = p.Next(1)
		assert.Equal(t, next[2], p.boundaries[p.index])
		p = p.Next(1)
		assert.Equal(t, next[3], p.boundaries[p.index])
		p = p.Next(1)
		assert.Equal(t, next[4], p.boundaries[p.index])
		p = p.Next(1) // next too far should nil
		assert.Nil(t, p)
		p = p.Next(1) // next on nil should nil
		assert.Nil(t, p)
	})

	t.Run("NextPrev", func(t *testing.T) {
		p := p.Next(3)
		assert.Equal(t, next[2], p.Key)
		p = p.Prev(6)
		assert.Equal(t, prev[2], p.Key)
		p = p.Next(16)
		assert.Nil(t, p)
	})

	t.Run("First", func(t *testing.T) {
		p := p.First()
		require.NotNil(t, p)
		assert.Equal(t, uint(math.MaxUint), p.Key)
		p = nil
		p = p.First()
		assert.Nil(t, p)
	})

	t.Run("NilLast", func(t *testing.T) {
		p := p.Last()
		assert.Nil(t, p)
		p.WithLast(0, 50)
	})

	last := uint(5)
	lastNr := 7
	p = p.WithLast(last, lastNr)

	t.Run("Last", func(t *testing.T) {
		p := p.Last()
		require.NotNil(t, p)
		assert.Equal(t, last, p.Key)
		assert.Equal(t, lastNr, p.Nr)
	})
}
