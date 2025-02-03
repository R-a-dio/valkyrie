package config

import (
	"bytes"
	"net/netip"
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

	value := Value(cfg, func(cfg Config) URL {
		return cfg.Conf().Tracker.MasterServer
	})

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		_ = value()
	}
}

func TestAddrPort(t *testing.T) {
	t.Run("empty addr", func(t *testing.T) {
		ap, err := ParseAddrPort(":5000")
		if assert.NoError(t, err) {
			assert.Equal(t, uint16(5000), ap.Port())
			assert.Equal(t, localAddr, ap.Addr())
		}
	})
	t.Run("localhost addr", func(t *testing.T) {
		ap, err := ParseAddrPort("localhost:5000")
		if assert.NoError(t, err) {
			assert.Equal(t, uint16(5000), ap.Port())
			assert.Equal(t, localAddr, ap.Addr())
		}
	})
	t.Run("large port number", func(t *testing.T) {
		// big port number should fail
		_, err := ParseAddrPort(":80000")
		assert.Error(t, err)
	})
	t.Run("full addrport", func(t *testing.T) {
		ap, err := ParseAddrPort("0.0.0.0:6000")
		if assert.NoError(t, err) {
			assert.Equal(t, uint16(6000), ap.Port())
			assert.Equal(t, netip.MustParseAddr("0.0.0.0"), ap.Addr())
		}
	})
}
