package audio

import (
	"fmt"
	"io"
	"log"
	"os"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/tcolgate/mp3"
)

const MP3EncoderDelay = time.Millisecond * 1000.0 / 44100.0 * 576.0

// NewMP3Buffer returns a Buffer that is mp3-aware to calculate playback
// duration and frame validation.
func NewMP3Buffer(name string, f *os.File) (*MP3Buffer, error) {
	buf, err := NewMemoryBuffer(name, f)
	if err != nil {
		return nil, err
	}

	var mb MP3Buffer
	mb.decoder = mp3.NewDecoder(&mb.decoderBuf)
	mb.totalLength = new(atomic.Int64)
	mb.MemoryBuffer = buf

	return &mb, nil
}

// decoderBuffer is a simple io.Reader that we control the contents
// of by using the byte slice inside
type decoderBuffer struct {
	data []byte
}

func (b *decoderBuffer) Read(p []byte) (n int, err error) {
	n = copy(p, b.data)
	b.data = b.data[n:]
	if n == 0 {
		return 0, io.EOF
	}
	return n, nil
}

// MP3Buffer is a Buffer that is mp3-aware
type MP3Buffer struct {
	*MemoryBuffer
	decoderBuf  decoderBuffer
	decoder     *mp3.Decoder
	frame       mp3.Frame
	totalLength *atomic.Int64
}

func (mb *MP3Buffer) Write(p []byte) (n int, err error) {
	// first we just add all the bytes to the decoder buffer
	mb.decoderBuf.data = append(mb.decoderBuf.data, p...)

	var skipped int
	for {
		beforeData := mb.decoderBuf.data
		// now we try and decode the input data as mp3 frames
		err = mb.decoder.Decode(&mb.frame, &skipped)
		if err != nil {
			// an error occurs in two scenarios:
			// 1. we didn't have a full frame of data
			// 2. the data doesn't contain a valid frame
			//
			// for #1 we just hold the data back and assume the next
			// call to Write will finish it.
			//
			// for #2 we hold the data back as well, because the
			// decoder will skip invalid frames for us once it finds
			// the next frame
			mb.decoderBuf.data = beforeData
			break
		}

		// skipped tells us how much data the decoder skipped
		if skipped > 0 {
			log.Println("skipped on write:", skipped)
		}

		n, err := mb.MemoryBuffer.Write((*mp3frame)(unsafe.Pointer(&mb.frame)).buf)
		if err != nil {
			return n, err
		}

		mb.totalLength.Add(int64(mb.frame.Duration()))
	}

	return len(p), nil
}

// BufferBytes returns unwritten bytes in the internal buffer. return value is
// only valid until next Write call.
func (mb *MP3Buffer) BufferBytes() []byte {
	return mb.decoderBuf.data
}

// TotalLength returns the total duration of the contents of the buffer.
func (mb *MP3Buffer) TotalLength() time.Duration {
	return time.Duration(mb.totalLength.Load())
}

// Reader returns a reader over the buffer
func (mb *MP3Buffer) Reader() (*MP3Reader, error) {
	mbr, err := mb.MemoryBuffer.Reader()
	if err != nil {
		return nil, err
	}

	return newMP3Reader(mbr, mb.totalLength), nil
}

func newMP3Reader(mbr *MemoryReader, length *atomic.Int64) *MP3Reader {
	if length == nil {
		length = new(atomic.Int64)
		// TODO: add an estimated length calculation
	}

	var frame mp3.Frame

	return &MP3Reader{
		MemoryReader: mbr,
		decoder:      mp3.NewDecoder(mbr),
		totalLength:  length,
		frame:        &frame,
		frame2:       (*mp3frame)(unsafe.Pointer(&frame)),
	}
}

func NewMP3Reader(f *os.File) *MP3Reader {
	mb, _ := NewMemoryBuffer("", f)
	if mb == nil {
		return nil
	}
	mb.CloseWrite()

	mbr := &MemoryReader{
		File:     f,
		parent:   mb,
		parentMu: mb.mu.RLocker(),
	}

	return newMP3Reader(mbr, nil)
}

type MP3Reader struct {
	// fields set by parent
	*MemoryReader
	totalLength *atomic.Int64
	// fields for our own use
	progress atomic.Int64
	decoder  *mp3.Decoder

	frame  *mp3.Frame
	frame2 *mp3frame
}

func (mpr *MP3Reader) GetFile() *os.File {
	return mpr.File
}

func (mpr *MP3Reader) Close() error {
	return mpr.MemoryReader.Close()
}

func (mpr *MP3Reader) Read(p []byte) (n int, err error) {
	var skipped int
	var startOffset int64

	for {
		// store where we are in the file
		startOffset, err = mpr.Seek(0, io.SeekCurrent)
		if err != nil {
			break
		}

		// try and decode a frame
		err = mpr.decoder.Decode(mpr.frame, &skipped)
		if err != nil {
			break
		}

		// check if the frame we just decoded fits into p
		if len(p) < n+len(mpr.frame2.buf) {
			// we don't fit, seek back to the start of this frame
			_, err = mpr.MemoryReader.Seek(startOffset, io.SeekStart)
			if err != nil {
				break
			}
			// see if this is the first frame
			if n == 0 {
				// buffer too small to even fit one frame
				return 0, fmt.Errorf("%w: need atleast %d", io.ErrShortBuffer, len(mpr.frame2.buf))
			}
			// not the first frame, return what we have so far
			break
		}

		// copy the frame we just decoded to the output
		n += copy(p[n:], mpr.frame2.buf)
		// add the real-time duration of the frame to our progress
		mpr.progress.Add(int64(mpr.frame.Duration()))
	}

	if n > 0 {
		return n, nil
	}
	return 0, err
}

// TotalLength returns the total length of the reader
func (mpr *MP3Reader) TotalLength() time.Duration {
	return time.Duration(mpr.totalLength.Load())
}

// RemainingLength returns the remaining duration of the reader, that is
// (TotalLength - Progress)
func (mpr *MP3Reader) RemainingLength() time.Duration {
	return mpr.TotalLength() - mpr.Progress()
}

// Progress returns the duration of the audio data we've read so far
func (mpr *MP3Reader) Progress() time.Duration {
	return time.Duration(mpr.progress.Load())
}

func init() {
	if unsafe.Sizeof(mp3frame{}) != unsafe.Sizeof(mp3.Frame{}) {
		panic("mp3 frame size is different")
	}
}

type mp3frame struct {
	buf []byte
}
