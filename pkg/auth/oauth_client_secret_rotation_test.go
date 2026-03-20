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
