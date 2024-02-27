package markdown

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/util"
)

var data []byte

func TestMemeQuotesExtension(t *testing.T) {
	data = []byte(`	`)

	md := goldmark.New(RadioMarkdownOptions()...)

	var buf bytes.Buffer
	err := md.Convert([]byte(data), &buf)

	fmt.Println(err)
	fmt.Println(buf.String())
}

func FuzzRadioStyleMarkdown(f *testing.F) {
	f.Fuzz(func(t *testing.T, orig string) {
		markdown := goldmark.New(RadioMarkdownOptions()...)
		var buf bytes.Buffer
		err := markdown.Convert(util.StringToReadOnlyBytes(orig), &buf)
		if err != nil {
			panic(err)
		}
	})
}
