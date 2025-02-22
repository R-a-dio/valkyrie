package bleve

import (
	"context"
	"testing"

	"github.com/blevesearch/bleve/v2/search"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type queryCase struct {
	raw    string
	query  string
	fields map[string]string
	sort   search.SortOrder
}

var testNewQueryCases = []queryCase{
	{
		raw:   `!!title:"term multi" hello world !!>title !!<id`,
		query: "hello world",
		fields: map[string]string{
			"title": "term multi",
		},
		sort: search.SortOrder{
			NewFieldSort("title", false),
			NewFieldSort("id", true),
		},
	},
	{
		raw:   "just a normal query",
		query: "just a normal query",
	},
	{
		raw:   `!!artist:"single"`,
		query: "",
		fields: map[string]string{
			"artist": "single",
		},
	},
	{
		raw:   `!!invalid:"nothing"`,
		query: `!!invalid:"nothing"`,
	},
}

func TestNewQuery(t *testing.T) {
	ctx := context.Background()

	for _, c := range testNewQueryCases {
		// NewQuery always allocates FieldQueries, so allocate it here too if
		// it was omitted from the case
		if c.fields == nil {
			c.fields = make(map[string]string)
		}
		if c.sort == nil { // default sort if nil
			c.sort = newPrioScoreSort()
		}

		q, err := NewQuery(ctx, c.raw, true)
		require.NoError(t, err)
		require.Equal(t, c.query, q.Query)
		require.Equal(t, c.fields, q.FieldQueries)
		require.EqualValues(t, c.sort, q.Sort)
	}
}

func BenchmarkNewQuery(b *testing.B) {
	for b.Loop() {
		NewQuery(b.Context(), testNewQueryCases[0].raw, false)
	}
}

func BenchmarkValidField(b *testing.B) {
	b.Run("start", func(b *testing.B) {
		for range b.N {
			_ = isValidField("artist")
		}
	})
	b.Run("middle", func(b *testing.B) {
		for range b.N {
			_ = isValidField("editor")
		}
	})
	b.Run("end", func(b *testing.B) {
		for range b.N {
			_ = isValidField("rc")
		}
	})
	b.Run("invalid", func(b *testing.B) {
		for range b.N {
			_ = isValidField("this isn't valid")
		}
	})
}

func FuzzNewQuery(f *testing.F) {
	for _, c := range testNewQueryCases {
		f.Add(c.raw)
	}

	f.Fuzz(func(t *testing.T, query string) {
		q, err := NewQuery(t.Context(), query, false)
		if err == nil {
			assert.NotNil(t, q)
		}

		//assert.Equal(t, query, q.RawQuery)
	})
}
