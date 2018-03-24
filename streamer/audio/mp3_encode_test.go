package audio

import (
	"fmt"
	"io"
	"os"
	"testing"
	"time"
)

func TestNewLame(t *testing.T) {
	l, err := NewLAME(AudioFormat{2, 2, 44100})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	var mp3 = NewMP3Buffer()
	pcm, err := DecodeFile("F:/cloud9.mp3")
	if err != nil {
		t.Fatal(err)
	}
	if err = pcm.Wait(); err != nil {
		t.Fatal(err)
	}

	var r = pcm.Reader()
	var b = make([]byte, 1024*16)
	for {
		n, err := r.Read(b)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}

		out, err := l.Encode(b[:n])
		if err != nil {
			t.Fatal(err)
		}

		_, err = mp3.Write(out)
		if err != nil {
			t.Fatal(err)
		}
	}

	mp3.Write(l.Flush())

	mp3.Close()

	fmt.Println(pcm.Length(), mp3.Length())
	t.Log(pcm.Length(), mp3.Length())

	f, err := os.Create("F:/test.mp3")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	_, err = io.Copy(f, mp3.Reader())
	if err != nil {
		t.Fatal(err)
	}
}

func TestNewLameLimit(t *testing.T) {
	l, err := NewLAME(AudioFormat{2, 2, 44100})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	var mp3 = NewMP3Buffer()
	pcm, err := DecodeFile("F:/cloud9.mp3")
	if err != nil {
		t.Fatal(err)
	}
	if err = pcm.Wait(); err != nil {
		t.Fatal(err)
	}

	var b = make([]byte, 1024*16)
	for mp3.Length() < time.Hour*24 {
		fmt.Printf("%s\r", mp3.Length())
		var r = pcm.Reader()
		for {
			n, err := r.Read(b)
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatal(err)
			}

			out, err := l.Encode(b[:n])
			if err != nil {
				t.Fatal(err)
			}

			_, err = mp3.Write(out)
			if err != nil {
				t.Fatal(err)
			}
		}
	}

	mp3.Write(l.Flush())
	mp3.Close()

	f, err := os.Create("F:/testlong.mp3")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	_, err = io.Copy(f, mp3.Reader())
	if err != nil {
		t.Fatal(err)
	}
}
