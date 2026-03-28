package handlers

import (
	"testing"

	"github.com/equaltoai/lesser/pkg/auth"
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
