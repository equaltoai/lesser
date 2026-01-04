package common

import (
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type endlessReader struct{}

func (endlessReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'a'
	}
	return len(p), nil
}

type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

func TestReadUntrustedHTTPResponseBody_NotTruncated(t *testing.T) {
	body, truncated, err := ReadUntrustedHTTPResponseBody(strings.NewReader("ok"), 10)
	require.NoError(t, err)
	assert.False(t, truncated)
	assert.Equal(t, []byte("ok"), body)
}

func TestReadUntrustedHTTPResponseBody_TruncatesAndMarks(t *testing.T) {
	body, truncated, err := ReadUntrustedHTTPResponseBody(strings.NewReader("0123456789"), 5)
	require.NoError(t, err)
	assert.True(t, truncated)
	assert.Equal(t, []byte("01234"), body)

	snippet := FormatUntrustedHTTPBodySnippet(body, truncated)
	assert.Equal(t, "01234"+TruncatedBodyMarker, snippet)
}

func TestReadUntrustedHTTPResponseBody_ReadsAtMostLimitPlusOne(t *testing.T) {
	const limit = int64(1024)
	cr := &countingReader{r: endlessReader{}}

	body, truncated, err := ReadUntrustedHTTPResponseBody(cr, limit)
	require.NoError(t, err)
	assert.True(t, truncated)
	assert.Len(t, body, int(limit))
	assert.LessOrEqual(t, cr.n, limit+1)
}
