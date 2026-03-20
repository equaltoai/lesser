package inmemory

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/require"
)

func TestOAuthRepository_UpdateOAuthClient_CoversRegistrationSourceRound21(t *testing.T) {
	t.Parallel()

	repo := NewOAuthRepository()
	require.NoError(t, repo.CreateOAuthClient(context.Background(), &storage.OAuthClient{
		ClientID:           "client-1",
		Name:               "Before",
		RedirectURIs:       []string{"https://example.com/callback"},
		GrantTypes:         []string{"authorization_code"},
		Scopes:             []string{"read"},
		RegistrationSource: "manual",
	}))

	require.NoError(t, repo.UpdateOAuthClient(context.Background(), "client-1", map[string]any{
		"name":                "After",
		"registration_source": "dynamic",
		"confidential":        true,
	}))

	client, err := repo.GetOAuthClient(context.Background(), "client-1")
	require.NoError(t, err)
	require.Equal(t, "After", client.Name)
	require.Equal(t, "dynamic", client.RegistrationSource)
	require.True(t, client.Confidential)
}

func TestApplyInMemoryOAuthClientUpdate_Round21(t *testing.T) {
	t.Parallel()

	client := &storage.OAuthClient{}
	applyInMemoryOAuthClientUpdate(client, "registration_source", "dynamic")
	applyInMemoryOAuthClientUpdate(client, "website", "https://example.com/client")

	require.Equal(t, "dynamic", client.RegistrationSource)
	require.Equal(t, "https://example.com/client", client.Website)
}
