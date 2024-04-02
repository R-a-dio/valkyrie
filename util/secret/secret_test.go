package secret

import (
	"crypto/sha256"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSecretKeyGeneration(t *testing.T) {
	t.Parallel()

	s1, err := NewSecret(16)
	require.NoError(t, err)
	defer setupNowMock(s1, nil)()
	require.True(t, s1.Equal(s1.Get(nil), nil), "s1 should equal itself")

	s2, err := NewSecret(16)
	require.NoError(t, err)
	defer setupNowMock(s2, nil)()
	require.True(t, s2.Equal(s2.Get(nil), nil), "s2 should equal itself")

	// compare to each other. should never be true
	assert.False(t, s1.Equal(s2.Get(nil), nil), "s2 should not equal s1")
	assert.False(t, s2.Equal(s1.Get(nil), nil), "s1 should not equal s2")
}

func TestSecretSaltComparison(t *testing.T) {
	for i := 1; i < sha256.Size*2; i++ {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()

			s, err := NewSecret(i)
			require.NoError(t, err)
			defer setupNowMock(s, nil)()

			salt := []byte("testing world")
			differentSalt := []byte("hello world")

			assert.True(t, s.Equal(s.Get(salt), salt), "same salt should equal")
			assert.True(t, s.Equal(s.Get(differentSalt), differentSalt), "same salt should equal")
			assert.False(t, s.Equal(s.Get(salt), nil), "salt and no salt should not equal")
			assert.False(t, s.Equal(s.Get(nil), salt), "no salt and salt should not equal")
			assert.False(t, s.Equal(s.Get(salt), differentSalt), "salt and differentSalt should not equal")
			assert.False(t, s.Equal(s.Get(differentSalt), salt), "differentSalt and salt should not equal")
		})
	}
}

func TestSecretDateChange(t *testing.T) {
	t.Parallel()

	si, err := NewSecret(DaypassLength)
	require.NoError(t, err)
	s := si.(*secret)

	rd := time.Date(2003, time.April, 24, 5, 5, 5, 0, time.UTC)
	defer setupNowMock(si, func() time.Time {
		return rd
	})()

	salt := []byte("testing salt")
	last := s.Get(nil)
	lastSalted := s.Get(salt)

	for i := 0; i < 365*2; i++ {
		require.True(t, s.Equal(last, nil), "should equal before day change")
		require.True(t, s.Equal(lastSalted, salt), "should equal before day change")
		t.Log(rd)
		rd = rd.AddDate(0, 0, 1)
		require.False(t, s.Equal(last, nil), "should not equal after day change")
		require.False(t, s.Equal(lastSalted, salt), "should not equal after day change")
		last = s.Get(nil)
		lastSalted = s.Get(salt)
	}
}

func setupNowMock(si Secret, fn func() time.Time) func() {
	s := si.(*secret)
	// store the previous function
	prev := s.now

	// if passed nil we mock it with the current time, but a constant
	// one so that tests don't break if they happen to run during midnight
	if fn == nil {
		constant := time.Now()
		fn = func() time.Time {
			return constant
		}
	}
	s.now = fn

	// return function that restores the previous function so the caller can
	// defer it
	return func() {
		s.now = prev
	}
}
