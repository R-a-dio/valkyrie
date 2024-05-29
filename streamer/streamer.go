package streamer

import (
	"context"
	"net"
	"net/url"
	"sync"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/streamer/audio"
	"github.com/R-a-dio/valkyrie/streamer/icecast"
	"github.com/R-a-dio/valkyrie/util"
	"github.com/cenkalti/backoff"
	"github.com/rs/zerolog"
)

var (
	bufferMP3Size = 1024 * 32 // about 1.3 seconds of audio
	bufferPCMSize = 1024 * 64 // about 0.4 seconds of audio
)

// NewStreamer returns a new streamer using the state given
func NewStreamer(ctx context.Context, cfg config.Config, qs radio.QueueService, us radio.UserStorage) (*Streamer2, error) {
	const op errors.Op = "streamer.NewStreamer"

	var s = &Streamer2{
		Config:  cfg,
		baseCtx: ctx,
		queue:   qs,
	}

	// the expected audio format for the stream, this is basically
	// static so we just define it here
	s.AudioFormat = audio.AudioFormat{
		ChannelCount:   2,
		BytesPerSample: 2,
		SampleRate:     44100,
	}

	username := cfg.Conf().Streamer.StreamUsername
	if username == "" {
		// try to get it from the url instead
		username = cfg.Conf().Streamer.StreamURL.URL().User.Username()
	}

	// grab the full user from the database
	user, err := us.Get(username)
	if err != nil {
		return nil, errors.E(op, err)
	}
	s.StreamUser = *user

	zerolog.Ctx(ctx).Info().Str("username", user.Username).Msg("this is me")

	// timer we use for starting the streamer if nobody is on
	startTimer := util.NewCallbackTimer(func() {
		s.Start(ctx)
	})
	// user value to tell us who is streaming according to the proxy
	s.userValue = util.StreamValue(ctx, cfg.Manager.CurrentUser, func(ctx context.Context, user *radio.User) {
		s.userChange(ctx, user, startTimer)
	})
	return s, nil
}

func (s *Streamer2) userChange(ctx context.Context, user *radio.User, timer *util.CallbackTimer) {
	// nobody is streaming
	if !user.IsValid() {
		zerolog.Ctx(ctx).Info().Msg("nobody streaming")

		// we are allowed to connect after a timeout if one is set
		timeout := s.Conf().Streamer.ConnectTimeout
		if timeout > 0 {
			zerolog.Ctx(ctx).Info().
				Dur("timeout", time.Duration(timeout)).
				Msg("starting after timeout")
			timer.Start(time.Second)
			//timer.Start(time.Duration(timeout))
			return
		}
	}
	// if we are supposed to be streaming, we can connect
	if user.ID == s.StreamUser.ID {
		zerolog.Ctx(ctx).Info().Msg("starting because (me)")
		s.Start(context.WithoutCancel(ctx))
		return
	}

	zerolog.Ctx(ctx).Info().
		Str("me", s.StreamUser.Username).
		Str("user", user.Username).
		Msg("not starting")
}

var (
	ErrDecoder = errors.New("decoder error")
	ErrForce   = errors.New("force stop")
)

type Streamer2 struct {
	config.Config
	AudioFormat audio.AudioFormat
	StreamUser  radio.User

	queue     radio.QueueService
	userValue *util.Value[*radio.User]

	// baseCtx is the one passed to NewStreamer
	baseCtx context.Context

	// mu protects cancel and done
	mu sync.Mutex
	// running is true if there is a run instance running
	running bool
	// cancel can be called to force stop the streamer
	cancel context.CancelCauseFunc
	// done is closed after run exits
	done chan struct{}
}

func (s *Streamer2) Start(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	// check if we're not already running
	if s.running {
		return nil
	}
	// don't use the passed in context, it will get canceled once
	// Start returns under normal RPC usage, so we can't use it for
	// anything here, just use our baseCtx instead.
	var ctx context.Context

	ctx, s.cancel = context.WithCancelCause(s.baseCtx)
	s.done = make(chan struct{})

	go func() {
		err := s.run(ctx)
		if err != nil {
			zerolog.Ctx(ctx).Error().Err(err).Msg("run exit")
		}
	}()
	s.running = true
	return nil
}

func (s *Streamer2) Stop(ctx context.Context, force bool) error {
	if force || !force {
		s.mu.Lock()
		s.cancel(ErrForce)
		close(s.done)
		s.mu.Unlock()
		return nil
	}

	return nil
}

// Wait waits until run exits
func (s *Streamer2) Wait() error {
	s.mu.Lock()
	done := s.done
	s.mu.Unlock()
	if done == nil {
		return nil
	}
	<-done
	return nil
}

func runQueue[T any](ctx context.Context, queue chan chan T, returnCh chan T, value T) error {
	for {
		select {
		case ch := <-queue:
			// give this channel the value
			select {
			case ch <- value:
			case <-ctx.Done():
				return ctx.Err()
			}

			// now wait for it to give the value back
			select {
			case value = <-returnCh:
			case <-ctx.Done():
				return ctx.Err()
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (s *Streamer2) run(ctx context.Context) error {
	defer func() {
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
	}()

	logger := zerolog.Ctx(ctx)

	ticket := make(chan struct{}, 2)
	ticket <- struct{}{}
	ticket <- struct{}{}

	// setup the encoder
	var encoder audio.LAME
	if err := s.newEncoder(&encoder); err != nil {
		return err
	}
	// the encoder queue
	encoderQueue := make(chan chan *audio.LAME, 5)
	encoderReturn := make(chan *audio.LAME, 1)
	go runQueue(ctx, encoderQueue, encoderReturn, &encoder)
	ctx = context.WithValue(ctx, encoderReturnKey{}, encoderReturn)

	// setup the icecast connection
	var icecastConn net.Conn
	if err := s.newIcecastConn(ctx, &icecastConn); err != nil {
		return err
	}
	// setup the icecast connection queue
	icecastConnQueue := make(chan chan net.Conn, 5)
	icecastConnReturn := make(chan net.Conn, 1)
	go runQueue(ctx, icecastConnQueue, icecastConnReturn, icecastConn)
	ctx = context.WithValue(ctx, icecastConnReturnKey{}, icecastConnReturn)

	for {
		// grab the next entry
		entry, err := s.queue.ReserveNext(ctx)
		if err != nil {
			logger.Error().Err(err).Msg("failed to get next entry")
			// try again in a bit if we failed to get the next entry
			time.Sleep(time.Second * 2)
			continue
		}

		// setup our context values
		entryCtx := ctx
		// setup the channel we will use to give it the encoder
		encoderCh := make(chan *audio.LAME)
		encoderQueue <- encoderCh
		entryCtx = context.WithValue(entryCtx, encoderKey{}, encoderCh)

		// and the icecast connection
		icecastConnCh := make(chan net.Conn)
		icecastConnQueue <- icecastConnCh
		entryCtx = context.WithValue(entryCtx, icecastConnKey{}, icecastConnCh)

		// wait for us to be allowed to run
		select {
		case <-ticket:
		case <-ctx.Done():
			return ctx.Err()
		}

		// start a goroutine
		go func(ctx context.Context, entry *radio.QueueEntry) {
			defer func() {
				// return our ticket back after we exit
				ticket <- struct{}{}
			}()
			err := s.decodeEntry(ctx, entry)
			if err != nil {
				// TODO: check the error type to see what to do exactly
				// TODO: log the return value of this probably
				_, _ = s.queue.Remove(ctx, entry.QueueID)
				logger.Error().Err(err).Msg("failed")
			}
		}(entryCtx, entry)
	}
}

func get[K any, V any]() func(ctx context.Context) chan V {
	var key K
	return func(ctx context.Context) chan V {
		return ctx.Value(key).(chan V)
	}
}

type icecastConnKey struct{}
type icecastConnReturnKey struct{}
type encoderKey struct{}
type encoderReturnKey struct{}

var getIcecastCh = get[icecastConnKey, net.Conn]()
var getIcecastReturnCh = get[icecastConnReturnKey, net.Conn]()
var getEncoderCh = get[encoderKey, *audio.LAME]()
var getEncoderReturnCh = get[encoderReturnKey, *audio.LAME]()

// PCM is an alias for the type we use for storing PCM audio data
type PCM = audio.PCMReader

func (s *Streamer2) decodeEntry(ctx context.Context, entry *radio.QueueEntry) error {
	const op errors.Op = "streamer/Streamer.decodeEntry"

	// make sure our path is absolute
	filename := util.AbsolutePath(s.Conf().MusicPath, entry.FilePath)
	zerolog.Ctx(ctx).Info().
		Str("id", entry.QueueID.String()).
		Str("filename", filename).
		Msg("decoding")
	// decode our file into pcm data
	pcm, err := audio.DecodeFileGain(s.AudioFormat, filename)
	if err != nil {
		return errors.E(op, err)
	}

	zerolog.Ctx(ctx).Info().
		Str("id", entry.QueueID.String()).
		Str("filename", filename).
		Str("length", pcm.TotalLength().String()).
		Msg("decoded")
	return s.queueEncodeEntry(ctx, entry, pcm)
}

func (s *Streamer2) queueEncodeEntry(
	ctx context.Context,
	entry *radio.QueueEntry,
	pcm *PCM,
) error {

	zerolog.Ctx(ctx).Info().
		Str("id", entry.QueueID.String()).
		Msg("waiting for encoder")
	select {
	case encoder := <-getEncoderCh(ctx):
		var mp3r *audio.MP3Reader
		var err error

		func() {
			defer func() {
				// send back the encoder after we finish encoding
				getEncoderReturnCh(ctx) <- encoder
			}()
			mp3r, err = s.encodeEntry(ctx, entry, pcm, encoder)
		}()
		if err != nil {
			return err
		}
		return s.queueStreamToIcecast(ctx, entry, mp3r)
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Streamer2) encodeEntry(
	ctx context.Context,
	entry *radio.QueueEntry,
	pcm *PCM,
	encoder *audio.LAME,
) (*audio.MP3Reader, error) {
	const op errors.Op = "streamer/Streamer.encodeEntry"
	// close the pcm buffer we receive no matter what
	defer pcm.Close()

	buf := make([]byte, bufferPCMSize)
	mp3, err := audio.NewMP3Buffer()
	if err != nil {
		return nil, err
	}

	zerolog.Ctx(ctx).Info().
		Str("id", entry.QueueID.String()).
		Msg("encoding")
	start := time.Now()

	for {
		n, err := pcm.Read(buf)
		if err != nil && n == 0 {
			break
		}

		mp3buf, err := encoder.Encode(buf[:n])
		if err != nil {
			// the encoder shouldn't be giving us both an error and
			// data but just to make sure our assumption holds we just
			// bail out if it doesn't
			if mp3buf != nil {
				return nil, errors.E(op, err)
			}

			// encoding failed, try and get data from a flush
			mp3buf = encoder.Flush()
			// and then get rid of the encoder and make a new one
			_ = encoder.Close()

			err = s.newEncoder(encoder)
			if err != nil {
				// failed to get a new encoder, that's a problem so fail
				return nil, errors.E(op, err)
			}
		}

		// we either write what we got from Encode, or whatever was leftover
		// from the Flush call on an error
		_, err = mp3.Write(mp3buf)
		if err != nil {
			return nil, errors.E(op, err)
		}
	}

	// we finished our pcm data, flush the encoder and add it to the end
	_, _ = mp3.Write(encoder.Flush())
	// close our mp3 buffer after we make a reader
	defer mp3.Close()

	zerolog.Ctx(ctx).Info().
		Str("id", entry.QueueID.String()).
		Str("length", mp3.TotalLength().String()).
		Str("elapsed", time.Since(start).String()).
		Msg("encoded")
	return mp3.Reader()
}

func (s *Streamer2) queueStreamToIcecast(
	ctx context.Context,
	entry *radio.QueueEntry,
	mp3 *audio.MP3Reader,
) error {

	select {
	case conn := <-getIcecastCh(ctx):
		defer func() {
			getIcecastReturnCh(ctx) <- conn
		}()
		err := s.streamToIcecast(ctx, entry, mp3, &conn)
		if err == nil {
			mp3.Close()
			return nil
		}
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Streamer2) streamToIcecast(
	ctx context.Context,
	entry *radio.QueueEntry,
	mp3r *audio.MP3Reader,
	conn *net.Conn,
) error {
	const op errors.Op = "streamer/Streamer.streamToIcecast"

	// remove the entry from the queue
	s.queue.Remove(ctx, entry.QueueID)
	// send the metadata of this entry to icecast concurrently
	go s.metadataToIcecast(ctx, entry)

	// bufferEnd is when the audio data we've read so far is supposed
	// to end if played back in realtime
	bufferEnd := time.Now()
	// bufferSlack is the period of time we subtract from our sleeping
	// period, to make sure we don't sleep too long
	bufferSlack := time.Second * 2
	// lastProgress is the value of the previous loops Progress call
	lastProgress := time.Duration(0)

	buf := make([]byte, bufferMP3Size)

	zerolog.Ctx(ctx).Info().
		Str("id", entry.QueueID.String()).
		Str("length", mp3r.TotalLength().String()).
		Msg("sending")

	for {
		n, err := mp3r.Read(buf)
		if err != nil && n == 0 {
			break
		}

		// reset the end time if it somehow managed to get below
		// the current time
		if bufferEnd.Before(time.Now()) {
			bufferEnd = time.Now()
		}

		// see how far we are in the mp3 buffer
		curProgress := mp3r.Progress()
		// add the diff between last and cur to the buffer duration
		bufferEnd = bufferEnd.Add(curProgress - lastProgress)
		lastProgress = curProgress

		// write the actual data
		_, err = (*conn).Write(buf[:n])
		if err != nil {
			// error on write probably means our connection is busted
			// close the old one and grab a new one.
			err = s.newIcecastConn(ctx, conn)
			if err != nil {
				return errors.E(op, err)
			}
		}

		time.Sleep(time.Until(bufferEnd) - bufferSlack)
	}

	zerolog.Ctx(ctx).Info().
		Str("id", entry.QueueID.String()).
		Msg("send")

	return nil
}

func (s *Streamer2) metadataToIcecast(ctx context.Context, entry *radio.QueueEntry) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second*30)
	defer cancel()
	bo := config.NewConnectionBackoff(ctx)

	return backoff.RetryNotify(func() error {
		err := icecast.MetadataURL(
			s.streamURL(),
			icecast.UserAgent(s.Conf().UserAgent),
		)(ctx, entry.Metadata)
		if err != nil {
			return err

		}
		return nil
	}, bo, func(err error, d time.Duration) {
		zerolog.Ctx(ctx).Error().Err(err).Dur("backoff", d).Msg("icecast metadata failure")
	})
}

func (s *Streamer2) newEncoder(encoder *audio.LAME) error {
	new, err := audio.NewLAME(s.AudioFormat)
	if err != nil {
		return err
	}
	*encoder = *new
	return nil
}

func (s *Streamer2) newIcecastConn(ctx context.Context, conn *net.Conn) error {
	bo := config.NewConnectionBackoff(ctx)
	var newConn net.Conn
	var err error

	err = backoff.RetryNotify(func() error {
		newConn, err = icecast.DialURL(ctx, s.streamURL(),
			icecast.ContentType("audio/mpeg"),
			icecast.UserAgent(s.Conf().UserAgent),
		)
		if err != nil {
			return err
		}
		return nil
	}, bo, func(err error, d time.Duration) {
		zerolog.Ctx(ctx).Error().Err(err).Dur("backoff", d).Msg("icecast connection failure")
	})
	if err != nil {
		return err
	}
	*conn = newConn
	return nil
}

func (s *Streamer2) streamURL() *url.URL {
	sCfg := s.Conf().Streamer
	// grab the configured url
	uri := sCfg.StreamURL.URL()
	// replace/add username, password combo if one is configured
	if username := sCfg.StreamUsername; username != "" {
		uri.User = url.UserPassword(username, sCfg.StreamPassword)
	}
	return uri
}
