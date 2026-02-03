package crawler

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRobotsHandler_ReturnsStaticPolicy(t *testing.T) {
	resp, err := RobotsHandler(nil)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, 200, resp.Status)

	require.Equal(t, []string{"text/plain; charset=utf-8"}, resp.Headers["content-type"])
	require.Equal(t, []string{"public, max-age=86400"}, resp.Headers["cache-control"])
	require.Equal(t, []string{"noindex"}, resp.Headers["x-robots-tag"])

	body := string(resp.Body)
	require.True(t, strings.HasPrefix(body, "# Lesser ActivityPub Instance"))
	require.Contains(t, body, "User-agent: GPTBot")
	require.Contains(t, body, "Disallow: /")
}
