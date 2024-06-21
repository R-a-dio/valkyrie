package audio

import (
	"context"
	"testing"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteMetadata(t *testing.T) {
	fsys := afero.NewOsFs()
	f, err := fsys.Open("testdata/MP3_2MG.mp3")
	require.NoError(t, err)

	song := radio.Song{
		DatabaseTrack: &radio.DatabaseTrack{
			Title:  "test",
			Artist: "some kind of artist",
			Album:  "a kind of album",
			Tags:   "test effective very",
		},
	}

	out, err := WriteMetadata(context.Background(), f, song)
	require.NoError(t, err)

	// probe the output file
	info, err := probeText(context.Background(), out.Memfd.File)
	require.NoError(t, err)

	assert.Equal(t, song.Title, info.Title)
	assert.Equal(t, song.Artist, info.Artist)
	assert.Equal(t, song.Album, info.Album)
	assert.Equal(t, song.Tags, info.Comment)
}
