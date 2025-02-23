package mariadb_test

import (
	"context"
	"os"
	"testing"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/storage/mariadb"
	storagetest "github.com/R-a-dio/valkyrie/storage/test"
	"github.com/davecgh/go-spew/spew"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type splitCase struct {
	query  string
	expect []string
}

var splitQueryCases = []splitCase{
	{`"hello world" and testing`, []string{`"hello world"`, `and`, `testing`}},
	{`"hello world and testing`, []string{`hello`, `world`, `and`, `testing`}},
	{`hello world" and testing`, []string{`hello`, `world`, `and`, `testing`}},
	{`(hello world) and testing`, []string{`(hello world)`, `and`, `testing`}},
	{`("hello world") and testing`, []string{`("hello world")`, `and`, `testing`}},
	{`("hello world" and testing`, []string{`(`, `"hello world"`, `and`, `testing`}},
}

type processCase struct {
	query  string
	expect string
}

var processQueryCases = []processCase{
	{``, ``},
	{`"hello world" and testing`, `"hello world" +and* +testing*`},
	{`"hello world" and* testing`, `"hello world" +and* +testing*`},
	{`   "hello world" and* testing`, `"hello world" +and* +testing*`},
	//{`"hello-world" and testing`, `"hello-world" +and* +testing*`},
	{`"hello-world" and testing`, `"hello world" +and* +testing*`},
	//{`"hello-world" and -testing`, `"hello-world" +and* +testing*`},
	{`"hello-world" and -testing`, `"hello world" +and* +testing*`},
	{`(hello world) and testing`, `+hello* +world* +and* +testing*`},
	{`("hello world") and testing`, `"hello world" +and* +testing*`},
	{`("hello world" and more) and testing`, `"hello world" +and* +more* +and* +testing*`},
}

func TestSplitQuery(t *testing.T) {
	for _, c := range splitQueryCases {
		result := mariadb.SplitQuery(c.query)

		res := make([]string, len(result))
		for i, e := range result {
			res[i] = string(e)
		}
		assert.Equal(t, c.expect, res)
	}
}

func TestProcessQuery(t *testing.T) {
	for _, c := range processQueryCases {
		result := mariadb.ProcessQuery(c.query)
		_ = result
		assert.Equal(t, c.expect, result, c.query)
	}
}

func BenchmarkProcessQuery(b *testing.B) {
	for n := 0; n < b.N; n++ {
		for _, c := range processQueryCases {
			mariadb.ProcessQuery(c.query)
		}
	}
}

func FuzzProcessQuery(f *testing.F) {
	ctx := context.Background()
	ctx = zerolog.New(os.Stdout).Level(zerolog.ErrorLevel).WithContext(ctx)
	ctx = storagetest.PutT(ctx, f)

	setupCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	setup := new(MariaDBSetup)
	err := setup.Setup(setupCtx)
	require.NoError(f, err)
	defer setup.TearDown(ctx)

	s := setup.CreateStorage(ctx)

	ss := s.(interface {
		Search() radio.SearchService
	}).Search()

	f.Fuzz(func(t *testing.T, ogq string) {
		q := mariadb.ProcessQuery(ogq)

		_, err := ss.Search(ctx, q, radio.SearchOptions{Limit: 20})
		if !errors.Is(errors.SearchNoResults, err) && !assert.NoError(t, err) {
			t.Error(spew.Sdump([]byte(ogq)))
			t.Error(spew.Sdump([]byte(q)))
		}
	})
}
