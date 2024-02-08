package public

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/mocks"
	"github.com/R-a-dio/valkyrie/templates"
)

func TestGetSubmit(t *testing.T) {
	storage := &mocks.StorageServiceMock{}
	storage.SubmissionsFunc = func(contextMoqParam context.Context) radio.SubmissionStorage {
		return &mocks.SubmissionStorageMock{
			SubmissionStatsFunc: func(identifier string) (radio.SubmissionStats, error) {
				return radio.SubmissionStats{}, nil
			},
		}
	}
	executor := &mocks.ExecutorMock{}
	executor.ExecuteFunc = func(w io.Writer, r *http.Request, input templates.TemplateSelectable) error {
		return nil
	}

	state := State{
		Storage:   storage,
		Templates: executor,
	}

	req := httptest.NewRequest(http.MethodGet, "/submit", nil)
	w := httptest.NewRecorder()

	// make a multipart post body
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("track", "we're testing a file upload here.flac")
	if err != nil {
		t.Fatal(err)
	}

	var file io.Reader = strings.NewReader("Hello world and some other garbage")
	if _, err = io.Copy(part, file); err != nil {
		t.Fatal(err)
	}

	writer.WriteField("daypass", "")
	writer.WriteField("", "")

	// send request
	state.GetSubmit(w, req)
}
