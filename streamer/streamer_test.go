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
		ts := newTracks(context.Background(), nil, values)

		require.Equal(t, values, ts.tracks)
	})

	t.Run("short", func(t *testing.T) {
		ts := newTracks(context.Background(), nil, nil)

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
		ts := newTracks(context.Background(), nil, nil)

		require.Nil(t, ts.pop())
	})

	t.Run("add and pop", func(t *testing.T) {
		ts := NewTracks(context.Background(), nil, nil)
		defer ts.Stop()

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

		ts := newTracks(context.Background(), nil, values)

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

		ts := NewTracks(context.Background(), nil, values)
		defer ts.Stop()

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

		ts := NewTracks(context.Background(), nil, values)
		defer ts.Stop()

		ch := ts.NotifyCh()
		go func() {
			<-ch
			t.Log("notified")
			ts.CyclePopCh()
		}()

		popper := ts.PopCh()
		for i := range 20 {
			track, ok := <-popper
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
