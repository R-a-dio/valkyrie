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
	checkLength := func(t *testing.T, ts *tracks, n int) {
		ts.mu.Lock()
		defer ts.mu.Unlock()
		require.Len(t, ts.tracks, n)
	}

	t.Run("uses init value", func(t *testing.T) {
		var values []StreamTrack
		for i := range 10 {
			values = append(values, StreamTrack{
				QueueEntry: radio.QueueEntry{Song: radio.Song{ID: radio.SongID(i)}},
				Audio:      newTestAudio(time.Minute * 2),
			})
		}
		ts := newTracks(nil, values)
		require.Equal(t, values, ts.tracks)
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
		case <-ts.PopCh():
		case <-time.After(time.Second * 2):
			t.Error("expected track to be in the PopCh")
		}

		select {
		case <-waiter:
		case <-time.After(time.Microsecond * 40):
			t.Error("expected waiter to be ready after we popped")
		}
	})

	t.Run("pop", func(t *testing.T) {
		var values []StreamTrack
		for i := range 10 {
			values = append(values, StreamTrack{
				QueueEntry: radio.QueueEntry{Song: radio.Song{ID: radio.SongID(i)}},
				Audio:      newTestAudio(time.Minute * 2),
			})
		}

		ts := newTracks(nil, values)

		checkLength(t, ts, 10)

		for i := range 10 {
			require.EqualValues(t, i, ts.pop().ID)
		}

		checkLength(t, ts, 0)
	})

	t.Run("pop through channel", func(t *testing.T) {
		var values []StreamTrack
		for i := range 10 {
			values = append(values, StreamTrack{
				QueueEntry: radio.QueueEntry{Song: radio.Song{ID: radio.SongID(i)}},
				Audio:      newTestAudio(time.Minute * 2),
			})
		}

		ts := newTracks(context.Background(), values)

		checkLength(t, ts, 10)

		for i := range 10 {
			require.EqualValues(t, i, (<-ts.PopCh()).ID)
		}

		checkLength(t, ts, 0)
	})

	t.Run("cancel pop through cycle", func(t *testing.T) {
		var values []StreamTrack
		for i := range 10 {
			values = append(values, StreamTrack{
				QueueEntry: radio.QueueEntry{Song: radio.Song{ID: radio.SongID(i)}},
				Audio:      newTestAudio(time.Minute * 2),
			})
		}

		ts := newTracks(context.Background(), values)

		go func() {
			<-ts.NotifyCh()
			ts.CyclePopCh()
		}()

		checkLength(t, ts, 10)
		for i := range 20 {
			track, ok := <-ts.PopCh()
			if !ok {
				break
			}

			require.EqualValues(t, i, track.ID)
		}
		checkLength(t, ts, 0)
	})
}

func newTestAudio(dur time.Duration) audio.Reader {
	return &mocks.ReaderMock{
		TotalLengthFunc: func() time.Duration {
			return dur
		},
	}
}
