package handlers

import (
	"testing"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/require"
)

func TestNormalizeAuthorizeScopes(t *testing.T) {
	t.Parallel()

	h := &Handler{}

	scopes, err := h.normalizeAuthorizeScopes("")
	require.NoError(t, err)
	require.Equal(t, auth.DefaultScopes(), scopes)

	scopes, err = h.normalizeAuthorizeScopes("write:follows")
	require.NoError(t, err)
	require.Equal(t, []string{"write:follows"}, scopes)

	_, err = h.normalizeAuthorizeScopes("admin")
	require.Error(t, err)
}

func TestResolveOAuthClientRequestedScopes(t *testing.T) {
	t.Parallel()

	t.Run("canonical follow allowed by broad write registration", func(t *testing.T) {
		client := &storage.OAuthClient{Scopes: []string{auth.ScopeWrite}}
		scopes, err := resolveOAuthClientRequestedScopes(client, auth.ScopeFollow)
		require.NoError(t, err)
		require.Equal(t, []string{auth.ScopeFollow}, scopes)
	})

	t.Run("legacy write_follows allowed by follow registration", func(t *testing.T) {
		client := &storage.OAuthClient{Scopes: []string{auth.ScopeFollow}}
		scopes, err := resolveOAuthClientRequestedScopes(client, "write:follows")
		require.NoError(t, err)
		require.Equal(t, []string{"write:follows"}, scopes)
	})

	t.Run("admin remains non requestable", func(t *testing.T) {
		client := &storage.OAuthClient{Scopes: []string{auth.ScopeWrite}}
		_, err := resolveOAuthClientRequestedScopes(client, auth.ScopeAdmin)
		require.ErrorIs(t, err, auth.ErrInvalidScope)
	})
}
