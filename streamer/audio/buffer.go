package audio

import (
	"context"
	"errors"
	"io"
	"os"
	"strconv"
	"sync"

	"github.com/justincormack/go-memfd"
)

type AudioFormat struct {
	ChannelCount   int
	BytesPerSample int
	SampleRate     int
}

func NewBuffer(initialSize int) *Buffer {
	var b = Buffer{
		mu:   new(sync.RWMutex),
		buf:  make([]byte, 0, initialSize),
		done: make(chan struct{}),
	}

	b.cond = sync.NewCond(b.mu.RLocker())
	return &b
}

type Buffer struct {
	mu       *sync.RWMutex
	cond     *sync.Cond
	buf      []byte
	err      error
	isClosed bool
	done     chan struct{}
}

func (b *Buffer) Write(p []byte) (n int, err error) {
	if len(p) == 0 {
		return 0, nil
	}

	b.mu.Lock()
	b.buf = append(b.buf, p...)
	b.mu.Unlock()

	b.cond.Broadcast()
	return len(p), nil
}

// Close closes the buffer, any writes will return an error and any readers
// that are blocked will receive an EOF
func (b *Buffer) Close() error {
	b.mu.Lock()
	if !b.isClosed {
		close(b.done)
	}
	b.isClosed = true
	b.mu.Unlock()
	b.cond.Broadcast()
	return nil
}

// Wait waits until Close is called and returns with any error that occured.
func (b *Buffer) Wait() error {
	<-b.done
	b.mu.RLock()
	err := b.err
	b.mu.RUnlock()
	return err
}

// SetError sets an error that is returned by all Readers when Read is called.
// An error set this way does not wait for readers to finish reading data. After
// setting the error, Close is called.
//
// Passing in a nil error is a no-op
func (b *Buffer) SetError(err error) {
	if err == nil {
		return
	}
	b.mu.Lock()
	b.err = err
	b.mu.Unlock()
	b.Close()
}

// Error returns error set previously or nil
func (b *Buffer) Error() (err error) {
	b.mu.Lock()
	err = b.err
	b.mu.Unlock()
	return err
}

// Reader returns a reader over the data contained in the buffer
func (b *Buffer) Reader() *BufferReader {
	return &BufferReader{
		parentMu: b.mu.RLocker(),
		parent:   b,
	}
}

// BufferReader is an io.Reader on top of a Buffer, multiple readers per
// Buffer can be created
type BufferReader struct {
	mu sync.Mutex
	// pos is the position of this reader in parent.buf
	pos uint64

	// mu is an inherited lock from the parent and should be locked when
	// accessing the protected parent fields
	parentMu sync.Locker
	// parent is the Buffer of this reader
	parent *Buffer
}

func (br *BufferReader) Read(p []byte) (n int, err error) {
	br.mu.Lock() // write lock for ourselves
	defer br.mu.Unlock()
	br.parentMu.Lock() // read lock for parent
	defer br.parentMu.Unlock()

	for br.pos == uint64(len(br.parent.buf)) {
		if br.parent.err != nil {
			return 0, br.parent.err
		}
		if br.parent.isClosed {
			return 0, io.EOF
		}

		br.parent.cond.Wait()
	}

	n = copy(p, br.parent.buf[br.pos:])
	br.pos += uint64(n)
	return n, nil
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

func MustFile(f interface{ File() (*os.File, error) }) *os.File {
	fd, err := f.File()
	if err != nil {
		panic("failed MustFile call: " + err.Error())
	}
	return fd
}
