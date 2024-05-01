package public

import (
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/mocks"
	"github.com/R-a-dio/valkyrie/util/secret"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/csrf"
	"github.com/jxskiss/base62"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSearchSharedInputURLFix(t *testing.T) {
	ss := &mocks.SearchServiceMock{
		SearchFunc: func(ctx context.Context, query string, limit, offset int64) (*radio.SearchResult, error) {
			return &radio.SearchResult{}, nil
		},
	}
	rs := &mocks.RequestStorageMock{
		LastRequestFunc: func(identifier string) (time.Time, error) {
			return time.Now().Add(-time.Hour * 12), nil
		},
	}

	r := httptest.NewRequest(http.MethodPost, "/v1/request?page=5&q=test&trackid=100", nil)

	input, err := NewSearchSharedInput(ss, rs, r, time.Hour, searchPageSize)
	require.NoError(t, err)
	require.NotNil(t, input)

	assert.Contains(t, "/search", input.Page.BaseURL())
	assert.NotContains(t, "/v1/request", input.Page.BaseURL())
}

func TestCSRFLegacyFix(t *testing.T) {
	// this is the regular expression the old (and new) android apps use
	// to retrieve the csrf token from the search page
	re := regexp.MustCompile("value=\"(\\w+)\"")

	// setup a chi router with the csrf middleware
	key, err := secret.NewKey(32)
	require.NoError(t, err)

	router := chi.NewRouter()
	router.Use(csrf.Protect(key,
		csrf.Secure(false),
		// regexp above doesn't allow non-alphanumeric which means base64
		// is out of the running, use base62 instead which is alphanumeric
		csrf.Encoding(base62.StdEncoding),
	))
	router.Get("/search", func(w http.ResponseWriter, r *http.Request) {
		// generate the "legacy fix" which is just a HTML comment with
		// contents that the app expects, which is a line starting with
		// '<form' after whitespace trimming and then matching against it
		// with the regexp shown above
		html := csrfLegacyFix(r)
		assert.NotEmpty(t, html)

		// split on lines, since this is what the android app does
		s := strings.Split(string(html), "\n")
		for _, line := range s {
			// then trim spaces
			line = strings.TrimSpace(line)
			// see if we start with the expected '<form'
			if strings.HasPrefix(line, "<form") {
				// and then try to match the regexp
				assert.Regexp(t, re, line)
			}
		}
	})

	srv := httptest.NewServer(router)
	defer srv.Close()

	client := srv.Client()

	// do this a few times just to make sure the token is always caught
	for i := 0; i < 100; i++ {
		resp, err := client.Get(srv.URL + "/search")
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
	}
}
