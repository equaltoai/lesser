package auth

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildPublicMCPAccessBundle(t *testing.T) {
	t.Run("returns actor scoped urls and canonical guidance", func(t *testing.T) {
		bundle := BuildPublicMCPAccessBundle("https://example.com/", "agent-one")

		require.Equal(t, "https://api.example.com/mcp/agent-one", bundle.MCPURL)
		require.Equal(t, "https://api.example.com/.well-known/oauth-protected-resource/mcp/agent-one", bundle.ProtectedResourceURL)
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

	t.Run("keeps local hosts unchanged for development", func(t *testing.T) {
		bundle := BuildPublicMCPAccessBundle("http://localhost:8788", "agent-one")

		require.Equal(t, "http://localhost:8788/mcp/agent-one", bundle.MCPURL)
		require.Equal(t, "http://localhost:8788/.well-known/oauth-protected-resource/mcp/agent-one", bundle.ProtectedResourceURL)
		require.Equal(t, "http://localhost:8788/.well-known/oauth-authorization-server", bundle.AuthorizationServerURL)
		require.Equal(t, "http://localhost:8788/oauth/register", bundle.RegistrationURL)
	})

	t.Run("preserves api hosts when already canonical", func(t *testing.T) {
		bundle := BuildPublicMCPAccessBundle("https://api.dev.example.com", "agent-one")

		require.Equal(t, "https://api.dev.example.com/mcp/agent-one", bundle.MCPURL)
		require.Equal(t, "https://api.dev.example.com/.well-known/oauth-protected-resource/mcp/agent-one", bundle.ProtectedResourceURL)
		require.Equal(t, "https://api.dev.example.com/.well-known/oauth-authorization-server", bundle.AuthorizationServerURL)
		require.Equal(t, "https://api.dev.example.com/oauth/register", bundle.RegistrationURL)
	})
}
