package markdown

import (
	"bytes"
	"testing"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/util"
)

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
