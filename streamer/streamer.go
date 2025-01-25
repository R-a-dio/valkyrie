package streamer

import (
	"context"
	"net"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/rpc"
	"github.com/R-a-dio/valkyrie/streamer/audio"
	"github.com/R-a-dio/valkyrie/streamer/icecast"
	"github.com/R-a-dio/valkyrie/util"
	"github.com/Wessie/fdstore"
	"github.com/cenkalti/backoff"
	"github.com/rs/zerolog"
)

var (
	bufferMP3Size = 1024 * 32 // about 1.3 seconds of audio
	bufferPCMSize = 1024 * 64 // about 0.4 seconds of audio

	fdstoreIcecastConn     = "streamer-icecast"
	fdstoreStreamerCurrent = "streamer-current"
	fdstoreEncoder         = "streamer-encoder"
)

// NewStreamer returns a new streamer using the state given
func NewStreamer(ctx context.Context, cfg config.Config,
	fdstorage *fdstore.Store,
	qs radio.QueueService,
	us radio.UserStorage,
) (*Streamer, error) {
	const op errors.Op = "streamer.NewStreamer"

	var s = &Streamer{
		Config:    cfg,
		baseCtx:   ctx,
		queue:     qs,
		fdstorage: fdstorage,
		lastStartPoke: util.NewTypedValue(
			time.Now().Add(-time.Duration(cfg.Conf().Streamer.ConnectTimeout) * 2),
		),
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

	zerolog.Ctx(ctx).Info().Ctx(ctx).Str("username", user.Username).Msg("this is me")

	// before we check for the user from the manager, check if we are doing a restart
	// and have saved state in the fdstore
	s.checkFDStore(ctx, fdstorage)
	if s.trackStore == nil {
		// only create a new track storage if checkFDStore didn't make one for us
		s.trackStore = NewTracks(ctx, fdstorage, nil)
	}

	// timer we use for starting the streamer if nobody is on
	startTimer := util.NewCallbackTimer(func() {
		zerolog.Ctx(ctx).Info().Ctx(ctx).Msg("calling start after timeout")
		s.Start(ctx)
	})
	// user value to tell us who is streaming according to the proxy
	s.userValue = util.StreamValue(ctx, cfg.Manager.CurrentUser, func(ctx context.Context, user *radio.User) {
		s.userChange(ctx, user, startTimer)
	})

	// now we start the encoder routine
	go func() {
		err := s.encoder(ctx, nil)
		if err != nil {
			zerolog.Ctx(ctx).Error().Ctx(ctx).Err(err).Msg("encoder exit")
			return
		}
		zerolog.Ctx(ctx).Info().Ctx(ctx).Msg("encoder exit")
	}()
	return s, nil
}

func (s *Streamer) checkFDStore(ctx context.Context, store *fdstore.Store) {
	if store == nil {
		zerolog.Ctx(ctx).Info().Ctx(ctx).Msg("nothing to restore from fdstore")
		return
	}

	var conn net.Conn
	var current *audio.MP3Reader
	var currentEntry radio.QueueEntry

	// recover the icecast connection if any
	connEntries, err := store.RemoveConn(fdstoreIcecastConn)
	if err != nil {
		zerolog.Ctx(ctx).Error().Ctx(ctx).Err(err).Msg("failed to get stored icecast conn")
	}

	if len(connEntries) > 0 {
		zerolog.Ctx(ctx).Info().Ctx(ctx).Msg("recovered an icecast connection")
		// grab the first entry
		conn = connEntries[0].Conn
		// close the rest
		for _, entry := range connEntries[1:] {
			zerolog.Ctx(ctx).Info().Ctx(ctx).Msg("received extra icecast connections")
			entry.Conn.Close()
		}
	}

	// recover the currently playing song if any
	currentEntries := store.RemoveFile(fdstoreStreamerCurrent)
	if len(currentEntries) > 0 {
		// grab the first entry again, should only be one anyway
		entry := currentEntries[0]

		// decode the queue entry that should be in the data
		currentEntry, err = rpc.DecodeQueueEntry(entry.Data)
		if err != nil {
			zerolog.Ctx(ctx).Error().Ctx(ctx).Err(err).Msg("failed to get stored current song")
		}

		// make an MP3Reader from the file again
		current = audio.NewMP3Reader(entry.File)

		// close the rest
		for _, entry := range currentEntries[1:] {
			zerolog.Ctx(ctx).Info().Ctx(ctx).Msg("received extra current files")
			entry.File.Close()
		}
	}

	var entries []StreamTrack

	if current != nil {
		zerolog.Ctx(ctx).Info().Ctx(ctx).Msg("recovered the current song")
		entries = append(entries, StreamTrack{
			QueueEntry: currentEntry,
			Audio:      current,
		})
	}

	// recover any songs that were already encoded before we restarted
	encoderEntries := store.RemoveFile(fdstoreEncoder)
	ee := make(map[radio.QueueID]StreamTrack, len(encoderEntries))
	for _, entry := range encoderEntries {
		queueEntry, err := rpc.DecodeQueueEntry(entry.Data)
		if err != nil {
			zerolog.Ctx(ctx).Error().Ctx(ctx).Err(err).Msg("failed to get stored queue song")
			entry.File.Close()
			continue
		}

		reader := audio.NewMP3Reader(entry.File)
		if reader == nil {
			zerolog.Ctx(ctx).Error().Ctx(ctx).Msg("failed to create mp3reader")
			entry.File.Close()
			continue
		}

		zerolog.Ctx(ctx).Info().Ctx(ctx).Msg("recovered an encoder song")
		// store them temporarily
		ee[queueEntry.QueueID] = StreamTrack{
			QueueEntry: queueEntry,
			Audio:      reader,
		}
	}

	// now go over the restored tracks and see if they're still in our queue
	// use a temporary slice because of a potential mismatch between the queue
	// and what we restored.
	ees := make([]StreamTrack, 0)
	for range len(ee) {
		// for each song we recover from the encoder we should reserve a song
		entry, err := s.queue.ReserveNext(ctx)
		if err != nil {
			zerolog.Ctx(ctx).Error().Ctx(ctx).Err(err).Msg("failed to reserve next from queue")
			continue
		}

		// see if the entry is in our restored entries
		st, ok := ee[entry.QueueID]
		if !ok {
			// if it isn't, it probably means something else mutated the queue while we were restarting
			// we just throw away everything else we restored in this case
			zerolog.Ctx(ctx).Warn().Msg("next queue entry wasn't in our recovered songs")
			// reset anything we reserved
			s.queue.ResetReserved(ctx)
			break
		}

		ees = append(ees, st)
	}

	// move the finished product
	for _, st := range ees {
		entries = append(entries, st)
		// remove everything we're using from the map
		delete(ee, st.QueueID)
	}

	// any leftovers in ee need to be cleaned up since we're not going to be using them
	for _, st := range ee {
		st.Audio.Close()
	}

	s.trackStore = NewTracks(ctx, store, entries)
	if current != nil || conn != nil {
		// only force a start if we recovered something
		s.start(ctx, conn)
	}
}

func (s *Streamer) userChange(ctx context.Context, user *radio.User, timer *util.CallbackTimer) {
	// nobody is streaming
	if !user.IsValid() {
		zerolog.Ctx(ctx).Info().Ctx(ctx).Msg("nobody streaming")

		// we are allowed to connect after a timeout if one is set
		timeout := s.Conf().Streamer.ConnectTimeout
		if timeout == 0 {
			zerolog.Ctx(ctx).Info().Ctx(ctx).Msg("timeout is zero, not connecting")
			return
		}

		if time.Since(s.lastStartPoke.Load()) < time.Duration(timeout) {
			// we have been poked recently, so just connect instantly
			zerolog.Ctx(ctx).Info().Ctx(ctx).Msg("starting because recent poke")
			s.Start(context.WithoutCancel(ctx))
			return
		}

		// otherwise just start our timeout period
		zerolog.Ctx(ctx).Info().
			Dur("timeout", time.Duration(timeout)).
			Msg("starting after timeout")

		timer.Start(time.Duration(timeout))
		return
	}

	// stop the potential timer if we got a new user
	timer.Stop()

	// if we are supposed to be streaming, we can connect
	if user.ID == s.StreamUser.ID {
		zerolog.Ctx(ctx).Info().Ctx(ctx).Msg("starting because (me)")
		s.Start(context.WithoutCancel(ctx))
		return
	}

	// we're not supposed to be streaming, don't start if that was asked of us
	// but we also need to stop if we are somehow already running, do this with
	// force because the only way this should happen is if we got kicked from
	// icecast.
	s.Stop(context.WithoutCancel(ctx), true)
	zerolog.Ctx(ctx).Info().
		Str("me", s.StreamUser.Username).
		Str("user", user.Username).
		Msg("not starting")
}

var (
	ErrDecoder = errors.New("decoder error")
	ErrForce   = errors.New("force stop")
)

type Streamer struct {
	// configuration fields, these shouldn't change after creation
	config.Config
	// AudioFormat is the format of the audio we're streaming
	AudioFormat audio.AudioFormat
	// StreamUser is the user we're streaming as
	StreamUser radio.User
	// fdstorage is for graceful restarts
	fdstorage *fdstore.Store
	// queue is the queue service we use to get what to play
	queue radio.QueueService
	// baseCtx is the base context used when Start is called
	baseCtx context.Context
	// trackStore holds preloaded tracks for the encoder
	trackStore *trackstore

	userValue     *util.Value[*radio.User]
	lastStartPoke *util.TypedValue[time.Time]

	// mu protected fields
	mu      sync.Mutex
	running bool
	done    chan struct{}

	cancel context.CancelCauseFunc

	// atomics
	forced  atomic.Bool // true when Stop was called with force set to true
	restart atomic.Bool // true if we want to keep state for a restart
}

func (s *Streamer) Start(_ context.Context) error {
	if latest := s.userValue.Latest(); latest.IsValid() && latest.ID != s.StreamUser.ID {
		zerolog.Ctx(s.baseCtx).Info().
			Time("time", time.Now()).
			Str("current dj", latest.Username).
			Msg("start poked")
		// if someone is streaming, we don't start but just record that
		// we have been poked at this point in time
		s.lastStartPoke.Store(time.Now())
		return nil
	}

	s.start(s.baseCtx, nil)
	return nil
}

func (s *Streamer) start(ctx context.Context, conn net.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running { // already running
		zerolog.Ctx(ctx).Info().Ctx(ctx).Msg("start called while we're already running")
		return
	}

	ctx, s.cancel = context.WithCancelCause(ctx)
	// create a channel we can use in Wait to see if we exited
	s.done = make(chan struct{})
	// reset force state
	s.forced.Store(false)
	// reset the restart state, this should never be needed
	s.restart.Store(false)

	popperCh := s.trackStore.PopCh()
	go func(done chan struct{}) { // icecast
		defer func() {
			s.mu.Lock()
			s.running = false
			s.mu.Unlock()
		}()
		// close the done channel, no streaming without this icecast routine
		defer close(done)

		err := s.icecast(ctx, conn, popperCh)
		if err != nil {
			zerolog.Ctx(ctx).Error().Ctx(ctx).Err(err).Msg("icecast exit")
			return
		}
		zerolog.Ctx(ctx).Info().Ctx(ctx).Msg("icecast exit")
	}(s.done)

	// mark ourselves as running
	s.running = true
}

func (s *Streamer) Stop(ctx context.Context, force bool) error {
	const op errors.Op = "streamer/Streamer.Stop"

	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		// we're not even running
		return errors.E(op, errors.StreamerNotRunning)
	}

	// we have three methods of stopping, this function handles two of them, the third
	// is handled in handleRestart
	if force {
		// #1 is a force stop, we just stop anything we're doing and exit right away
		// we achieve this by canceling the context we passed in and setting a force
		// flag that is checked in all the loops we run
		s.cancel(ErrForce)
		s.forced.Store(true)
		s.mu.Unlock()
		return s.Wait(ctx)
	}

	// #2 is a normal stop, this will exit once the current song ends, we achieve this
	// by closing the input channel then waiting for the icecast to notice the close.
	s.trackStore.CyclePopCh()
	s.mu.Unlock()
	return s.Wait(ctx)
}

func (s *Streamer) Wait(ctx context.Context) error {
	s.mu.Lock()
	if !s.running { // we're not running, nothing to wait on
		s.mu.Unlock()
		return nil
	}
	// otherwise grab the done channel
	done := s.done
	s.mu.Unlock()

	// and wait for it to be closed, or our ctx to finish
	select {
	case <-done:
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
}

// restart tries to start a restart with state passing
func (s *Streamer) handleRestart(ctx context.Context) error {
	// #3 is a force stop, but we instruct the routines to store their state by
	// using the fdstorage and restart afterwards
	s.restart.Store(true)
	return s.Stop(ctx, true)
}

func (s *Streamer) encoder(ctx context.Context, encoder *audio.LAME) error {
	logger := zerolog.Ctx(ctx)
	buf := make([]byte, bufferPCMSize)

	// before we enter our main loop, check to see what the state is of our tracks
	// storage, it might already contain songs and we should wait for one (or more)
	// of them to be removed first
	select {
	case <-s.trackStore.NotifyCh():
	case <-ctx.Done():
		return context.Cause(ctx)
	}

	defer s.queue.ResetReserved(context.WithoutCancel(ctx))
	for !s.forced.Load() {
		entry, err := s.queue.ReserveNext(ctx)
		if err != nil {
			logger.Error().Ctx(ctx).Err(err).Msg("failed to get next queue entry")
			time.Sleep(time.Second * 2)
			continue
		}

		// make sure our path is absolute
		filename := util.AbsolutePath(s.Conf().MusicPath, entry.FilePath)

		start := time.Now()
		logger.Info().
			Str("queue_id", entry.QueueID.String()).
			Uint64("trackid", uint64(entry.TrackID)).
			Str("metadata", entry.Metadata).
			Msg("starting decoding")
		pcm, err := audio.DecodeFileGain(s.AudioFormat, filename)
		if err != nil {
			s.queue.Remove(ctx, entry.QueueID)
			continue
		}
		logger.Info().
			Str("queue_id", entry.QueueID.String()).
			Uint64("trackid", uint64(entry.TrackID)).
			Str("metadata", entry.Metadata).
			Dur("elapsed", time.Since(start)).
			Msg("finished decoding")

		mp3, err := audio.NewMP3Buffer(entry.Metadata, nil)
		if err != nil {
			logger.Error().Ctx(ctx).Err(err).Msg("failed to create buffer")
			continue
		}

		start = time.Now()
		logger.Info().
			Str("queue_id", entry.QueueID.String()).
			Uint64("trackid", uint64(entry.TrackID)).
			Str("metadata", entry.Metadata).
			Msg("starting encoding")
		for !s.forced.Load() {
			// check if we have an encoder
			if encoder == nil {
				// otherwise create one
				encoder, err = audio.NewLAME(s.AudioFormat)
				if err != nil {
					// encoder creation failed
					// TODO: fail harder after several tries
					logger.Error().Ctx(ctx).Err(err).Msg("failed to create encoder")
					continue
				}
			}

			n, err := pcm.Read(buf)
			if err != nil && n == 0 {
				break
			}

			mp3buf, err := encoder.Encode(buf[:n])
			if err != nil {
				// error from the encoder, try and flush the buffers and
				// then throw it away
				mp3buf = encoder.Flush()
				_ = encoder.Close()
				encoder = nil
			}

			// we either write what we got from Encode, or whatever was leftover
			// from the Flush call on an error
			_, err = mp3.Write(mp3buf)
			if err != nil {
				logger.Error().Ctx(ctx).Err(err).Msg("failed to write mp3 data")
				break
			}
		}
		pcm.Close()

		// we finished our pcm data, flush the encoder and add it to the end,
		// this uses a nogap flush so it should have no audible gaps between
		// tracks
		_, err = mp3.Write(encoder.Flush())
		if err != nil {
			logger.Error().Ctx(ctx).Err(err).Msg("failed to write mp3 data")
			continue
		}
		logger.Info().
			Str("queue_id", entry.QueueID.String()).
			Uint64("trackid", uint64(entry.TrackID)).
			Str("metadata", entry.Metadata).
			Dur("elapsed", time.Since(start)).
			Dur("length", mp3.TotalLength()).
			Msg("finished encoding")

		// make a reader out of our buffer
		mp3r, err := mp3.Reader()
		if err != nil {
			logger.Error().Ctx(ctx).Err(err).Msg("failed to create reader")
			continue
		}
		// close the write side
		mp3.Close()

		// send the data to the icecast routine
		select {
		case <-s.trackStore.add(StreamTrack{*entry, mp3r}):
		case <-ctx.Done():
			return context.Cause(ctx)
		}
	}

	return nil
}

const preloadLengthTarget = time.Second * 60

var closedChannel = make(chan struct{})

func init() {
	close(closedChannel)
}

type trackstore struct {
	popCh   *util.TypedValue[chan StreamTrack]
	cycleCh *util.TypedValue[chan struct{}]
	cancel  context.CancelFunc

	mu              sync.Mutex
	tracks          []StreamTrack
	preloadedLength time.Duration

	addNotify chan struct{}
}

func NewTracks(ctx context.Context, fds *fdstore.Store, st []StreamTrack) *trackstore {
	ts := newTracks(st)
	ctx, ts.cancel = context.WithCancel(ctx)
	go ts.run(ctx, fds)
	return ts
}

func newTracks(st []StreamTrack) *trackstore {
	ts := &trackstore{
		popCh:   util.NewTypedValue(make(chan StreamTrack)),
		cycleCh: util.NewTypedValue(make(chan struct{})),
		tracks:  st,
	}

	for _, track := range st {
		ts.preloadedLength += track.TotalLength()
	}

	return ts
}

func (ts *trackstore) Stop() {
	ts.cancel()
}

func (ts *trackstore) PopCh() <-chan StreamTrack {
	return ts.popCh.Load()
}

func (ts *trackstore) CyclePopCh() {
	// we can't just directly close our popCh because our run method
	// is possibly trying to write to it, this means it will panic if
	// we just close it underneath it, instead we send a signal through
	// a second channel to the run method to indicate that we want to
	// close the popCh
	ch := make(chan struct{})
	old := ts.cycleCh.Swap(ch)
	close(old)
}

func (ts *trackstore) NotifyCh() <-chan struct{} {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	if ts.addNotify != nil {
		panic("tracks.NotifyCh called while there is an outstanding notify channel")
	}

	return ts.notifyChLocked()
}

// notifyChLocked returns the appropriate notify channel, ts.mu
// should be held when this gets called
func (ts *trackstore) notifyChLocked() <-chan struct{} {
	if ts.preloadedLength < preloadLengthTarget {
		// we didnt hit our preload length target yet, return a ready channel
		// straight away
		return closedChannel
	}

	ts.addNotify = make(chan struct{})
	return ts.addNotify
}

func (ts *trackstore) add(track StreamTrack) <-chan struct{} {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	if ts.addNotify != nil {
		panic("tracks.add called while there is an outstanding notify channel")
	}

	ts.tracks = append(ts.tracks, track)
	// add total song length to our counter
	ts.preloadedLength += track.TotalLength()
	return ts.notifyChLocked()
}

// notify notifies the adder that a track was just consumed by the popper
func (ts *trackstore) notify(track StreamTrack) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	// remove total song length from our counter
	ts.preloadedLength -= track.TotalLength()

	// check if there was a notify channel
	if ts.addNotify == nil {
		// no channel so we can return here
		return
	}

	// otherwise we need to check if we're still past our preload length target
	if ts.preloadedLength >= preloadLengthTarget {
		// we are, so we don't want to notify the adder yet
		return
	}

	// otherwise we do want to notify the adder so close the channel
	close(ts.addNotify)
	ts.addNotify = nil
}

func (ts *trackstore) run(ctx context.Context, fds *fdstore.Store) {
	var track *StreamTrack

	cycleCh := ts.cycleCh.Load()
	cyclePopCh := func() {
		new := make(chan StreamTrack)
		old := ts.popCh.Swap(new)
		close(old)
		cycleCh = ts.cycleCh.Load()
	}

tracksRun:
	for {
		// wait for a track to be ready
		for track == nil {
			// quick path
			if track = ts.pop(); track != nil {
				break
			}

			// otherwise wait a little and check again
			select {
			case <-ctx.Done():
				break tracksRun
			case <-time.After(time.Second):
				track = ts.pop()
			case <-cycleCh:
				cyclePopCh()
			}
		}

		// we now have a track, block on sending it to the PopCh
		select {
		case <-cycleCh:
			cyclePopCh()
		case ts.popCh.Load() <- *track:
			ts.notify(*track)
			track = nil
		case <-ctx.Done():
			break tracksRun
		}
	}

	// TODO: we "own" all the things inside ts.tracks and should be cleaning
	// 		those up when we exit
	// no fdstore, so just exit without storing anything
	if fds == nil {
		return
	}

	// check if we had a track waiting to be send
	if track != nil {
		// if we did it should be the first one entered into storage on
		// a restart
		err := track.StoreSelf(fds)
		if err != nil {
			zerolog.Ctx(ctx).Error().Ctx(ctx).Err(err).Msg("failed to store track")
			return
		}
	}

	ts.mu.Lock()
	defer ts.mu.Unlock()

	// then add all the other tracks
	for _, track := range ts.tracks {
		err := track.StoreSelf(fds)
		if err != nil {
			zerolog.Ctx(ctx).Error().Ctx(ctx).Err(err).Msg("failed to store track")
			return
		}
	}
}

func (ts *trackstore) pop() *StreamTrack {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	if len(ts.tracks) == 0 {
		return nil
	}

	track := ts.tracks[0]
	copy(ts.tracks, ts.tracks[1:])
	ts.tracks = ts.tracks[:len(ts.tracks)-1]
	return &track
}

func (s *Streamer) icecast(ctx context.Context, conn net.Conn, trackCh <-chan StreamTrack) error {
	defer func() {
		// we take ownership of the conn passed in, close it once we exit
		if conn != nil {
			conn.Close()
		}
	}()
	logger := zerolog.Ctx(ctx)

	var track StreamTrack
	var ok bool
	buf := make([]byte, bufferMP3Size)
	// bufferEnd is when the audio data we've read so far is supposed
	// to end if played back in realtime
	bufferEnd := time.Now()
	// bufferSlack is the period of time we subtract from our sleeping
	// period, to make sure we don't sleep too long
	bufferSlack := time.Second * 2

	for !s.forced.Load() {
		select {
		case track, ok = <-trackCh:
			if !ok {
				return nil
			}
		case <-ctx.Done():
			return context.Cause(ctx)
		}

		// remove the entry we're about to play from the queue
		ok, err := s.queue.Remove(ctx, track.QueueID)
		if err != nil {
			logger.Error().Ctx(ctx).Err(err).Msg("failed to remove queue entry")
		}
		if !ok {
			logger.Warn().Msg("failed to remove queue entry")
		}

		// send the entries metadata to icecast
		go s.metadataToIcecast(ctx, track.QueueEntry)

		// lastProgress is the value of the previous loops Progress call
		lastProgress := time.Duration(0)

		for !s.forced.Load() {
			// read some audio data
			n, err := track.Audio.Read(buf)
			if err != nil && n == 0 {
				// if we get an error and no data we exit our loop
				break
			}

			// if our conn doesn't exist go create one
			if conn == nil {
				conn, err = s.newIcecastConn(ctx)
				if err != nil {
					return err
				}
			}

			// reset the end time if it somehow managed to get below
			// the current time
			if bufferEnd.Before(time.Now()) {
				bufferEnd = time.Now()
			}

			// see how far we are in the audio buffer
			curProgress := track.Audio.Progress()
			// add the diff between last and cur to the buffer duration
			bufferEnd = bufferEnd.Add(curProgress - lastProgress)
			lastProgress = curProgress

			// write the actual data
			_, err = conn.Write(buf[:n])
			if err != nil {
				logger.Error().Ctx(ctx).Err(err).Msg("icecast connection broken")
				conn.Close()
				conn = nil
				continue
			}

			time.Sleep(time.Until(bufferEnd) - bufferSlack)
		}

		if s.restart.Load() {
			// if we exited because of a restart we want to store the file
			// we have here, encode the queue entry as our state
			entryData, err := rpc.EncodeQueueEntry(track.QueueEntry)
			if err != nil {
				track.Audio.Close()
				return err
			}

			// and then add the audio file as state leftover
			err = s.fdstorage.AddFile(track.Audio.GetFile(), fdstoreStreamerCurrent, entryData)
			if err != nil {
				track.Audio.Close()
				return err
			}

			// AddFile above will duplicate the fd if it succeeded, so we can
			// close our copy of it here as normal
		}
		_ = track.Audio.Close()
	}

	if s.restart.Load() && conn != nil {
		// if we are exiting because of a restart we want to hold onto the conn
		// to icecast, so store it in the fdstorage before we exit. This will
		// duplicate the fd so we can close ours as normal afterwards
		err := s.fdstorage.AddConn(conn, fdstoreIcecastConn, nil)
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *Streamer) newIcecastConn(ctx context.Context) (net.Conn, error) {
	bo := config.NewConnectionBackoff(ctx)
	var conn net.Conn
	var err error

	uri := s.streamURL()
	err = backoff.RetryNotify(func() error {
		uri = s.streamURL()
		conn, err = icecast.DialURL(ctx, uri,
			icecast.ContentType("audio/mpeg"),
			icecast.UserAgent(s.Conf().UserAgent),
		)
		if err != nil {
			return err
		}

		zerolog.Ctx(ctx).Info().
			Str("endpoint", uri.Redacted()).
			Msg("connected to icecast")
		return nil
	}, bo, func(err error, d time.Duration) {
		zerolog.Ctx(ctx).Error().
			Err(err).
			Dur("backoff", d).
			Str("endpoint", uri.Redacted()).
			Msg("icecast connection failure")
	})
	if err != nil {
		return nil, err
	}

	return conn, nil
}

func (s *Streamer) metadataToIcecast(ctx context.Context, entry radio.QueueEntry) error {
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
		zerolog.Ctx(ctx).Error().Ctx(ctx).Err(err).Dur("backoff", d).Msg("icecast metadata failure")
	})
}

func (s *Streamer) streamURL() *url.URL {
	sCfg := s.Conf().Streamer
	// grab the configured url
	uri := sCfg.StreamURL.URL()
	// replace/add username, password combo if one is configured
	if username := sCfg.StreamUsername; username != "" {
		uri.User = url.UserPassword(username, sCfg.StreamPassword)
	}
	return uri
}

type StreamTrack struct {
	radio.QueueEntry
	Audio audio.Reader
}

func (st *StreamTrack) StoreSelf(fdstorage *fdstore.Store) error {
	data, err := rpc.EncodeQueueEntry(st.QueueEntry)
	if err != nil {
		return err
	}

	err = fdstorage.AddFile(st.Audio.GetFile(), fdstoreEncoder, data)
	if err != nil {
		return err
	}
	return nil
}

func (st *StreamTrack) TotalLength() time.Duration {
	if d := st.Audio.TotalLength(); d > 0 {
		return d
	}

	return st.Length
}
