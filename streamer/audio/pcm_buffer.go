package audio

import (
	"io"
	"sync/atomic"
	"time"
)

const pcmBootstrapSize = 1024 * 1024
const pcmReadFromSize = 1024 * 16

//const pcmReadFromSize = (44100 * 2 * 2) * 15 // about 15-seconds of data

// NewPCMBuffer returns a new PCMBuffer with the Format given.
func NewPCMBuffer(f AudioFormat) *PCMBuffer {
	return &PCMBuffer{
		AudioFormat: f,
		Buffer:      NewBuffer(pcmBootstrapSize),
	}
}

// PCMBuffer is a pcm-aware Buffer
type PCMBuffer struct {
	length uint64
	AudioFormat

	*Buffer
}

// Write implements io.Writer
func (pb *PCMBuffer) Write(p []byte) (n int, err error) {
	n, err = pb.Buffer.Write(p)
	atomic.AddUint64(&pb.length, uint64(n))
	return n, err
}

// ReadFrom implements io.ReaderFrom
func (pb *PCMBuffer) ReadFrom(src io.Reader) (n int64, err error) {
	var rn int
	var buf = make([]byte, pcmReadFromSize)
	for err == nil {
		rn, err = src.Read(buf)
		pb.Write(buf[:rn])
		n += int64(rn)
	}
	if err == io.EOF {
		err = nil
	}
	return
}

// Length returns the playback length of the data in the buffer
func (pb *PCMBuffer) Length() time.Duration {
	return time.Duration(atomic.LoadUint64(&pb.length)) * time.Second /
		time.Duration(pb.BytesPerSample*pb.ChannelCount*pb.SampleRate)
}

// Reader returns a reader over the buffer
func (pb *PCMBuffer) Reader() *PCMBufferReader {
	return &PCMBufferReader{
		pcmParent:    pb,
		BufferReader: pb.Buffer.Reader(),
	}
}

// PCMBufferReader is a buffer aware of its contents
type PCMBufferReader struct {
	pcmParent *PCMBuffer

	*BufferReader
}

// Length returns the playback duration of the contents of the buffer.
// i.e. calling Read lowers the duration of the buffer.
func (pbr *PCMBufferReader) Length() time.Duration {
	y := atomic.LoadUint64(&pbr.pcmParent.length) - atomic.LoadUint64(&pbr.pos)
	x := pbr.pcmParent.BytesPerSample * pbr.pcmParent.ChannelCount *
		pbr.pcmParent.SampleRate
	return time.Duration(y) * time.Second / time.Duration(x)
}

func (pbr *PCMBufferReader) Progress() time.Duration {
	y := atomic.LoadUint64(&pbr.pos)
	x := pbr.pcmParent.BytesPerSample * pbr.pcmParent.ChannelCount *
		pbr.pcmParent.SampleRate
	return time.Duration(y) * time.Second / time.Duration(x)
}
