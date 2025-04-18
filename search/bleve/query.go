package bleve

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/R-a-dio/valkyrie/errors"
	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/analysis"
	"github.com/blevesearch/bleve/v2/mapping"
	"github.com/blevesearch/bleve/v2/numeric"
	"github.com/blevesearch/bleve/v2/search"
	"github.com/blevesearch/bleve/v2/search/query"
	index "github.com/blevesearch/bleve_index_api"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

const MaxQuerySize = 512

func NewFieldSort(field string, desc bool) search.SearchSort {
	return &search.SortField{
		Field: sortField(field),
		Desc:  desc,
	}
}

func NewQuery(ctx context.Context, query string, exactOnly bool) (*RadioQuery, error) {
	query = strings.TrimSpace(query)

	if len(query) > MaxQuerySize { // cut off the query if it goes past our MaxQuerySize
		// but in a nice way, where we remove any invalid utf8 characters from the end
		query = CutoffAtRune(query[:MaxQuerySize])
	}

	var (
		rq = &RadioQuery{
			RawQuery:     query,
			FieldQueries: make(map[string]string),
			ExactOnly:    exactOnly,
		}
		splitQuery []string
		fieldName  string
		inBlock    = false
		block      []string
	)

	for _, part := range strings.Fields(query) {
		if !inBlock {
			// operators start with !!
			after, isOperator := strings.CutPrefix(part, "!!")
			if isOperator {
				// !!<field or !!>field is a sort operator
				if len(after) > 0 && (after[0] == '<' || after[0] == '>') {
					// sort order; the rest of this should be a field
					field := after[1:]
					if isValidField(field) {
						var desc = after[0] == '<'

						rq.Sort = append(rq.Sort, NewFieldSort(field, desc))
						continue
					}
				}

				// !!field: is a field-specific query, these can be either quoted for
				// things containing spaces, or be a single term, examples:
				//		!!field:single
				//		!!field:"multi term"
				//		!!field:"single"

				var fieldValue string
				var found bool
				fieldName, fieldValue, found = strings.Cut(after, ":")
				if found && isValidField(fieldName) {
					fieldValue, isQuoted := strings.CutPrefix(fieldValue, "\"")
					if isQuoted {
						if len(fieldValue) > 0 && fieldValue[len(fieldValue)-1] == '"' {
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

	if rq.Sort == nil {
		rq.Sort = newPrioScoreSort()
	}

	return rq, nil
}

func isValidField(s string) bool {
	switch s {
	case "artist", "title", "album", "tags",
		"id", "acceptor", "editor", "priority",
		"lr", "lp", "rc":
		return true
	default:
		return false
	}
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
	RawQuery     string            `json:"raw_query"`
	Query        string            `json:"query"`
	FieldQueries map[string]string `json:"field_queries"`
	Sort         search.SortOrder  `json:"sort"`
	ExactOnly    bool              `json:"exact_only"`
}

func NewSearchRequest(query *RadioQuery, limit, offset int) *bleve.SearchRequest {
	return &bleve.SearchRequest{
		Query:  query,
		Sort:   query.Sort,
		Size:   limit,
		From:   offset,
		Fields: dataField,
	}
}

var _ search.SearchSort = &prioScoreSort{}

func newPrioScoreSort() search.SortOrder {
	return search.SortOrder{&prioScoreSort{
		Desc: true,
	}}
}

// prioScoreSort sorts the documents by their priority and score
type prioScoreSort struct {
	Desc bool
	prio float64
}

func (s *prioScoreSort) UpdateVisitor(field string, term []byte) {
	if field != "priority" {
		return
	}
	valid, shift := numeric.ValidPrefixCodedTermBytes(term)
	if !valid || shift != 0 {
		return
	}
	prio, _ := numeric.PrefixCoded(term).Int64()
	s.prio = numeric.Int64ToFloat64(prio)
}

func (s *prioScoreSort) Value(a *search.DocumentMatch) string {
	// boost sort score if we had a large match score; this means that
	// there were exact matches
	if a.Score > 0.5 {
		s.prio += 1000
	}

	return fmt.Sprintf("%010d", int(s.prio))
}

func (s *prioScoreSort) Descending() bool {
	return s.Desc
}

func (s *prioScoreSort) RequiresDocID() bool {
	return false
}

func (s *prioScoreSort) RequiresScoring() bool {
	return false
}

func (s *prioScoreSort) RequiresFields() []string {
	return []string{"priority"}
}

func (s *prioScoreSort) Reverse() {
	s.Desc = !s.Desc
}

func (s prioScoreSort) Copy() search.SearchSort {
	return &s
}

func (rq *RadioQuery) generateFieldQuery(m mapping.IndexMapping, field string, q string, boost float64) query.Query {
	if q == "*" {
		// special case for matching everything
		return bleve.NewMatchAllQuery()
	}

	var analyzerName string
	switch field {
	case exactCompositeField:
		analyzerName = exactAnalyzerName
	case radioCompositeField:
		analyzerName = radioAnalyzerName
	default:
		analyzerName = m.AnalyzerNameForPath(field)
	}

	analyzer := m.AnalyzerNamed(analyzerName)

	tokens := analyzer.Analyze([]byte(q))

	queries := make([]query.Query, 0, len(tokens))
	for _, token := range tokens {
		if token.Type == analysis.Shingle {
			continue
		}

		tq := query.NewTermQuery(string(token.Term))
		tq.SetField(field)
		tq.SetBoost(boost)
		queries = append(queries, tq)
	}

	if len(queries) == 1 {
		return queries[0]
	}

	return query.NewConjunctionQuery(queries)
}

func sortField(f string) string {
	switch f {
	case "id":
		return "sort.id"
	case "artist":
		return "sort.artist"
	case "title":
		return "sort.title"
	case "album":
		return "sort.album"
	case "tags":
		return "sort.tags"
	default:
		return f
	}
}

func exactField(f string) string {
	switch f {
	case "_all":
		return "_exact"
	case "artist":
		return "exact.artist"
	case "title":
		return "exact.title"
	case "album":
		return "exact.album"
	case "tags":
		return "exact.tags"
	default:
		return f
	}
}

func ngramField(f string) string {
	switch f {
	case "_all":
		return "_radio"
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

func (rq *RadioQuery) generateQuery(m mapping.IndexMapping, field string, q string) query.Query {
	const exactBoost = 2.0
	const ngramBoost = 0.2

	switch field {
	case "title", "artist", "album", "tags", "_all":
		queries := make([]query.Query, 0, 2)

		exact := rq.generateFieldQuery(m, exactField(field), q, exactBoost)
		queries = append(queries, exact)

		if !rq.ExactOnly {
			ngram := rq.generateFieldQuery(m, ngramField(field), q, ngramBoost)
			queries = append(queries, ngram)
		}

		if len(queries) == 1 {
			return queries[0]
		}
		return query.NewDisjunctionQuery(queries)
	default:
		return rq.generateFieldQuery(m, field, q, exactBoost)
	}
}

func (rq *RadioQuery) Searcher(ctx context.Context, i index.IndexReader, m mapping.IndexMapping, options search.SearcherOptions) (search.Searcher, error) {
	const op errors.Op = "search/bleve.RadioQuery.Searcher"
	// generate a trace span with the query so we can find "slow" queries
	ctx, span := otel.Tracer("").Start(ctx, string(op))
	defer span.End()
	if span.IsRecording() {
		attr := make([]attribute.KeyValue, 0, len(rq.FieldQueries)+2)

		attr = append(attr, attribute.KeyValue{
			Key:   "raw_query",
			Value: attribute.StringValue(rq.RawQuery),
		})
		attr = append(attr, attribute.KeyValue{
			Key:   "query",
			Value: attribute.StringValue(rq.Query),
		})

		for field, query := range rq.FieldQueries {
			attr = append(attr, attribute.KeyValue{
				Key:   attribute.Key("field_" + field),
				Value: attribute.StringValue(query),
			})
		}

		span.SetAttributes(attr...)
	}

	var queries []query.Query
	if rq.Query != "" {
		q := rq.generateQuery(m, "_all", rq.Query)
		queries = append(queries, q)
	}

	for field, query := range rq.FieldQueries {
		q := rq.generateQuery(m, field, query)
		queries = append(queries, q)
	}

	if len(queries) == 0 {
		// no subqueries, so we just match nothing
		noneQuery := query.NewMatchNoneQuery()
		return noneQuery.Searcher(ctx, i, m, options)
	}

	q := query.NewConjunctionQuery(queries)
	q.SetBoost(1.0)

	return q.Searcher(ctx, i, m, options)
}
