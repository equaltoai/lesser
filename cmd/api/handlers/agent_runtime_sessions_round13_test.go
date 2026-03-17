package handlers

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	storagetypes "github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func buildRuntimeRefreshToken(t *testing.T, token, username, clientID, sessionID, familyID, deviceLabel string, generation int, current, revoked bool, now time.Time) storagemodels.RefreshToken {
	t.Helper()

	record := storagemodels.RefreshToken{
		Token:             token,
		ClientID:          clientID,
		Username:          username,
		Scopes:            []string{auth.ScopeRead},
		CreatedAt:         now.Add(-30 * time.Minute),
		ExpiresAt:         now.Add(24 * time.Hour),
		ClientClass:       auth.ClientClassAgent,
		SessionID:         sessionID,
		FamilyID:          familyID,
		Generation:        generation,
		Current:           current,
		Revoked:           revoked,
		DeviceLabel:       deviceLabel,
		LastUsedAt:        now.Add(-5 * time.Minute),
		IdleExpiresAt:     now.Add(24 * time.Hour),
		AbsoluteExpiresAt: now.Add(7 * 24 * time.Hour),
		SessionCreatedAt:  now.Add(-48 * time.Hour),
		AccessTTLSeconds:  1800,
	}
	if revoked {
		record.RevokedAt = now.Add(-2 * time.Minute)
		record.RevokedReason = "rotated"
	}
	require.NoError(t, record.BeforeCreate())
	return record
}

func TestOAuthRuntimeRefreshGrant_SelfSovereignRotatesSession(t *testing.T) {
	cfg := round10TestConfig()
	now := time.Now().UTC()
	state := &round10QueryState{
		refreshTokensByToken: map[string]storagemodels.RefreshToken{
			"rt-agent-1": buildRuntimeRefreshToken(t, "rt-agent-1", "agent1", selfSovereignAgentClientID, "sid-agent-1", "family-agent-1", "sim-runtime", 1, true, false, now),
		},
	}

	h, _, _ := round11NewHandler(t, cfg, state)
	h.repos.Account().SetEncryptor(noopEncryptor{})

	ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte("grant_type=refresh_token&refresh_token=rt-agent-1&client_id="+selfSovereignAgentClientID))
	resp := requireStatus(t, http.StatusOK)(h.HandleOAuthTokenLift(ctx))

	var body apimodels.OAuthTokenResponse
	require.NoError(t, json.Unmarshal(resp.Body, &body))
	require.NotEmpty(t, body.AccessToken)
	require.NotEmpty(t, body.RefreshToken)
	require.NotEqual(t, "rt-agent-1", body.RefreshToken)

	oldToken := state.refreshTokensByToken["rt-agent-1"]
	require.True(t, oldToken.Revoked)
	require.False(t, oldToken.Current)

	newToken, ok := state.refreshTokensByToken[body.RefreshToken]
	require.True(t, ok)
	require.True(t, newToken.Current)
	require.False(t, newToken.Revoked)
	require.Equal(t, oldToken.SessionID, newToken.SessionID)
	require.Equal(t, oldToken.FamilyID, newToken.FamilyID)
	require.Equal(t, oldToken.Generation+1, newToken.Generation)
}

func TestOAuthRuntimeRefreshGrant_ReusedTokenRevokesFamily(t *testing.T) {
	cfg := round10TestConfig()
	now := time.Now().UTC()
	oldToken := buildRuntimeRefreshToken(t, "rt-agent-old", "agent1", delegatedAgentClientID, "sid-agent-2", "family-agent-2", "local-agent", 1, false, true, now)
	currentToken := buildRuntimeRefreshToken(t, "rt-agent-current", "agent1", delegatedAgentClientID, "sid-agent-2", "family-agent-2", "local-agent", 2, true, false, now)
	state := &round10QueryState{
		refreshTokensByToken: map[string]storagemodels.RefreshToken{
			oldToken.Token:     oldToken,
			currentToken.Token: currentToken,
		},
	}

	h, _, _ := round11NewHandler(t, cfg, state)
	h.repos.Account().SetEncryptor(noopEncryptor{})

	ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte("grant_type=refresh_token&refresh_token=rt-agent-old&client_id="+delegatedAgentClientID))
	resp := requireStatus(t, http.StatusBadRequest)(h.HandleOAuthTokenLift(ctx))

	var body map[string]string
	require.NoError(t, json.Unmarshal(resp.Body, &body))
	require.Equal(t, "invalid_grant", body["error"])

	updatedCurrent := state.refreshTokensByToken["rt-agent-current"]
	require.True(t, updatedCurrent.Revoked)
	require.Equal(t, "refresh_token_reuse_detected", updatedCurrent.RevokedReason)
	require.False(t, updatedCurrent.ReuseDetectedAt.IsZero())
}

func TestAgentRuntimeSessions_ListAndRevoke(t *testing.T) {
	cfg := round10TestConfig()
	cfg.AllowAgents = true
	now := time.Now().UTC()

	owner := storagemodels.User{Username: "owner", Role: "user", Approved: true}
	agent := storagemodels.User{
		Username:   "agent1",
		Role:       "user",
		Approved:   true,
		IsAgent:    true,
		AgentOwner: "@owner",
	}
	runtimeToken := buildRuntimeRefreshToken(t, "rt-runtime-1", "agent1", delegatedAgentClientID, "sid-runtime-1", "family-runtime-1", "sim-runtime", 1, true, false, now)
	state := &round10QueryState{
		agentInstanceConfig: &storagemodels.AgentInstanceConfig{
			AllowAgents:            true,
			AllowAgentRegistration: true,
		},
		usersByUsername: map[string]storagemodels.User{
			"owner":  owner,
			"agent1": agent,
		},
		actorsByUser: map[string]storagemodels.Actor{
			"agent1": {
				Username: "agent1",
				Actor: &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID:   "https://example.com/users/agent1",
						Type: activitypub.ServiceType,
					},
					PreferredUsername: "agent1",
				},
			},
		},
		refreshTokensByToken: map[string]storagemodels.RefreshToken{
			runtimeToken.Token: runtimeToken,
		},
	}

	h, _, _ := round11NewHandler(t, cfg, state)
	h.repos.Account().SetEncryptor(noopEncryptor{})

	ownerToken := round11SignAccessToken(t, cfg.JWTSecret, "owner", []string{"write:accounts", auth.ScopeWrite})
	headers := map[string]string{"Authorization": "Bearer " + ownerToken}

	listCtx, err := round10NewLiftContext(http.MethodGet, "/api/v1/agents/agent1/runtime-sessions", headers, nil, nil)
	require.NoError(t, err)
	listCtx.Params["username"] = "agent1"
	listResp := requireStatus(t, http.StatusOK)(h.HandleListAgentRuntimeSessionsLift(listCtx))

	var sessions []apimodels.AgentRuntimeSession
	require.NoError(t, json.Unmarshal(listResp.Body, &sessions))
	require.Len(t, sessions, 1)
	require.Equal(t, "sid-runtime-1", sessions[0].SessionID)
	require.Equal(t, "sim-runtime", sessions[0].DeviceLabel)

	revokeCtx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent1/runtime-sessions/sid-runtime-1/revoke", headers, nil, apimodels.RevokeAgentRuntimeSessionRequest{
		Reason: "operator_retired_runtime",
	})
	require.NoError(t, err)
	revokeCtx.Params["username"] = "agent1"
	revokeCtx.Params["sessionID"] = "sid-runtime-1"
	revokeResp := requireStatus(t, http.StatusOK)(h.HandleRevokeAgentRuntimeSessionLift(revokeCtx))

	var revoked apimodels.AgentRuntimeSession
	require.NoError(t, json.Unmarshal(revokeResp.Body, &revoked))
	require.True(t, revoked.Revoked)
	require.Equal(t, "operator_retired_runtime", revoked.RevokedReason)

	updated := state.refreshTokensByToken["rt-runtime-1"]
	require.True(t, updated.Revoked)
	require.Equal(t, "operator_retired_runtime", updated.RevokedReason)
}

func TestRuntimeRefreshAccessTTL_UsesStoredBounds(t *testing.T) {
	now := time.Now().UTC()
	token := &storagetypes.RefreshToken{
		AccessTTLSeconds:  3600,
		IdleExpiresAt:     now.Add(30 * time.Minute),
		AbsoluteExpiresAt: now.Add(45 * time.Minute),
	}

	ttl := runtimeRefreshAccessTTL(round10TestConfig(), token)
	require.Greater(t, ttl, 0*time.Second)
	require.LessOrEqual(t, ttl, 30*time.Minute)
}
