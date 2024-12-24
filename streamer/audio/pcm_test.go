package audio

import (
	"bytes"
	"crypto/rand"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

const pcmBootstrapSize = 1024 * 1024

func TestPCMBuffer(t *testing.T) {
	var data = make([]byte, pcmBootstrapSize*2)
	n, err := rand.Read(data)
	if err != nil {
		t.Fatal("failed to get random data", n, err)
	}
	data = data[:n]

	af := AudioFormat{2, 2, 44100}
	p, err := NewMemoryBuffer("testing", nil)
	require.NoError(t, err)
	defer p.Close()

	n, err = p.Write(data)
	require.NoError(t, err)
	require.Equal(t, len(data), n, "failed to write full data")

	var data2 = make([]byte, pcmBootstrapSize*2)
	mr, err := p.Reader()
	require.NoError(t, err)
	defer mr.Close()
	pr := NewPCMReader(af, mr)

	require.NotZero(t, pr.TotalLength(), "should contain data")
	require.Equal(t, pr.TotalLength(), pr.RemainingLength(),
		"haven't read yet so should be equal")
	require.Zero(t, pr.Progress(), "haven't read yet so should be zero")

	t.Logf("playback length reader: %s", pr.TotalLength())
	t.Logf("playback progress reader: %s", pr.Progress())

	n, err = io.ReadAtLeast(pr, data2, len(data2))
	require.NoError(t, err)
	require.Equal(t, len(data2), n, "failed to read full data")

	t.Logf("playback length reader: %s", pr.TotalLength())
	t.Logf("playback progress reader: %s", pr.Progress())

	if !bytes.Equal(data, data2) {
		t.Fatal("data not equal to what was written")
	}

	go func() {
		_, _ = p.Write(data)
	}()

	n, err = io.ReadAtLeast(pr, data2, len(data2))
	require.NoError(t, err)
	require.Equal(t, len(data2), n, "failed to read full data")

	if !bytes.Equal(data, data2) {
		t.Fatal("data not equal to what was written")
	}
}
