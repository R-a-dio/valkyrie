package mariadb

import (
	"context"
	"regexp"
	"strings"
	"unicode"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
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
	const op errors.Op = "mariadb/SearchService.Search"
	handle := handle{ss.db, ctx, "search"}
	handle, deferFn := handle.span(op)
	defer deferFn()

	search_query = ProcessQuery(search_query)

	var total int
	var result []searchTrack

	err := sqlx.Select(handle, &result, searchSearchQuery, search_query, limit, offset)
	if err != nil {
		return nil, errors.E(op, err)
	}

	err = sqlx.Get(handle, &total, searchTotalQuery, search_query)
	if err != nil {
		return nil, errors.E(op, err)
	}

	var songs = make([]radio.Song, len(result))
	for i, tmp := range result {
		songs[i] = tmp.Song
	}

	return &radio.SearchResult{Songs: songs, TotalHits: total}, nil
}

const maxQuerySize = 128

func ProcessQuery(q string) string {
	if len(q) > maxQuerySize {
		q = q[:maxQuerySize]
	}

	q = strings.Map(func(r rune) rune {
		if !unicode.IsGraphic(r) {
			return -1
		}

		switch r {
		case '*', '(', ')', '%', '@', '+', '<', '>', '~', '-':
			return ' '
		default:
			return r
		}
	}, q)

	terms := SplitQuery(q)
	for i, term := range terms {
		// trim any extra whitespace
		term = strings.TrimSpace(term)

		// no extra handling if the term is quoted, we pass it as-is
		if isQuoted(term) {
			continue
		}

		// then try and add a + to the start of the term and a * at the end
		if len(term) > 0 {
			term = "+" + term + "*"
		}

		terms[i] = term
	}

	return strings.Join(terms, " ")
}

// splits on any whitespace but keeps quoted sections together
// var splitRe = regexp.MustCompile(`[^\s"]+|"([^"]*)"`)
var splitRe = regexp.MustCompile(`\(([^)]*)\)|[^\s"]+|"([^"]*)"`)

func SplitQuery(q string) []string {
	return splitRe.FindAllString(q, -1)
}

func isQuoted(s string) bool {
	if len(s) == 0 {
		return false
	}
	return s[0] == '"' && s[len(s)-1] == '"'
}

func (ss SearchService) Update(context.Context, ...radio.Song) error {
	// noop since we use the active storage as index
	return nil
}

func (ss SearchService) Delete(context.Context, ...radio.Song) error {
	// noop since we use the active storage as index
	return nil
}
