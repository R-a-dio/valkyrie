package mariadb

import (
	"context"
	"slices"
	"testing"

	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/search"
	_ "github.com/R-a-dio/valkyrie/search/storage"
)

type splitCase struct {
	query  string
	expect []string
}

var splitQueryCases = []splitCase{
	{`"hello world" and testing`, []string{`"hello world"`, `and`, `testing`}},
}

type processCase struct {
	query  string
	expect string
}

var processQueryCases = []processCase{
	{`"hello world" and testing`, `"hello world" and* testing*`},
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
		if result != c.expect {
			t.Error(result, c.expect)
		}
	}
}

func TestSearch(t *testing.T) {
	cfg, _ := config.LoadFile()

	ss, err := search.Open(context.Background(), cfg)
	if err != nil {
		t.Error(err)
		return
	}

	res, err := ss.Search(context.Background(), "''adv*", 10, 0)
	if err != nil {
		t.Error(err)
		return
	}

	t.Log(res)
}
