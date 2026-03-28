package auth

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildPublicMCPAccessBundle(t *testing.T) {
	t.Run("returns actor scoped urls and canonical guidance", func(t *testing.T) {
		bundle := BuildPublicMCPAccessBundle("https://example.com/", "agent-one")

		require.Equal(t, "https://example.com/mcp/agent-one", bundle.MCPURL)
		require.Equal(t, "https://example.com/.well-known/oauth-protected-resource/mcp/agent-one", bundle.ProtectedResourceURL)
		require.Equal(t, "https://example.com/.well-known/oauth-authorization-server", bundle.AuthorizationServerURL)
		require.Equal(t, "https://example.com/oauth/register", bundle.RegistrationURL)
		require.Equal(t, []string{ScopeRead, ScopeWrite, ScopeFollow, ScopePush}, bundle.SupportedScopes)
		require.Len(t, bundle.Guidance, 5)
		require.Contains(t, bundle.Guidance[1], "OAuth resource")
		require.Contains(t, bundle.Guidance[4], "client_credentials")
	})

	t.Run("preserves client neutral guidance without runtime base url", func(t *testing.T) {
		bundle := BuildPublicMCPAccessBundle("", "agent-one")

		require.Empty(t, bundle.MCPURL)
		require.Empty(t, bundle.ProtectedResourceURL)
		require.Empty(t, bundle.AuthorizationServerURL)
		require.Empty(t, bundle.RegistrationURL)
		require.Equal(t, []string{ScopeRead, ScopeWrite, ScopeFollow, ScopePush}, bundle.SupportedScopes)
		require.Len(t, bundle.Guidance, 5)
	})
}
