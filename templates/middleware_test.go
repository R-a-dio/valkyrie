package templates

import (
	"testing"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/util"
	"github.com/stretchr/testify/assert"
)

type cookieTest struct {
	theme        string
	dj           bool
	holiday      bool
	expected     string
	fromExpected bool
}

func TestCookieDecode(t *testing.T) {
	tests := []cookieTest{
		{
			theme:    "default",
			dj:       true,
			holiday:  false,
			expected: "default:10",
		},
		{
			theme:    "default",
			dj:       false,
			holiday:  false,
			expected: "default:00",
		},
		{
			theme:    "default",
			dj:       true,
			holiday:  true,
			expected: "default:11",
		},
		{
			theme:    "default",
			dj:       false,
			holiday:  true,
			expected: "default:01",
		},
		{
			theme:        "default",
			dj:           false,
			holiday:      false,
			expected:     "default",
			fromExpected: true,
		},
		{
			theme:        "default:1",
			dj:           false,
			holiday:      false,
			expected:     "default:1",
			fromExpected: true,
		},
	}

	for _, test := range tests {
		value := test.expected
		if !test.fromExpected {
			value = cookieEncode(test.theme, test.dj, test.holiday)
			assert.Equal(t, test.expected, value)
		}

		theme, dj, holiday := cookieDecode(value)
		assert.Equal(t, test.theme, theme)
		assert.Equal(t, test.dj, dj)
		assert.Equal(t, test.holiday, holiday)
	}
}

func BenchmarkCookieDecode(b *testing.B) {
	value := "default:11"
	for range b.N {
		_, _, _ = cookieDecode(value)
	}
}

func BenchmarkCookieEncode(b *testing.B) {
	for range b.N {
		_ = cookieEncode("default", true, false)
	}
}

func BenchmarkDecideTheme(b *testing.B) {
	fn := func(holiday, user radio.ThemeName) func(radio.ThemeName) radio.ThemeName {
		tv := NewThemeValues()
		tv.StoreHoliday(holiday)
		tv.StoreDJ(user)
		return tv.decide
	}

	b.ResetTimer()

	b.Run("default-with-holiday", func(b *testing.B) {
		decider := fn("holiday", "")
		for range b.N {
			_ = decider(ThemeDefault)
		}
	})
	b.Run("default-with-user", func(b *testing.B) {
		decider := fn("", "user")
		for range b.N {
			_ = decider(ThemeDefault)
		}
	})
	b.Run("default", func(b *testing.B) {
		decider := fn("", "")
		for range b.N {
			_ = decider(ThemeDefault)
		}
	})
}
