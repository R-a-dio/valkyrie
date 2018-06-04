package manager

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"
)

const maxMetadataLength = 255 * 16

// MetaBufSize is the amount of buffered values in the Listener channels
const MetaBufSize = 1

// Listener implements an icecast MP3 listener that discards audio data and
// only exposes stream metadata through the channels given
type Listener struct {
	ctx    context.Context
	cancel context.CancelFunc

	mu  sync.Mutex // protects err
	err error

	// Err receives any error that is unrecoverable by the listener
	//
	// Close returns the same error as received on Err
	Err chan error
	// Meta receives the metadata parsed from the stream.
	//
	// Both Meta and MetaErr buffer up to MetaBufSize
	// values before discarding values;
	Meta chan map[string]string
}

// NewListener creates a new Listener that tries to listen to the url given.
//
func NewListener(ctx context.Context, streamurl string) (*Listener, error) {
	var l Listener
	l.Meta = make(chan map[string]string, MetaBufSize)
	l.Err = make(chan error, 1)
	l.ctx, l.cancel = context.WithCancel(ctx)

	req, err := http.NewRequest("GET", streamurl, nil)
	if err != nil {
		return nil, err
	}
	// we don't want to re-use connections for the audio stream
	req.Close = true
	// we want interleaved metadata so we have to ask for it
	req.Header.Add("Icy-MetaData", "1")
	req = req.WithContext(l.ctx)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		resp.Body.Close()
		return nil, errors.New("listener: request error: " + resp.Status)
	}

	metasize, err := strconv.Atoi(resp.Header.Get("icy-metaint"))
	if err != nil {
		return nil, err
	}

	go func() {
		// cancel our internal context once we're done
		defer l.cancel()
		// close the body after we're done parseing metadata from it
		defer resp.Body.Close()

		err := l.parseResponse(metasize, resp.Body)
		if err == context.Canceled {
			// we use cancel internally to stop parseResponse; So don't
			// expose that internal error to the user
			err = nil
		}

		l.mu.Lock()
		l.err = err
		l.mu.Unlock()
		l.Err <- err
	}()

	return &l, nil
}

// Close closes the listener and returns any error that occured
func (l *Listener) Close() error {
	l.cancel()
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.err
}

func (l *Listener) parseResponse(metasize int, src io.Reader) error {
	r := bufio.NewReader(src)

	var meta map[string]string
	var buf = make([]byte, metasize)
	if metasize <= maxMetadataLength {
		// we allocate one extra byte to support semicolon insertion in
		// metadata parsing
		buf = make([]byte, maxMetadataLength+1)
	}

	for {
		// we first get actual mp3 data from icecast
		_, err := io.ReadFull(r, buf[:metasize])
		if err != nil {
			return err
		}

		// then we get a single byte indicating metadata length
		b, err := r.ReadByte()
		if err != nil {
			return err
		}

		// if the length is set to 0 we're not expecting any metadata and can
		// read data again
		if b == 0 {
			continue
		}

		// else metadata length needs to be multiplied by 16 from the wire
		length := int(b * 16)
		_, err = io.ReadFull(r, buf[:length])
		if err != nil {
			return err
		}

		// now parse the metadata
		meta = parseMetadata(buf[:length])

		select {
		case l.Meta <- meta:
		case <-time.After(time.Second):
		}
	}
}

func parseMetadata(b []byte) map[string]string {
	var meta = make(map[string]string, 2)

	// trim any padding nul bytes and insert a trailing semicolon if one
	// doesn't exist yet
	for i := len(b) - 1; i > 0; i-- {
		if b[i] == '\x00' {
			continue
		}

		if b[i] == ';' {
			// already have a trailing semicolon
			b = b[:i+1]
			break
		}

		// don't have one, so add one
		b = append(b[:i+1], ';')
		break
	}

	for {
		var key, value string

		b, key = findSequence(b, '=', '\'')
		b, value = findSequence(b, '\'', ';')

		if value == "" {
			break
		}

		meta[key] = value
	}

	return meta
}

func findSequence(seq []byte, a, b byte) ([]byte, string) {
	for i := 1; i < len(seq); i++ {
		if seq[i-1] == a && seq[i] == b {
			return seq[i+1:], string(seq[:i-1])
		}
	}

	return nil, ""
}
