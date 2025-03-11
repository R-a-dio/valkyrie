package functions

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHumanDuration(t *testing.T) {
	tests := []struct {
		name     string
		in       string // in time.ParseDuration format
		expected string
	}{
		{
			name:     "small duration",
			in:       "5h6m",
			expected: "5h6m0s",
		},
		{
			name:     "longer than a day",
			in:       "50h33m22s",
			expected: "2d2h33m22s",
		},
		{
			name:     "exactly a day",
			in:       "24h",
			expected: "1d0s",
		},
		{
			name:     "small",
			in:       "5ns",
			expected: "0s",
		},
		{
			name:     "small",
			in:       "5us",
			expected: "0s",
		},
		{
			name:     "small",
			in:       "5ms",
			expected: "0s",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			in, err := time.ParseDuration(test.in)
			require.NoError(t, err)
			out := HumanDuration(in)
			assert.Equal(t, test.expected, out)
		})
	}

	// special case check if it handles zero times

	t.Run("zero-time", func(t *testing.T) {
		out := HumanDuration(time.Since(time.Time{}))
		assert.Equal(t, "never", out)
	})

}

func TestTimeAgo(t *testing.T) {
	tests := []struct {
		name     string
		expected string
		in       time.Time
		in2      string
	}{
		{
			name:     "long",
			in:       time.Date(2000, 00, 00, 00, 00, 00, 00, time.UTC),
			in2:      "%y years, %d days, %h hours",
			expected: "25 years, 75 days, 9 hours",
		},
	}

	fn := TimeAgo(func() time.Time {
		return time.Date(2025, 2, 6, 9, 48, 15, 0, time.UTC)
	})
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			out := fn(test.in, test.in2)
			assert.Equal(t, test.expected, out)
		})
	}
}

func BenchmarkTimeAgo(b *testing.B) {
	in := time.Now().Add(-time.Hour * 24 * 365 * 2)
	fn := TimeAgo(time.Now)
	for range b.N {
		_ = fn(in, "%y years, %d days, %h hours, %m minutes")
	}
}

func BenchmarkHumanDuration(b *testing.B) {
	b.Run("long-form", func(b *testing.B) {
		for range b.N {
			_ = HumanDuration(time.Hour * 30)
		}
	})
	b.Run("short-form", func(b *testing.B) {
		for range b.N {
			_ = HumanDuration(time.Hour + time.Minute*14 + time.Second*10)
		}
	})
}

func BenchmarkAbsoluteDate(b *testing.B) {
	b.Run("long-form", func(b *testing.B) {
		in := time.Now().Add(-time.Hour * 24 * 50)
		for range b.N {
			_ = AbsoluteDate(in)
		}
	})
	b.Run("short-form", func(b *testing.B) {
		in := time.Now()
		for range b.N {
			_ = AbsoluteDate(in)
		}
	})
}

func BenchmarkMediaDuration(b *testing.B) {
	for range b.N {
		_ = MediaDuration(time.Minute*5 + time.Second*15)
	}
}

func BenchmarkPrettyDuration(b *testing.B) {
	b.Run("future-<minute", func(b *testing.B) {
		for range b.N {
			_ = PrettyDuration(time.Second * 50)
		}
	})
	b.Run("future-minute", func(b *testing.B) {
		for range b.N {
			_ = PrettyDuration(time.Minute + time.Second*15)
		}
	})
	b.Run("future-minutes", func(b *testing.B) {
		for range b.N {
			_ = PrettyDuration(time.Minute * 6)
		}
	})
	b.Run("past-<minute", func(b *testing.B) {
		for range b.N {
			_ = PrettyDuration(-time.Second * 50)
		}
	})
	b.Run("past-minute", func(b *testing.B) {
		for range b.N {
			_ = PrettyDuration(-(time.Minute + time.Second*15))
		}
	})
	b.Run("past-minutes", func(b *testing.B) {
		for range b.N {
			_ = PrettyDuration(-time.Minute * 6)
		}
	})
}

func BenchmarkTimeagoDuration(b *testing.B) {
	b.Run("future-<minute", func(b *testing.B) {
		for range b.N {
			_ = TimeagoDuration(time.Second * 50)
		}
	})
	b.Run("future-minute", func(b *testing.B) {
		for range b.N {
			_ = TimeagoDuration(time.Minute + time.Second*15)
		}
	})
	b.Run("future-minutes", func(b *testing.B) {
		for range b.N {
			_ = TimeagoDuration(time.Minute * 6)
		}
	})
	b.Run("past-<minute", func(b *testing.B) {
		for range b.N {
			_ = TimeagoDuration(-time.Second * 50)
		}
	})
	b.Run("past-minute", func(b *testing.B) {
		for range b.N {
			_ = TimeagoDuration(-(time.Minute + time.Second*15))
		}
	})
	b.Run("past-minutes", func(b *testing.B) {
		for range b.N {
			_ = TimeagoDuration(-time.Minute * 6)
		}
	})
}

func BenchmarkIsImageThread(b *testing.B) {
	b.Run("yes", func(b *testing.B) {
		for range b.N {
			_ = IsImageThread("image:something")
		}
	})
	b.Run("no", func(b *testing.B) {
		for range b.N {
			_ = IsImageThread("something")
		}
	})
}

func BenchmarkIsValidThread(b *testing.B) {
	b.Run("true", func(b *testing.B) {
		for range b.N {
			_ = IsValidThread("THIS IS A THREAD")
		}
	})
	b.Run("false", func(b *testing.B) {
		for range b.N {
			_ = IsValidThread("NONE")
		}
	})
}

func TestIsValidThread(t *testing.T) {
	assert.False(t, IsValidThread(""))
	assert.False(t, IsValidThread("NONE"))
	assert.True(t, IsValidThread("yep that's a thread"))
}
