package templates

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
}
