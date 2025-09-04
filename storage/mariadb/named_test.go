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
	{"trackRandomFavoriteOf", trackRandomFavoriteOfQuery, RandomFavoriteOfParams{}},
	{"songCreateQuery", songCreateQuery, radio.Song{DatabaseTrack: &radio.DatabaseTrack{}}},
	{"newsCreateQuery", newsCreateQuery, radio.NewsPost{}},
	{"newsUpdateQuery", newsUpdateQuery, radio.NewsPost{}},
	{"newsGetQuery", newsGetQuery, NewsGetParams{}},
	{"newsDeleteQuery", newsDeleteQuery, NewsDeleteParams{}},
	{"newsListQuery", newsListQuery, NewsListParams{}},
	{"newsListPublicQuery", newsListPublicQuery, NewsListParams{}},
	{"newsComments", newsCommentsQuery, NewsCommentsParams{}},
	{"newsCommentsPublic", newsCommentsPublicQuery, NewsCommentsParams{}},
	{"submissionInsertPostPendingQuery", submissionInsertPostPendingQuery, adjustedPendingSong{}},
	{"trackFilterSongsFavoriteOfQuery", trackFilterSongsFavoriteOfQuery, FilterSongsFavoriteOfParams{}},
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

func TestNamedQueries(t *testing.T) {
	for identifier, err := range invalidQueries {
		t.Errorf("invalid query at: %s: cause: %s", identifier, err)
	}
}
