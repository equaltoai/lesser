package handlers

import (
	"context"
	"net/http"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestAgentHelpersRound20(t *testing.T) {
	cfg := round10TestConfig()
	cfg.AllowAgents = true
	cfg.AllowAgentRegistration = true

	now := time.Now().UTC()
	baseState := &round10QueryState{
		agentInstanceConfig: func() *storagemodels.AgentInstanceConfig {
			p := storagemodels.NewAgentInstanceConfig()
			p.AllowAgents = true
			p.AllowAgentRegistration = true
			return p
		}(),
		usersByUsername: map[string]storagemodels.User{
			"agent1": {
				PK:           "USER#agent1",
				SK:           storagemodels.SKMetadata,
				Username:     "agent1",
				Approved:     true,
				Version:      1,
				CreatedAt:    now.Add(-24 * time.Hour),
				IsAgent:      true,
				AgentOwner:   "@owner",
				AgentType:    agentTypeCustom,
				AgentVersion: "v1",
			},
			"human": {
				PK:        "USER#human",
				SK:        storagemodels.SKMetadata,
				Username:  "human",
				Approved:  true,
				Version:   1,
				CreatedAt: now.Add(-24 * time.Hour),
				IsAgent:   false,
			},
		},
		agentGovernanceByUsername: map[string]storagemodels.AgentGovernanceState{
			"agent1": {
				PK:              "USER#agent1",
				SK:              storagemodels.SKAgentGovernance,
				Username:        "agent1",
				DelegatedScopes: []string{auth.ScopeRead, "write:statuses"},
				CreatedAt:       now.Add(-24 * time.Hour),
				UpdatedAt:       now.Add(-time.Hour),
			},
		},
	}

	t.Run("resolveDelegatedAgentAccount branches", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, baseState)
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/delegate", nil, nil, nil)
		require.NoError(t, err)

		account, resp, err := h.resolveDelegatedAgentAccount(ctx, &auth.Claims{Username: "owner"}, "agent1", []string{"write:statuses"})
		require.NoError(t, err)
		require.Nil(t, resp)
		require.NotNil(t, account)

		account, resp, err = h.resolveDelegatedAgentAccount(ctx, &auth.Claims{Username: "owner"}, "human", []string{"read"})
		require.NoError(t, err)
		require.Nil(t, account)
		require.NotNil(t, resp)

		account, resp, err = h.resolveDelegatedAgentAccount(ctx, &auth.Claims{Username: "intruder"}, "agent1", []string{"read"})
		require.NoError(t, err)
		require.Nil(t, account)
		require.NotNil(t, resp)

		account, resp, err = h.resolveDelegatedAgentAccount(ctx, &auth.Claims{Username: "owner"}, "agent1", []string{"follow"})
		require.NoError(t, err)
		require.Nil(t, account)
		require.NotNil(t, resp)
	})

	t.Run("validateAgentDelegationRequest invalid username", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, baseState)
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/delegate", nil, nil, nil)
		require.NoError(t, err)
		resp, err := h.validateAgentDelegationRequest(ctx, &apimodels.AgentDelegationRequest{AgentUsername: "not valid"})
		require.NotNil(t, resp)
		require.NoError(t, err)
	})

	t.Run("agent envelope helpers", func(t *testing.T) {
		governance := &storage.AgentGovernanceState{
			DelegatedScopes: []string{"read", "write:statuses"},
		}
		scopes, ok := agentDelegationEnvelope(governance)
		require.True(t, ok)
		require.Equal(t, []string{"read", "write:statuses"}, scopes)

		scopes, ok = agentDelegationEnvelope(&storage.AgentGovernanceState{})
		require.False(t, ok)
		require.Nil(t, scopes)

		require.NoError(t, validateDelegationAgainstAgentEnvelope(governance, []string{"read"}))
		require.Error(t, validateDelegationAgainstAgentEnvelope(governance, []string{"follow"}))
		require.Error(t, validateDelegationAgainstAgentEnvelope(governance, []string{"push"}))
	})

	t.Run("deriveAgentCapabilitiesFromScopes", func(t *testing.T) {
		caps := deriveAgentCapabilitiesFromScopes([]string{"write", "follow", "write:statuses"})
		require.True(t, caps.CanPost)
		require.True(t, caps.CanReply)
		require.True(t, caps.CanBoost)
		require.True(t, caps.CanDM)
		require.True(t, caps.CanFollow)
	})

	t.Run("agentFromStorageUser only exposes verifiedAt for verified agents", func(t *testing.T) {
		createdAt := now.Add(-2 * time.Hour)
		verifiedAt := now.Add(-30 * time.Minute)
		user := &storage.User{
			Username:     "agent1",
			DisplayName:  "Agent One",
			Note:         "bio",
			CreatedAt:    createdAt,
			AgentOwner:   "@owner",
			IsAgent:      true,
			AgentVersion: "",
			AgentType:    "",
		}

		unverified := agentFromStorageUser(user, &storage.AgentGovernanceState{
			Username:        "agent1",
			Verified:        false,
			VerifiedAt:      &verifiedAt,
			DelegatedScopes: []string{"read"},
		})
		require.False(t, unverified.Verified)
		require.Nil(t, unverified.VerifiedAt)
		require.Equal(t, agentTypeCustom, unverified.AgentType)
		require.Equal(t, agentVersionUnknown, unverified.AgentVersion)
		require.Equal(t, []string{"read"}, unverified.DelegatedScopes)
		require.NotNil(t, unverified.CreatedAt)
		require.Equal(t, createdAt, *unverified.CreatedAt)
		require.Empty(t, unverified.MCPAccess.MCPURL)
		require.Equal(t, []string{auth.ScopeRead, auth.ScopeWrite, auth.ScopeFollow, auth.ScopePush}, unverified.MCPAccess.Scopes)

		verified := agentFromStorageUserWithBaseURL(user, &storage.AgentGovernanceState{
			Username:   "agent1",
			Verified:   true,
			VerifiedAt: &verifiedAt,
		}, "https://example.com/")
		bundle := auth.BuildPublicMCPAccessBundle("https://example.com/", "agent1")
		require.True(t, verified.Verified)
		require.NotNil(t, verified.VerifiedAt)
		require.Equal(t, verifiedAt.UTC(), *verified.VerifiedAt)
		require.Equal(t, bundle.MCPURL, verified.MCPAccess.MCPURL)
		require.Equal(t, bundle.ProtectedResourceURL, verified.MCPAccess.ProtectedResourceURL)
		require.Equal(t, bundle.AuthorizationServerURL, verified.MCPAccess.AuthorizationServerURL)
		require.Equal(t, bundle.RegistrationURL, verified.MCPAccess.RegistrationURL)
		require.Len(t, verified.MCPAccess.Guidance, 5)
	})

	t.Run("agent governance state helpers handle nil found and missing rows", func(t *testing.T) {
		single, err := loadAgentGovernanceState(context.Background(), nil, "agent1")
		require.NoError(t, err)
		require.Nil(t, single)

		batch, err := loadAgentGovernanceStates(context.Background(), nil, []string{"agent1"})
		require.NoError(t, err)
		require.Empty(t, batch)

		h, _, _ := round11NewHandler(t, cfg, baseState)

		single, err = loadAgentGovernanceState(context.Background(), h.repos, "agent1")
		require.NoError(t, err)
		require.NotNil(t, single)
		require.Equal(t, "agent1", single.Username)
		require.Equal(t, []string{auth.ScopeRead, "write:statuses"}, single.DelegatedScopes)

		batch, err = loadAgentGovernanceStates(context.Background(), h.repos, []string{"agent1", "missing"})
		require.NoError(t, err)
		require.Contains(t, batch, "agent1")
		require.NotContains(t, batch, "missing")

		missingHandler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		single, err = loadAgentGovernanceState(context.Background(), missingHandler.repos, "missing")
		require.NoError(t, err)
		require.Nil(t, single)

		required, err := requireAgentGovernanceState(context.Background(), missingHandler.repos, "missing")
		require.Nil(t, required)
		require.ErrorIs(t, err, errAgentGovernanceUnavailable)

		batch, err = loadAgentGovernanceStates(context.Background(), missingHandler.repos, []string{"missing"})
		require.NoError(t, err)
		require.Empty(t, batch)
	})

	t.Run("agent governance write errors map version conflicts to 409", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent1", nil, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusConflict)(respondAgentGovernanceWriteError(ctx, storage.ErrVersionConflict))
	})
}
