package headers

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStaticHTMLPageCSP(t *testing.T) {
	csp := StaticHTMLPageCSP()
	require.Contains(t, csp, "default-src 'none'")
	require.Contains(t, csp, "script-src 'none'")
}

func TestEmbedHTMLPageCSP(t *testing.T) {
	require.Contains(t, EmbedHTMLPageCSP(""), "script-src 'none'")
	require.Contains(t, EmbedHTMLPageCSP("  "), "script-src 'none'")

	withNonce := EmbedHTMLPageCSP("  abc123  ")
	require.Contains(t, withNonce, "frame-ancestors *")
	require.Contains(t, withNonce, "script-src 'nonce-abc123'")
	require.NotContains(t, withNonce, "script-src 'none'")
}
