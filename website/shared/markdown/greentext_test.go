package markdown

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/util"
)

var data []byte

func TestMemeQuotesExtension(t *testing.T) {
	data = []byte(`>>5555

>>6666
yes
	`)

	md := goldmark.New(RadioMarkdownOptions(false)...)

	var buf bytes.Buffer
	err := md.Convert([]byte(data), &buf)
	require.NoError(t, err)

	result := buf.String()
	assert.Contains(t, result, `href="#comment-5555"`, "result did not contain href")
	assert.Contains(t, result, `href="#comment-6666"`, "result did not contain href")
}

func FuzzRadioStyleMarkdown(f *testing.F) {
	f.Fuzz(func(t *testing.T, orig string) {
		markdown := goldmark.New(RadioMarkdownOptions(false)...)
		var buf bytes.Buffer
		err := markdown.Convert(util.StringToReadOnlyBytes(orig), &buf)
		if err != nil {
			panic(err)
		}
	})
}
