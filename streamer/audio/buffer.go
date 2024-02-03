package audio

import (
	"io"
	"sync"
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
