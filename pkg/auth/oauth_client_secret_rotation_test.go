package auth

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/require"
)

func TestOAuthService_ValidateClient_SecretRotationGraceWindow(t *testing.T) {
	t.Parallel()

	activeHash, err := HashOAuthClientSecret("secret-new")
	require.NoError(t, err)
	previousHash, err := HashOAuthClientSecret("secret-old")
	require.NoError(t, err)

	repo := &oauthAccountRepoStub{
		clients: map[string]*storage.OAuthClient{
			"client-1": {
				ClientID:                           "client-1",
				ClientSecretHash:                   activeHash,
				PreviousClientSecretHash:           previousHash,
				PreviousClientSecretGraceExpiresAt: time.Now().Add(2 * time.Hour),
				RedirectURIs:                       []string{"https://example.com/callback"},
			},
		},
	}

	svc := &OAuthService{
		jwtSecret:   []byte("test-secret"),
		accountRepo: repo,
	}

	require.NoError(t, svc.ValidateClient(context.Background(), "client-1", "secret-new"))
	require.NoError(t, svc.ValidateClient(context.Background(), "client-1", "secret-old"))
}

func TestValidateOAuthClientSecretAt_RejectsExpiredOrForcedPreviousSecret(t *testing.T) {
	t.Parallel()

	activeHash, err := HashOAuthClientSecret("secret-new")
	require.NoError(t, err)
	previousHash, err := HashOAuthClientSecret("secret-old")
	require.NoError(t, err)

	now := time.Date(2026, time.March, 19, 16, 0, 0, 0, time.UTC)

	t.Run("expired previous secret fails", func(t *testing.T) {
		result, err := validateOAuthClientSecretAt(now, &storage.OAuthClient{
			ClientSecretHash:                   activeHash,
			PreviousClientSecretHash:           previousHash,
			PreviousClientSecretGraceExpiresAt: now.Add(-1 * time.Minute),
		}, "secret-old")
		require.NoError(t, err)
		require.False(t, result.matchedCurrent)
		require.False(t, result.matchedPrevious)
	})

	t.Run("forced invalidation disables previous secret immediately", func(t *testing.T) {
		result, err := validateOAuthClientSecretAt(now, &storage.OAuthClient{
			ClientSecretHash:                   activeHash,
			PreviousClientSecretHash:           previousHash,
			PreviousClientSecretGraceExpiresAt: time.Time{},
		}, "secret-old")
		require.NoError(t, err)
		require.False(t, result.matchedCurrent)
		require.False(t, result.matchedPrevious)
	})
}

func TestValidateOAuthClientSecretAt_CoversMigrationAndMalformedPreviousHash(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.March, 19, 16, 0, 0, 0, time.UTC)

	t.Run("active secret helper prefers hashed then plaintext values", func(t *testing.T) {
		require.Empty(t, oauthClientActiveSecretValue(nil))
		require.Equal(t, "hash-value", oauthClientActiveSecretValue(&storage.OAuthClient{
			ClientSecretHash: "hash-value",
			ClientSecret:     "plaintext-value",
		}))
		require.Equal(t, "plaintext-value", oauthClientActiveSecretValue(&storage.OAuthClient{
			ClientSecret: "plaintext-value",
		}))
	})

	t.Run("legacy plaintext active secret marks migration needed", func(t *testing.T) {
		result, err := validateOAuthClientSecretAt(now, &storage.OAuthClient{
			ClientSecret: "secret-current",
		}, "secret-current")
		require.NoError(t, err)
		require.True(t, result.matchedCurrent)
		require.False(t, result.matchedPrevious)
		require.True(t, result.currentNeedsMigration)
	})

	t.Run("malformed previous hash returns verifier error", func(t *testing.T) {
		activeHash, err := HashOAuthClientSecret("secret-new")
		require.NoError(t, err)

		_, err = validateOAuthClientSecretAt(now, &storage.OAuthClient{
			ClientSecretHash:                   activeHash,
			PreviousClientSecretHash:           "bcrypt:not-a-valid-bcrypt-hash",
			PreviousClientSecretGraceExpiresAt: now.Add(1 * time.Hour),
		}, "secret-old")
		require.Error(t, err)
	})
}
