package config

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	cfg, err := LoadFile()
	require.NoError(t, err)

	assert.Equal(t, defaultConfig, cfg.Conf())

	t.Run("Roundtrip", func(t *testing.T) {
		var buf bytes.Buffer

		err := cfg.Save(&buf)
		require.NoError(t, err)

		other, err := Load(&buf)
		require.NoError(t, err)

		assert.Equal(t, defaultConfig, other.Conf())
	})
}

func BenchmarkConfigAccess(b *testing.B) {
	cfg, err := LoadFile()
	require.NoError(b, err)

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		_ = cfg.Conf().Tracker.MasterServer
	}
}

func BenchmarkConfigAccessMultiple(b *testing.B) {
	cfg, err := LoadFile()
	require.NoError(b, err)

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		conf := cfg.Conf()
		_ = conf.Tracker.MasterServer
		_ = conf.Tracker.MasterUsername
		_ = conf.Tracker.MasterPassword
	}
}

func BenchmarkConfigValueAccess(b *testing.B) {
	cfg, err := LoadFile()
	require.NoError(b, err)

	value := Value(cfg, func(c Config) URL {
		return cfg.Conf().Tracker.MasterServer
	})

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		_ = value()
	}
}
