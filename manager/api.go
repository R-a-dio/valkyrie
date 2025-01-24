package manager

import (
	"context"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/rpc"
	"github.com/R-a-dio/valkyrie/util/eventstream"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel"
	"google.golang.org/grpc"
)

// NewGRPCServer sets up a net/http server ready to serve RPC requests
func NewGRPCServer(ctx context.Context, m *Manager, g *GuestService) (*grpc.Server, error) {
	gs := rpc.NewGrpcServer(ctx)
	rpc.RegisterManagerServer(gs, rpc.NewManager(m))
	rpc.RegisterGuestServer(gs, rpc.NewGuest(g))

	return gs, nil
}

func (m *Manager) CurrentUser(ctx context.Context) (eventstream.Stream[*radio.User], error) {
	return m.userStream.SubStream(ctx), nil
}

func (m *Manager) CurrentThread(ctx context.Context) (eventstream.Stream[radio.Thread], error) {
	return m.threadStream.SubStream(ctx), nil
}

func (m *Manager) CurrentSong(ctx context.Context) (eventstream.Stream[*radio.SongUpdate], error) {
	return m.songStream.SubStream(ctx), nil
}

func (m *Manager) CurrentListeners(ctx context.Context) (eventstream.Stream[radio.Listeners], error) {
	return m.listenerStream.SubStream(ctx), nil
}

func (m *Manager) CurrentStatus(ctx context.Context) (eventstream.Stream[radio.Status], error) {
	return m.statusStream.SubStream(ctx), nil
}

// Status returns the current status of the radio
func (m *Manager) Status(ctx context.Context) (*radio.Status, error) {
	m.mu.Lock()
	status := m.status
	m.mu.Unlock()
	return &status, nil
}

// UpdateUser sets information about the current streamer
func (m *Manager) UpdateUser(ctx context.Context, u *radio.User) error {
	const op errors.Op = "manager/Manager.UpdateUser"
	_, span := otel.Tracer("").Start(ctx, string(op))
	defer span.End()

	// update the user from storage here, this is a temporary fix until the
	// new guest system is introduced since this really just works around a
	// small update issue with the users display name
	if u != nil {
		new, err := m.Storage.User(ctx).GetByID(u.ID)
		if err != nil {
			// if this fails just log and ignore
			zerolog.Ctx(ctx).Error().
				Err(err).
				Uint64("user_id", uint64(u.ID)).
				Msg("failed to freshen user from storage")
		} else {
			u = new
		}
	}

	m.userStream.Send(u)
	if u != nil {
		m.logger.Info().Str("username", u.Username).Msg("updating stream user")
	} else {
		m.logger.Info().Str("username", "fallback").Msg("updating stream user")
	}
	return nil
}

// UpdateSong sets information about the currently playing song
func (m *Manager) UpdateSong(ctx context.Context, su *radio.SongUpdate) error {
	const op errors.Op = "manager/Manager.UpdateSong"
	ctx, span := otel.Tracer("").Start(ctx, string(op))
	defer span.End()

	// hydrate the song we got, this will deal with any weird metadata whitespace and
	// fills in the hashes if they don't exist
	su.Song.Hydrate()

	// empty metadata, we ignore
	if su.Song.Metadata == "" {
		m.logger.Info().Msg("skipping empty metadata")
		return nil
	}

	// fill in the rest of the song data
	ss := m.Storage.Song(ctx)
	// songs send to this endpoint only require their metadata to be set, and since
	// we can get a hash from that we use that to lookup more information
	song, err := ss.FromHash(su.Song.Hash)
	if err != nil && !errors.Is(errors.SongUnknown, err) {
		return errors.E(op, err)
	}

	// if we don't have this song in the database create a new entry for it
	if song == nil {
		song, err = ss.Create(su.Song)
		if err != nil {
			return errors.E(op, err)
		}
	}

	var info = su.Info
	// calculate start and end time only if they're zero
	if info.Start.IsZero() {
		// we assume the song just started if it wasn't set
		info.Start = time.Now()
	}
	if info.End.IsZero() {
		// set end to start if we got passed a zero time
		info.End = info.Start
	}

	// check if end is equal to start, either from what we just did above or from
	// the updater having given us both equal
	if info.End.Equal(info.Start) {
		// if we got a song length from either the updater or the storage we add
		// it to the end time as an estimated end
		if su.Song.Length > 0 { // updater song
			info.End = info.End.Add(su.Song.Length)
		} else if song.Length > 0 { // storage song
			info.End = info.End.Add(song.Length)
		}
	}

	// now we've filled in any missing information on both 'song' and 'info' so we
	// can now check if we are even interested in this thing
	if song.EqualTo(m.songStream.Latest().Song) {
		// same song as latest, so skip the update
		return nil
	}

	m.logger.Info().Str("metadata", song.Metadata).Dur("song_length", song.Length).Msg("updating stream song")
	m.songStream.Send(&radio.SongUpdate{Song: *song, Info: info})
	return nil
}

// UpdateThread sets the current thread information on the front page and chats
func (m *Manager) UpdateThread(ctx context.Context, thread radio.Thread) error {
	const op errors.Op = "manager/Manager.UpdateThread"
	_, span := otel.Tracer("").Start(ctx, string(op))
	defer span.End()

	m.threadStream.Send(thread)
	return nil
}

// UpdateListeners sets the listener count
func (m *Manager) UpdateListeners(ctx context.Context, listeners radio.Listeners) error {
	const op errors.Op = "manager/Manager.UpdateListeners"
	_, span := otel.Tracer("").Start(ctx, string(op))
	defer span.End()

	m.listenerStream.Send(listeners)
	return nil
}

func (m *Manager) UpdateFromStorage(ctx context.Context) error {
	const op errors.Op = "manager/Manager.UpdateFromStorage"
	_, span := otel.Tracer("").Start(ctx, string(op))
	defer span.End()

	{ // user update
		current := m.userStream.Latest()

		new, err := m.Storage.User(ctx).GetByID(current.ID)
		if err != nil {
			zerolog.Ctx(ctx).Error().
				Err(err).
				Uint64("user_id", uint64(current.ID)).
				Msg("failed to update user from storage")
		} else {
			m.userStream.CompareAndSend(new, func(new, old *radio.User) bool {
				if old == nil {
					return new == nil
				}

				return new.ID == old.ID
			})
		}
	}

	{ // song update
		current := m.songStream.Latest()

		new, err := m.Storage.Song(ctx).FromHash(current.Hash)
		if err != nil {
			zerolog.Ctx(ctx).Error().
				Err(err).
				Str("hash", current.Hash.String()).
				Msg("failed to update song from storage")
		} else {
			m.songStream.CompareAndSend(&radio.SongUpdate{
				Song: *new,
				Info: current.Info,
			}, func(new, old *radio.SongUpdate) bool {
				if old == nil {
					return new == nil
				}

				return new.EqualTo(old.Song)
			})
		}
	}

	return nil
}

// statusFromStreams constructs a radio.Status from the individual data streams using
// their latest value
func (m *Manager) statusFromStreams() radio.Status {
	var status radio.Status

	status.Thread = m.threadStream.Latest()
	status.Listeners = m.listenerStream.Latest()
	status.StreamUser = m.userStream.Latest()
	if u := status.StreamUser; u != nil {
		status.User = *u
		status.StreamerName = u.DJ.Name
	}
	if su := m.songStream.Latest(); su != nil {
		status.Song = su.Song
		status.SongInfo = su.Info
	}

	return status
}

// runStatusUpdates is in charge of keeping m.status up-to-date from the other
// data streams.
func (m *Manager) runStatusUpdates(ctx context.Context, ready chan struct{}) {
	m.running.Store(true)

	userCh := m.userStream.Sub()
	defer m.userStream.Leave(userCh)
	<-userCh // eat initial value

	threadCh := m.threadStream.Sub()
	defer m.threadStream.Leave(threadCh)
	<-threadCh // eat initial value

	songCh := m.songStream.Sub()
	defer m.songStream.Leave(songCh)
	<-songCh // eat initial value

	listenerCh := m.listenerStream.Sub()
	defer m.listenerStream.Leave(listenerCh)
	// store initial value, for if we get a song update before a listener update
	listenerCount := <-listenerCh
	var songStartListenerCount radio.Listeners

	// communicate that we are ready to handle events
	close(ready)

	for m.running.Load() {
		var sendStatus = true

		select {
		case <-ctx.Done():
			return
		case user := <-userCh:
			m.mu.Lock()
			m.status.StreamUser = user
			if user == nil {
				// skip nil users for the User and StreamerName fields
				break
			}
			zerolog.Ctx(ctx).Info().Any("user", user).Msg("running status update")
			m.status.StreamerName = user.DJ.Name
			m.status.User = *user
		case thread := <-threadCh:
			zerolog.Ctx(ctx).Info().Str("thread", thread).Msg("running status update")
			m.mu.Lock()
			m.status.Thread = thread
		case su := <-songCh:
			zerolog.Ctx(ctx).Info().Any("song", su).Msg("running status update")
			m.mu.Lock()

			if su == nil || !m.status.Song.EqualTo(su.Song) {
				err := m.finishSong(ctx, m.status, songStartListenerCount)
				if err != nil {
					zerolog.Ctx(ctx).Error().Err(err).Msg("failed finishSong")
				}
			}

			if su != nil {
				// if we are doing a song update we want to record how many
				// listeners we have at the start of it
				if !m.status.Song.EqualTo(su.Song) {
					songStartListenerCount = listenerCount
				}
				m.status.Song = su.Song
				m.status.SongInfo = su.Info
			}
		case listenerCount = <-listenerCh:
			zerolog.Ctx(ctx).Info().Int64("listeners", listenerCount).Msg("running status update")
			m.mu.Lock()
			m.status.Listeners = listenerCount
			sendStatus = false // don't send status updates for listener count updates
		}

		if sendStatus {
			// send a copy to the status stream
			m.statusStream.Send(m.status)
		}
		// and a copy to the persistent storage
		m.updateStreamStatus(m.status)
		m.mu.Unlock()
	}
}

func (m *Manager) finishSong(ctx context.Context, status radio.Status, startListenerCount radio.Listeners) error {
	const op errors.Op = "manager/Manager.finishSong"

	// calculate the listener difference
	diff := startListenerCount - status.Listeners
	var ldiff = &diff
	// check if we want to skip the listener diff; this is mostly here
	// to avoid fallback jumps to register as 0s
	if status.Listeners < 10 || status.Listeners+diff < 10 {
		ldiff = nil
	}

	// start a transaction for if any of the storage calls fail
	ss, tx, err := m.Storage.SongTx(ctx, nil)
	if err != nil {
		return errors.E(op, err)
	}
	defer tx.Rollback()

	// add a play to the song
	err = ss.AddPlay(status.Song, status.User, ldiff)
	if err != nil {
		return errors.E(op, err)
	}

	// update the song length if it didn't have one yet
	if status.Song.Length == 0 {
		err = ss.UpdateLength(status.Song, time.Since(status.SongInfo.Start))
		if err != nil {
			return errors.E(op, err, status.Song)
		}
	}

	// if we have the song in the database, also update that
	if status.Song.HasTrack() {
		ts, _, err := m.Storage.TrackTx(ctx, tx)
		if err != nil {
			return errors.E(op, err)
		}

		err = ts.UpdateLastPlayed(status.Song.TrackID)
		if err != nil {
			return errors.E(op, err, status.Song)
		}
	}

	// commit the transaction
	if err = tx.Commit(); err != nil {
		return errors.E(op, err)
	}

	return nil
}
