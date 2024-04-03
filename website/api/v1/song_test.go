package v1

import (
	"context"
	"encoding/base64"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/mocks"
	"github.com/R-a-dio/valkyrie/util/secret"
	"github.com/rs/zerolog"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testAPI struct {
	ctx         context.Context
	storageMock *mocks.StorageServiceMock
	trackMock   *mocks.TrackStorageMock

	GetArg radio.TrackID
	GetRet *radio.Song
	GetErr error
}

func newTestAPI(t *testing.T) (*testAPI, *API) {
	var api testAPI
	var err error

	ctx := context.Background()
	api.ctx = zerolog.New(os.Stdout).WithContext(ctx)

	cfg, err := config.LoadFile()
	require.NoError(t, err)

	songSecret, err := secret.NewSecret(secret.SongLength)
	require.NoError(t, err)

	api.trackMock = &mocks.TrackStorageMock{
		GetFunc: func(trackID radio.TrackID) (*radio.Song, error) {
			if trackID == api.GetArg {
				return api.GetRet, api.GetErr
			}
			return nil, errors.E(errors.SongUnknown)
		},
	}
	api.storageMock = &mocks.StorageServiceMock{
		TrackFunc: func(contextMoqParam context.Context) radio.TrackStorage {
			return api.trackMock
		},
	}

	fs := afero.NewMemMapFs()

	return &api, &API{
		storage:    api.storageMock,
		Config:     cfg,
		songSecret: songSecret,
		fs:         fs,
	}
}

func createSong(t *testing.T, api *API, id radio.TrackID, filename, metadata string) (*radio.Song, string) {
	song := &radio.Song{
		Metadata: metadata,
		DatabaseTrack: &radio.DatabaseTrack{
			TrackID:  id,
			FilePath: filename,
		},
	}
	song.Hydrate()

	data := make([]byte, 128)
	_, err := io.ReadFull(rand.New(rand.NewSource(42)), data)
	require.NoError(t, err)
	sdata := base64.URLEncoding.EncodeToString(data)

	fullPath := filepath.Join(api.Config.Conf().MusicPath, filename)
	require.NoError(t, afero.WriteFile(api.fs, fullPath, []byte(sdata), 0775))
	return song, sdata
}

func TestGetSong(t *testing.T) {
	var data string
	tapi, api := newTestAPI(t)

	tapi.GetArg = 50
	tapi.GetRet, data = createSong(t, api, tapi.GetArg, "random.mp3", "testing - hello world")

	createValues := func(api *API, song *radio.Song) url.Values {
		values := url.Values{}
		values.Set("key", api.songSecret.Get(song.Hash[:]))
		values.Set("id", song.TrackID.String())
		return values
	}
	t.Run("success", func(t *testing.T) {
		assert.HTTPStatusCode(t, api.GetSong, http.MethodGet, "/song",
			createValues(api, tapi.GetRet),
			http.StatusOK)
		assert.HTTPBodyContains(t, api.GetSong, http.MethodGet, "/song",
			createValues(api, tapi.GetRet), data)
	})
	t.Run("missing id", func(t *testing.T) {
		values := createValues(api, tapi.GetRet)
		values.Del("id")
		assert.HTTPStatusCode(t, api.GetSong, http.MethodGet, "/song", values, http.StatusBadRequest)
		assert.HTTPBodyContains(t, api.GetSong, http.MethodGet, "/song", values, "missing")
	})
	t.Run("invalid id", func(t *testing.T) {
		values := createValues(api, tapi.GetRet)
		values.Set("id", "this is not a number")
		assert.HTTPStatusCode(t, api.GetSong, http.MethodGet, "/song", values, http.StatusBadRequest)
		assert.HTTPBodyContains(t, api.GetSong, http.MethodGet, "/song", values, "invalid")
	})
	t.Run("unknown id", func(t *testing.T) {
		values := createValues(api, tapi.GetRet)
		values.Set("id", "100")
		assert.HTTPStatusCode(t, api.GetSong, http.MethodGet, "/song", values, http.StatusNotFound)
		assert.HTTPBodyContains(t, api.GetSong, http.MethodGet, "/song", values, "unknown")
	})
	t.Run("invalid key", func(t *testing.T) {
		values := createValues(api, tapi.GetRet)
		values.Set("key", "randomdata")
		assert.HTTPStatusCode(t, api.GetSong, http.MethodGet, "/song", values, http.StatusUnauthorized)
		assert.HTTPBodyContains(t, api.GetSong, http.MethodGet, "/song", values, "invalid key")
	})
}
