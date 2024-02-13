package radio

import (
	"testing"
	"time"
)

func TestSongEqualTo(t *testing.T) {
	// helper functions
	set := func(id SongID, tid TrackID) Song {
		return Song{
			ID: id,
			DatabaseTrack: &DatabaseTrack{
				TrackID: tid,
			},
		}
	}
	// tests
	var tests = []struct {
		a, b     Song
		expected bool
	}{
		{set(0, 0), set(0, 0), false},
		{set(100, 0), set(100, 600), true},
		{set(0, 200), set(200, 0), false},
		{set(0, 300), set(0, 300), true},
		{set(300, 200), set(200, 300), false},
		{set(400, 0), set(400, 0), true},
		{set(400, 0), set(400, 500), true},
		{set(0, 0), set(400, 500), false},
		{Song{ID: 200}, Song{ID: 200}, true},
		{Song{ID: 0}, Song{ID: 200}, false},
		{Song{ID: 200}, Song{ID: 0}, false},
		{Song{ID: 0}, Song{ID: 0}, false},
	}

	// run
	for _, u := range tests {
		actual := u.a.EqualTo(u.b)
		if actual != u.expected {
			t.Errorf("expected %t got %t instead: %v %v", u.expected, actual, u.a, u.b)
		}
		if actual != u.b.EqualTo(u.a) {
			t.Errorf("%v != %v", u.a, u.b)
		}
	}
}

func TestSongRequestable(t *testing.T) {
	// helper functions
	set := func(delay time.Duration, lp time.Time, lr time.Time) Song {
		return Song{
			LastPlayed: lp,
			DatabaseTrack: &DatabaseTrack{
				LastRequested: lr,
				RequestCount:  0,
			},
		}
	}
	add := func(d time.Duration) time.Time {
		return time.Now().Add(d)
	}
	sub := func(d time.Duration) time.Time {
		return time.Now().Add(-d)
	}

	// tests
	var tests = []struct {
		a        Song
		expected bool
	}{
		{set(0, time.Now(), time.Now()), false},
		{set(time.Hour, time.Now(), time.Now()), false},
		{set(time.Hour, add(time.Hour*2), time.Now()), false},
		{set(time.Hour, add(0), add(time.Hour*2)), false},
		{set(time.Hour, sub(time.Hour*2), sub(time.Hour*2)), true},
		{set(time.Hour, sub(time.Hour), sub(time.Hour)), true},
		{set(time.Hour, sub(time.Minute*30), sub(time.Hour)), false},
	}

	// run
	for i, u := range tests {
		actual := u.a.Requestable()
		if actual != u.expected {
			t.Errorf("%d: expected %t got %t instead: %v", i, u.expected, actual, u.a)
		}
		if actual && u.a.UntilRequestable() != 0 {
			t.Errorf("%d: expected 0 from UntilRequestable if requestable song: got %s",
				i, u.a.UntilRequestable())
		}
	}
}

func TestCalculateCooldown(t *testing.T) {
	tests := []struct {
		delay time.Duration
		last  time.Time
		ok    bool
	}{
		{time.Hour, time.Now(), false},
		{time.Hour, time.Now().Add(-time.Hour * 2), true},
		{time.Hour, time.Now().Add(time.Hour * 2), false},
	}

	for _, test := range tests {
		d, ok := CalculateCooldown(test.delay, test.last)
		if ok != test.ok {
			t.Errorf("failed %s on %s, returned: %s", test.last, test.delay, d)
		}
	}
}
