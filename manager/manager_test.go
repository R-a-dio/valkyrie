package manager

import (
	"context"
	"sync"
	"testing"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/mocks"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManager(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = zerolog.New(zerolog.NewTestWriter(t)).WithContext(ctx)

	songs := make(map[radio.SongHash]radio.Song)
	var songsMu sync.Mutex

	initSong := radio.NewSong("#1 initial value")
	initUser := radio.User{
		ID:       50,
		Username: "initial",
		DJ: radio.DJ{
			ID:   5,
			Name: "initial",
		},
	}
	initThread := ""

	var updateUser *radio.User

	us := &mocks.UserStorageMock{
		GetByDJIDFunc: func(dJID radio.DJID) (*radio.User, error) {
			return &initUser, nil
		},
		GetByIDFunc: func(userID radio.UserID) (*radio.User, error) {
			return updateUser, nil
		},
	}
	sts := &mocks.StatusStorageMock{
		LoadFunc: func() (*radio.Status, error) {
			return &radio.Status{
				StreamUser: &initUser,
				Song:       initSong,
				User:       initUser,
				Thread:     initThread,
			}, nil
		},
		StoreFunc: func(status radio.Status) error {
			return nil
		},
	}
	sos := &mocks.SongStorageMock{
		FromMetadataFunc: func(metadata string) (*radio.Song, error) {
			return &initSong, nil
		},
		FromHashFunc: func(songHash radio.SongHash) (*radio.Song, error) {
			songsMu.Lock()
			song, ok := songs[songHash]
			songsMu.Unlock()
			if !ok {
				return nil, errors.E(errors.SongUnknown)
			}
			return &song, nil
		},
		CreateFunc: func(song radio.Song) (*radio.Song, error) {
			return &song, nil
		},
		AddPlayFunc: func(song radio.Song, streamer radio.User, ldiff *int64) error {
			return nil
		},
		UpdateLengthFunc: func(song radio.Song, duration time.Duration) error {
			return nil
		},
	}

	storage := &mocks.StorageServiceMock{
		StatusFunc: func(contextMoqParam context.Context) radio.StatusStorage {
			return sts
		},
		SongFunc: func(contextMoqParam context.Context) radio.SongStorage {
			return sos
		},
		SongTxFunc: func(contextMoqParam context.Context, storageTx radio.StorageTx) (radio.SongStorage, radio.StorageTx, error) {
			return sos, mocks.CommitTx(t), nil
		},
		TrackTxFunc: func(contextMoqParam context.Context, storageTx radio.StorageTx) (radio.TrackStorage, radio.StorageTx, error) {
			return &mocks.TrackStorageMock{
				UpdateLastPlayedFunc: func(trackID radio.TrackID) error {
					return nil
				},
			}, mocks.CommitTx(t), nil
		},
		UserFunc: func(contextMoqParam context.Context) radio.UserStorage {
			return us
		},
	}
	prober := func(ctx context.Context, song radio.Song) (time.Duration, error) {
		return 0, errors.New("not implemented")
	}

	m, err := NewManager(ctx, storage, prober, nil)
	require.NoError(t, err)
	require.NotNil(t, m)
	// the status should now be our initial song and user
	require.Equal(t, initSong.Metadata, m.status.Song.Metadata)
	require.Equal(t, initUser.ID, m.status.User.ID)
	require.Equal(t, initUser.DJ.Name, m.status.User.DJ.Name)

	statusCh := m.statusStream.Sub()
	<-statusCh // eat the initial

	status := func() radio.Status {
		m.mu.Lock()
		defer m.mu.Unlock()
		return m.status
	}

	newsong := func(meta string, length ...time.Duration) radio.Song {
		song := radio.NewSong(meta, length...)
		songs[song.Hash] = song
		return song
	}

	t.Run("UpdateThread", func(t *testing.T) {
		threadCh := m.threadStream.Sub()
		defer m.threadStream.Leave(threadCh)
		require.Equal(t, initThread, <-threadCh)

		thread := "http://example.org"

		err := m.UpdateThread(ctx, thread)
		require.NoError(t, err)
		// should show up in the thread stream
		require.Equal(t, thread, <-threadCh)
		// and should show up in the status afterwards
		require.Equal(t, thread, (<-statusCh).Thread)
	})

	t.Run("UpdateListeners", func(t *testing.T) {
		listCh := m.listenerStream.Sub()
		defer m.listenerStream.Leave(listCh)
		require.EqualValues(t, 0, <-listCh)

		list := int64(50)

		err := m.UpdateListeners(ctx, list)
		require.NoError(t, err)
		// should show up in the listener stream
		require.EqualValues(t, list, <-listCh)
		// and should show up in the status afterwards
		require.Eventually(t, func() bool {
			return assert.ObjectsAreEqualValues(list, status().Listeners)
		}, time.Second, time.Millisecond*50)
		// should have no status updates, so don't check
	})

	t.Run("UpdateUser", func(t *testing.T) {
		userCh := m.userStream.Sub()
		defer m.userStream.Leave(userCh)
		require.EqualValues(t, &initUser, <-userCh)

		// set it to nil first, see if it updates
		err := m.UpdateUser(ctx, nil)
		require.NoError(t, err)
		// nil should show up in the user stream
		require.Nil(t, <-userCh)
		// and in a status update
		status := <-statusCh
		// actual nil for StreamUser
		require.Nil(t, status.StreamUser)
		// the old user for User
		require.EqualValues(t, initUser, status.User)

		// now set an actual value
		user := &radio.User{
			ID:       100,
			Username: "testing",
			DJ: radio.DJ{
				ID:   100,
				Name: "The Best",
			},
		}

		updateUser = user
		err = m.UpdateUser(ctx, user)
		require.NoError(t, err)
		// should show up in the user stream
		require.EqualValues(t, user, <-userCh)
		// and in the status update
		require.EqualValues(t, *user, (<-statusCh).User)
	})

	compareSongUpdate := func(t *testing.T, expected, actual *radio.SongUpdate) bool {
		if !assert.EqualExportedValues(t, expected, actual) {
			return false
		}

		// check if our input had a zero time
		if expected.Info.Start.IsZero() {
			// if zero we expect it to have been set to Now for actual
			if !assert.WithinDuration(t, time.Now(), actual.Info.Start, time.Second*5) {
				return false
			}
		}
		// check if our input had a zero time
		if expected.Info.End.IsZero() {
			// if zero we expect it to have been set to the same as Start
			expectedTime := actual.Info.Start
			// but if the Song has a Length set it should be added to the expected
			// end time
			if expected.Song.Length > 0 {
				expectedTime = expectedTime.Add(expected.Song.Length)
			} else if actual.Song.Length > 0 {
				expectedTime = expectedTime.Add(actual.Song.Length)
			}
			// then do the comparison check
			if !assert.WithinDuration(t, expectedTime, actual.Info.End, time.Second*5) {
				return false
			}
		}

		return true
	}

	compareStatusUpdate := func(t *testing.T, expected *radio.SongUpdate, actual radio.Status) bool {
		return compareSongUpdate(t, expected, &radio.SongUpdate{
			Song: actual.Song,
			Info: actual.SongInfo,
		})
	}

	t.Run("UpdateSong", func(t *testing.T) {
		suCh := m.songStream.Sub()
		defer m.songStream.Leave(suCh)
		require.EqualValues(t, initSong, (<-suCh).Song)

		su := &radio.SongUpdate{
			Song: newsong("#2 yes - no", time.Second*126),
			Info: radio.SongInfo{},
		}

		err := m.UpdateSong(ctx, su)
		require.NoError(t, err)
		compareSongUpdate(t, su, <-suCh)
		compareStatusUpdate(t, su, <-statusCh)

		// now do a duplicate update of the same song
		err = m.UpdateSong(ctx, su)
		require.NoError(t, err)
		// this should not give us an update on the songStream
		// testing for absence is hard so just put this here and continue
		// if it did send an update the next test should get this phantom value
		// instead of what it expects

		su = &radio.SongUpdate{
			Song: radio.NewSong("#3 does not exist", time.Second*187),
			Info: radio.SongInfo{},
		}

		err = m.UpdateSong(ctx, su)
		require.NoError(t, err)
		compareSongUpdate(t, su, <-suCh)
		compareStatusUpdate(t, su, <-statusCh)
	})

	t.Run("UpdateSongWithTrack", func(t *testing.T) {
		suCh := m.songStream.Sub()
		defer m.songStream.Leave(suCh)
		<-suCh // eat the initial

		song := newsong("#4 me - a testing song", time.Second*322)
		song.DatabaseTrack = &radio.DatabaseTrack{
			TrackID: 50,
			Artist:  "#4 me",
			Title:   "a testing song",
			Album:   "that's a test",
		}
		songsMu.Lock()
		songs[song.Hash] = song // store updated song in map
		songsMu.Unlock()

		su := &radio.SongUpdate{
			Song: song,
			Info: radio.SongInfo{},
		}

		err = m.UpdateSong(ctx, su)
		require.NoError(t, err)
		compareSongUpdate(t, su, <-suCh)
		compareStatusUpdate(t, su, <-statusCh)

		// check duplicate update again
		err = m.UpdateSong(ctx, su)
		require.NoError(t, err)

		// now update with any other song
		su = &radio.SongUpdate{
			Song: newsong("#5 not a database song", time.Second*50),
		}

		err = m.UpdateSong(ctx, su)
		require.NoError(t, err)
		compareSongUpdate(t, su, <-suCh)
		compareStatusUpdate(t, su, <-statusCh)
	})

	t.Run("Status", func(t *testing.T) {
		s, err := m.Status(ctx)
		require.NoError(t, err)
		require.NotNil(t, s)

		require.EqualExportedValues(t, status(), *s)
	})

	t.Run("statusFromStreams", func(t *testing.T) {
		require.EqualExportedValues(t, status(), m.statusFromStreams())
	})
}
