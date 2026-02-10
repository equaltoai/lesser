package authz

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeRole(t *testing.T) {
	require.Equal(t, "admin", NormalizeRole(" ADMIN "))
	require.Equal(t, "moderator", NormalizeRole("moderator"))
	require.Equal(t, "user", NormalizeRole(" user "))
}

func TestRoleChecks(t *testing.T) {
	require.True(t, IsAdmin("admin"))
	require.True(t, IsAdmin(" ADMIN "))
	require.False(t, IsAdmin("moderator"))

	require.True(t, IsModeratorOrAdmin("moderator"))
	require.True(t, IsModeratorOrAdmin(" ADMIN "))
	require.False(t, IsModeratorOrAdmin("user"))
}
