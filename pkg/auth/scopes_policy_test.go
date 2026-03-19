package auth

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCanonicalOAuthScopes(t *testing.T) {
	t.Parallel()

	require.Equal(t, []string{ScopeRead, ScopeWrite, ScopeFollow, ScopePush}, CanonicalOAuthScopes())
	require.Equal(t, []string{ScopeRead, ScopeWrite}, DefaultScopes())
}

func TestValidatePublicOAuthScopes(t *testing.T) {
	t.Parallel()

	require.NoError(t, ValidatePublicOAuthScopes([]string{ScopeRead, ScopeWrite, ScopeFollow, ScopePush}))
	require.NoError(t, ValidatePublicOAuthScopes([]string{"write:follows", "write:statuses", "read:notifications"}))
	require.Equal(t, ErrInvalidScope, ValidatePublicOAuthScopes([]string{ScopeAdmin}))
	require.Equal(t, ErrInvalidScope, ValidatePublicOAuthScopes([]string{"admin:write"}))
	require.Equal(t, ErrInvalidScope, ValidatePublicOAuthScopes([]string{"custom"}))
}

func TestScopeGrantAllows(t *testing.T) {
	t.Parallel()

	require.True(t, ScopeGrantAllows(ScopeWrite, "write:statuses"))
	require.True(t, ScopeGrantAllows(ScopeWrite, ScopeFollow))
	require.True(t, ScopeGrantAllows("write:follows", ScopeFollow))
	require.True(t, ScopeGrantAllows(ScopeFollow, "write:follows"))
	require.True(t, ScopeGrantAllows(ScopeRead, "read:notifications"))

	require.False(t, ScopeGrantAllows("read:follows", ScopeFollow))
	require.False(t, ScopeGrantAllows(ScopePush, ScopeRead))
	require.False(t, ScopeGrantAllows("write:statuses", ScopeWrite))
}

func TestScopeSetAllows(t *testing.T) {
	t.Parallel()

	require.True(t, ScopeSetAllows([]string{ScopeWrite}, []string{ScopeFollow}))
	require.True(t, ScopeSetAllows([]string{ScopeFollow}, []string{"write:follows"}))
	require.True(t, ScopeSetAllows([]string{ScopeRead, ScopePush}, []string{"read:notifications", ScopePush}))
	require.False(t, ScopeSetAllows([]string{ScopeRead}, []string{ScopeWrite}))
}
