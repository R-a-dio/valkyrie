package mariadb

import (
	"context"
	"regexp"
	"strings"

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

	search_query, err := processQuery(search_query)
	if err != nil {
		return nil, err
	}

	var total int
	var result []searchTrack

	err = sqlx.Select(h, &result, searchSearchQuery, search_query, limit, offset)
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

func processQuery(q string) (string, error) {
	terms := splitQuery(q)
	for i, term := range terms {
		if isQuoted(term) {
			continue
		}

		if term[len(term)-1] != '*' {
			terms[i] = term + "*"
		}
	}
	return strings.Join(terms, " "), nil
}

var splitRe = regexp.MustCompile(`[^\s"]+|"([^"]*)"`)

func splitQuery(q string) []string {
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
