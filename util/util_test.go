package util

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsHTMX(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	assert.False(t, IsHTMX(req))

	req.Header.Add("Hx-Request", "true")
	assert.True(t, IsHTMX(req))
}

func TestAbsolutePath(t *testing.T) {
	cases := []struct {
		name     string
		dir      string
		path     string
		expected string
	}{
		{
			name:     "absolute path",
			dir:      "/h/t",
			path:     "/a/b",
			expected: "/a/b",
		},
		{
			name:     "relative path",
			dir:      "/h/t",
			path:     "c",
			expected: "/h/t/c",
		},
		{
			name:     "relative path with dir",
			dir:      "/h/t",
			path:     "b/c",
			expected: "/h/t/b/c",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res := AbsolutePath(c.dir, c.path)
			assert.Equal(t, c.expected, res)
		})
	}
}

func TestAddContentDisposition(t *testing.T) {
	w := httptest.NewRecorder()
	assert.Empty(t, w.Header().Get(headerContentDisposition))

	AddContentDisposition(w, "test.mp3")
	value := w.Header().Get(headerContentDisposition)
	assert.NotEmpty(t, value)
	assert.Equal(t, `attachment; filename="test.mp3"; filename*=UTF-8''test.mp3`, value)
	assert.Equal(t, "audio/mpeg", w.Header().Get("Content-Type"))
}

func TestAddContentDispositionSong(t *testing.T) {
	w := httptest.NewRecorder()
	assert.Empty(t, w.Header().Get(headerContentDisposition))

	AddContentDispositionSong(w, "hello - world", "test.flac")
	value := w.Header().Get(headerContentDisposition)
	assert.NotEmpty(t, value)
	assert.Equal(t, `attachment; filename="hello - world.flac"; filename*=UTF-8''hello%20-%20world.flac`, value)
	assert.Equal(t, "audio/flac", w.Header().Get("Content-Type"))
}
