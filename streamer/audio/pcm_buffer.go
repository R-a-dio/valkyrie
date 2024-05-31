package audio

import (
	"io"
	"time"
)

//const pcmReadFromSize = (44100 * 2 * 2) * 15 // about 15-seconds of data

// PCMLength calculates the expected duration of a file
// that contains PCM audio data in the AudioFormat given
func PCMLength(af AudioFormat, mb *MemoryReader) time.Duration {
	fi, err := mb.Stat()
	if err != nil {
		return 0
	}

	size := fi.Size()
	return time.Duration(size) * time.Second /
		time.Duration(af.BytesPerSample*af.ChannelCount*af.SampleRate)
}

// PCMProgress calculates the duration of the bytes read so far
func PCMProgress(af AudioFormat, mb *MemoryReader) time.Duration {
	pos, err := mb.Seek(0, io.SeekCurrent)
	if err != nil {
		return 0
	}

	return time.Duration(pos) * time.Second /
		time.Duration(af.BytesPerSample*af.ChannelCount*af.SampleRate)
}

// NewPCMReader returns a new PCMReader with the AudioFormat given.
func NewPCMReader(af AudioFormat, mr *MemoryReader) *PCMReader {
	return &PCMReader{
		AudioFormat:  af,
		MemoryReader: mr,
	}
}

// PCMBuffer is a pcm-aware Buffer
type PCMReader struct {
	AudioFormat
	*MemoryReader
}

// TotalLength returns the total length of the reader
func (pr *PCMReader) TotalLength() time.Duration {
	return PCMLength(pr.AudioFormat, pr.MemoryReader)
}

// RemainingLength returns the remaining duration of the reader
func (pr *PCMReader) RemainingLength() time.Duration {
	return PCMLength(pr.AudioFormat, pr.MemoryReader) - PCMProgress(pr.AudioFormat, pr.MemoryReader)
}

// Progress returns the duration of the data we've read from the start of the
// file to the current position
func (pr *PCMReader) Progress() time.Duration {
	return PCMProgress(pr.AudioFormat, pr.MemoryReader)
}
