package mariadb

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type splitCase struct {
	query  string
	expect []string
}

var splitQueryCases = []splitCase{
	{`"hello world" and testing`, []string{`"hello world"`, `and`, `testing`}},
	{`"hello world and testing`, []string{`hello`, `world`, `and`, `testing`}},
	{`(hello world) and testing`, []string{`(hello world)`, `and`, `testing`}},
	{`("hello world") and testing`, []string{`("hello world")`, `and`, `testing`}},
	{`("hello world" and testing`, []string{`"hello world"`, `and`, `testing`}},
}

type processCase struct {
	query  string
	expect string
}

var processQueryCases = []processCase{
	{`"hello world" and testing`, `"hello world" and* testing*`},
	{`"hello-world" and testing`, `"hello-world" and* testing*`},
	{`"hello-world" and -testing`, `"hello-world" and* testing*`},
	{`(hello world) and testing`, `(hello* world*) and* testing*`},
	{`("hello world") and testing`, `("hello world") and* testing*`},
	{`("hello world" and more) and testing`, `("hello world" and* more*) and* testing*`},
}

func TestSplitQuery(t *testing.T) {
	for _, c := range splitQueryCases {
		result := splitQuery(c.query)

		res := make([]string, len(result))
		for i, e := range result {
			res[i] = string(e)
		}
		assert.Equal(t, c.expect, res)
	}
}

func TestProcessQuery(t *testing.T) {
	for _, c := range processQueryCases {
		result := processQuery(c.query)
		assert.Equal(t, c.expect, result, c.query)
	}
}

func BenchmarkProcessQuery(b *testing.B) {
	for n := 0; n < b.N; n++ {
		for _, c := range processQueryCases {
			processQuery(c.query)
		}
	}
}

func BenchmarkProcessQueryB(b *testing.B) {
	for n := 0; n < b.N; n++ {
		for _, c := range processQueryCases {
			processQueryB([]byte(c.query))
		}
	}
}

func BenchmarkSplitQueryRegexString(b *testing.B) {
	for n := 0; n < b.N; n++ {
		for _, c := range splitQueryCases {
			splitRe.FindAllString(c.query, -1)
		}
	}
}

func BenchmarkSplitQueryRegexBytesIndex(b *testing.B) {
	var a [][]byte
	for _, c := range splitQueryCases {
		a = append(a, []byte(c.query))
	}

	for n := 0; n < b.N; n++ {
		for _, q := range a {
			splitRe.FindAllIndex(q, -1)
		}
	}
}

func BenchmarkSplitQueryRegexStringIndex(b *testing.B) {
	for n := 0; n < b.N; n++ {
		for _, c := range splitQueryCases {
			splitRe.FindAllStringIndex(c.query, -1)
		}
	}
}

func BenchmarkSplitQueryRegexBytes(b *testing.B) {
	var a [][]byte
	for _, c := range splitQueryCases {
		a = append(a, []byte(c.query))
	}

	for n := 0; n < b.N; n++ {
		for _, q := range a {
			splitRe.FindAll(q, -1)
		}
	}
}

func FuzzProcessQuery(f *testing.F) {
	f.Fuzz(func(t *testing.T, q string) {
		processQuery(q)
	})
}
