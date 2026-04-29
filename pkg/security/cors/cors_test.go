package cors

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeOrigin(t *testing.T) {
	origin, parsed, ok := NormalizeOrigin(" HTTPS://Example.COM/ ")
	require.True(t, ok)
	require.Equal(t, "https://example.com", origin)
	require.NotNil(t, parsed)

	for _, raw := range []string{
		"",
		"example.com",
		"https://example.com/path",
		"https://example.com?x=1",
		"https://user@example.com",
	} {
		_, _, ok := NormalizeOrigin(raw)
		require.False(t, ok, raw)
	}
}

func TestParseAllowedOrigins(t *testing.T) {
	require.Equal(t, []string{"*"}, ParseAllowedOrigins("https://example.com,*"))
	require.Equal(t,
		[]string{"https://example.com", "https://admin.example.com"},
		ParseAllowedOrigins(" https://EXAMPLE.com/ , https://example.com, https://admin.example.com, https://bad.example/path "),
	)
	require.Empty(t, ParseAllowedOrigins("https://bad.example/path"))
}

func TestNormalizeAllowedOriginsForDeploy(t *testing.T) {
	require.Equal(t, "", NormalizeAllowedOriginsForDeploy("   "))
	require.Equal(t, "*", NormalizeAllowedOriginsForDeploy("*"))
	require.Equal(t, "https://example.com,https://admin.example.com", NormalizeAllowedOriginsForDeploy(" https://EXAMPLE.com/ , https://admin.example.com "))
	require.Equal(t, DenyAllAllowlist, NormalizeAllowedOriginsForDeploy("https://bad.example/path"))
}
