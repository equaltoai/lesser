package repositories

import (
	"testing"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestApplyOAuthClientModelUpdate(t *testing.T) {
	t.Parallel()

	applyOAuthClientModelUpdate(nil, FieldName, "ignored")

	client := &models.OAuthClient{}
	applyOAuthClientModelUpdate(client, FieldName, "Example")
	applyOAuthClientModelUpdate(client, FieldWebsite, "https://example.com")
	applyOAuthClientModelUpdate(client, FieldRedirectURIs, []string{"https://example.com/callback"})
	applyOAuthClientModelUpdate(client, FieldScopes, []string{"read", "write"})
	applyOAuthClientModelUpdate(client, FieldAgentUsername, "agent-1")
	applyOAuthClientModelUpdate(client, "unsupported", "ignored")

	require.Equal(t, "Example", client.Name)
	require.Equal(t, "https://example.com", client.Website)
	require.Equal(t, []string{"https://example.com/callback"}, client.RedirectURIs)
	require.Equal(t, []string{"read", "write"}, client.Scopes)
	require.Equal(t, "agent-1", client.AgentUsername)
}

func TestApplyOAuthClientStorageUpdate(t *testing.T) {
	t.Parallel()

	applyOAuthClientStorageUpdate(nil, FieldName, "ignored")

	client := &storage.OAuthClient{}
	applyOAuthClientStorageUpdate(client, FieldName, "Example")
	applyOAuthClientStorageUpdate(client, "description", "OAuth app")
	applyOAuthClientStorageUpdate(client, FieldRedirectURIs, []string{"https://example.com/callback"})
	applyOAuthClientStorageUpdate(client, "grant_types", []string{"authorization_code"})
	applyOAuthClientStorageUpdate(client, FieldScopes, []string{"read"})
	applyOAuthClientStorageUpdate(client, FieldWebsite, "https://example.com")
	applyOAuthClientStorageUpdate(client, FieldAgentUsername, "agent-1")
	applyOAuthClientStorageUpdate(client, "confidential", true)
	applyOAuthClientStorageUpdate(client, "unsupported", "ignored")

	require.Equal(t, "Example", client.Name)
	require.Equal(t, "OAuth app", client.Description)
	require.Equal(t, []string{"https://example.com/callback"}, client.RedirectURIs)
	require.Equal(t, []string{"authorization_code"}, client.GrantTypes)
	require.Equal(t, []string{"read"}, client.Scopes)
	require.Equal(t, "https://example.com", client.Website)
	require.Equal(t, "agent-1", client.AgentUsername)
	require.True(t, client.Confidential)
}
