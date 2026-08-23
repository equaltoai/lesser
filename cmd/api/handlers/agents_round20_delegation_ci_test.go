package handlers

import (
	"errors"
	"net/http"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestAgentsRound20_AuthenticateAgentOwner(t *testing.T) {
	cfg := round10TestConfig()
	cfg.AllowAgents = true

	h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

	t.Run("missing bearer token returns unauthorized", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/delegate", nil, nil, nil)
		require.NoError(t, err)

		_, resp, respErr := h.authenticateAgentOwner(ctx)
		requireStatus(t, http.StatusUnauthorized)(resp, respErr)
	})

	t.Run("invalid bearer token returns unauthorized", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/delegate", map[string]string{
			"Authorization": "Bearer definitely-not-a-jwt",
		}, nil, nil)
		require.NoError(t, err)

		_, resp, respErr := h.authenticateAgentOwner(ctx)
		requireStatus(t, http.StatusUnauthorized)(resp, respErr)
	})

	t.Run("missing account write scope returns forbidden", func(t *testing.T) {
		token := round11SignAccessToken(t, cfg.JWTSecret, "owner", []string{auth.ScopeRead})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/delegate", map[string]string{
			"Authorization": "Bearer " + token,
		}, nil, nil)
		require.NoError(t, err)

		_, resp, respErr := h.authenticateAgentOwner(ctx)
		requireStatus(t, http.StatusForbidden)(resp, respErr)
	})

	t.Run("write scoped owner token is accepted", func(t *testing.T) {
		token := round11SignAccessToken(t, cfg.JWTSecret, "owner", []string{auth.ScopeWrite})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/delegate", map[string]string{
			"Authorization": "Bearer " + token,
		}, nil, nil)
		require.NoError(t, err)

		claims, resp, respErr := h.authenticateAgentOwner(ctx)
		require.NoError(t, respErr)
		require.Nil(t, resp)
		require.NotNil(t, claims)
		require.Equal(t, "owner", claims.Username)
	})
}

func TestAgentsRound20_ResolveDelegatedAgentAccount_Branches(t *testing.T) {
	t.Run("missing repositories returns internal server error", func(t *testing.T) {
		h := &Handler{}
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/delegate", nil, nil, nil)
		require.NoError(t, err)

		account, resp, respErr := h.resolveDelegatedAgentAccount(ctx, &auth.Claims{Username: "owner"}, "agent1", []string{"read"})
		require.Nil(t, account)
		requireStatus(t, http.StatusInternalServerError)(resp, respErr)
	})

	t.Run("missing agent returns not found", func(t *testing.T) {
		cfg := round10TestConfig()
		cfg.AllowAgents = true

		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/delegate", nil, nil, nil)
		require.NoError(t, err)

		account, resp, respErr := h.resolveDelegatedAgentAccount(ctx, &auth.Claims{Username: "owner", Scopes: []string{auth.ScopeWrite}}, "missing", []string{"read"})
		require.Nil(t, account)
		requireStatus(t, http.StatusNotFound)(resp, respErr)
	})

	t.Run("account lookup errors return internal server error", func(t *testing.T) {
		cfg := round10TestConfig()
		cfg.AllowAgents = true

		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{
			firstErrorOnce: errors.New("boom"),
		})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/delegate", nil, nil, nil)
		require.NoError(t, err)

		account, resp, respErr := h.resolveDelegatedAgentAccount(ctx, &auth.Claims{Username: "owner", Scopes: []string{auth.ScopeWrite}}, "agent1", []string{"read"})
		require.Nil(t, account)
		requireStatus(t, http.StatusInternalServerError)(resp, respErr)
	})

	t.Run("suspended agent returns not found", func(t *testing.T) {
		cfg := round10TestConfig()
		cfg.AllowAgents = true

		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{
			usersByUsername: map[string]storagemodels.User{
				"agent1": {
					PK:         "USER#agent1",
					SK:         storagemodels.SKMetadata,
					Username:   "agent1",
					Approved:   true,
					Version:    1,
					IsAgent:    true,
					Suspended:  true,
					AgentOwner: "@owner",
				},
			},
		})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/delegate", nil, nil, nil)
		require.NoError(t, err)

		account, resp, respErr := h.resolveDelegatedAgentAccount(ctx, &auth.Claims{Username: "owner", Scopes: []string{auth.ScopeWrite}}, "agent1", []string{"read"})
		require.Nil(t, account)
		requireStatus(t, http.StatusNotFound)(resp, respErr)
	})

	t.Run("non-owner without admin scope returns forbidden", func(t *testing.T) {
		cfg := round10TestConfig()
		cfg.AllowAgents = true

		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{
			usersByUsername: map[string]storagemodels.User{
				"agent1": {
					PK:         "USER#agent1",
					SK:         storagemodels.SKMetadata,
					Username:   "agent1",
					Approved:   true,
					Version:    1,
					IsAgent:    true,
					AgentOwner: "@owner",
				},
			},
		})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/delegate", nil, nil, nil)
		require.NoError(t, err)

		account, resp, respErr := h.resolveDelegatedAgentAccount(ctx, &auth.Claims{Username: "intruder", Scopes: []string{auth.ScopeWrite}}, "agent1", []string{"read"})
		require.Nil(t, account)
		requireStatus(t, http.StatusForbidden)(resp, respErr)
	})

	t.Run("stored delegation envelope is enforced", func(t *testing.T) {
		cfg := round10TestConfig()
		cfg.AllowAgents = true

		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{
			usersByUsername: map[string]storagemodels.User{
				"agent1": {
					PK:         "USER#agent1",
					SK:         storagemodels.SKMetadata,
					Username:   "agent1",
					Approved:   true,
					Version:    1,
					IsAgent:    true,
					AgentOwner: "@owner",
				},
			},
			agentGovernanceByUsername: map[string]storagemodels.AgentGovernanceState{
				"agent1": {
					PK:              "USER#agent1",
					SK:              storagemodels.SKAgentGovernance,
					Username:        "agent1",
					DelegatedScopes: []string{auth.ScopeRead},
					CreatedAt:       time.Now().Add(-24 * time.Hour),
					UpdatedAt:       time.Now().Add(-time.Hour),
				},
			},
		})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/delegate", nil, nil, nil)
		require.NoError(t, err)

		account, resp, respErr := h.resolveDelegatedAgentAccount(ctx, &auth.Claims{Username: "owner", Scopes: []string{auth.ScopeWrite}}, "agent1", []string{"write:statuses"})
		require.Nil(t, account)
		requireStatus(t, http.StatusForbidden)(resp, respErr)
	})

	t.Run("missing governance fails closed", func(t *testing.T) {
		cfg := round10TestConfig()
		cfg.AllowAgents = true

		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{
			usersByUsername: map[string]storagemodels.User{
				"agent1": {
					PK:         "USER#agent1",
					SK:         storagemodels.SKMetadata,
					Username:   "agent1",
					Approved:   true,
					Version:    1,
					IsAgent:    true,
					AgentOwner: "@owner",
				},
			},
		})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/delegate", nil, nil, nil)
		require.NoError(t, err)

		account, resp, respErr := h.resolveDelegatedAgentAccount(ctx, &auth.Claims{Username: "owner", Scopes: []string{auth.ScopeWrite}}, "agent1", []string{"read"})
		require.Nil(t, account)
		requireStatus(t, http.StatusServiceUnavailable)(resp, respErr)
	})
}

func TestAgentsRound20_MintDelegatedAgentTokens_DoesNotPersistRefreshState(t *testing.T) {
	cfg := round10TestConfig()
	cfg.AllowAgents = true

	h, _, _ := round11NewHandler(t, cfg, &round10QueryState{
		createErrorOnce: errors.New("refresh token persistence failed"),
	})
	h.repos.Account().SetEncryptor(noopEncryptor{})

	ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/delegate", nil, nil, nil)
	require.NoError(t, err)

	token, mintErr := h.mintDelegatedAgentTokens(ctx, "agent1", []string{"read"}, 5*time.Minute, "", "", "", "")
	require.NoError(t, mintErr)
	require.NotEmpty(t, token.AccessToken)
	require.Empty(t, token.RefreshToken)
}

func TestAgentsRound20_MintDelegatedAgentTokens_BoundsStatelessAccessTTL(t *testing.T) {
	cfg := round10TestConfig()
	cfg.AllowAgents = true

	state := &round10QueryState{}
	h, _, _ := round11NewHandler(t, cfg, state)
	h.repos.Account().SetEncryptor(noopEncryptor{})

	ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/delegate", nil, nil, nil)
	require.NoError(t, err)

	requestedTTL := 10 * time.Minute
	token, mintErr := h.mintDelegatedAgentTokens(ctx, "agent1", []string{"read"}, requestedTTL, "test-runtime", "", "", "")
	require.NoError(t, mintErr)
	require.NotEmpty(t, token.AccessToken)
	require.Empty(t, token.RefreshToken)
	require.Equal(t, 600, token.ExpiresIn)
	require.Empty(t, state.refreshTokensByToken)
}

func TestAgentsRound20_AgentDelegationEnvelope_EmptyMetadataCases(t *testing.T) {
	scopes, ok := agentDelegationEnvelope(nil)
	require.False(t, ok)
	require.Nil(t, scopes)

	scopes, ok = agentDelegationEnvelope(&storage.AgentGovernanceState{})
	require.False(t, ok)
	require.Nil(t, scopes)
}

func TestAgentsRound20_HandleUpdateAndDeleteAgentLift_ErrorBranches(t *testing.T) {
	newAgentHandler := func(t *testing.T, state *round10QueryState) (*Handler, map[string]string, map[string]string) {
		t.Helper()

		cfg := round10TestConfig()
		cfg.AllowAgents = true

		policy := storagemodels.NewAgentInstanceConfig()
		policy.AllowAgents = true
		if state == nil {
			state = &round10QueryState{}
		}
		state.agentInstanceConfig = policy
		if state.usersByUsername == nil {
			state.usersByUsername = map[string]storagemodels.User{
				"agent1": {
					PK:         "USER#agent1",
					SK:         storagemodels.SKMetadata,
					Username:   "agent1",
					Approved:   true,
					Version:    1,
					IsAgent:    true,
					AgentOwner: "@owner",
				},
			}
		}
		if state.agentGovernanceByUsername == nil {
			state.agentGovernanceByUsername = map[string]storagemodels.AgentGovernanceState{
				"agent1": {
					PK:        "USER#agent1",
					SK:        storagemodels.SKAgentGovernance,
					Username:  "agent1",
					CreatedAt: time.Now().Add(-24 * time.Hour),
					UpdatedAt: time.Now().Add(-time.Hour),
					Version:   1,
				},
			}
		}

		h, _, _ := round11NewHandler(t, cfg, state)
		h.repos.Account().SetEncryptor(noopEncryptor{})

		ownerHeaders := map[string]string{
			"Authorization": "Bearer " + round11SignAccessToken(t, cfg.JWTSecret, "owner", []string{auth.ScopeWrite}),
		}
		intruderHeaders := map[string]string{
			"Authorization": "Bearer " + round11SignAccessToken(t, cfg.JWTSecret, "intruder", []string{auth.ScopeWrite}),
		}
		return h, ownerHeaders, intruderHeaders
	}

	t.Run("update rejects non-owner", func(t *testing.T) {
		h, _, intruderHeaders := newAgentHandler(t, nil)
		ctx, err := round10NewLiftContext(http.MethodPatch, "/api/v1/agents/agent1", intruderHeaders, nil, map[string]any{
			"display_name": "Renamed",
		})
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"

		requireStatus(t, http.StatusForbidden)(h.HandleUpdateAgentLift(ctx))
	})

	t.Run("update rejects invalid username", func(t *testing.T) {
		h, ownerHeaders, _ := newAgentHandler(t, nil)
		ctx, err := round10NewLiftContext(http.MethodPatch, "/api/v1/agents/not a username", ownerHeaders, nil, map[string]any{
			"display_name": "Renamed",
		})
		require.NoError(t, err)
		ctx.Params["username"] = "not a username"

		requireStatus(t, http.StatusBadRequest)(h.HandleUpdateAgentLift(ctx))
	})

	t.Run("update returns not found when agent is missing", func(t *testing.T) {
		h, ownerHeaders, _ := newAgentHandler(t, &round10QueryState{
			usersByUsername: map[string]storagemodels.User{},
		})
		ctx, err := round10NewLiftContext(http.MethodPatch, "/api/v1/agents/missing", ownerHeaders, nil, map[string]any{
			"display_name": "Renamed",
		})
		require.NoError(t, err)
		ctx.Params["username"] = "missing"

		requireStatus(t, http.StatusNotFound)(h.HandleUpdateAgentLift(ctx))
	})

	t.Run("update returns internal server error on persistence failure", func(t *testing.T) {
		h, ownerHeaders, _ := newAgentHandler(t, &round10QueryState{
			executeErrorOnce: errors.New("boom"),
		})
		ctx, err := round10NewLiftContext(http.MethodPatch, "/api/v1/agents/agent1", ownerHeaders, nil, map[string]any{
			"display_name": "Renamed",
		})
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"

		requireStatus(t, http.StatusInternalServerError)(h.HandleUpdateAgentLift(ctx))
	})

	t.Run("delete rejects non-owner", func(t *testing.T) {
		h, _, intruderHeaders := newAgentHandler(t, nil)
		ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/agents/agent1", intruderHeaders, nil, nil)
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"

		requireStatus(t, http.StatusForbidden)(h.HandleDeleteAgentLift(ctx))
	})

	t.Run("delete rejects invalid username", func(t *testing.T) {
		h, ownerHeaders, _ := newAgentHandler(t, nil)
		ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/agents/not a username", ownerHeaders, nil, nil)
		require.NoError(t, err)
		ctx.Params["username"] = "not a username"

		requireStatus(t, http.StatusBadRequest)(h.HandleDeleteAgentLift(ctx))
	})

	t.Run("delete returns not found when agent is missing", func(t *testing.T) {
		h, ownerHeaders, _ := newAgentHandler(t, &round10QueryState{
			usersByUsername: map[string]storagemodels.User{},
		})
		ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/agents/missing", ownerHeaders, nil, nil)
		require.NoError(t, err)
		ctx.Params["username"] = "missing"

		requireStatus(t, http.StatusNotFound)(h.HandleDeleteAgentLift(ctx))
	})

	t.Run("delete returns internal server error on persistence failure", func(t *testing.T) {
		h, ownerHeaders, _ := newAgentHandler(t, &round10QueryState{
			executeErrorOnce: errors.New("boom"),
		})
		ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/agents/agent1", ownerHeaders, nil, nil)
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"

		requireStatus(t, http.StatusInternalServerError)(h.HandleDeleteAgentLift(ctx))
	})
}

func TestAgentsRound20_AgentGovernanceHelpers(t *testing.T) {
	broadCaps := deriveAgentCapabilitiesFromScopes([]string{"write", "follow"})
	require.True(t, broadCaps.CanPost)
	require.True(t, broadCaps.CanReply)
	require.True(t, broadCaps.CanBoost)
	require.True(t, broadCaps.CanDM)
	require.True(t, broadCaps.CanFollow)

	granularCaps := deriveAgentCapabilitiesFromScopes([]string{"write:statuses", "write:follows"})
	require.True(t, granularCaps.CanPost)
	require.True(t, granularCaps.CanReply)
	require.True(t, granularCaps.CanBoost)
	require.True(t, granularCaps.CanDM)
	require.True(t, granularCaps.CanFollow)

	require.False(t, agentVerifiedState(nil))
	require.False(t, agentVerifiedState(&storage.AgentGovernanceState{}))
	require.True(t, agentVerifiedState(&storage.AgentGovernanceState{Verified: true}))

	require.Nil(t, agentDelegatedScopes(nil))
	require.Nil(t, agentDelegatedScopes(&storage.AgentGovernanceState{}))
	require.Equal(t, []string{"read", "write"}, agentDelegatedScopes(&storage.AgentGovernanceState{
		DelegatedScopes: []string{"read", "write"},
	}))
}

func TestAgentsRound20_AgentCapabilityAndActorHelpers(t *testing.T) {
	cfg := round10TestConfig()
	cfg.AllowAgents = true

	policy := storagemodels.NewAgentInstanceConfig()
	policy.AllowAgents = true
	policy.AgentMaxPostsPerHour = 10
	policy.VerifiedAgentMaxPostsPerHour = 25

	h, _, _ := round11NewHandler(t, cfg, &round10QueryState{
		agentInstanceConfig: policy,
	})

	ctx, err := round10NewLiftContext(http.MethodPatch, "/api/v1/agents/agent1", nil, nil, nil)
	require.NoError(t, err)

	t.Run("applyAgentCapabilitiesUpdate clamps to unverified and verified policy caps", func(t *testing.T) {
		user := &storage.User{}
		h.applyAgentCapabilitiesUpdate(ctx, user, nil, &apimodels.AgentCapabilities{
			CanPost:         true,
			MaxPostsPerHour: 99,
		})
		require.NotNil(t, user.AgentCapabilities)
		require.Equal(t, 10, user.AgentCapabilities.MaxPostsPerHour)

		h.applyAgentCapabilitiesUpdate(ctx, user, &storage.AgentGovernanceState{Verified: true}, &apimodels.AgentCapabilities{
			CanPost:         true,
			MaxPostsPerHour: 99,
		})
		require.Equal(t, 25, user.AgentCapabilities.MaxPostsPerHour)
	})

	t.Run("ensureAgentActor hydrates and normalizes local actor data", func(t *testing.T) {
		account := &storage.Account{
			User: &storage.User{
				Username: "agent1",
			},
		}

		h.ensureAgentActor("agent1", account)
		require.NotNil(t, account.Actor)
		require.True(t, account.User.IsAgent)
		require.Equal(t, activitypub.ServiceType, account.Actor.Type)
		require.Equal(t, "agent1", account.Actor.PreferredUsername)
	})
}
