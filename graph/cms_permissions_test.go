package graph

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEnsureAuthorCanWriteCMSRequiresAuth(t *testing.T) {
	t.Parallel()

	resolver := &Resolver{}

	err := resolver.ensureAuthorCanWriteCMS(context.Background(), "", "https://localhost/users/alice")
	require.ErrorIs(t, err, ErrAuthenticationRequired)
}

func TestEnsureAuthorCanWriteCMSAllowsLocalActorMatch(t *testing.T) {
	t.Parallel()

	resolver := &Resolver{}

	err := resolver.ensureAuthorCanWriteCMS(context.Background(), "alice", "https://localhost/users/alice")
	require.NoError(t, err)
}

func TestEnsureAuthorCanWriteCMSRejectsCrossTenantActorURL(t *testing.T) {
	t.Parallel()

	resolver := &Resolver{}

	err := resolver.ensureAuthorCanWriteCMS(context.Background(), "alice", "https://example.com/users/alice")
	require.Error(t, err)
}

func TestEnsureAuthorCanWriteCMSAllowsLegacyUsernameMatch(t *testing.T) {
	t.Parallel()

	resolver := &Resolver{}

	err := resolver.ensureAuthorCanWriteCMS(context.Background(), "alice", "alice")
	require.NoError(t, err)
}
