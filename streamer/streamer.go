package streamer

import (
	"context"
	"fmt"
	"io"
	"net"
	"path/filepath"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/streamer/audio"
	"github.com/R-a-dio/valkyrie/streamer/icecast"
	"github.com/cenkalti/backoff"
	"github.com/rs/zerolog"
)

var (
	bufferMP3Size = 1024 * 32 // about 1.3 seconds of audio
	bufferPCMSize = 1024 * 64 // about 0.4 seconds of audio
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

	config.Config
	logger *zerolog.Logger

	// queue used by the streamer
	queue radio.QueueService
	// Format of the PCM audio data
	AudioFormat audio.AudioFormat

	// sync primitives
	wg sync.WaitGroup
	// wgDone gets closed when wg.Wait returns
	wgDone chan struct{}
	cancel context.CancelFunc

	err error
}

// NewStreamer returns a new streamer using the state given
func NewStreamer(ctx context.Context, cfg config.Config, queue radio.QueueService) (*Streamer, error) {
	var s = &Streamer{
		Config: cfg,
		logger: zerolog.Ctx(ctx),
		queue:  queue,
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
		s.logger.Info().Msg("already running")
		return
	}

	// reset our state, use atomics for these since they are read by the
	// HTTP server sometimes
	atomic.StoreInt32(&s.forceDone, 0)
	atomic.StoreInt32(&s.stopping, 0)

	s.err = nil
	s.wg = sync.WaitGroup{}
	s.wgDone = make(chan struct{})
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
			s.logger.Error().Err(err).Msg("pipeline error")
			once.Do(func() {
				s.err = err
				s.cancel()
			})
		}
	}

	s.logger.Info().Msg("setting up pipeline")
	var task = streamerTask{Context: ctx}
	var ch chan streamerTrack
	var start = make(chan streamerTrack)

	task._head = &task
	task.in = start
	for n, fn := range pipeline {
		if n < len(pipeline)-1 {
			ch = make(chan streamerTrack)
			task.out = ch
		} else {
			task.out = start
		}

		s.wg.Add(1)
		go callFunc(fn, task) // SHOULD exit on context cancellation

		task.in = ch
	}

	s.logger.Info().Msg("starting pipeline")
	go func() { // exit on wg done
		defer s.cancel()
		defer close(s.wgDone)
		s.wg.Wait()

		// we now know we're not running anymore, so update our state
		atomic.StoreInt32(&s.started, 0)
	}()
	// and kickstart the head
	start <- streamerTrack{}
}

// Stop stops the streamer, but waits until the current track is done
func (s *Streamer) Stop(ctx context.Context) error {
	const op errors.Op = "streamer/Streamer.Stop"

	log := s.logger.Info().Str("event", "stop").Bool("force", false)
	if atomic.LoadInt32(&s.started) == 0 {
		// we're not running
		log.Msg("not running")
		return errors.E(op, errors.StreamerNotRunning)
	}
	if !atomic.CompareAndSwapInt32(&s.stopping, 0, 1) {
		// we're already trying to stop or have already stopped
		log.Msg("already stopping")
		return errors.E(op, errors.StreamerAlreadyStopped)
	}

	if s.cancel != nil {
		s.cancel()
	}

	log.Msg("waiting")
	select {
	case <-ctx.Done():
	case <-s.wgDone:
	}

	log.Msg("finished")
	if s.err != nil {
		return errors.E(op, s.err)
	}
	return nil
}

// ForceStop stops the streamer and tries to stop as soon as possible
func (s *Streamer) ForceStop(ctx context.Context) error {
	const op errors.Op = "streamer/Streamer.ForceStop"

	// set force unconditionally, since arguments might change between two
	// stop calls (first stop with force=false, second with force=true)
	s.logger.Info().Str("event", "stop").Bool("force", true).Msg("")
	atomic.StoreInt32(&s.forceDone, 1)

	err := s.Stop(ctx)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

// Wait waits for the streamer to stop running; either by an error occuring or
// by someone else calling Stop or ForceStop.
func (s *Streamer) Wait() error {
	const op errors.Op = "streamer/Streamer.Wait"
	s.wg.Wait()
	if s.err != nil {
		return errors.E(op, s.err)
	}
	return nil
}

type pipelineFunc func(streamerTask) error

type streamerTrack struct {
	filepath string
	track    radio.QueueEntry
	pcm      *audio.PCMBuffer
	mp3      *audio.MP3Buffer

	once *sync.Once
}

type streamerTask struct {
	context.Context

	in  <-chan streamerTrack
	out chan<- streamerTrack

	// _head is the task for the headTask, used to signal errors to the front
	_head *streamerTask
}

func (t streamerTrack) String() string {
	return fmt.Sprintf("<%s>", t.track.Metadata)
}

// errored should be called when a recoverable error occurs when handling
// a track. Calling errored implies you're skipping the current track and start
// work on the next track
func (s *Streamer) errored(task streamerTask, track streamerTrack) {
	if task._head == nil {
		panic("errored called with nil head")
	}

	track.once.Do(func() {
		s.logger.Error().Str("metadata", track.track.Metadata).Msg("error in pipeline")
		_, err := s.queue.Remove(task.Context, track.track)
		if err != nil {
			s.logger.Error().Err(err).Msg("queue removal")
		}

		select {
		case task._head.out <- streamerTrack{}:
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

		track.once.Do(func() {
			s.logger.Info().Str("metadata", track.track.Metadata).Msg("working")
			_, err := s.queue.Remove(task.Context, track.track)
			if err != nil {
				s.logger.Error().Err(err).Msg("queue removal")
			}

			select {
			case task.out <- streamerTrack{}:
			case <-task.Done():
			}
		})
	}
}

func (s *Streamer) queueFiles(task streamerTask) error {
	defer func() {
		s.queue.ResetReserved(context.Background())
	}()

	for {
		var track streamerTrack

		select {
		case track = <-task.in:
		case <-task.Done():
			return nil
		}

		entry, err := s.queue.ReserveNext(task.Context)
		if err != nil {
			return err
		}
		track.track = *entry

		track.filepath = track.track.FilePath
		if !filepath.IsAbs(track.filepath) {
			track.filepath = filepath.Join(s.Conf().MusicPath, track.filepath)
		}

		select {
		case task.out <- track:
		case <-task.Done():
			return nil
		}
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

		track.pcm, err = audio.DecodeFileGain(track.filepath)
		if err != nil {
			s.logger.Error().Err(err).Str("metadata", track.track.Metadata).Msg("")
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
	var pcmbuf = make([]byte, bufferPCMSize)
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

		go func() { // exit with context cancellation or task finish
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
					panic("streamer: encoder returned both data and error")
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
		fmt.Fprint(io.Discard, pcm)
		go func() { // exit on task finish
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
	return icecast.Dial(context.TODO(), streamurl,
		icecast.ContentType("audio/mpeg"),
		icecast.UserAgent(s.Conf().UserAgent),
	)
}

func (s *Streamer) streamToIcecast(task streamerTask) error {
	var buf = make([]byte, bufferMP3Size)
	var bufferEnd time.Time
	var bufferLen = time.Second * 2
	var track streamerTrack
	var conn net.Conn

	// setup helpers for backoff handling of the icecast connection
	var newConn = func() error {
		c, err := s.newIcecastConn(s.Conf().Streamer.StreamURL)
		if err != nil {
			return err
		}
		conn = c // move c to method scope if no error occured
		return nil
	}
	var backOff backoff.BackOff = config.NewConnectionBackoff()
	backOff = backoff.WithContext(backOff, task.Context)

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
				err = backoff.Retry(newConn, backOff)
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
	metaFn, err := icecast.Metadata(s.Conf().Streamer.StreamURL,
		icecast.UserAgent(s.Conf().UserAgent),
	)
	if err != nil {
		return err
	}

	// for retrying the metadata request
	var boff backoff.BackOff = config.NewConnectionBackoff()
	boff = backoff.WithContext(boff, task.Context)

	var boffCh <-chan time.Time
	var retrying bool
	var track streamerTrack

	for {
		// only set the channel when we're actually in need of retrying our
		// previous track, otherwise we have no need for a backoff
		if retrying {
			boffCh = time.After(boff.NextBackOff())
		} else {
			boffCh = nil
		}

		select {
		case <-boffCh:
		case track = <-task.in:
			retrying = false
			boff.Reset()
		case <-task.Done():
			return nil
		}

		// only send to the next part of the pipeline if we're not retrying
		// an operation
		if !retrying {
			select {
			case task.out <- track:
			case <-task.Done():
				return nil
			}
		}

		// use a timeout so we don't hang on a request for ages
		ctx, cancel := context.WithTimeout(task.Context, time.Second*5)
		err = metaFn(ctx, track.track.Metadata)
		cancel()
		if err != nil {
			s.logger.Error().Err(err).Msg("failed to send metadata")
			// try and retry the operation in a little while
			retrying = true
		}
	}
}
