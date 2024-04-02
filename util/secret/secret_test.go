package secret_test

import (
	"crypto/sha256"
	"strconv"
	"testing"

	"github.com/R-a-dio/valkyrie/util/secret"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSecretKeyGeneration(t *testing.T) {
	s1, err := secret.NewSecret(16)
	require.NoError(t, err)
	require.True(t, s1.Equal(s1.Get(nil), nil), "s1 should equal itself")

	s2, err := secret.NewSecret(16)
	require.NoError(t, err)
	require.True(t, s2.Equal(s2.Get(nil), nil), "s2 should equal itself")

	// compare to each other. should never be true
	assert.False(t, s1.Equal(s2.Get(nil), nil), "s2 should not equal s1")
	assert.False(t, s2.Equal(s1.Get(nil), nil), "s1 should not equal s2")
}

func TestSecretSaltComparison(t *testing.T) {
	for i := 1; i < sha256.Size*2; i++ {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			s, err := secret.NewSecret(i)
			require.NoError(t, err)

			salt := []byte("testing world")
			differentSalt := []byte("hello world")

			assert.True(t, s.Equal(s.Get(salt), salt), "same salt should equal")
			assert.False(t, s.Equal(s.Get(salt), nil), "salt and no salt should not equal")
			assert.False(t, s.Equal(s.Get(nil), salt), "no salt and salt should not equal")
			assert.False(t, s.Equal(s.Get(salt), differentSalt), "salt and differentSalt should not equal")
			assert.False(t, s.Equal(s.Get(differentSalt), salt), "differentSalt and salt should not equal")
		})
	}
}
