package bleve

import (
	"context"
	"encoding/json"
	"fmt"
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

func NewQuery(ctx context.Context, query string, exactOnly bool) (*RadioQuery, error) {
	query = strings.TrimSpace(query)

	if len(query) > MaxQuerySize { // cut off the query if it goes past our MaxQuerySize
		// but in a nice way, where we remove any invalid utf8 characters from the end
		query = CutoffAtRune(query[:MaxQuerySize])
	}

	var (
		rq = &RadioQuery{
			FieldQueries: make(map[string]string),
		}
		splitQuery []string
		fieldName  string
		inBlock    = false
		block      []string
	)

	for _, part := range strings.Fields(query) {
		if !inBlock {
			after, isOperator := strings.CutPrefix(part, "!!")
			if isOperator {
				// this is an operator.
				// !!<field
				if len(after) > 0 && (after[0] == '<' || after[0] == '>') {
					// sort order; the rest of this should be a field
					field := after[1:]
					if isValidField(field) {
						rq.SortField = fieldWithPrefix(field)
						rq.Descending = after[0] == '<'
						continue
					}
				}
				//    !!field:term
				// or !!field:"term
				colonIdx := strings.Index(after, ":")
				if colonIdx != -1 && isValidField(after[0:colonIdx]) {
					fieldName = after[0:colonIdx]
					fieldValue := after[colonIdx+1:]
					fieldValue, isQuoted := strings.CutPrefix(fieldValue, "\"")
					if isQuoted {
						if fieldValue[len(fieldValue)-1] == '"' {
							// quoted single-term !!field:"term"
							rq.FieldQueries[fieldName] = fieldValue[:len(fieldValue)-1]
						} else {
							// quoted multi-term !!field:"multi term"
							inBlock = true
							block = append(block, fieldValue)
						}
					} else {
						// unquoted field !!field:term
						rq.FieldQueries[fieldName] = fieldValue
					}
					continue
				}
				// we couldn't parse this operator, so just pass it as
				// a regular term
			}
		}

		if inBlock {
			before, found := strings.CutSuffix(part, "\"")
			block = append(block, before)
			if found {
				// end the block and save it as a field query
				rq.FieldQueries[fieldName] = strings.Join(block, " ")
				block, inBlock = nil, false
				continue
			}
		} else {
			splitQuery = append(splitQuery, part)
		}
	}

	rq.Query = strings.Join(splitQuery, " ")

	if inBlock {
		// if the block was not terminated, don't return any results
		rq.Query = ""
		rq.FieldQueries = nil
	}

	return rq, nil
}

var fields = []string{
	"artist", "title", "album", "tags", "id", "acceptor", "editor",
	"priority", "lastrequested", "lastplayed",
}

func isValidField(s string) bool {
	for _, f := range fields {
		if f == s {
			return true
		}
	}
	return false
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
	Query        string            `json:"query"`
	FieldQueries map[string]string `json:"fieldQueries"`
	SortField    string            `json:"sort"`
	Descending   bool              `json:"desc"`
	ExactOnly    bool              `json:"exact_only"`
}

func fieldWithPrefix(f string) string {
	switch f {
	case "artist":
		return "radio.artist"
	case "title":
		return "radio.title"
	case "album":
		return "radio.album"
	case "tags":
		return "radio.tags"
	default:
		return f
	}
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

	// analyze our query with the default analyzer, this should be the same one as
	// used for the index generation

	fmt.Println(rq.FieldQueries)
	km, _ := json.MarshalIndent(m, "", "  ")
	fmt.Println(string(km))
	root_list := make([]query.Query, 0, 4)

	analyze := func(ngram_field string, exact_field string, q string) {

		ngram_list := make([]query.Query, 0, 32)
		exact_list := make([]query.Query, 0, 32)

		// Special case for star to match everything
		if q == "*" {
			matchAll := bleve.NewMatchAllQuery()
			matchAll.SetBoost(0.2)
			root_list = append(root_list, matchAll)
			return
		}
		analyzerName := m.AnalyzerNameForPath(ngram_field)
		fmt.Println(ngram_field, analyzerName)
		analyzer := m.AnalyzerNamed(analyzerName)
		tokens := analyzer.Analyze([]byte(q))

		// otherwise do some light filtering on the tokens returned, while we want these for
		// the indexing operation, we don't need (or want) all of them for the query search
		for _, token := range tokens {
			// skip ngram analyzer if we only want exact matches
			if rq.ExactOnly && analyzerName == radioAnalyzerName {
				continue
			}
			// skip shingle tokens, these will only match if they're in the exact order and exact token composition
			// which is not useful for our purpose
			if token.Type == analysis.Shingle {
				// TODO: check if we want to add these to a disjunction query
				continue
			}
			// skip tokens longer than our ngram filter, these won't match ever unless it's an exact match with
			// what is in the index
			//if utf8.RuneCount(token.Term) > NgramFilterMax {
			//	continue
			//}

			tq := query.NewTermQuery(string(token.Term))
			tq.SetField(ngram_field)
			tq.SetBoost(0.2)
			ngram_list = append(ngram_list, tq)
		}

		// check the exact field for exact matches, they boost the score
		// Only do this if the exact field exists
		if (m.FieldMappingForPath(exact_field) != mapping.FieldMapping{}) {
			exactAnalyzer := m.AnalyzerNamed(exactAnalyzerName)
			exactTokens := exactAnalyzer.Analyze([]byte(q))
			for _, token := range exactTokens {
				// skip shingle tokens, these will only match if they're in the exact order and exact token composition
				// which is not useful for our purpose
				if token.Type == analysis.Shingle {
					// TODO: check if we want to add these to a disjunction query
					continue
				}
				tq := query.NewTermQuery(string(token.Term))
				tq.SetField(exact_field)
				tq.SetBoost(2.0)
				exact_list = append(exact_list, tq)
			}
		}

		if len(ngram_list) == 0 && len(exact_list) == 0 {
			return
		}

		ngram := query.NewConjunctionQuery(ngram_list)
		ngram.SetBoost(1.0)

		exact := query.NewConjunctionQuery(exact_list)
		exact.SetBoost(1.0)

		comb := query.NewDisjunctionQuery([]query.Query{ /*exact,*/ ngram})
		comb.SetBoost(1.0)

		root_list = append(root_list, comb)
	}

	analyze("ngram_", "exact_", rq.Query)

	for field, query := range rq.FieldQueries {
		analyze(fieldWithPrefix(field), "exact."+field, query)
	}

	if len(root_list) == 0 {
		// no subqueries, so we just match nothing
		noneQuery := query.NewMatchNoneQuery()
		return noneQuery.Searcher(ctx, i, m, options)
	}

	root := query.NewConjunctionQuery(root_list)
	root.SetBoost(1.0)

	fmt.Println(query.DumpQuery(m, root))
	return root.Searcher(ctx, i, m, options)
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
