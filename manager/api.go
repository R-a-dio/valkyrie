package manager

import (
	"context"
	"strings"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/rpc"
	"github.com/R-a-dio/valkyrie/util/eventstream"
	"go.opentelemetry.io/otel"
	"google.golang.org/grpc"
)

// NewGRPCServer sets up a net/http server ready to serve RPC requests
func NewGRPCServer(ctx context.Context, m *Manager) (*grpc.Server, error) {
	gs := rpc.NewGrpcServer(ctx)
	rpc.RegisterManagerServer(gs, rpc.NewManager(m))

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
	ctx, span := otel.Tracer("").Start(ctx, string(op))
	defer span.End()

	m.userStream.Send(u)

	// only update status if we get a non-nil user, this leaves the status
	// data with the last known DJ so that we can display something
	if u != nil {
		m.mu.Lock()
		m.status.StreamerName = u.DJ.Name
		m.status.User = *u
		go m.updateStreamStatus(true, m.status)
		m.mu.Unlock()
	}

	if u != nil {
		m.logger.Info().Str("username", u.Username).Msg("updating stream user")
	} else {
		m.logger.Info().Str("username", "Fallback").Msg("updating stream user")
	}
	return nil
}

// UpdateSong sets information about the currently playing song
func (m *Manager) UpdateSong(ctx context.Context, update *radio.SongUpdate) error {
	const op errors.Op = "manager/Manager.UpdateSong"
	ctx, span := otel.Tracer("").Start(ctx, string(op))
	defer span.End()

	new := update
	info := update.Info

	// trim any whitespace on the edges
	new.Metadata = strings.TrimSpace(new.Metadata)

	// empty metadata, we ignore
	if new.Metadata == "" {
		m.logger.Info().Msg("skipping empty metadata")
		return nil
	}
	// first we check if this is the same song as the previous one we received to
	// avoid double announcement or drifting start/end timings
	m.mu.Lock()
	if m.status.Song.Metadata == new.Metadata {
		m.mu.Unlock()
		return nil
	}

	// otherwise it's a legit song change
	ss, tx, err := m.Storage.SongTx(ctx, nil)
	if err != nil {
		return errors.E(op, err)
	}
	defer tx.Rollback()

	// we assume that the song we received has very little or no data except for the
	// Metadata field. So we try and find more info from that
	song, err := ss.FromMetadata(new.Metadata)
	if err != nil && !errors.Is(errors.SongUnknown, err) {
		return errors.E(op, err)
	}

	// if we don't have this song in the database create a new entry for it
	if song == nil {
		song, err = ss.Create(radio.NewSong(new.Metadata))
		if err != nil {
			return errors.E(op, err)
		}
	}

	// calculate start and end time only if they're zero
	if info.Start.IsZero() {
		// we assume the song just started if it wasn't set
		info.Start = time.Now()
	}
	if info.End.IsZero() {
		// set end to start if we got passed a zero time
		info.End = info.Start
	}
	if song.Length > 0 && info.End.Equal(info.Start) {
		// add the song length if we have one
		info.End = info.End.Add(song.Length)
	}

	// store copies of the information we need later
	prevStatus := m.status
	songListenerDiff := m.songStartListenerCount

	// now update the fields we should update
	m.status.Song = *song
	m.status.SongInfo = info
	m.songStartListenerCount = m.status.Listeners
	go m.updateStreamStatus(true, m.status)

	// calculate the listener diff between start of song and end of song
	songListenerDiff -= m.status.Listeners

	m.logger.Info().Str("metadata", song.Metadata).Dur("song_length", song.Length).Msg("updating stream song")
	m.mu.Unlock()

	// finish updating extra fields for the previous status
	err = m.finishSongUpdate(ctx, tx, prevStatus, &songListenerDiff)
	if err != nil {
		return errors.E(op, err)
	}

	if err = tx.Commit(); err != nil {
		return errors.E(op, errors.TransactionCommit, err, prevStatus)
	}

	// send an event out
	m.songStream.Send(&radio.SongUpdate{Song: *song, Info: info})
	return nil
}

func (m *Manager) finishSongUpdate(ctx context.Context, tx radio.StorageTx, status radio.Status, ldiff *radio.Listeners) error {
	const op errors.Op = "manager/Manager.finishSongUpdate"

	if status.Song.ID == 0 {
		// no song to update
		return nil
	}
	if tx == nil {
		// no transaction was passed to us, we require one
		panic("no tx given to finishSongUpdate")
	}

	// check if we want to skip inserting a listener diff; this is mostly here
	// to avoid fallback jumps to register as 0s
	if ldiff != nil && (status.Listeners < 10 || status.Listeners+*ldiff < 10) {
		ldiff = nil
	}

	ss, _, err := m.Storage.SongTx(ctx, tx)
	if err != nil {
		return errors.E(op, err)
	}

	// insert an entry that this song was played
	err = ss.AddPlay(status.Song, status.User, ldiff)
	if err != nil {
		return errors.E(op, err)
	}

	// if we have the song in the database, also update that
	if status.Song.HasTrack() {
		ts, _, err := m.Storage.TrackTx(ctx, tx)
		if err != nil {
			return errors.E(op, err)
		}

		err = ts.UpdateLastPlayed(status.Song.TrackID)
		if err != nil {
			return errors.E(op, err, status)
		}
	}

	// and the song length if it was still unknown
	if status.Song.Length == 0 {
		err = ss.UpdateLength(status.Song, time.Since(status.SongInfo.Start))
		if err != nil {
			return errors.E(op, err, status)
		}
	}

	return nil
}

// UpdateThread sets the current thread information on the front page and chats
func (m *Manager) UpdateThread(ctx context.Context, thread radio.Thread) error {
	const op errors.Op = "manager/Manager.UpdateThread"
	ctx, span := otel.Tracer("").Start(ctx, string(op))
	defer span.End()

	m.threadStream.Send(thread)

	m.mu.Lock()
	m.status.Thread = thread
	go m.updateStreamStatus(true, m.status)
	m.mu.Unlock()
	return nil
}

// UpdateListeners sets the listener count
func (m *Manager) UpdateListeners(ctx context.Context, listeners radio.Listeners) error {
	const op errors.Op = "manager/Manager.UpdateListeners"
	ctx, span := otel.Tracer("").Start(ctx, string(op))
	defer span.End()

	m.listenerStream.Send(listeners)

	m.mu.Lock()
	m.status.Listeners = listeners
	go m.updateStreamStatus(false, m.status)
	m.mu.Unlock()
	return nil
}
