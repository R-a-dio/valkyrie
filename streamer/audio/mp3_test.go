package audio

import (
	"fmt"
	"io"
	"os"
	"testing"
)

func TestMP3Reader(t *testing.T) {
	fnc := func(path string) {
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()

		m := NewMP3Buffer()

		_, err = io.Copy(m, f)
		if err != nil {
			t.Fatal("copying:", err)
		}
		fmt.Println(cap(m.decBuf.buf))

		buf, err := DecodeFile(path)
		if err != nil {
			t.Fatal("decoding:", err)
		}

		buf.Wait()

		t.Logf("MP3: %s PCM: %s", m.Length(), buf.Length())
	}

	fnc(`F:\cloud9.mp3`)

	fnc(`F:\MoP\music_for_programming_1-datassette.mp3`)

	fnc(`F:\MoP\music_for_programming_2-sunjammer.mp3`)

	fnc(`F:\MoP\music_for_programming_3-datassette.mp3`)

	fnc(`F:\MoP\music_for_programming_4-com_truise.mp3`)

	fnc(`F:\MoP\music_for_programming_5-abe_mangger.mp3`)
}
