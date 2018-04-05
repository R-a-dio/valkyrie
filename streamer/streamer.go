package streamer

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/R-a-dio/valkyrie/database"
	"github.com/R-a-dio/valkyrie/streamer/audio"
)

var (
	httpOK           = []byte("HTTP/1.0 200 OK")
	httpMountInUse   = []byte("HTTP/1.0 403 Mountpoint in use")
	httpUnauthorized = []byte("HTTP/1.0 401 Unauthorized")
)

// ZeroStreamerTrack is a zero'd track
var ZeroStreamerTrack StreamerTrack

type StreamerTrack struct {
	pcm   *audio.PCMBuffer
	mp3   *audio.MP3Buffer
	track database.Track

	once *sync.Once
}

func (st StreamerTrack) String() string {
	return fmt.Sprintf("<%s>", st.track.Metadata)
}

type Streamer struct {
	// started is set when Start is called and unset when (Force)Stop is called
	// it avoids multiple start or stop calls
	started int32
	// forceDone is set when ForceStop is called and unset when Start is called
	// 	it prompts the data sending to icecast to stop right away instead of
	// 	waiting on track end.
	forceDone int32
	// the audio duration send to the encoder but not yet retrieved on the
	// other end of it as encoded audio
	encoderLength int64 // atomic time.Duration

	// State is our shared state across several components
	*State
	// AudioFormat that we're dealing with, in terms of PCM audio data
	AudioFormat audio.AudioFormat

	// channels to communicate between our goroutines
	preloadNext   chan struct{}
	preloadQueue  chan StreamerTrack
	encoderQueue  chan StreamerTrack
	icecastQueue  chan StreamerTrack
	metadataQueue chan StreamerTrack

	// sync primitives
	wg     sync.WaitGroup
	cancel context.CancelFunc
	// error returned by any of our goroutines, returned by Wait/Stop
	err error

	// connection used for icecast
	conn net.Conn
}

// NewStreamer returns a new streamer using the state given
func NewStreamer(state *State) (*Streamer, error) {
	var s = &Streamer{
		State: state,
	}

	s.AudioFormat = audio.AudioFormat{
		ChannelCount:   2,
		BytesPerSample: 2,
		SampleRate:     44100,
	}

	return s, nil
}

func (s *Streamer) makeChannels() {
	s.preloadNext = make(chan struct{})
	s.preloadQueue = make(chan StreamerTrack)
	s.encoderQueue = make(chan StreamerTrack)
	s.icecastQueue = make(chan StreamerTrack)
	s.metadataQueue = make(chan StreamerTrack)
}

// Start starts the streamer, streamer will be shutdown if the context passed
// is canceled.
func (s *Streamer) Start(parentCtx context.Context) {
	if !atomic.CompareAndSwapInt32(&s.started, 0, 1) {
		fmt.Println("streamer is already running")
		return
	}

	var ctx context.Context
	ctx, s.cancel = context.WithCancel(parentCtx)

	// reset sync primitives
	s.wg = sync.WaitGroup{}
	var once sync.Once
	atomic.StoreInt32(&s.forceDone, 0)
	// reset channels
	s.makeChannels()
	// reset error
	s.err = nil

	callFn := func(fn func(context.Context) error) {
		defer s.wg.Done()
		if err := fn(ctx); err != nil {
			once.Do(func() {
				s.err = err
				fmt.Println("streamer exiting:", err)
				s.cancel()
			})
		}

		fmt.Println("finished:", fn)
	}

	methods := []func(context.Context) error{
		s.metadataSend,
		s.icecastWrite,
		s.encoderRun,
		s.preloadTracks,
	}

	for _, fn := range methods {
		s.wg.Add(1)
		go callFn(fn)
	}
}

// Stop stops the streamer and waits for it to complete, returns any errors
// encountered
func (s *Streamer) Stop() error {
	return s.stop(false, false)
}

// ForceStop stops the streamer and interrupts any loops running
func (s *Streamer) ForceStop() error {
	return s.stop(true, false)
}

func (s *Streamer) stop(force bool, restart bool) error {
	if !atomic.CompareAndSwapInt32(&s.started, 1, 0) {
		fmt.Println("streamer not running")
		return nil
	}
	s.cancel()
	if force {
		atomic.StoreInt32(&s.forceDone, 1)
	}
	s.wg.Wait()

	// clean up our icecast connection if it's still open
	if !restart && s.conn != nil {
		s.conn.Close()
	}
	return s.err
}

// Wait waits for the streamer to be stopped by either calling Stop or an error
// occuring
func (s *Streamer) Wait() error {
	s.wg.Wait()
	return s.err
}

func (s *Streamer) metadataSend(ctx context.Context) error {
	// metaurl creates the required URL using StreamURL as base
	metaurl := func(meta string) (string, error) {
		uri, err := url.Parse(s.Conf().StreamURL)
		if err != nil {
			return "", err
		}

		q := url.Values{}
		q.Set("mode", "updinfo")
		q.Set("mount", uri.Path)
		q.Set("charset", "utf8")
		q.Set("song", meta)

		uri.Path = "/admin/metadata"
		uri.RawQuery = q.Encode()

		return uri.String(), nil
	}

	// backoff timer, we set the timer when a request fails and we want to
	// retry it in a little bit
	var backoff = time.NewTimer(time.Hour)
	backoff.Stop()
	// the amount of times we've failed a request, we use this value to increase
	// the delay between requests
	var backoffCount int

	var st StreamerTrack

	for {
		select {
		case <-backoff.C:
			backoffCount *= 2
		case st = <-s.metadataQueue:
			backoffCount = 1
			backoff.Stop()
		case <-ctx.Done():
			return nil
		}

		// protect against the initial timer firing
		if st == ZeroStreamerTrack {
			continue
		}

		select {
		case <-backoff.C:
		default:
		}

		fmt.Println("sending metadata:", st)
		uri, err := metaurl(st.track.Metadata)
		if err != nil {
			// StreamURL is invalid
			return err
		}

		req, err := http.NewRequest("GET", uri, nil)
		if err != nil {
			// request creation failed, either wrong method or URL invalid
			return err
		}
		req = req.WithContext(ctx)

		req.Header.Set("User-Agent", s.Conf().UserAgent)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			backoff.Reset(time.Second * 2 * time.Duration(backoffCount))
		} else {
			resp.Body.Close()
		}
	}
}

// icecastRequest sets up a TCP connection to an icecast-like server and returns
// a net.Conn ready to receive audio data.
func (s *Streamer) icecastRequest() (net.Conn, error) {
	var buf = new(bytes.Buffer)

	uri, err := url.Parse(s.Conf().StreamURL)
	if err != nil {
		return nil, err
	}

	// start of http request
	_, err = fmt.Fprintf(buf, "SOURCE %s HTTP/1.0\r\n", uri.RequestURI())
	if err != nil {
		return nil, err
	}

	// host header to send
	_, err = fmt.Fprintf(buf, "Host: %s\r\n", uri.Host)
	if err != nil {
		return nil, err
	}

	// base64 encode our username:password combination
	auth := base64.StdEncoding.EncodeToString([]byte(uri.User.String()))
	// normal headers
	var h = http.Header{}
	h.Set("Authorization", "Basic "+auth)
	h.Set("User-Agent", s.Conf().UserAgent)
	h.Set("Content-Type", "audio/mpeg")
	if err = h.Write(buf); err != nil {
		return nil, err
	}

	// end of headers
	_, err = fmt.Fprintf(buf, "\r\n")
	if err != nil {
		return nil, err
	}

	// now we connect and write our request
	conn, err := net.Dial("tcp", uri.Host)
	//conn, err := net.DialTimeout("tcp", uri.Host, time.Second*5)
	if err != nil {
		return nil, err
	}

	// write request
	_, err = buf.WriteTo(conn)
	if err != nil {
		return nil, err
	}

	var b = make([]byte, 40)
	// read response
	_, err = conn.Read(b)
	if err != nil {
		return nil, err
	}

	// parse response for errors
	if bytes.HasPrefix(b, httpUnauthorized) {
		return nil, errors.New("connection error: wrong password")
	} else if bytes.HasPrefix(b, httpMountInUse) {
		return nil, errors.New("connection error: mount in use")
	} else if !bytes.HasPrefix(b, httpOK) {
		return nil, errors.New("connection error: unknown error:\n" + string(b))
	}

	return conn, nil
}

func (s *Streamer) icecastWrite(ctx context.Context) error {
	var buf = make([]byte, 1024*16)

	var bufferEnd time.Time            // time point of when our buffer ends
	var bufferLength = time.Second * 2 // buffer length we want to acquire

	var st StreamerTrack
	var mr *audio.MP3BufferReader

	// send initial preload request
	select {
	case s.preloadNext <- struct{}{}:
	case <-ctx.Done():
		return nil
	}

	for {
		select {
		case st = <-s.icecastQueue:
			// create our reader
			mr = st.mp3.Reader()
		case <-ctx.Done():
			return nil
		}
		fmt.Println(st.pcm)
		// we do a second select here because the select above selects at
		// random when both channels are ready, so we eliminate that by using
		// a single-case select
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		select {
		case <-s.gracefulWait:
		case <-ctx.Done():
			return nil
		}

		s.clearTrack(ctx, st)

		// send our metadata to the server async
		select {
		case s.metadataQueue <- st:
		case <-ctx.Done():
			return nil
		}

		var progress time.Duration
		for atomic.LoadInt32(&s.forceDone) == 0 {
			var n int
			var err error

			// if we don't have a connection, create one. When a write errors
			// to a conn we try and create a new one; But fail if the initial
			// creation errors
			if s.conn == nil {
				s.conn, err = s.icecastRequest()
				if err != nil {
					return err
				}

				// also want to resend metadata if we disconnected
				select {
				case s.metadataQueue <- st:
				case <-ctx.Done():
					return nil
				}
			}

			n, err = mr.Read(buf)
			if err != nil && err != io.EOF {
				fmt.Println("icecast reading error:", err)
				break
			}
			if err == io.EOF && n == 0 {
				// end of file and no bytes read
				break
			}

			if bufferEnd.Before(time.Now()) {
				bufferEnd = time.Now()
			}
			bufferEnd = bufferEnd.Add(mr.Progress() - progress)
			progress = mr.Progress()

			_, err = s.conn.Write(buf[:n])
			if err != nil {
				if werr, ok := err.(net.Error); ok {
					fmt.Println(werr, werr.Temporary(), werr.Timeout())
				}
				fmt.Println("icecast write error:", err, n)
				s.conn.Close()
				s.conn = nil
				break
			}

			//fmt.Printf("%s: %s\r", st.track.Metadata, time.Until(bufferEnd)-bufferLength)
			time.Sleep(time.Until(bufferEnd) - bufferLength)
		}
	}
}

func (s *Streamer) encoderRun(ctx context.Context) error {
	var st StreamerTrack
	var pcmBuf = make([]byte, 1024*16)
	var mp3Buf []byte
	var leftover []byte

	// encoder instance, need to call Close when done with it
	var enc *audio.LAME
	defer func() {
		if enc != nil {
			enc.Close()
		}
	}()

	for {
		var err error

		// initialize encoder if we have none
		if enc == nil {
			enc, err = audio.NewLAME(s.AudioFormat)
			if err != nil {
				return err
			}
		}

		select {
		case st = <-s.preloadQueue:
		case <-ctx.Done():
			if st.mp3 != nil {
				st.mp3.Close()
			}
			return nil
		}

		var progress time.Duration // progress in the current track

		// check if we have leftover bytes from the previous track
		if len(leftover) > 0 {
			_, err = st.mp3.Write(leftover)
			if err != nil {
				// this is the start of the track and there should never be
				// an ErrBufferFull returned, and if it does we ignore this
				// track and use the leftover bytes on the next track instead
				//
				// however, we haven't send the track to the next user yet, so
				// we need to clear the track ourselves.
				s.clearTrack(ctx, st)
				continue
			}

			atomic.AddInt64(&s.encoderLength, -int64(st.mp3.Length()-progress))
			progress = st.mp3.Length()

			leftover = nil
		}

		var r = st.pcm.Reader()
		// remove the pcm buffer from the track, so we don't keep it in memory
		st.pcm = nil

		// send the song to the icecast-sending routine after we've handled
		// leftovers, since there might be a broken track
		select {
		case s.icecastQueue <- st:
		case <-ctx.Done():
			return nil
		}

		for {
			n, err := r.Read(pcmBuf)
			if err != nil && err != io.EOF {
				// unknown error, wait for next track
				break
			}
			if err == io.EOF && n == 0 {
				// end of track
				break
			}

			mp3Buf, err = enc.Encode(pcmBuf[:n])
			if err != nil {
				// lame error
				enc.Close()
				enc = nil
				break
			}

			_, err = st.mp3.Write(mp3Buf)

			atomic.AddInt64(&s.encoderLength, -int64(st.mp3.Length()-progress))
			progress = st.mp3.Length()

			if err != nil {
				// either we reached the end of the track, or an unknown error
				// occured in the buffer
				break
			}
		}

		st.mp3.Close()
		leftover = st.mp3.BufferBytes()

		r = nil // discard our PCM reader, it keeps a lot of memory alive
		fmt.Fprint(ioutil.Discard, r)
	}
}

func (s *Streamer) preloadTracks(ctx context.Context) error {
	trace := func(s string, x ...interface{}) {
		s = fmt.Sprintf("preloader: %s\n", s)
		fmt.Printf(s, x...)
	}

	trace("started")
	defer trace("stopped")

	for {
		trace("waiting")
		var st StreamerTrack

		select {
		case <-s.preloadNext:
			trace("running")
		case <-ctx.Done():
			return nil
		}

	again:
		t := s.queue.Peek()
		if t == database.NoTrack {
			return ErrEmptyQueue
		}

		path := filepath.Join(s.Conf().MusicPath, t.FilePath)

		trace("starting: %s", t.Metadata)
		buf, err := audio.DecodeFile(path)
		if err != nil {
			trace("skipping: %s", t.Metadata)
			s.queue.Pop()
			goto again
		}
		if buf.Error() != nil {
			trace("skipping: %s", t.Metadata)
			s.queue.Pop()
			goto again
		}

		st = StreamerTrack{
			pcm:   buf,
			mp3:   audio.NewMP3Buffer(),
			track: t,
			once:  new(sync.Once),
		}

		trace("started: %s", t.Metadata)
		select {
		case s.preloadQueue <- st:
		case <-ctx.Done():
			return nil
		}

		// wait for decoding to be finished
		err = buf.Wait()
		if err != nil {
			trace("wait error: %s", err)
		}
		// set expected length of the output buffer
		st.mp3.SetCap(buf.Length())
	}
}

func (s *Streamer) clearTrack(ctx context.Context, st StreamerTrack) {
	st.once.Do(func() {
		s.queue.PopTrack(st.track)

		select {
		case s.preloadNext <- struct{}{}:
		case <-ctx.Done():
		}
	})
}
