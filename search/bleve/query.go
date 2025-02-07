package bleve

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/R-a-dio/valkyrie/errors"
	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/analysis"
	"github.com/blevesearch/bleve/v2/mapping"
	"github.com/blevesearch/bleve/v2/search"
	"github.com/blevesearch/bleve/v2/search/query"
	index "github.com/blevesearch/bleve_index_api"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

const MaxQuerySize = 512

func NewQuery(ctx context.Context, query string) (query.Query, error) {
	query = strings.TrimSpace(query)
	if query == "*" {
		// as a special case a singular wildcard query is turned into a match all query
		return bleve.NewMatchAllQuery(), nil
	}
	if len(query) > MaxQuerySize { // cut off the query if it goes past our MaxQuerySize
		// but in a nice way, where we remove any invalid utf8 characters from the end
		query = CutoffAtRune(query[:MaxQuerySize])
	}
	return &RadioQuery{query}, nil
}

func CutoffAtRune(s string) string {
	for len(s) > 0 {
		r, size := utf8.DecodeLastRuneInString(s)
		if r != utf8.RuneError {
			break
		}
		s = s[:len(s)-size]
	}
	return s
}

type RadioQuery struct {
	Query string `json:"query"`
}

func (rq *RadioQuery) Searcher(ctx context.Context, i index.IndexReader, m mapping.IndexMapping, options search.SearcherOptions) (search.Searcher, error) {
	const op errors.Op = "search/bleve.RadioQuery.Searcher"
	// generate a trace span with the query so we can find "slow" queries
	ctx, span := otel.Tracer("").Start(ctx, string(op))
	defer span.End()
	if span.IsRecording() {
		span.SetAttributes(attribute.KeyValue{
			Key:   "query",
			Value: attribute.StringValue(rq.Query),
		})
	}

	field := m.DefaultSearchField()
	analyzerName := m.AnalyzerNameForPath(field)
	analyzer := m.AnalyzerNamed(analyzerName)

	// analyze our query with the default analyzer, this should be the same one as
	// used for the index generation
	tokens := analyzer.Analyze([]byte(rq.Query))
	if len(tokens) == 0 {
		// no tokens, so we just match nothing
		noneQuery := query.NewMatchNoneQuery()
		return noneQuery.Searcher(ctx, i, m, options)
	}

	// otherwise do some light filtering on the tokens returned, while we want these for
	// the indexing operation, we don't need (or want) all of them for the query search
	//should := make([]query.Query, 0, 0)
	must := make([]query.Query, 0, len(tokens))
	for _, token := range tokens {
		// skip shingle tokens, these will only match if they're in the exact order and exact token composition
		// which is not useful for our purpose
		if token.Type == analysis.Shingle {
			// TODO: check if we want to add these to a disjunction query
			continue
		}
		// skip tokens longer than our ngram filter, these won't match ever unless it's an exact match with
		// what is in the index
		if utf8.RuneCount(token.Term) > NgramFilterMax {
			// TODO: check if we want to add these to a disjunction query
			continue
		}

		tq := query.NewTermQuery(string(token.Term))
		tq.SetField(field)
		tq.SetBoost(1.0)
		must = append(must, tq)
	}

	cq := query.NewConjunctionQuery(must)
	cq.SetBoost(1.0)

	//fmt.Println(query.DumpQuery(m, cq))
	return cq.Searcher(ctx, i, m, options)
}

func filterQuery(q *query.Query, filter ...func(q *query.Query)) {
	switch v := (*q).(type) {
	case *query.BooleanQuery:
		filterQuery(&v.Must, filter...)
		filterQuery(&v.MustNot, filter...)
		filterQuery(&v.Should, filter...)
	case *query.ConjunctionQuery:
		for i := range v.Conjuncts {
			filterQuery(&v.Conjuncts[i], filter...)
		}
	case *query.DisjunctionQuery:
		for i := range v.Disjuncts {
			filterQuery(&v.Disjuncts[i], filter...)
		}
	case nil:
	default:
		for _, fn := range filter {
			fn(q)
		}
	}
}

func ChangeLoneWildcardIntoMatchAll(q *query.Query) {
	wq, ok := (*q).(*query.WildcardQuery)
	if !ok {
		return
	}
	if strings.TrimSpace(wq.Wildcard) == "*" {
		*q = bleve.NewMatchAllQuery()
	}
}

func AddFuzzy(q *query.Query) {
	const fuzzyMin = 3

	switch fq := (*q).(type) {
	case *query.MatchQuery:
		if len(fq.Match) > fuzzyMin {
			fq.SetFuzziness(1)
		}
	case *query.FuzzyQuery:
		if len(fq.Term) > fuzzyMin && fq.Fuzziness == 0 {
			fq.SetFuzziness(1)
		}
	case *query.MatchPhraseQuery:
		if len(fq.MatchPhrase) > fuzzyMin {
			fq.SetFuzziness(1)
		}
	}
}
