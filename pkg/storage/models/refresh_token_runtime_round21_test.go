package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRefreshTokenRuntimeMetadataBoundaries(t *testing.T) {
	t.Run("standard refresh token keeps runtime metadata empty", func(t *testing.T) {
		expiresAt := time.Now().UTC().Add(time.Hour)
		token := &RefreshToken{
			Token:     "public-rt",
			ClientID:  "client-1",
			Username:  "alice",
			SessionID: "sess-1",
			ExpiresAt: expiresAt,
		}

		require.NoError(t, token.BeforeCreate())
		require.False(t, token.carriesRuntimeSessionState())
		require.Zero(t, token.Generation)
		require.False(t, token.Current)
		require.True(t, token.SessionCreatedAt.IsZero())
		require.True(t, token.LastUsedAt.IsZero())
		require.True(t, token.IdleExpiresAt.IsZero())
		require.True(t, token.AbsoluteExpiresAt.IsZero())
		require.Empty(t, token.GSI1PK)
		require.Empty(t, token.GSI2PK)
		require.Empty(t, token.GSI3PK)
	})

	t.Run("runtime refresh token populates runtime defaults and indexes", func(t *testing.T) {
		expiresAt := time.Now().UTC().Add(time.Hour)
		token := &RefreshToken{
			Token:     "runtime-rt",
			ClientID:  "lesser-agent-delegation",
			Username:  "agent1",
			SessionID: "sid-1",
			FamilyID:  "fam-1",
			ExpiresAt: expiresAt,
		}

		require.NoError(t, token.BeforeCreate())
		require.True(t, token.carriesRuntimeSessionState())
		require.Equal(t, 1, token.Generation)
		require.True(t, token.Current)
		require.WithinDuration(t, token.CreatedAt, token.SessionCreatedAt, time.Second)
		require.WithinDuration(t, token.CreatedAt, token.LastUsedAt, time.Second)
		require.Equal(t, expiresAt, token.IdleExpiresAt)
		require.Equal(t, expiresAt, token.AbsoluteExpiresAt)
		require.Equal(t, "RUNTIME_USER#agent1#lesser-agent-delegation", token.GSI1PK)
		require.Equal(t, "RUNTIME_FAMILY#fam-1", token.GSI2PK)
		require.Equal(t, "RUNTIME_SESSION#sid-1", token.GSI3PK)
	})

	t.Run("standard lineage populates only the family index", func(t *testing.T) {
		token := &RefreshToken{
			Token:      "standard-lineage",
			ClientID:   "dynamic-client",
			Username:   "alice",
			SessionID:  "sess-1",
			FamilyID:   "fam-standard",
			Generation: 2,
			Current:    true,
			ExpiresAt:  time.Now().UTC().Add(time.Hour),
		}

		require.NoError(t, token.BeforeCreate())
		require.False(t, token.carriesRuntimeSessionState())
		require.Equal(t, "RUNTIME_FAMILY#fam-standard", token.GSI2PK)
		require.Equal(t, "00000002", token.GSI2SK)
		require.Empty(t, token.GSI1PK)
		require.Empty(t, token.GSI3PK)
		require.True(t, token.SessionCreatedAt.IsZero())
		require.True(t, token.LastUsedAt.IsZero())
	})
}
