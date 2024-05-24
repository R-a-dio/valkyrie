package mariadb

import (
	"testing"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/jmoiron/sqlx"
)

type namedTest struct {
	name  string
	query string
	in    any
}

var sqlxNamedTests = []namedTest{
	{"trackInsertQuery", trackInsertQuery, radio.Song{DatabaseTrack: &radio.DatabaseTrack{}}},
	{"trackUpdateMetadataQuery", trackUpdateMetadataQuery, radio.Song{DatabaseTrack: &radio.DatabaseTrack{}}},
	{"songCreateQuery", songCreateQuery, radio.Song{DatabaseTrack: &radio.DatabaseTrack{}}},
	{"newsCreateQuery", newsCreateQuery, radio.NewsPost{}},
	{"newsUpdateQuery", newsUpdateQuery, radio.NewsPost{}},
	{"submissionInsertPostPendingQuery", submissionInsertPostPendingQuery, adjustedPendingSong{}},
}

// TestSqlxNamed tests if arguments are properly named in queries listed in sqlxNamedTests
func TestSqlxNamed(t *testing.T) {
	for _, c := range sqlxNamedTests {
		t.Run(c.name, func(t *testing.T) {
			_, _, err := sqlx.Named(c.query, c.in)
			if err != nil {
				t.Error(err)
			}
		})
	}
}
