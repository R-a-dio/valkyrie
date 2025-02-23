package bleve

import (
	"context"
	"testing"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/stretchr/testify/require"
)

func newTestIndex(t testing.TB) *indexWrap {
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
	idx := newTestIndex(t)

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
	t.Run("ngram romaji", func(t *testing.T) {
		rq, err := NewQuery(ctx, "motome hana", false)
		require.NoError(t, err)

		t.Log(rq)
		req := NewSearchRequest(rq, 100, 0)

		res, err := idx.index.Search(req)
		require.NoError(t, err)
		require.Equal(t, 1, res.Hits.Len())
	})

	t.Run("exact only", func(t *testing.T) {
		rq, err := NewQuery(ctx, "toyosaki aki", true)
		require.NoError(t, err)

		t.Log(rq)
		req := NewSearchRequest(rq, 100, 0)

		res, err := idx.index.Search(req)
		require.NoError(t, err)
		require.Equal(t, 1, res.Hits.Len())
	})
}

func TestAnalyzer(t *testing.T) {
	idx := newTestIndex(t)

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
