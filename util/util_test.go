package util

import (
	"net/http"
	"net/http/httptest"
	"strconv"
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

func TestReduceWithStep(t *testing.T) {
	var in []int

	for i := range 50 {
		in = append(in, i)
	}

	tests := []struct {
		step     int
		expected []int
		leftover bool
	}{
		{
			step:     10,
			expected: []int{9, 19, 29, 39, 49},
			leftover: false,
		},
		{
			step:     9,
			expected: []int{8, 17, 26, 35, 44},
			leftover: true,
		},
		{
			step:     8,
			expected: []int{7, 15, 23, 31, 39, 47},
			leftover: true,
		},
		{
			step:     7,
			expected: []int{6, 13, 20, 27, 34, 41, 48},
			leftover: true,
		},
		{
			step:     6,
			expected: []int{5, 11, 17, 23, 29, 35, 41, 47},
			leftover: true,
		},
		{
			step:     5,
			expected: []int{4, 9, 14, 19, 24, 29, 34, 39, 44, 49},
			leftover: false,
		},
		{
			step:     4,
			expected: []int{3, 7, 11, 15, 19, 23, 27, 31, 35, 39, 43, 47},
			leftover: true,
		},
		{
			step:     3,
			expected: []int{2, 5, 8, 11, 14, 17, 20, 23, 26, 29, 32, 35, 38, 41, 44, 47},
			leftover: true,
		},
		{
			step:     2,
			expected: []int{1, 3, 5, 7, 9, 11, 13, 15, 17, 19, 21, 23, 25, 27, 29, 31, 33, 35, 37, 39, 41, 43, 45, 47, 49},
			leftover: false,
		},
		{
			step:     1,
			expected: in,
			leftover: false,
		},
		{
			step:     -100,
			expected: in,
			leftover: false,
		},
		{
			step:     0,
			expected: in,
			leftover: false,
		},
	}

	for _, test := range tests {
		t.Run(strconv.Itoa(test.step), func(t *testing.T) {
			assert.Equal(t, test.leftover, ReduceHasLeftover(in, test.step))
			out := ReduceWithStep(in, test.step)
			assert.Equal(t, test.expected, out)
		})
	}
}
