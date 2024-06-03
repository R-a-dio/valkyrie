package storagetest

import (
	"testing"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func (suite *Suite) TestStatusStore(t *testing.T) {
	ss := suite.Storage(t).Status(suite.ctx)

	in := radio.Status{
		User: radio.User{
			DJ: radio.DJ{
				ID: 500,
			},
		},
		Song: radio.Song{
			Metadata: "testing data",
			DatabaseTrack: &radio.DatabaseTrack{
				TrackID: 1500,
			},
		},
		SongInfo: radio.SongInfo{
			Start: time.Date(2000, time.April, 1, 5, 6, 7, 8, time.UTC),
			End:   time.Date(2010, time.February, 10, 15, 16, 17, 18, time.UTC),
		},
		Listeners:    900,
		StreamerName: "test",
		Thread:       "a cool thread",
	}

	err := ss.Store(in)
	require.NoError(t, err)

	out, err := ss.Load()
	require.NoError(t, err)
	assert.Condition(t, func() (success bool) {
		return assert.Equal(t, in.User, out.User) &&
			assert.Equal(t, in.Song, out.Song) &&
			assert.WithinDuration(t, in.SongInfo.Start, out.SongInfo.Start, time.Second) &&
			assert.WithinDuration(t, in.SongInfo.End, out.SongInfo.End, time.Second) &&
			assert.Equal(t, in.Listeners, out.Listeners) &&
			assert.Equal(t, in.StreamerName, out.StreamerName) &&
			assert.Equal(t, in.Thread, out.Thread)
	})
}

func (suite *Suite) TestStatusStoreEmpty(t *testing.T) {
	ss := suite.Storage(t).Status(suite.ctx)

	// just an empty status
	err := ss.Store(radio.Status{})
	Require(t, errors.InvalidArgument, err)
}
