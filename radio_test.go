package radio

import (
	"reflect"
	"testing"
	"time"
	"unicode"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/arbitrary"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

func TestSongEqualTo(t *testing.T) {
	tp := gopter.DefaultTestParameters()
	tp.MinSuccessfulTests = 500
	a := arbitrary.DefaultArbitraries()
	p := gopter.NewProperties(tp)

	// we only want songs with actual string data to compare against
	a.RegisterGen(gen.UnicodeString(unicode.Katakana).SuchThat(func(s string) bool { return len(s) > 5 }))

	p.Property("song a != b",
		a.ForAll(func(a, b Song) bool {
			return !a.EqualTo(b)
		}),
	)
	p.Property("song b != a",
		a.ForAll(func(a, b Song) bool {
			return !b.EqualTo(a)
		}),
	)
	p.Property("song a == a",
		a.ForAll(func(a, b Song) bool {
			return a.EqualTo(a)
		}),
	)
	p.Property("song b == b",
		a.ForAll(func(a, b Song) bool {
			return b.EqualTo(b)
		}),
	)

	p.TestingRun(t)
}

func TestSongRequestable(t *testing.T) {
	tp := gopter.DefaultTestParameters()
	tp.MinSuccessfulTests = 500
	a := arbitrary.DefaultArbitraries()
	p := gopter.NewProperties(tp)
	aNoTimes := arbitrary.DefaultArbitraries()
	aNoTimes.RegisterGen(gen.TimeRange(time.Time{}, time.Hour))

	songType := reflect.TypeOf(Song{})

	p.Property("if Requestable then UntilRequestable == 0", prop.ForAll(
		func(s Song) bool {
			return s.UntilRequestable() == 0
		},
		aNoTimes.GenForType(songType).SuchThat(func(s Song) bool { return s.Requestable() }),
	))

	p.Property("if not Requestable then UntilRequestable > 0", prop.ForAll(
		func(s Song) bool {
			return s.UntilRequestable() > 0
		}, a.GenForType(songType).SuchThat(func(s Song) bool { return !s.Requestable() }),
	))

	p.TestingRun(t)
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

func TestCalculateRequestDelay(t *testing.T) {
	tp := gopter.DefaultTestParameters()
	tp.MinSuccessfulTests = 10000
	a := arbitrary.DefaultArbitraries()
	p := gopter.NewProperties(tp)

	p.Property("+1 should be higher or equal", a.ForAll(func(i int) bool {
		return CalculateRequestDelay(i) <= CalculateRequestDelay(i+1)
	}))
	p.TestingRun(t)
}
