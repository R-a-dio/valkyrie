package audio

import (
	"errors"
	"fmt"
	"io"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/tcolgate/mp3"
)

const MP3EncoderDelay = time.Millisecond * 1000.0 / 44100.0 * 576.0
const mp3BootstrapSize = 1024 * 1024 / 2
const maxInt64 = 1<<63 - 1

// ErrBufferFull is returned when using MP3Buffer.SetCap and a Write exceeding
// said cap occurs.
var ErrBufferFull = errors.New("buffer full")

// NewMP3Buffer returns a Buffer that is mp3-aware to calculate playback
// duration and frame validation.
func NewMP3Buffer() *MP3Buffer {
	buf := NewBuffer(mp3BootstrapSize)

	m := &MP3Buffer{
		Buffer:    buf,
		lengthCap: maxInt64,
	}

	m.dec = mp3.NewDecoder(&m.decBuf)

	return m
}

type decoderBuffer struct {
	buf []byte
}

func (b *decoderBuffer) Read(p []byte) (n int, err error) {
	n = copy(p, b.buf)
	b.buf = b.buf[n:]
	if n == 0 {
		return 0, io.EOF
	}
	return n, nil
}

// MP3Buffer is a Buffer that is mp3-aware
type MP3Buffer struct {
	length    int64
	lengthCap int64

	dec    *mp3.Decoder
	decBuf decoderBuffer
	frame  mp3.Frame

	*Buffer
}

// SetCap sets a length cap, this means any writes past this cap will fail
// with a BufferFull error. Short-writes can occur when using SetCap.
func (mb *MP3Buffer) SetCap(dur time.Duration) {
	atomic.StoreInt64(&mb.lengthCap, int64(dur))
}

func (mb *MP3Buffer) Write(p []byte) (n int, err error) {
	mb.decBuf.buf = append(mb.decBuf.buf, p...)

	var length = atomic.LoadInt64(&mb.length)
	var skipped int
	var hold []byte

	for err == nil {
		if atomic.LoadInt64(&mb.lengthCap)-length < 0 {
			mb.Close()
			return len(p) - len(mb.decBuf.buf), ErrBufferFull
		}

		// hold the current buffer position, Decode reads a frame in sections
		// so when an error is returned it might've read bytes before returning
		hold = mb.decBuf.buf
		decErr := mb.dec.Decode(&mb.frame, &skipped)
		if decErr != nil {
			// restore our buffer position from where the previous frame ended
			mb.decBuf.buf = hold
			break
		}
		if skipped > 0 {
			fmt.Println(p)
			fmt.Println("skipped on write:", skipped, mb.Length(), mb.frame.Size())
		}

		// Write can't return an error
		_, _ = mb.Buffer.Write((*mp3frame)(unsafe.Pointer(&mb.frame)).buf)

		length = atomic.AddInt64(&mb.length, int64(mb.frame.Duration()))
	}

	return len(p), err
}

// BufferBytes returns unwritten bytes in the internal buffer. return value is
// only valid until next Write call.
func (mb *MP3Buffer) BufferBytes() []byte {
	return mb.decBuf.buf
}

// Reader returns a reader over the buffer
func (mb *MP3Buffer) Reader() *MP3BufferReader {
	r := mb.Buffer.Reader()

	return &MP3BufferReader{
		dec:          mp3.NewDecoder(r),
		parent:       mb,
		BufferReader: r,
	}
}

// Length returns the playback duration of the contents of the buffer.
// i.e. calling Write increases the duration
func (mb *MP3Buffer) Length() time.Duration {
	return time.Duration(atomic.LoadInt64(&mb.length))
}

type MP3BufferReader struct {
	// readLength is how much mp3 audio went through Read so far, a call to
	// Read will increase this field
	readLength  int64
	sleepLength int64

	dec           *mp3.Decoder
	frame         mp3.Frame
	frameLeftover bool

	parent *MP3Buffer
	*BufferReader
}

func (mbr *MP3BufferReader) Read(p []byte) (n int, err error) {
	var skipped int
	var buf []byte

	for {
		if !mbr.frameLeftover {
			err = mbr.dec.Decode(&mbr.frame, &skipped)
		}

		mbr.frameLeftover = false
		if skipped > 0 {
			fmt.Println("skipped:", skipped)
		}
		if err != nil {
			return
		}

		buf = mbr.readFrameBuf()
		if len(p) < n+len(buf) {
			if n == 0 {
				err = fmt.Errorf(
					"buffer too small; need atleast: %d bytes got %d",
					len(buf), len(p))
			}
			mbr.frameLeftover = true
			return
		}

		n += copy(p[n:], buf)

		atomic.AddInt64(&mbr.readLength, int64(mbr.frame.Duration()))
		atomic.AddInt64(&mbr.sleepLength, int64(mbr.frame.Duration()))
	}
}

func (mbr *MP3BufferReader) Sleep() {
	l := atomic.LoadInt64(&mbr.sleepLength)
	time.Sleep(time.Duration(l))
	atomic.AddInt64(&mbr.sleepLength, -l)
}

// Length returns the playback duration of the contents of the buffer.
// i.e. calling Read lowers the duration of the buffer.
func (mbr *MP3BufferReader) Length() time.Duration {
	var pl = mbr.parent.Length()
	var rl = time.Duration(atomic.LoadInt64(&mbr.readLength))

	// since we calculate the length concurrently, and separately from writes
	// we have a tiny logic race where read length `rl` can be higher than
	// the parent length `pl`; We just return a zero length when this occurs
	if rl > pl {
		return 0
	}

	return pl - rl
}

func (mbr *MP3BufferReader) Progress() time.Duration {
	return time.Duration(atomic.LoadInt64(&mbr.readLength))
}

func init() {
	if unsafe.Sizeof(mp3frame{}) != unsafe.Sizeof(mp3.Frame{}) {
		panic("mp3 frame size is different")
	}
}

type mp3frame struct {
	buf []byte
}

// readFrameBuf converts the mp3.Frame to an mp3frame and returns the
// internal (private) buf field for use
func (mbr *MP3BufferReader) readFrameBuf() []byte {
	return (*mp3frame)(unsafe.Pointer(&mbr.frame)).buf
}
