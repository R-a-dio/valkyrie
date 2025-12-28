package markdown

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark"
)

type timecase struct {
	name, in, out string
}

func TestTimeExtension(t *testing.T) {
	cases := []timecase{
		{"valid", "{26 Dec 25 10:30 +0000}", `<p><time datetime="1766745000" data-dur="0" data-type="local">26 Dec 25 10:30 +0000</time></p>`},
		{"valid with dur", "{26 Dec 25 10:30 +0000|1h}", `<p><time datetime="1766745000" data-dur="3600000" data-type="local">26 Dec 25 10:30 +0000</time></p>`},
		{"invalid", "{26 Dec 25 1as2d|1h}", `<p>{26 Dec 25 1as2d|1h}</p>`},
		{"opening with no time", "{: this is a weird smily {:", `<p>{: this is a weird smily {:</p>`},
		{"opening with closing no time", "this is a valid brace with nothing good in it {}", `<p>this is a valid brace with nothing good in it {}</p>`},
		{"newline", "newline time {yes \n}", "<p>newline time {yes<br>\n}</p>"},
		{"newline in valid date", "{26 Dec 25 10:30 +0000|1h\n}", "<p>{26 Dec 25 10:30 +0000|1h<br>\n}</p>"},
	}

	md := goldmark.New(RadioMarkdownOptions(false)...)

	for _, c := range cases {
		var buf bytes.Buffer
		err := md.Convert([]byte(c.in), &buf)
		require.NoError(t, err)

		result := buf.String()
		// renderer appends a newline, trim it
		result = strings.TrimSpace(result)
		// t.Log(result)
		assert.EqualValues(t, c.out, result)
	}
}
