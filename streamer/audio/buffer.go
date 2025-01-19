package audio

import (
	"context"
	"errors"
	"io"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/justincormack/go-memfd"
)

type Reader interface {
	TotalLength() time.Duration
	Progress() time.Duration
	Read(p []byte) (n int, err error)
	Close() error
	GetFile() *os.File
}

type AudioFormat struct {
	ChannelCount   int
	BytesPerSample int
	SampleRate     int
}

type MemoryBuffer struct {
	mu   sync.RWMutex
	cond *sync.Cond

	*memfd.Memfd
	isClosed bool
	done     chan struct{}
	err      error
}

func NewMemoryBuffer(name string, f *os.File) (*MemoryBuffer, error) {
	var mf *memfd.Memfd
	var err error
	if f == nil {
		mf, err = memfd.CreateNameFlags(name, memfd.Cloexec|memfd.AllowSealing)
		if err != nil {
			return nil, err
		}
	} else {
		mf = &memfd.Memfd{File: f}
	}

	mb := &MemoryBuffer{
		Memfd: mf,
		done:  make(chan struct{}),
	}

	mb.cond = sync.NewCond(mb.mu.RLocker())
	return mb, nil
}

func (mb *MemoryBuffer) Write(p []byte) (n int, err error) {
	n, err = mb.Memfd.Write(p)
	mb.cond.Broadcast()
	return n, err
}

// Close calls both CloseWrite and Close on the underlying resource, this makes
// the contents of the buffer void if no readers exist
func (mb *MemoryBuffer) Close() error {
	mb.CloseWrite()
	return mb.Memfd.Close()
}

// CloseWrite marks the buffer as closed for writing, and will have all readers
// return io.EOF when they get to the end of the buffer instead of blocking
func (mb *MemoryBuffer) CloseWrite() {
	mb.mu.Lock()
	if !mb.isClosed {
		close(mb.done)
	}
	mb.isClosed = true
	mb.mu.Unlock()
	mb.cond.Broadcast()
}

func (mb *MemoryBuffer) File() (*os.File, error) {
	return mb.Memfd.File, nil
}

func (mb *MemoryBuffer) Wait(ctx context.Context) error {
	select {
	case <-mb.done:
		mb.mu.RLock()
		defer mb.mu.RUnlock()
		return mb.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (mb *MemoryBuffer) SetError(err error) {
	if err == nil {
		return
	}

	mb.mu.Lock()
	mb.err = err
	mb.mu.Unlock()
	mb.Close()
}

// Error returns error set previously or nil
func (mb *MemoryBuffer) Error() (err error) {
	mb.mu.RLock()
	defer mb.mu.RUnlock()
	return mb.err
}

func (mb *MemoryBuffer) Reader() (*MemoryReader, error) {
	raw, err := mb.SyscallConn()
	if err != nil {
		return nil, err
	}

	var file *os.File
	var ferr error
	err = raw.Control(func(fd uintptr) {
		file, ferr = os.Open("/proc/self/fd/" + strconv.Itoa(int(fd)))
	})
	if err != nil {
		return nil, err
	}
	if ferr != nil {
		return nil, ferr
	}

	return &MemoryReader{
		File:     file,
		parent:   mb,
		parentMu: mb.mu.RLocker(),
	}, nil
}

type MemoryReader struct {
	*os.File
	parent   *MemoryBuffer
	parentMu sync.Locker
}

func (mr *MemoryReader) Read(p []byte) (n int, err error) {
	mr.parentMu.Lock()
	defer mr.parentMu.Unlock()

	for {
		// first just try reading from the file
		n, err = mr.File.Read(p)
		if err == nil {
			// happy path, there was just data in the file
			return n, nil
		}

		// otherwise check if there was a non-EOF error
		if !errors.Is(err, io.EOF) {
			// those we just return
			return n, err
		}

		// otherwise check if we are closed and should return EOF
		if mr.parent.isClosed {
			break
		}

		// but if it was EOF, we might not actually be done and wait for a
		// write to occur and then try again
		mr.parent.cond.Wait()
	}

	return 0, io.EOF
}

func (mr *MemoryReader) Close() error {
	return mr.File.Close()
}
