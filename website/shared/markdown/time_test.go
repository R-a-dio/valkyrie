package markdown

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark"
)

func TestTimeExtension(t *testing.T) {
	data := []byte(`{2025-12-26T10:30:00Z} - {2025-12-26T11:30:00Z} do some stuff`)

	md := goldmark.New(RadioMarkdownOptions(false)...)

	var buf bytes.Buffer
	err := md.Convert([]byte(data), &buf)
	require.NoError(t, err)

	result := buf.String()
	assert.Contains(t, result, `1766745000`, "result did not contain correct time")
	assert.Contains(t, result, `1766748600`, "result did not contain correct time")
}
