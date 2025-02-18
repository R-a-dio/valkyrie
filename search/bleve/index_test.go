package bleve

import (
	"context"
	"testing"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/search"
	"github.com/stretchr/testify/require"
)

func newIndex(t testing.TB) *indexWrap {
	idx, err := NewIndex(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() {
		idx.Close()
	})
	return idx
}

type song struct {
	artist, title string
}

var testData = []song{
	{"桃色技術音楽堂×中路もとめ feat. Hanamori Rie", "Pink smoothie"},
	{"School Food Punishment", "How To Go"},
	{"Taishi feat. みとせのりこ", "Personalizer"},
	{"Toyosaki Aki & Hikasa Youko & Satou Satomi & Kotobuki Minako & Taketatsu Ayana", "U&I (Studio Mix)"},
}

func TestIndexing(t *testing.T) {
	ctx := context.Background()
	idx := newIndex(t)

	var songs []radio.Song
	for i, song := range testData {
		songs = append(songs, radio.Song{
			DatabaseTrack: &radio.DatabaseTrack{
				TrackID: radio.TrackID(i),
				Title:   song.title,
				Artist:  song.artist,
			},
		})
	}

	// add the songs to the index
	err := idx.Index(ctx, songs)
	require.NoError(t, err)

	// make sure they now exist
	count, err := idx.index.DocCount()
	require.NoError(t, err)
	require.EqualValues(t, len(testData), count)

	// now do our search tests
	rq, err := NewQuery(ctx, "motome hana", true)
	require.NoError(t, err)

	req := bleve.NewSearchRequestOptions(rq, 100, 0, true)
	req.SortByCustom(search.SortOrder{new(prioScoreSort)})
	req.Fields = dataField

	res, err := idx.index.Search(req)
	require.NoError(t, err)

	t.Log(res)
}

func TestAnalyzer(t *testing.T) {
	idx := newIndex(t)

	ian := idx.index.Mapping().AnalyzerNamed(radioAnalyzerName)

	for _, in := range testData {
		in := radio.Metadata(in.artist, in.title)
		ires := ian.Analyze([]byte(in))

		t.Log("============", in, "============")
		t.Log("============ INDEX ============")
		for i, token := range ires {
			t.Logf("%d %s\n", i, token)
		}
		t.Log("===============================")
	}
}
