package public

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/mocks"
	"github.com/R-a-dio/valkyrie/templates"
	"github.com/stretchr/testify/require"
)

func TestPostSubmit(t *testing.T) {
	ctx := context.Background()
	cfg := config.TestConfig()

	c := cfg.Conf()
	c.MusicPath = t.TempDir()
	cfg.StoreConf(c)
	require.NoError(t, os.MkdirAll(filepath.Join(c.MusicPath, "pending"), 0755))
	_ = ctx

	storage := &mocks.StorageServiceMock{}
	storage.SubmissionsFunc = func(contextMoqParam context.Context) radio.SubmissionStorage {
		return &mocks.SubmissionStorageMock{
			SubmissionStatsFunc: func(identifier string) (radio.SubmissionStats, error) {
				return radio.SubmissionStats{}, nil
			},
			LastSubmissionTimeFunc: func(identifier string) (time.Time, error) {
				return time.Now().Add(-time.Hour * 24), nil
			},
			InsertSubmissionFunc: func(pendingSong radio.PendingSong) error {
				return nil
			},
		}
	}
	storage.TrackFunc = func(contextMoqParam context.Context) radio.TrackStorage {
		return &mocks.TrackStorageMock{}
	}
	executor := &mocks.ExecutorMock{}
	executor.ExecuteFunc = func(w io.Writer, r *http.Request, input templates.TemplateSelectable) error {
		return nil
	}

	state := State{
		Config:    cfg,
		Storage:   storage,
		Templates: executor,
	}

	// make a multipart post body
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("track", "we're testing a file upload here.flac")
	require.NoError(t, err)

	var file io.Reader = strings.NewReader("Hello world and some other garbage")
	_, err = io.Copy(part, file)
	require.NoError(t, err)
	require.NoError(t, writer.WriteField("daypass", ""))
	require.NoError(t, writer.WriteField("comment", "hello"))
	require.NoError(t, writer.Close())

	// setup request

	req := httptest.NewRequest(http.MethodPost, "/submit", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()

	// send request
	_, err = state.postSubmit(w, req)
	require.NoError(t, err)
}
