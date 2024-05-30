package audio

import (
	"errors"
	"io"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMP3Buffer(t *testing.T) {
	tests := []struct {
		filename         string
		expectedDuration time.Duration
	}{
		{
			filename:         "testdata/MP3_700KB.mp3",
			expectedDuration: time.Second * 42,
		},
		{
			filename:         "testdata/MP3_1MG.mp3",
			expectedDuration: time.Second * 58,
		},
		{
			filename:         "testdata/MP3_2MG.mp3",
			expectedDuration: time.Second * 54,
		},
		{
			filename:         "testdata/MP3_5MG.mp3",
			expectedDuration: time.Minute*2 + time.Second*13,
		},
	}

	for _, test := range tests {
		f, err := os.Open(test.filename)
		require.NoError(t, err)

		mpb, err := NewMP3Buffer("test", nil)
		require.NoError(t, err)
		defer mpb.Close()

		var buf = make([]byte, 256) // small buffer to see if it handles partial frames
		for {
			n, err := f.Read(buf)
			if err != nil {
				break
			}

			n, err = mpb.Write(buf[:n])
			if err != nil && n == 0 {
				break
			}
		}

		assert.InDelta(t, test.expectedDuration, mpb.TotalLength(), float64(time.Second/2))
		fi, _ := f.Stat()
		assert.Equal(t, mpb.Memfd.Size(), fi.Size(), "buffer should contain all of input")

		// a reader we make should also have the same size
		mpr, err := mpb.Reader()
		if !assert.NoError(t, err) {
			continue
		}
		fi, _ = mpr.Stat()
		assert.Equal(t, mpb.Size(), fi.Size(), "reader should match its parent")

		// close the parent so this won't block forever
		mpb.CloseWrite()
		// try to read with a tiny buffer, this should error with the correct io error
		_, err = mpr.Read(buf[:1])
		assert.ErrorIs(t, err, io.ErrShortBuffer)

		// try and read all the data in the reader
		all := make([]byte, 0, mpb.Size())
		buf = make([]byte, 4096) // slightly larger buffer so it can fit an mp3 frame
		var n int
		for {
			n, err = mpr.Read(buf)
			if err != nil {
				if errors.Is(err, io.EOF) {
					err = nil
				}
				break
			}

			all = append(all, buf[:n]...)
		}

		if !assert.NoError(t, err, "read all should succeed") {
			continue
		}

		assert.Equal(t, mpb.Size(), int64(len(all)))
		assert.InDelta(t, test.expectedDuration, mpr.Progress(), float64(time.Second/2))
		assert.Equal(t, mpb.TotalLength(), mpr.Progress())
	}
}
