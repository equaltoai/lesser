package handlers

import (
	"context"
	"encoding/json"
	"errors"
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
	require.False(t, newToken.LastAuthSuccessAt.IsZero())
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
	require.Equal(t, "invalid_grant", updatedCurrent.LastAuthFailureCode)
	require.Equal(t, "Refresh token reuse detected; runtime session revoked", updatedCurrent.LastAuthFailureMsg)
	require.False(t, updatedCurrent.LastAuthFailureAt.IsZero())
}

func TestOAuthRuntimeRefreshGrant_ConcurrentRefreshConflictReturnsInvalidGrant(t *testing.T) {
	cfg := round10TestConfig()
	now := time.Now().UTC()
	parentToken := buildRuntimeRefreshToken(t, "rt-agent-parent", "agent1", delegatedAgentClientID, "sid-agent-3", "family-agent-3", "local-agent", 1, true, false, now)
	childToken := buildRuntimeRefreshToken(t, "rt-agent-child", "agent1", delegatedAgentClientID, "sid-agent-3", "family-agent-3", "local-agent", 2, true, false, now)
	state := &round10QueryState{
		refreshTokensByToken: map[string]storagemodels.RefreshToken{
			parentToken.Token: parentToken,
			childToken.Token:  childToken,
		},
		updateErrorOnce: errors.New("ConditionalCheckFailedException"),
	}

	h, _, _ := round11NewHandler(t, cfg, state)
	h.repos.Account().SetEncryptor(noopEncryptor{})

	ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte("grant_type=refresh_token&refresh_token=rt-agent-parent&client_id="+delegatedAgentClientID))
	resp := requireStatus(t, http.StatusBadRequest)(h.HandleOAuthTokenLift(ctx))

	var body map[string]string
	require.NoError(t, json.Unmarshal(resp.Body, &body))
	require.Equal(t, "invalid_grant", body["error"])

	updatedChild := state.refreshTokensByToken["rt-agent-child"]
	require.Equal(t, "invalid_grant", updatedChild.LastAuthFailureCode)
	require.Equal(t, "Concurrent refresh detected; runtime session revoked", updatedChild.LastAuthFailureMsg)
	require.False(t, updatedChild.LastAuthFailureAt.IsZero())
}

func TestOAuthRuntimeRefreshGrant_ZeroIdleExpiryDoesNotRevokeFamily(t *testing.T) {
	cfg := round10TestConfig()
	now := time.Now().UTC()
	token := buildRuntimeRefreshToken(t, "rt-agent-zero-idle", "agent1", delegatedAgentClientID, "sid-agent-zero-idle", "family-agent-zero-idle", "local-agent", 1, true, false, now)
	token.IdleExpiresAt = time.Time{}
	state := &round10QueryState{
		refreshTokensByToken: map[string]storagemodels.RefreshToken{
			token.Token: token,
		},
	}

	h, _, _ := round11NewHandler(t, cfg, state)
	h.repos.Account().SetEncryptor(noopEncryptor{})

	ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte("grant_type=refresh_token&refresh_token=rt-agent-zero-idle&client_id="+delegatedAgentClientID))
	resp := requireStatus(t, http.StatusOK)(h.HandleOAuthTokenLift(ctx))

	var body apimodels.OAuthTokenResponse
	require.NoError(t, json.Unmarshal(resp.Body, &body))
	require.NotEmpty(t, body.RefreshToken)

	oldToken := state.refreshTokensByToken["rt-agent-zero-idle"]
	require.True(t, oldToken.Revoked)
	require.Equal(t, "rotated", oldToken.RevokedReason)

	newToken, ok := state.refreshTokensByToken[body.RefreshToken]
	require.True(t, ok)
	require.False(t, newToken.Revoked)
	require.Equal(t, token.AbsoluteExpiresAt, newToken.AbsoluteExpiresAt)
}

func TestOAuthRuntimeRefreshGrant_ZeroAbsoluteExpiryDoesNotRevokeFamily(t *testing.T) {
	cfg := round10TestConfig()
	now := time.Now().UTC()
	token := buildRuntimeRefreshToken(t, "rt-agent-zero-absolute", "agent1", delegatedAgentClientID, "sid-agent-zero-absolute", "family-agent-zero-absolute", "local-agent", 1, true, false, now)
	token.AbsoluteExpiresAt = time.Time{}
	state := &round10QueryState{
		refreshTokensByToken: map[string]storagemodels.RefreshToken{
			token.Token: token,
		},
	}

	h, _, _ := round11NewHandler(t, cfg, state)
	h.repos.Account().SetEncryptor(noopEncryptor{})

	ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte("grant_type=refresh_token&refresh_token=rt-agent-zero-absolute&client_id="+delegatedAgentClientID))
	resp := requireStatus(t, http.StatusOK)(h.HandleOAuthTokenLift(ctx))

	var body apimodels.OAuthTokenResponse
	require.NoError(t, json.Unmarshal(resp.Body, &body))
	require.NotEmpty(t, body.RefreshToken)

	oldToken := state.refreshTokensByToken["rt-agent-zero-absolute"]
	require.True(t, oldToken.Revoked)
	require.Equal(t, "rotated", oldToken.RevokedReason)

	newToken, ok := state.refreshTokensByToken[body.RefreshToken]
	require.True(t, ok)
	require.False(t, newToken.Revoked)
	require.False(t, newToken.AbsoluteExpiresAt.IsZero())
}

func TestOAuthRuntimeRefreshGrant_ExpiredBoundsStillRevokeFamily(t *testing.T) {
	cfg := round10TestConfig()
	now := time.Now().UTC()

	t.Run("expired idle", func(t *testing.T) {
		token := buildRuntimeRefreshToken(t, "rt-agent-expired-idle", "agent1", delegatedAgentClientID, "sid-agent-expired-idle", "family-agent-expired-idle", "local-agent", 1, true, false, now)
		token.IdleExpiresAt = now.Add(-1 * time.Minute)
		state := &round10QueryState{
			refreshTokensByToken: map[string]storagemodels.RefreshToken{
				token.Token: token,
			},
		}

		h, _, _ := round11NewHandler(t, cfg, state)
		h.repos.Account().SetEncryptor(noopEncryptor{})

		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte("grant_type=refresh_token&refresh_token=rt-agent-expired-idle&client_id="+delegatedAgentClientID))
		resp := requireStatus(t, http.StatusBadRequest)(h.HandleOAuthTokenLift(ctx))
		var body map[string]string
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "invalid_grant", body["error"])

		updated := state.refreshTokensByToken["rt-agent-expired-idle"]
		require.True(t, updated.Revoked)
		require.Equal(t, "runtime_session_expired", updated.RevokedReason)
		require.Equal(t, "invalid_grant", updated.LastAuthFailureCode)
		require.Equal(t, "Runtime session expired", updated.LastAuthFailureMsg)
		require.False(t, updated.LastAuthFailureAt.IsZero())
	})

	t.Run("expired absolute", func(t *testing.T) {
		token := buildRuntimeRefreshToken(t, "rt-agent-expired-absolute", "agent1", delegatedAgentClientID, "sid-agent-expired-absolute", "family-agent-expired-absolute", "local-agent", 1, true, false, now)
		token.AbsoluteExpiresAt = now.Add(-1 * time.Minute)
		state := &round10QueryState{
			refreshTokensByToken: map[string]storagemodels.RefreshToken{
				token.Token: token,
			},
		}

		h, _, _ := round11NewHandler(t, cfg, state)
		h.repos.Account().SetEncryptor(noopEncryptor{})

		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte("grant_type=refresh_token&refresh_token=rt-agent-expired-absolute&client_id="+delegatedAgentClientID))
		resp := requireStatus(t, http.StatusBadRequest)(h.HandleOAuthTokenLift(ctx))
		var body map[string]string
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "invalid_grant", body["error"])

		updated := state.refreshTokensByToken["rt-agent-expired-absolute"]
		require.True(t, updated.Revoked)
		require.Equal(t, "runtime_session_expired", updated.RevokedReason)
		require.Equal(t, "invalid_grant", updated.LastAuthFailureCode)
		require.Equal(t, "Runtime session expired", updated.LastAuthFailureMsg)
		require.False(t, updated.LastAuthFailureAt.IsZero())
	})
}

func TestOAuthRuntimeRefreshGrant_InvalidClientPersistsRuntimeDiagnostic(t *testing.T) {
	cfg := round10TestConfig()
	now := time.Now().UTC()
	token := buildRuntimeRefreshToken(t, "rt-agent-secret", "agent1", "client-agent-secret", "sid-agent-secret", "family-agent-secret", "connector-app", 1, true, false, now)
	activeHash, err := auth.HashOAuthClientSecret("secret-good")
	require.NoError(t, err)

	state := &round10QueryState{
		oauthClientsByID: map[string]storagemodels.OAuthClient{
			"client-agent-secret": {
				ClientID:      "client-agent-secret",
				ClientSecret:  activeHash,
				Name:          "connector-app",
				ClientClass:   auth.ClientClassAgent,
				AgentUsername: "agent1",
				Confidential:  true,
				CreatedAt:     now.Add(-24 * time.Hour),
			},
		},
		refreshTokensByToken: map[string]storagemodels.RefreshToken{
			token.Token: token,
		},
	}

	h, _, _ := round11NewHandler(t, cfg, state)
	h.repos.Account().SetEncryptor(noopEncryptor{})

	ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte("grant_type=refresh_token&refresh_token=rt-agent-secret&client_id=client-agent-secret&client_secret=secret-bad"))
	resp := requireStatus(t, http.StatusBadRequest)(h.HandleOAuthTokenLift(ctx))

	var body map[string]string
	require.NoError(t, json.Unmarshal(resp.Body, &body))
	require.Equal(t, "invalid_client", body["error"])

	updated := state.refreshTokensByToken["rt-agent-secret"]
	require.Equal(t, "invalid_client", updated.LastAuthFailureCode)
	require.Equal(t, "Invalid client credentials", updated.LastAuthFailureMsg)
	require.False(t, updated.LastAuthFailureAt.IsZero())
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
	connectorToken := buildRuntimeRefreshToken(t, "rt-connector-1", "agent1", "client-agent-1", "sid-connector-1", "family-connector-1", "connector-app", 1, true, false, now.Add(-1*time.Minute))
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
			runtimeToken.Token:   runtimeToken,
			connectorToken.Token: connectorToken,
		},
		oauthClientsByID: map[string]storagemodels.OAuthClient{
			"client-agent-1": {
				ClientID:      "client-agent-1",
				Name:          "connector-app",
				ClientClass:   auth.ClientClassAgent,
				AgentUsername: "agent1",
				CreatedAt:     now.Add(-24 * time.Hour),
			},
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
	require.Len(t, sessions, 2)
	require.Equal(t, "sid-runtime-1", sessions[0].SessionID)
	require.Equal(t, "sim-runtime", sessions[0].DeviceLabel)
	require.Equal(t, "sid-connector-1", sessions[1].SessionID)
	require.Equal(t, "connector-app", sessions[1].DeviceLabel)

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

func TestRuntimeRefreshAccessTTL_IgnoresZeroBounds(t *testing.T) {
	now := time.Now().UTC()

	t.Run("zero idle", func(t *testing.T) {
		token := &storagetypes.RefreshToken{
			AccessTTLSeconds:  3600,
			IdleExpiresAt:     time.Time{},
			AbsoluteExpiresAt: now.Add(45 * time.Minute),
		}
		ttl := runtimeRefreshAccessTTL(round10TestConfig(), token)
		require.Greater(t, ttl, 0*time.Second)
		require.LessOrEqual(t, ttl, 45*time.Minute)
	})

	t.Run("zero absolute", func(t *testing.T) {
		token := &storagetypes.RefreshToken{
			AccessTTLSeconds:  3600,
			IdleExpiresAt:     now.Add(30 * time.Minute),
			AbsoluteExpiresAt: time.Time{},
		}
		ttl := runtimeRefreshAccessTTL(round10TestConfig(), token)
		require.Greater(t, ttl, 0*time.Second)
		require.LessOrEqual(t, ttl, 30*time.Minute)
	})

	t.Run("both zero", func(t *testing.T) {
		token := &storagetypes.RefreshToken{
			AccessTTLSeconds:  1800,
			IdleExpiresAt:     time.Time{},
			AbsoluteExpiresAt: time.Time{},
		}
		ttl := runtimeRefreshAccessTTL(round10TestConfig(), token)
		require.Greater(t, ttl, 29*time.Minute)
		require.LessOrEqual(t, ttl, 30*time.Minute)
	})
}

func TestRuntimeRefreshAccessTTL_FallsBackWhenStoredTTLIsNonPositive(t *testing.T) {
	cfg := round10TestConfig()
	now := time.Now().UTC()

	t.Run("zero stored ttl uses config default", func(t *testing.T) {
		token := &storagetypes.RefreshToken{
			AccessTTLSeconds:  0,
			IdleExpiresAt:     now.Add(90 * time.Minute),
			AbsoluteExpiresAt: now.Add(2 * time.Hour),
		}

		ttl := runtimeRefreshAccessTTL(cfg, token)
		require.Equal(t, auth.AgentAccessTokenTTL(cfg), ttl)
	})

	t.Run("negative stored ttl still respects nearer runtime bound", func(t *testing.T) {
		token := &storagetypes.RefreshToken{
			AccessTTLSeconds:  -1,
			IdleExpiresAt:     now.Add(20 * time.Minute),
			AbsoluteExpiresAt: now.Add(45 * time.Minute),
		}

		ttl := runtimeRefreshAccessTTL(cfg, token)
		require.Greater(t, ttl, 0*time.Second)
		require.LessOrEqual(t, ttl, 20*time.Minute)
	})
}

func TestNoteAgentRuntimeRefreshFailure_NoOpBranches(t *testing.T) {
	cfg := round10TestConfig()
	now := time.Now().UTC()
	token := buildRuntimeRefreshToken(t, "rt-agent-note", "agent1", "client-agent-note", "sid-agent-note", "family-agent-note", "connector-app", 1, true, false, now)
	state := &round10QueryState{
		refreshTokensByToken: map[string]storagemodels.RefreshToken{
			token.Token: token,
		},
	}

	h, _, _ := round11NewHandler(t, cfg, state)
	h.repos.Account().SetEncryptor(noopEncryptor{})

	h.noteAgentRuntimeRefreshFailure(context.Background(), "", token.ClientID, "invalid_client", "Invalid client credentials")
	h.noteAgentRuntimeRefreshFailure(context.Background(), token.Token, "different-client", "invalid_client", "Invalid client credentials")

	updated := state.refreshTokensByToken[token.Token]
	require.Empty(t, updated.LastAuthFailureCode)
	require.True(t, updated.LastAuthFailureAt.IsZero())
}

func TestOAuthRuntimeRefreshGrant_UpdateFailureReturnsRawError(t *testing.T) {
	cfg := round10TestConfig()
	now := time.Now().UTC()
	token := buildRuntimeRefreshToken(t, "rt-agent-update-fail", "agent1", delegatedAgentClientID, "sid-agent-update-fail", "family-agent-update-fail", "local-agent", 1, true, false, now)
	state := &round10QueryState{
		refreshTokensByToken: map[string]storagemodels.RefreshToken{
			token.Token: token,
		},
		updateErrorOnce: errors.New("update failed"),
	}

	h, _, _ := round11NewHandler(t, cfg, state)
	h.repos.Account().SetEncryptor(noopEncryptor{})
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, nil)

	accessToken, newRefreshToken, scopes, err := h.exchangeAgentRuntimeRefreshToken(context.Background(), oauthSvc, token.Token, delegatedAgentClientID, "", "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "Failed to update refresh token")
	require.Empty(t, accessToken)
	require.Empty(t, newRefreshToken)
	require.Nil(t, scopes)

	updated := state.refreshTokensByToken[token.Token]
	require.False(t, updated.Revoked)
	require.Empty(t, updated.RevokedReason)
	require.Empty(t, updated.LastAuthFailureCode)
}

func TestOAuthRuntimeRefreshGrant_CreateFailureRevokesFamilyAndPersistsDiagnostic(t *testing.T) {
	cfg := round10TestConfig()
	now := time.Now().UTC()
	token := buildRuntimeRefreshToken(t, "rt-agent-create-fail", "agent1", delegatedAgentClientID, "sid-agent-create-fail", "family-agent-create-fail", "local-agent", 1, true, false, now)
	state := &round10QueryState{
		refreshTokensByToken: map[string]storagemodels.RefreshToken{
			token.Token: token,
		},
		createErrorOnce: errors.New("create failed"),
	}

	h, _, _ := round11NewHandler(t, cfg, state)
	h.repos.Account().SetEncryptor(noopEncryptor{})
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, nil)

	accessToken, newRefreshToken, scopes, err := h.exchangeAgentRuntimeRefreshToken(context.Background(), oauthSvc, token.Token, delegatedAgentClientID, "198.51.100.10", "agent/1.0")
	require.Error(t, err)
	require.Contains(t, err.Error(), "Failed to create refresh token")
	require.Empty(t, accessToken)
	require.Empty(t, newRefreshToken)
	require.Nil(t, scopes)

	updated := state.refreshTokensByToken[token.Token]
	require.True(t, updated.Revoked)
	require.Equal(t, "rotated", updated.RevokedReason)
	require.Equal(t, "server_error", updated.LastAuthFailureCode)
	require.Equal(t, "Runtime session rotation failed", updated.LastAuthFailureMsg)
	require.False(t, updated.LastAuthFailureAt.IsZero())
	require.False(t, updated.ReuseDetectedAt.IsZero())
}
