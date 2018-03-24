package audio

import (
	"bytes"
	"crypto/rand"
	"testing"
)

func TestPCMBuffer(t *testing.T) {
	var data = make([]byte, pcmBootstrapSize*2)
	n, err := rand.Read(data)
	if err != nil {
		t.Fatal("failed to get random data", n, err)
	}
	data = data[:n]

	p := NewPCMBuffer(AudioFormat{2, 2, 44100})
	n, _ = p.Write(data)
	if n != len(data) {
		t.Fatalf("failed to write full data: %d != %d", n, len(data))
	}

	if p.length != uint64(len(p.buf)) {
		t.Fatalf("internal length and actual length: %d != %d",
			p.length, len(p.buf))
	}

	t.Logf("playback length parent: %s", p.Length())

	var data2 = make([]byte, pcmBootstrapSize*2)
	pr := p.Reader()
	t.Logf("playback length reader: %s", pr.Length())

	n, _ = pr.Read(data2)
	if n != len(data2) {
		t.Fatalf("failed to read full data: %d != %d", n, len(data2))
	}

	t.Logf("playback length reader: %s", pr.Length())

	if !bytes.Equal(data, data2) {
		t.Fatal("data not equal to what was written")
	}

	go func() {
		p.Write(data)
	}()

	n, _ = pr.Read(data2)
	if n != len(data2) {
		t.Fatalf("failed to read full data (2): %d != %d", n, len(data2))
	}

	if !bytes.Equal(data, data2) {
		t.Fatal("data not equal to what was written")
	}
}
