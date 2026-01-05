package auth

import (
	"context"
	"strings"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/require"
)

type oauthAccountRepoWithSecretUpdateStub struct {
	clients    map[string]*storage.OAuthClient
	updateHits int
	updated    map[string]string
}

func (s *oauthAccountRepoWithSecretUpdateStub) GetOAuthClient(_ context.Context, clientID string) (*storage.OAuthClient, error) {
	if c, ok := s.clients[clientID]; ok {
		return c, nil
	}
	return nil, errNotFound
}

func (s *oauthAccountRepoWithSecretUpdateStub) UpdateOAuthClientSecretHash(_ context.Context, clientID, clientSecretHash string) error {
	s.updateHits++
	if s.updated == nil {
		s.updated = make(map[string]string)
	}
	s.updated[clientID] = clientSecretHash
	if c, ok := s.clients[clientID]; ok {
		c.ClientSecretHash = clientSecretHash
	}
	return nil
}

func TestOAuthService_ValidateClient_MigratesLegacyPlaintextSecret(t *testing.T) {
	t.Parallel()

	repo := &oauthAccountRepoWithSecretUpdateStub{
		clients: map[string]*storage.OAuthClient{
			"client-1": {
				ClientID:         "client-1",
				ClientSecretHash: "secret", // legacy plaintext-at-rest representation
				RedirectURIs:     []string{"https://example.com/callback"},
			},
		},
	}

	svc := &OAuthService{
		jwtSecret:   []byte("test-secret"),
		accountRepo: repo,
	}

	require.NoError(t, svc.ValidateClient(context.Background(), "client-1", "secret"))
	require.Equal(t, 1, repo.updateHits)
	require.True(t, strings.HasPrefix(repo.updated["client-1"], OAuthClientSecretHashPrefix))
	require.NotEqual(t, "secret", repo.updated["client-1"])

	// Second call should not re-migrate.
	require.NoError(t, svc.ValidateClient(context.Background(), "client-1", "secret"))
	require.Equal(t, 1, repo.updateHits)
}
