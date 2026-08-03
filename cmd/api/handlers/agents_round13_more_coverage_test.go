package handlers

import (
	"net/http"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v3/runtime"
	dynamormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"go.uber.org/zap"
)

func TestAgentsRound13_parseDelegationAgentInfo_ErrorBranches(t *testing.T) {
	_, err := parseDelegationAgentInfo(make(chan int))
	require.Error(t, err)

	_, err = parseDelegationAgentInfo(map[string]any{
		"capabilities": 7,
	})
	require.Error(t, err)

	info, err := parseDelegationAgentInfo(map[string]any{
		"agent_type": "CUSTOM",
		"version":    "v1",
		"capabilities": map[string]any{
			"can_post": true,
		},
	})
	require.NoError(t, err)
	require.Equal(t, "CUSTOM", info.AgentType)
}

func TestAgentsRound13_UpdateAgent_ValidatesDisplayNameAndBio(t *testing.T) {
	cfg := round10TestConfig()
	cfg.AllowAgents = true

	policy := storagemodels.NewAgentInstanceConfig()
	policy.AllowAgents = true

	state := &round10QueryState{
		agentInstanceConfig: policy,
		usersByUsername: map[string]storagemodels.User{
			"owner": {
				PK:        "USER#owner",
				SK:        storagemodels.SKMetadata,
				Username:  "owner",
				Role:      "user",
				Approved:  true,
				Version:   1,
				CreatedAt: time.Now().Add(-24 * time.Hour),
			},
			"alice": {
				PK:           "USER#alice",
				SK:           storagemodels.SKMetadata,
				Username:     "alice",
				Role:         "user",
				Approved:     true,
				Version:      1,
				CreatedAt:    time.Now().Add(-24 * time.Hour),
				IsAgent:      true,
				AgentOwner:   "@owner",
				AgentType:    agentTypeCustom,
				AgentVersion: "v1",
			},
		},
		agentGovernanceByUsername: map[string]storagemodels.AgentGovernanceState{
			"alice": {
				PK:        "USER#alice",
				SK:        storagemodels.SKAgentGovernance,
				Username:  "alice",
				CreatedAt: time.Now().Add(-24 * time.Hour),
				UpdatedAt: time.Now().Add(-time.Hour),
				Version:   1,
			},
		},
	}

	h, _, _ := round11NewHandler(t, cfg, state)
	h.repos.Account().SetEncryptor(noopEncryptor{})
	h.mastodonLogic = common.NewMastodonBusinessLogic(common.DefaultMastodonConfig(), zap.NewNop())

	ownerToken := round11SignAccessToken(t, cfg.JWTSecret, "owner", []string{auth.ScopeWrite, auth.ScopeRead})
	headers := map[string]string{"Authorization": "Bearer " + ownerToken}

	t.Run("invalid display name rejected", func(t *testing.T) {
		req := apimodels.UpdateAgentRequest{
			DisplayName: "this display name is way too long for mastodon rules",
		}
		ctx, err := round10NewLiftContext(http.MethodPatch, "/api/v1/agents/alice", headers, nil, req)
		require.NoError(t, err)
		ctx.Params["username"] = "alice"

		resp := requireStatus(t, http.StatusBadRequest)(h.HandleUpdateAgentLift(ctx))
		require.NotEmpty(t, resp.Body)
	})

	t.Run("invalid bio rejected", func(t *testing.T) {
		req := apimodels.UpdateAgentRequest{
			Bio: "this bio is way too long for mastodon rules this bio is way too long for mastodon rules this bio is way too long for mastodon rules this bio is way too long for mastodon rules this bio is way too long for mastodon rules",
		}
		ctx, err := round10NewLiftContext(http.MethodPatch, "/api/v1/agents/alice", headers, nil, req)
		require.NoError(t, err)
		ctx.Params["username"] = "alice"

		resp := requireStatus(t, http.StatusBadRequest)(h.HandleUpdateAgentLift(ctx))
		require.NotEmpty(t, resp.Body)
	})
}

func TestAgentsRound13_DelegateAgent_ErrorBranches(t *testing.T) {
	cfg := round10TestConfig()
	cfg.AllowAgents = true
	cfg.AllowAgentRegistration = true

	policy := storagemodels.NewAgentInstanceConfig()
	policy.AllowAgents = true
	policy.AllowAgentRegistration = true
	policy.AgentMaxPostsPerHour = 10

	state := &round10QueryState{
		agentInstanceConfig: policy,
		usersByUsername: map[string]storagemodels.User{
			"owner": {
				PK:        "USER#owner",
				SK:        storagemodels.SKMetadata,
				Username:  "owner",
				Role:      "user",
				Approved:  true,
				Version:   1,
				CreatedAt: time.Now().Add(-24 * time.Hour),
			},
			"agent1": {
				PK:           "USER#agent1",
				SK:           storagemodels.SKMetadata,
				Username:     "agent1",
				Role:         "user",
				Approved:     true,
				Version:      1,
				CreatedAt:    time.Now().Add(-24 * time.Hour),
				IsAgent:      true,
				AgentOwner:   "@owner",
				AgentType:    agentTypeCustom,
				AgentVersion: "v1",
			},
		},
		agentGovernanceByUsername: map[string]storagemodels.AgentGovernanceState{
			"agent1": {
				PK:              "USER#agent1",
				SK:              storagemodels.SKAgentGovernance,
				Username:        "agent1",
				DelegatedScopes: []string{auth.ScopeRead, "write:statuses"},
				CreatedAt:       time.Now().Add(-24 * time.Hour),
				UpdatedAt:       time.Now().Add(-time.Hour),
			},
		},
	}

	h, _, _ := round11NewHandler(t, cfg, state)
	h.repos.Account().SetEncryptor(noopEncryptor{})

	ownerWriteToken := round11SignAccessToken(t, cfg.JWTSecret, "owner", []string{auth.ScopeWrite, auth.ScopeRead, "follow"})
	ownerReadToken := round11SignAccessToken(t, cfg.JWTSecret, "owner", []string{auth.ScopeRead})
	writeHeaders := map[string]string{"Authorization": "Bearer " + ownerWriteToken}
	readHeaders := map[string]string{"Authorization": "Bearer " + ownerReadToken}

	t.Run("invalid request body", func(t *testing.T) {
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/agents/delegate", writeHeaders, nil, []byte("{bad"))
		requireStatus(t, http.StatusBadRequest)(h.HandleDelegateAgentLift(ctx))
	})

	t.Run("missing display_name is allowed for existing agents", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/delegate", writeHeaders, nil, apimodels.AgentDelegationRequest{
			AgentUsername: "agent1",
			DisplayName:   "",
			Scopes:        []string{"read"},
		})
		require.NoError(t, err)
		requireStatus(t, http.StatusOK)(h.HandleDelegateAgentLift(ctx))
	})

	t.Run("missing scopes is rejected", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/delegate", writeHeaders, nil, apimodels.AgentDelegationRequest{
			AgentUsername: "agent1",
			DisplayName:   "Agent",
			Scopes:        nil,
		})
		require.NoError(t, err)
		requireStatus(t, http.StatusBadRequest)(h.HandleDelegateAgentLift(ctx))
	})

	t.Run("scopes cannot exceed owner scopes", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/delegate", readHeaders, nil, apimodels.AgentDelegationRequest{
			AgentUsername: "agent1",
			DisplayName:   "Agent",
			Scopes:        []string{auth.ScopeWrite},
		})
		require.NoError(t, err)
		requireStatus(t, http.StatusForbidden)(h.HandleDelegateAgentLift(ctx))
	})

	t.Run("delegation scope validation rejects forbidden base scopes", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/delegate", writeHeaders, nil, apimodels.AgentDelegationRequest{
			AgentUsername: "agent1",
			DisplayName:   "Agent",
			Scopes:        []string{"push"},
		})
		require.NoError(t, err)
		requireStatus(t, http.StatusForbidden)(h.HandleDelegateAgentLift(ctx))
	})

	t.Run("legacy agent_info is ignored for existing agents", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/delegate", writeHeaders, nil, apimodels.AgentDelegationRequest{
			AgentUsername: "agent1",
			DisplayName:   "Agent",
			Scopes:        []string{"read"},
			AgentInfo: map[string]any{
				"capabilities": 7,
			},
		})
		require.NoError(t, err)
		requireStatus(t, http.StatusOK)(h.HandleDelegateAgentLift(ctx))
	})

	t.Run("existing agent delegation requires runtime session persistence", func(t *testing.T) {
		stateConflict := &round10QueryState{
			agentInstanceConfig: policy,
			usersByUsername: map[string]storagemodels.User{
				"owner":  state.usersByUsername["owner"],
				"agent1": state.usersByUsername["agent1"],
			},
			agentGovernanceByUsername: map[string]storagemodels.AgentGovernanceState{
				"agent1": state.agentGovernanceByUsername["agent1"],
			},
			createErrorOnce: dynamormerrors.ErrConditionFailed,
		}

		hConflict, _, _ := round11NewHandler(t, cfg, stateConflict)
		hConflict.repos.Account().SetEncryptor(noopEncryptor{})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/delegate", writeHeaders, nil, apimodels.AgentDelegationRequest{
			AgentUsername: "agent1",
			Scopes:        []string{"read"},
		})
		require.NoError(t, err)
		resp := requireStatus(t, http.StatusInternalServerError)(hConflict.HandleDelegateAgentLift(ctx))
		require.NotEmpty(t, resp.Body)
	})

	t.Run("missing agent returns 404", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/delegate", writeHeaders, nil, apimodels.AgentDelegationRequest{
			AgentUsername: "missing-agent",
			Scopes:        []string{"read"},
		})
		require.NoError(t, err)
		resp := requireStatus(t, http.StatusNotFound)(h.HandleDelegateAgentLift(ctx))
		require.NotEmpty(t, resp.Body)
	})

	t.Run("agent delegated scope envelope is enforced", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/delegate", writeHeaders, nil, apimodels.AgentDelegationRequest{
			AgentUsername: "agent1",
			Scopes:        []string{"follow"},
		})
		require.NoError(t, err)
		resp := requireStatus(t, http.StatusForbidden)(h.HandleDelegateAgentLift(ctx))
		require.NotEmpty(t, resp.Body)
	})

	t.Run("validateAgentAccessTokenTTL boundaries", func(t *testing.T) {
		ctx := &apptheory.Context{}
		_, resp, err := validateAgentAccessTokenTTL(ctx, 59)
		require.NoError(t, err)
		require.NotNil(t, resp)

		_, resp, err = validateAgentAccessTokenTTL(ctx, 8*24*60*60)
		require.NoError(t, err)
		require.NotNil(t, resp)
	})

	t.Run("authenticateAgentOwner rejects insufficient scope", func(t *testing.T) {
		ownerToken := round11SignAccessToken(t, cfg.JWTSecret, "owner", []string{auth.ScopeRead})
		headers := map[string]string{"Authorization": "Bearer " + ownerToken}
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/delegate", headers, nil, apimodels.AgentDelegationRequest{
			AgentUsername: "agent1",
			DisplayName:   "Agent",
			Scopes:        []string{"read"},
		})
		require.NoError(t, err)

		claims, resp, authErr := h.authenticateAgentOwner(ctx)
		require.Nil(t, claims)
		require.NoError(t, authErr)
		require.NotNil(t, resp)
	})

	t.Run("authenticateAgentOwner rejects missing token", func(t *testing.T) {
		ctx := &apptheory.Context{}
		claims, resp, authErr := h.authenticateAgentOwner(ctx)
		require.Nil(t, claims)
		require.NoError(t, authErr)
		require.NotNil(t, resp)
	})
}
