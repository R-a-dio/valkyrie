package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/mocks"
	"github.com/R-a-dio/valkyrie/util"
	"github.com/R-a-dio/valkyrie/website/middleware"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type newSongsFormCase struct {
	old radio.DatabaseTrack
	new radio.DatabaseTrack
}

func TestNewSongsForm(t *testing.T) {
	ctx := context.Background()

	original := radio.DatabaseTrack{
		TrackID:    500,
		Artist:     "You",
		Album:      "Welcome to testing",
		Title:      "Hello World",
		Tags:       "me testing",
		Acceptor:   "should not be updated",
		LastEditor: "should be updated",
	}
	user := radio.User{
		Username: "test",
	}

	cases := []newSongsFormCase{
		{
			old: original,
			new: radio.DatabaseTrack{
				TrackID:    original.TrackID,
				Artist:     "Someone else",
				Album:      original.Album,
				Title:      "Goodbye World",
				Tags:       original.Tags,
				Acceptor:   original.Acceptor,
				LastEditor: user.Username,
			},
		},
		{
			old: original,
			new: radio.DatabaseTrack{
				TrackID:    original.TrackID,
				Artist:     original.Artist,
				Album:      "Goodbye testing",
				Title:      original.Title,
				Tags:       "you testing me",
				Acceptor:   original.Acceptor,
				LastEditor: user.Username,
			},
		},
	}

	var c newSongsFormCase

	storage := &mocks.StorageServiceMock{}
	storage.TrackFunc = func(contextMoqParam context.Context) radio.TrackStorage {
		return &mocks.TrackStorageMock{
			GetFunc: func(trackID radio.TrackID) (*radio.Song, error) {
				if trackID == c.old.TrackID {
					return &radio.Song{DatabaseTrack: &c.old}, nil
				}

				return nil, errors.E(errors.SongUnknown)
			},
		}
	}

	ts := storage.Track(ctx)
	for i := 0; i < len(cases); i++ {
		c = cases[i]

		// create the form values
		values := url.Values{}
		values.Set("id", strconv.FormatUint(uint64(c.new.TrackID), 10))
		values.Set("artist", c.new.Artist)
		values.Set("album", c.new.Album)
		values.Set("title", c.new.Title)
		values.Set("tags", c.new.Tags)

		r := httptest.NewRequest(http.MethodPost, "/admin/songs", nil)
		r.Form = values

		form, err := NewSongsForm(ts, user, r)
		if !assert.NoError(t, err) {
			continue
		}
		assert.NotNil(t, form, "form should not be nil if there was no error")
		assert.Equal(t, c.new, *form.Song.DatabaseTrack)
	}
}

func TestPostSongs(t *testing.T) {
	//ctx := context.Background()
	cfg := config.TestConfig()

	filename := util.AbsolutePath(cfg.Conf().MusicPath, "testfile.mp3")
	var testSong = &radio.Song{DatabaseTrack: &radio.DatabaseTrack{
		TrackID:    500,
		Artist:     "You",
		Album:      "Welcome to testing",
		Title:      "Hello World",
		Tags:       "me testing",
		Acceptor:   "should not be updated",
		LastEditor: "should be updated",
		FilePath:   filename,
	}}
	var testValues, _ = url.ParseQuery("action=save&id=500&artist=You&title=Hello World&tags=me testing&album=Welcome to testing")
	var getArg radio.TrackID
	var getRet *radio.Song
	var getErr error
	var deleteArg radio.TrackID

	storage := &mocks.StorageServiceMock{}
	storage.TrackFunc = func(contextMoqParam context.Context) radio.TrackStorage {
		return &mocks.TrackStorageMock{
			GetFunc: func(trackID radio.TrackID) (*radio.Song, error) {
				if getArg == trackID {
					return getRet, getErr
				}
				return nil, errors.E(errors.SongUnknown)
			},
			DeleteFunc: func(trackID radio.TrackID) error {
				if deleteArg == trackID {
					return nil
				}
				return errors.E(errors.SongUnknown)
			},
			UpdateMetadataFunc: func(song radio.Song) error {
				return nil
			},
		}
	}

	// setup fake filesystem
	fs := afero.NewMemMapFs()
	path := cfg.Conf().MusicPath
	require.NoError(t, fs.MkdirAll(path, 0775))
	// create file that we expect to be kept until the end
	require.NoError(t, afero.WriteFile(fs, filename, []byte("hello world"), 0775))

	// setup state
	state := State{
		Storage: storage,
		Config:  cfg,
		FS:      fs,
	}

	// prepReq prepares a http.Request for use in the tests
	prepReq := func(user radio.User, values url.Values) *http.Request {
		body := strings.NewReader(values.Encode())
		req := httptest.NewRequest(http.MethodPost, "/admin/songs", body)
		req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
		return middleware.RequestWithUser(req, &user)
	}

	// checkExist is a helper function that lets us test if a filepath exists or not
	// based on its arguments
	checkExist := func(t *testing.T, shouldExist bool, fp string) bool {
		fi, err := state.FS.Stat(fp)

		if shouldExist {
			// file should exist, so the stat should have no error
			// and a non-nil fileinfo
			return assert.NoError(t, err) &&
				assert.NotNil(t, fi)
		}

		if assert.Error(t, err) {
			return assert.Nil(t, fi) &&
				assert.True(t, errors.IsE(err, os.ErrNotExist))
		}

		return false
	}

	// check if the file we created above is still there at the end, none of the tests
	// should be deleting it
	defer checkExist(t, true, filename)

	t.Run("happy path", func(t *testing.T) {
		getArg = testSong.TrackID
		getRet = testSong

		user := radio.User{
			Username: "test",
		}

		req := prepReq(user, testValues)

		form, err := state.postSongs(req)
		if assert.NoError(t, err) {
			if assert.NotNil(t, form) {
				assert.Equal(t, *testSong, form.Song)
				checkExist(t, true, form.Song.FilePath)
			}
		}
	})

	t.Run("delete path with absolute", func(t *testing.T) {
		getArg = testSong.TrackID
		c := *testSong
		dt := *c.DatabaseTrack
		c.DatabaseTrack = &dt
		c.FilePath = util.AbsolutePath(cfg.Conf().MusicPath, "todeleteabsolute.mp3")
		getRet = &c
		deleteArg = testSong.TrackID

		user := radio.User{
			Username: "test",
			UserPermissions: radio.UserPermissions{
				radio.PermActive:         struct{}{},
				radio.PermDatabaseEdit:   struct{}{},
				radio.PermDatabaseDelete: struct{}{},
			},
		}

		// create file that we expect to be deleted
		require.NoError(t, afero.WriteFile(state.FS, c.FilePath, []byte("hello world"), 0775))

		values, _ := url.ParseQuery(testValues.Encode())
		values.Set("action", "delete")

		req := prepReq(user, values)

		form, err := state.postSongs(req)
		if assert.NoError(t, err) {
			assert.Nil(t, form, "delete action should return nothing")
			checkExist(t, false, c.FilePath)
		}
	})

	t.Run("delete path with relative path", func(t *testing.T) {
		getArg = testSong.TrackID
		c := *testSong
		dt := *c.DatabaseTrack
		c.DatabaseTrack = &dt
		fullPath := util.AbsolutePath(cfg.Conf().MusicPath, "todeleterelative.mp3")
		c.FilePath = "todeleterelative.mp3"
		getRet = &c
		deleteArg = testSong.TrackID

		user := radio.User{
			Username: "test",
			UserPermissions: radio.UserPermissions{
				radio.PermActive:         struct{}{},
				radio.PermDatabaseEdit:   struct{}{},
				radio.PermDatabaseDelete: struct{}{},
			},
		}

		// create file that we expect to be deleted
		require.NoError(t, afero.WriteFile(state.FS, fullPath, []byte("hello world"), 0775))

		values, _ := url.ParseQuery(testValues.Encode())
		values.Set("action", "delete")

		req := prepReq(user, values)

		form, err := state.postSongs(req)
		if assert.NoError(t, err) {
			assert.Nil(t, form, "delete action should return nothing")
			checkExist(t, false, fullPath)
		}
	})

	t.Run("delete path but no access", func(t *testing.T) {
		getArg = testSong.TrackID
		getRet = testSong
		deleteArg = testSong.TrackID

		user := radio.User{
			Username: "test",
			UserPermissions: radio.UserPermissions{
				radio.PermActive:       struct{}{},
				radio.PermDatabaseEdit: struct{}{},
			},
		}

		values, _ := url.ParseQuery(testValues.Encode())
		values.Set("action", "delete")

		req := prepReq(user, values)

		form, err := state.postSongs(req)
		if assert.Error(t, err) {
			assert.True(t, errors.Is(errors.AccessDenied, err), "error should be AccessDenied")
		}
		assert.NotNil(t, form, "failed delete should give us the submitted form back")
		checkExist(t, true, form.Song.FilePath)
	})

	t.Run("no user request", func(t *testing.T) {
		getArg = testSong.TrackID
		getRet = testSong

		user := radio.User{
			Username: "test",
			UserPermissions: radio.UserPermissions{
				radio.PermActive:         struct{}{},
				radio.PermDatabaseEdit:   struct{}{},
				radio.PermDatabaseDelete: struct{}{},
			},
		}

		req := prepReq(user, testValues)
		// remove the context that carries the user
		req = req.WithContext(context.Background())

		form, err := state.postSongs(req)
		if assert.Error(t, err) {
			assert.True(t, errors.Is(errors.AccessDenied, err), "error should be AccessDenied")
		}
		assert.Nil(t, form, "no user request should give back nothing")
		checkExist(t, true, filename)
	})

	t.Run("mark replacement", func(t *testing.T) {
		getArg = testSong.TrackID
		getRet = testSong

		user := radio.User{
			Username: "test",
			UserPermissions: radio.UserPermissions{
				radio.PermActive:         struct{}{},
				radio.PermDatabaseEdit:   struct{}{},
				radio.PermDatabaseDelete: struct{}{},
			},
		}

		values, _ := url.ParseQuery(testValues.Encode())
		values.Set("action", "mark-replacement")

		req := prepReq(user, values)

		form, err := state.postSongs(req)
		if assert.NoError(t, err) {
			require.NotNil(t, form)
			assert.True(t, form.Song.NeedReplacement)
		}
	})

	t.Run("unmark replacement", func(t *testing.T) {
		getArg = testSong.TrackID

		c := *testSong
		dt := *c.DatabaseTrack
		c.DatabaseTrack = &dt
		c.NeedReplacement = true
		getRet = &c

		user := radio.User{
			Username: "test",
			UserPermissions: radio.UserPermissions{
				radio.PermActive:         struct{}{},
				radio.PermDatabaseEdit:   struct{}{},
				radio.PermDatabaseDelete: struct{}{},
			},
		}

		values, _ := url.ParseQuery(testValues.Encode())
		values.Set("action", "unmark-replacement")

		req := prepReq(user, values)

		form, err := state.postSongs(req)
		if assert.NoError(t, err) {
			require.NotNil(t, form)
			assert.False(t, form.Song.NeedReplacement)
		}
	})
}
