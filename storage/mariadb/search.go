package mariadb

import (
	"bytes"
	"context"
	"regexp"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/jmoiron/sqlx"
)

type SearchService struct {
	db *sqlx.DB
}

type searchTrack struct {
	radio.Song
	Score float64
}

var searchSearchQuery = expand(`
SELECT
	{trackColumns},
	{lastplayedSelect},
	{songColumns}
FROM
(SELECT *, MATCH (artist, track, album, tags) AGAINST (? IN BOOLEAN MODE) score
	FROM tracks 
	HAVING score > 0
	ORDER BY score DESC
	LIMIT ? OFFSET ?) AS tracks
JOIN
	esong ON esong.hash = tracks.hash
ORDER BY
	score DESC;
`)

var searchTotalQuery = `
SELECT COUNT(*) FROM 
(SELECT *, MATCH (artist, track, album, tags) AGAINST (? IN BOOLEAN MODE) score
	FROM tracks 
	HAVING score > 0) AS tracks;
`

func (ss SearchService) Search(ctx context.Context, search_query string, limit int64, offset int64) (*radio.SearchResult, error) {
	h := handle{ss.db, ctx, "search"}

	search_query = processQuery(search_query)

	var total int
	var result []searchTrack

	err := sqlx.Select(h, &result, searchSearchQuery, search_query, limit, offset)
	if err != nil {
		return nil, err
	}

	err = sqlx.Get(h, &total, searchTotalQuery, search_query)
	if err != nil {
		return nil, err
	}

	var songs = make([]radio.Song, len(result))
	for i, tmp := range result {
		songs[i] = tmp.Song
	}

	return &radio.SearchResult{Songs: songs, TotalHits: total}, nil
}

var (
	queryMinus = []byte("-")
	querySpace = []byte(" ")
)

func processQuery(q string) string {
	return string(processQueryB([]byte(q)))
}

func processQueryB(q []byte) []byte {
	terms := splitQueryB(q)
	for i, term := range terms {
		// no extra handling if the term is quoted, we pass it as-is
		if isQuoted(term) {
			continue
		}
		// if we're in a parenthesed block we run processQuery recursively
		if isParen(term) {
			noParen := term[1 : len(term)-1]
			noParen = processQueryB(noParen)
			term = append(term[:1], noParen...)
			terms[i] = append(term, ')')
			continue
		}

		// remove any -
		term = bytes.ReplaceAll(term, queryMinus, querySpace)
		term = bytes.TrimSpace(term)

		if len(term) > 0 {
			switch term[len(term)-1] {
			case ')':
			default:
				term = append(term, '*')
			}
		}

		term = bytes.TrimSpace(term)
		terms[i] = term
	}
	return bytes.Join(terms, querySpace)
}

// splits on any whitespace but keeps quoted sections together
// var splitRe = regexp.MustCompile(`[^\s"]+|"([^"]*)"`)
var splitRe = regexp.MustCompile(`\(([^)]*)\)|[^\s"]+|"([^"]*)"`)

func splitQueryB(q []byte) [][]byte {
	return splitRe.FindAll(q, -1)
}

func splitQuery(q string) [][]byte {
	return splitQueryB([]byte(q))
}

func isQuoted(s []byte) bool {
	if len(s) == 0 {
		return false
	}
	return s[0] == '"' && s[len(s)-1] == '"'
}

func isParen(s []byte) bool {
	if len(s) == 0 {
		return false
	}
	return s[0] == '(' && s[len(s)-1] == ')'
}

func (ss SearchService) Update(context.Context, ...radio.Song) error {
	// noop since we use the active storage as index
	return nil
}

func (ss SearchService) Delete(context.Context, ...radio.Song) error {
	// noop since we use the active storage as index
	return nil
}
