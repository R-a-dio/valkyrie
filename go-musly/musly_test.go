// +build musly

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
