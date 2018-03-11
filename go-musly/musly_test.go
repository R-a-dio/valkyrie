package musly

import (
	"os"
	"path/filepath"
	"testing"
)

func tempfile(name string) string {
	return filepath.Join(os.TempDir(), name)
}

func TestInfo(t *testing.T) {
	t.Log("version:", Version())
	d := ListDecoders()
	t.Log("decoders:")
	for i := range d {
		t.Log("-", d[i])
	}

	m := ListMethods()
	t.Log("methods:")
	for i := range m {
		t.Log("-", m[i])
	}
}

func TestTrackConversion(t *testing.T) {
	path := tempfile(t.Name())
	j, err := OpenBox(path)
	if err != nil {
		os.Remove(path)
		t.Fatal(err)
	}
	defer j.Close()
	defer os.Remove(path)

	track := j.NewTrack()

	if track != track {
		t.Fatal("magic", track)
	}

	b := j.trackToBytes(track)
	track2 := j.bytesToTrack(b)
	if track != track {
		t.Fatal("invalid track to bytes conversion", track, track2)
	}
}
