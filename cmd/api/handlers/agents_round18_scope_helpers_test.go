package handlers

import (
	"net/http"
	"testing"

	"github.com/equaltoai/lesser/pkg/auth"
	storage "github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/require"
)

func TestAgentsRound18_IsAgentOwnerOrAdmin(t *testing.T) {
	h := &Handler{}

	require.False(t, h.isAgentOwnerOrAdmin(nil, nil))
	require.False(t, h.isAgentOwnerOrAdmin(&auth.Claims{Username: "alice"}, nil))
	require.False(t, h.isAgentOwnerOrAdmin(nil, &storage.User{AgentOwner: "alice"}))

	require.True(t, h.isAgentOwnerOrAdmin(&auth.Claims{Username: "alice", Scopes: []string{auth.ScopeAdmin}}, &storage.User{AgentOwner: "bob"}))
	require.True(t, h.isAgentOwnerOrAdmin(&auth.Claims{Username: "alice", Scopes: []string{"admin:write"}}, &storage.User{AgentOwner: "bob"}))
	require.True(t, h.isAgentOwnerOrAdmin(&auth.Claims{Username: "alice", Scopes: []string{"admin:all"}}, &storage.User{AgentOwner: "bob"}))

	require.False(t, h.isAgentOwnerOrAdmin(&auth.Claims{Username: "alice"}, &storage.User{AgentOwner: ""}))
	require.True(t, h.isAgentOwnerOrAdmin(&auth.Claims{Username: "alice"}, &storage.User{AgentOwner: "@alice"}))
	require.False(t, h.isAgentOwnerOrAdmin(&auth.Claims{Username: "alice"}, &storage.User{AgentOwner: "bob"}))
}

func TestAgentsRound18_ValidateDelegationScopes(t *testing.T) {
	h := &Handler{}
	ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/delegate", nil, nil, nil)
	require.NoError(t, err)

	t.Run("requires scopes", func(t *testing.T) {
		_, resp, respErr := h.validateDelegationScopes(ctx, []string{"read"}, nil)
		require.NoError(t, respErr)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusBadRequest, resp.Status)
	})

	t.Run("rejects empty scope entries", func(t *testing.T) {
		_, resp, respErr := h.validateDelegationScopes(ctx, []string{"read"}, []string{"read", " "})
		require.NoError(t, respErr)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusBadRequest, resp.Status)
	})

	t.Run("rejects admin and push scopes", func(t *testing.T) {
		_, resp, respErr := h.validateDelegationScopes(ctx, []string{"read"}, []string{"admin"})
		require.NoError(t, respErr)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusForbidden, resp.Status)

		_, resp, respErr = h.validateDelegationScopes(ctx, []string{"read"}, []string{"push"})
		require.NoError(t, respErr)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusForbidden, resp.Status)
	})

	t.Run("rejects invalid scope syntax", func(t *testing.T) {
		_, resp, respErr := h.validateDelegationScopes(ctx, []string{"read"}, []string{"write statuses"})
		require.NoError(t, respErr)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusBadRequest, resp.Status)
	})

	t.Run("rejects requested scopes outside delegator", func(t *testing.T) {
		_, resp, respErr := h.validateDelegationScopes(ctx, []string{"read"}, []string{"write"})
		require.NoError(t, respErr)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusForbidden, resp.Status)
	})

	t.Run("success returns cleaned scopes", func(t *testing.T) {
		scopes, resp, respErr := h.validateDelegationScopes(ctx, []string{"read", "write"}, []string{" read "})
		require.NoError(t, respErr)
		require.Nil(t, resp)
		require.Equal(t, []string{"read"}, scopes)
	})
}
