package markdown

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark"
)

func TestTimeExtension(t *testing.T) {
	data := `{26 Dec 25 10:30 +0000} - {26 Dec 25 11:30 +0000} do some stuff
	
{26 Dec 25 10:30 +0000|1h}`

	md := goldmark.New(RadioMarkdownOptions(false)...)

	var buf bytes.Buffer
	err := md.Convert([]byte(data), &buf)
	require.NoError(t, err)

	result := buf.String()
	// t.Log(result)
	assert.Contains(t, result, `1766745000`, "result did not contain correct time")
	assert.Contains(t, result, `1766748600`, "result did not contain correct time")
	assert.Contains(t, result, `data-dur="3600000"`, "result did not contain correct duration")
}
