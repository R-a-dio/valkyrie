package mariadb

import (
	"slices"
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
}

type processCase struct {
	query  string
	expect string
}

var processQueryCases = []processCase{
	{`"hello world" and testing`, `"hello world" and* testing*`},
	{`"hello-world" and testing`, `"hello-world" and* testing*`},
	{`"hello-world" and -testing`, `"hello-world" and* testing*`},
}

func TestSplitQuery(t *testing.T) {
	for _, c := range splitQueryCases {
		result := splitQuery(c.query)
		if !slices.Equal(result, c.expect) {
			t.Error(result, c.expect)
		}
	}
}

func TestProcessQuery(t *testing.T) {
	for _, c := range processQueryCases {
		result, _ := processQuery(c.query)
		assert.Equal(t, c.expect, result)
	}
}

func BenchmarkProcessQuery(b *testing.B) {
	for n := 0; n < b.N; n++ {
		for _, c := range processQueryCases {
			processQuery(c.query)
		}
	}
}
