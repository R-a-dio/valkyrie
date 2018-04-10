package streamer

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"runtime/debug"
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

// Streamer represents a single icecast stream
type Streamer struct {
	// started is set if we're currently running
	started int32
	// stopping is set if we're in the progress of stopping
	stopping int32
	// forceDone is set when we want an immediate shutdown, instead of waiting
	// on work to finish before shutdown
	forceDone int32

	// State shared between queue and streamer
	*State
	// Format of the PCM audio data
	AudioFormat audio.AudioFormat

	// sync primitives
	wg     sync.WaitGroup
	cancel context.CancelFunc

	err error
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

// Start starts the streamer with the context given, Start is a noop if
// already started
func (s *Streamer) Start(ctx context.Context) {
	if !atomic.CompareAndSwapInt32(&s.started, 0, 1) {
		// we're already running
		log.Println("streamer.start: already running")
		return
	}

	// reset our state, use atomics for these since they are read by the
	// HTTP server sometimes
	atomic.StoreInt32(&s.forceDone, 0)
	atomic.StoreInt32(&s.stopping, 0)

	s.err = nil
	s.wg = sync.WaitGroup{}
	var once sync.Once

	ctx, s.cancel = context.WithCancel(ctx)

	pipeline := []pipelineFunc{
		s.headTask,
		s.queueFiles,
		s.decodeFiles,
		s.encodeToMP3,
		s.streamToIcecast,
		s.metadataToIcecast,
		s.tailTask,
	}

	callFunc := func(fn pipelineFunc, task streamerTask) {
		defer s.wg.Done()
		err := fn(task)
		if err != nil {
			log.Printf("streamer: pipeline error: %s\n", err)
			once.Do(func() {
				s.err = err
				s.cancel()
			})
		}
	}

	log.Println("streamer.start: setting up pipeline")
	var task = streamerTask{Context: ctx}
	var ch chan streamerTrack
	var start = make(chan streamerTrack)

	task.in = start
	for n, fn := range pipeline {
		if n < len(pipeline)-1 {
			ch = make(chan streamerTrack)
			task.out = ch
		} else {
			task.out = start
		}

		s.wg.Add(1)
		go callFunc(fn, task)

		task.in = ch
	}

	log.Println("streamer.start: starting pipeline")
	// and kickstart the head
	start <- streamerTrack{}
}

func (s *Streamer) stop(force bool, graceful bool) error {
	// set force unconditionally, since arguments might change between two
	// stop calls (first stop with force=false, second with force=true)
	if force {
		log.Println("streamer.stop: stopping with force=true")
		atomic.StoreInt32(&s.forceDone, 1)
	}

	if !atomic.CompareAndSwapInt32(&s.stopping, 0, 1) {
		// we're already trying to stop or have already stopped
		log.Println("streamer.stop: already stopping")
		return nil
	}

	s.cancel()
	log.Println("streamer.stop: waiting on completion")
	s.wg.Wait()
	// we now know we're not running anymore, so update our state
	atomic.StoreInt32(&s.started, 0)

	if !graceful {
		// TODO: cleanup resources
	}

	log.Println("streamer.stop: finished")
	return s.err
}

// Stop stops the streamer, but waits until the current track is done
func (s *Streamer) Stop() error {
	return s.stop(false, false)
}

// ForceStop stops the streamer and tries to stop as soon as possible
func (s *Streamer) ForceStop() error {
	return s.stop(true, false)
}

// Wait waits for the streamer to stop running; either by an error occuring or
// by someone else calling Stop or ForceStop.
func (s *Streamer) Wait() error {
	s.wg.Wait()
	return s.err
}

type pipelineFunc func(streamerTask) error

type streamerTrack struct {
	filepath string
	track    database.Track
	pcm      *audio.PCMBuffer
	mp3      *audio.MP3Buffer

	once *sync.Once
}

type streamerTask struct {
	context.Context

	in  <-chan streamerTrack
	out chan<- streamerTrack
}

func (t streamerTrack) String() string {
	return fmt.Sprintf("<%s>", t.track.Metadata)
}

// errored should be called when a recoverable error occurs when handling
// a track. Calling errored implies you're skipping the current track and start
// work on the next track
func (s *Streamer) errored(task streamerTask, track streamerTrack) {
	track.once.Do(func() {
		log.Printf("streamer.errored: on track %s\n", track)
		s.queue.RemoveTrack(track.track)

		select {
		case task.out <- streamerTrack{}:
		case <-task.Done():
		}
	})
}

// finished should only be called by the tailTask, use errored instead if
// you need to skip a track
func (s *Streamer) finished(task streamerTask, track streamerTrack) {
	track.once.Do(func() {
		log.Printf("streamer.finished: on track %s\n", track)
		s.queue.PopTrack(track.track)

		select {
		case task.out <- streamerTrack{}:
		case <-task.Done():
		}
	})
}

// headTask is the function running at the start of the pipeline
func (s *Streamer) headTask(task streamerTask) error {
	for {
		select {
		case <-task.in:
		case <-task.Done():
			return nil
		}

		track := streamerTrack{
			once: new(sync.Once),
		}

		select {
		case task.out <- track:
		case <-task.Done():
			return nil
		}

	}
}

// tailTask is the function running at the end of the pipeline
func (s *Streamer) tailTask(task streamerTask) error {
	for {
		var track streamerTrack

		select {
		case track = <-task.in:
		case <-task.Done():
			return nil
		}

		s.finished(task, track)
	}
}

func (s *Streamer) queueFiles(task streamerTask) error {
	var last streamerTrack
	for {
		var track streamerTrack

		select {
		case track = <-task.in:
		case <-task.Done():
			return nil
		}

		track.track = s.queue.PeekTrack(last.track)
		if track.track == database.NoTrack {
			return ErrEmptyQueue
		}

		track.filepath = filepath.Join(s.Conf().MusicPath, track.track.FilePath)

		select {
		case task.out <- track:
		case <-task.Done():
			return nil
		}

		last = track
	}
}

func (s *Streamer) decodeFiles(task streamerTask) error {
	for {
		var err error
		var track streamerTrack

		select {
		case track = <-task.in:
		case <-task.Done():
			return nil
		}

		track.pcm, err = audio.DecodeFile(track.filepath)
		if err != nil {
			s.errored(task, track)
			continue
		}
		track.mp3 = audio.NewMP3Buffer()

		select {
		case task.out <- track:
		case <-task.Done():
			return nil
		}

		// wait for decode completion, the error is not important because it
		// will also be returned on reading calls, so it will show up in the
		// next function in the pipeline
		_ = track.pcm.Wait()
		// set the expected length of the mp3 output
		track.mp3.SetCap(track.pcm.Length())
	}
}

func (s *Streamer) encodeToMP3(task streamerTask) error {
	var pcmbuf = make([]byte, 1024*16)
	var mp3buf []byte
	var enc *audio.LAME
	var err error

	defer func() {
		// free our encoder when we're exiting
		if enc != nil {
			_ = enc.Close()
		}
	}()

	for {
		var track streamerTrack

		select {
		case track = <-task.in:
		case <-task.Done():
			return nil
		}

		// handle any leftover data we overwrote into the previous track buffer
		if len(mp3buf) > 0 {
			_, err = track.mp3.Write(mp3buf)
			if err != nil {
				// we've just started a new track, so we shouldn't be getting
				// any kind of error from the buffer, if this does occur however
				// we're going to assume something is wrong and skip this track
				s.errored(task, track)
				continue
			}
		}

		pcm := track.pcm.Reader()
		// clear the pcm buffer reference so that it can be gc'd sooner,
		// the rest of the pipeline does not need it anymore
		track.pcm = nil

		// send over the track concurrently so that we can encode the track
		// before it starts playing
		send := make(chan bool)

		go func() {
			select {
			case task.out <- track:
				send <- false
			case <-task.Done():
				send <- true
			}
		}()

		// end our encoding when either an error occurs or we reach the
		// the end of the track, both are communicated with an error
		for err == nil {
			// create a new encoder if we don't have one, this is either at the
			// start of our run, or after an error was returned by the encoder
			if enc == nil {
				enc, err = audio.NewLAME(s.AudioFormat)
				if err != nil {
					// not being able to initialize the encoder is a fatal
					// error, so return completely
					return err
				}
			}

			var n int
			n, err = pcm.Read(pcmbuf)
			if err != nil && n == 0 {
				break
			}

			mp3buf, err = enc.Encode(pcmbuf[:n])
			if err != nil {
				// code at time of writing guarantees we either get data or an
				// error, and never both. But we check for it to be sure
				if mp3buf != nil {
					// TODO: log warning
				}
				// encoding error, we should try flushing the encoder buffers
				// and handling the data that was still in there
				mp3buf = enc.Flush()
				// now set the encoder to nil so we can make a new one at the
				// start of the next iteration
				_ = enc.Close()
				enc = nil
			}

			_, err = track.mp3.Write(mp3buf)
		}

		_ = track.mp3.Close()
		// the buffer used only accepts data that makes a full mp3 frame, and
		// keeps an internal buffer for data that started a frame but didn't
		// finish it yet. Since we're going to swap to a different buffer after
		// the track is done, we need to carry over this internal buffer to the
		// next one.
		mp3buf = track.mp3.BufferBytes()

		// -- HACK --
		// do a little trick to force the compiler to clear our reader variable
		// so that we don't keep it in memory longer than needed, this is all
		// the audio data stored in pcm
		pcm = nil
		fmt.Fprint(ioutil.Discard, pcm)
		go func() {
			time.Sleep(time.Second)
			debug.FreeOSMemory()
		}()
		// -- END HACK --

		// now wait for the track to have been send to the next function in
		// the pipeline. We get a true back if done was called on the task
		if <-send {
			return nil
		}
	}
}

func (s *Streamer) newIcecastConn(streamurl string) (conn net.Conn, err error) {
	defer func() {
		// we want to set the graceful conn to whatever we return here, there
		// is no point in keeping a broken conn in there, so also do it on
		// error paths
		s.graceful.setConn(conn)
	}()

	// check if there is a connection waiting from a previous process
	if conn = <-s.graceful.conn; conn != nil {
		return conn, nil
	}

	var buf = new(bytes.Buffer)

	uri, err := url.Parse(streamurl)
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
	conn, err = net.Dial("tcp", uri.Host)
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

func (s *Streamer) streamToIcecast(task streamerTask) error {
	var buf = make([]byte, 1024*16)
	var bufferEnd time.Time
	var bufferLen = time.Second * 2
	var track streamerTrack
	var conn net.Conn

	for {
		select {
		case track = <-task.in:
		case <-task.Done():
			return nil
		}

		select {
		case task.out <- track:
		case <-task.Done():
			return nil
		}

		var mp3 = track.mp3.Reader()
		var progress time.Duration
		var err error

		for atomic.LoadInt32(&s.forceDone) == 0 {
			if conn == nil {
				conn, err = s.newIcecastConn(s.Conf().StreamURL)
				if err != nil {
					return err
				}

				// since we have a new connection, send the metadata again
				select {
				case task.out <- track:
				case <-task.Done():
					return nil
				}
			}

			n, err := mp3.Read(buf)
			if err != nil && n == 0 {
				break
			}

			// calculate the amount of audio data we're sending over the
			// connection, we want to sleep if we've send enough data
			if bufferEnd.Before(time.Now()) {
				bufferEnd = time.Now()
			}
			bufferEnd = bufferEnd.Add(mp3.Progress() - progress)
			progress = mp3.Progress()

			_, err = conn.Write(buf[:n])
			if err != nil {
				conn = nil
				break
			}

			// sleep while the audio data we've already send is above
			// bufferLen duration
			time.Sleep(time.Until(bufferEnd) - bufferLen)
		}
	}
}

func (s *Streamer) metadataToIcecast(task streamerTask) error {
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
	var track streamerTrack

	for {
		select {
		case <-backoff.C:
			backoffCount *= 2
		case track = <-task.in:
			backoffCount = 1
			backoff.Stop()
		case <-task.Done():
			return nil
		}

		// protect against the initial timer firing
		if track == (streamerTrack{}) {
			continue
		}

		// only send the track ahead the first time around
		if backoffCount == 1 {
			select {
			case task.out <- track:
			case <-task.Done():
				return nil
			}
		}

		select {
		case <-backoff.C:
		default:
		}

		uri, err := metaurl(track.track.Metadata)
		if err != nil {
			// StreamURL is invalid
			return err
		}

		req, err := http.NewRequest("GET", uri, nil)
		if err != nil {
			// request creation failed, either wrong method or URL invalid
			return err
		}
		// use a timeout so we don't hang on a request for ages
		ctx, cancel := context.WithTimeout(task.Context, time.Second*5)
		req = req.WithContext(ctx)

		req.Header.Set("User-Agent", s.Conf().UserAgent)

		resp, err := http.DefaultClient.Do(req)
		cancel()
		if err != nil || resp.StatusCode != 200 {
			log.Printf("streamer.metadata: failed to send: %s", err)
			// try again if the request failed
			backoff.Reset(time.Second * 2 * time.Duration(backoffCount))
		}

		if err == nil {
			resp.Body.Close()
		}
	}
}
