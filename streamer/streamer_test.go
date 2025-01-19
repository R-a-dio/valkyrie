package streamer

import (
	"context"
	"testing"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/mocks"
	"github.com/R-a-dio/valkyrie/streamer/audio"
	"github.com/stretchr/testify/require"
)

func TestTracksType(t *testing.T) {
	t.Run("uses init value", func(t *testing.T) {
		in := make([]StreamTrack, 10)
		ts := newTracks(nil, in)
		require.Equal(t, in, ts.tracks)
	})

	t.Run("short", func(t *testing.T) {
		ts := newTracks(nil, nil)

		waiter := ts.add(StreamTrack{
			Audio: newTestAudio(preloadLengthTarget - time.Second),
		})

		select {
		case <-waiter:
		default:
			t.Error("expected waiter to be instantly ready")
		}
	})

	t.Run("empty pop", func(t *testing.T) {
		ts := newTracks(nil, nil)
		require.Nil(t, ts.pop())
	})

	t.Run("add and pop", func(t *testing.T) {
		ts := newTracks(context.Background(), nil)

		waiter := ts.add(StreamTrack{
			Audio: newTestAudio(preloadLengthTarget * 2),
		})

		select {
		case <-waiter:
			t.Error("expected waiter to not be ready yet")
		default:
		}

		select {
		case <-ts.PopCh:
		case <-time.After(time.Second * 2):
			t.Error("expected track to be in the PopCh")
		}

		select {
		case <-waiter:
		default:
			t.Error("expected waiter to be ready after we popped")
		}
	})

	t.Run("pop", func(t *testing.T) {
		var values []StreamTrack
		for i := range 10 {
			values = append(values, StreamTrack{QueueEntry: radio.QueueEntry{Song: radio.Song{ID: radio.SongID(i)}}})
		}

		ts := newTracks(nil, values)

		require.Len(t, ts.tracks, 10)

		for i := range 10 {
			require.EqualValues(t, i, ts.pop().ID)
		}

		require.Len(t, ts.tracks, 0)
	})
}

func newTestAudio(dur time.Duration) audio.Reader {
	return &mocks.ReaderMock{
		TotalLengthFunc: func() time.Duration {
			return dur
		},
	}
}
