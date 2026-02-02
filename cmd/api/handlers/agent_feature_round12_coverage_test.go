package handlers

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

type noopEncryptor struct{}

func (noopEncryptor) Encrypt(plaintext []byte) ([]byte, error) {
	return plaintext, nil
}

func (noopEncryptor) Decrypt(ciphertext []byte) ([]byte, error) {
	return ciphertext, nil
}

func round12SignClaims(t *testing.T, secret string, claims auth.Claims) string {
	t.Helper()

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	require.NoError(t, err)
	return signed
}

func round12SignAgentAccessToken(t *testing.T, secret, username string, scopes []string) string {
	t.Helper()

	now := time.Now()
	claims := auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   username,
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(1 * time.Hour)),
		},
		Username:  username,
		ClientID:  "test-client",
		Scopes:    scopes,
		SessionID: "sess-agent",
		DeviceID:  "device-agent",
		IsAgent:   true,
		AgentType: agentTypeCustom,
	}

	return round12SignClaims(t, secret, claims)
}

func TestAgentFeaturesRound12_DelegateAndScopes(t *testing.T) {
	cfg := round10TestConfig()
	cfg.AllowAgents = true
	cfg.AllowAgentRegistration = true

	now := time.Now()
	policy := storagemodels.NewAgentInstanceConfig()
	policy.AllowAgents = true
	policy.AllowAgentRegistration = true
	policy.DefaultQuarantineDays = 3
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
				CreatedAt: now.Add(-24 * time.Hour),
			},
			"agent1": {
				PK:           "USER#agent1",
				SK:           storagemodels.SKMetadata,
				Username:     "agent1",
				Role:         "user",
				Approved:     true,
				Version:      1,
				CreatedAt:    now.Add(-24 * time.Hour),
				IsAgent:      true,
				AgentOwner:   "@owner",
				AgentType:    agentTypeCustom,
				AgentVersion: "v1",
			},
		},
	}

	h, _, _ := round11NewHandler(t, cfg, state)
	h.repos.Account().SetEncryptor(noopEncryptor{})

	ownerToken := round11SignAccessToken(t, cfg.JWTSecret, "owner", []string{auth.ScopeWrite, auth.ScopeRead, "follow"})
	headers := map[string]string{"Authorization": "Bearer " + ownerToken}

	t.Run("success_delegates_agent", func(t *testing.T) {
		req := apimodels.AgentDelegationRequest{
			AgentUsername: "agent1",
			DisplayName:   "Agent One",
			Scopes:        []string{"write:statuses"},
		}

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/delegate", headers, nil, req)
		require.NoError(t, err)

		resp := requireStatus(t, http.StatusOK)(h.HandleDelegateAgentLift(ctx))

		var out apimodels.AgentDelegationResponse
		require.NoError(t, json.Unmarshal(resp.Body, &out))
		require.NotEmpty(t, out.Token.AccessToken)
		require.Equal(t, "Bearer", out.Token.TokenType)
	})

	t.Run("rejects_admin_scope", func(t *testing.T) {
		req := apimodels.AgentDelegationRequest{
			AgentUsername: "agent1",
			DisplayName:   "Agent One",
			Scopes:        []string{auth.ScopeAdmin},
		}

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/delegate", headers, nil, req)
		require.NoError(t, err)

		requireStatus(t, http.StatusForbidden)(h.HandleDelegateAgentLift(ctx))
	})
}

func TestAgentFeaturesRound12_AdminPolicyAndVerification(t *testing.T) {
	cfg := round10TestConfig()
	cfg.AllowAgents = true

	policy := storagemodels.NewAgentInstanceConfig()
	policy.AllowAgents = true
	policy.AllowAgentRegistration = true
	policy.AllowRemoteAgents = true
	policy.RemoteQuarantineDays = 7

	now := time.Now()
	state := &round10QueryState{
		agentInstanceConfig: policy,
		usersByUsername: map[string]storagemodels.User{
			"admin": {
				PK:        "USER#admin",
				SK:        storagemodels.SKMetadata,
				Username:  "admin",
				Role:      "admin",
				Approved:  true,
				Version:   1,
				CreatedAt: now.Add(-24 * time.Hour),
			},
			"agent1": {
				PK:           "USER#agent1",
				SK:           storagemodels.SKMetadata,
				Username:     "agent1",
				Role:         "user",
				Approved:     true,
				Version:      1,
				CreatedAt:    now.Add(-24 * time.Hour),
				IsAgent:      true,
				AgentOwner:   "@owner",
				AgentType:    agentTypeCustom,
				AgentVersion: "v1",
			},
		},
	}

	h, _, _ := round11NewHandler(t, cfg, state)
	h.repos.Account().SetEncryptor(noopEncryptor{})

	adminToken := round11SignAccessToken(t, cfg.JWTSecret, "admin", []string{auth.ScopeAdmin})
	headers := map[string]string{"Authorization": "Bearer " + adminToken}

	t.Run("update_policy", func(t *testing.T) {
		req := apimodels.UpdateAdminAgentPolicyRequest{
			AllowAgents:            true,
			AllowAgentRegistration: true,
			DefaultQuarantineDays:  7,
			MaxAgentsPerOwner:      3,
			AllowRemoteAgents:      true,
			RemoteQuarantineDays:   7,
			BlockedAgentDomains:    []string{"HTTP://Bad.EXAMPLE/", "bad.example"},
			TrustedAgentDomains:    []string{"trusted.example"},
			AgentMaxPostsPerHour:   50,
		}

		ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/admin/agents/policy", headers, nil, req)
		require.NoError(t, err)

		resp := requireStatus(t, http.StatusOK)(h.HandleAdminUpdateAgentPolicyLift(ctx))

		var out apimodels.AdminAgentPolicy
		require.NoError(t, json.Unmarshal(resp.Body, &out))
		require.True(t, out.AllowAgents)
		require.True(t, out.AllowAgentRegistration)
		require.Contains(t, out.BlockedAgentDomains, "bad.example")
	})

	t.Run("verify_and_unverify_agent", func(t *testing.T) {
		verifyReq := apimodels.AdminVerifyAgentRequest{
			Reason:         "ok",
			ExitQuarantine: true,
		}
		verifyCtx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/agents/agent1/verify", headers, nil, verifyReq)
		require.NoError(t, err)
		verifyCtx.Params["username"] = "agent1"

		verifyResp := requireStatus(t, http.StatusOK)(h.HandleAdminVerifyAgentLift(verifyCtx))
		var verified apimodels.Agent
		require.NoError(t, json.Unmarshal(verifyResp.Body, &verified))
		require.True(t, verified.Verified)

		unverifyReq := apimodels.AdminVerifyAgentRequest{Reason: "nope"}
		unverifyCtx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/agents/agent1/unverify", headers, nil, unverifyReq)
		require.NoError(t, err)
		unverifyCtx.Params["username"] = "agent1"

		unverifyResp := requireStatus(t, http.StatusOK)(h.HandleAdminUnverifyAgentLift(unverifyCtx))
		var unverified apimodels.Agent
		require.NoError(t, json.Unmarshal(unverifyResp.Body, &unverified))
		require.False(t, unverified.Verified)
	})
}

func TestAgentFeaturesRound12_StatusAttributionAndMemoryEvents(t *testing.T) {
	cfg := round10TestConfig()
	cfg.AllowAgents = true

	now := time.Now().UTC()
	policy := storagemodels.NewAgentInstanceConfig()
	policy.AllowAgents = true

	state := &round10QueryState{
		agentInstanceConfig: policy,
		usersByUsername: map[string]storagemodels.User{
			"agent": {
				PK:           "USER#agent",
				SK:           storagemodels.SKMetadata,
				Username:     "agent",
				Role:         "user",
				Approved:     true,
				Version:      1,
				CreatedAt:    now.Add(-24 * time.Hour),
				IsAgent:      true,
				AgentOwner:   "@owner",
				AgentType:    agentTypeCustom,
				AgentVersion: "v1",
			},
		},
		statusByID: map[string]storagemodels.Status{
			"orig1": {
				StatusID:       "orig1",
				AuthorUsername: "agent",
				Content:        "original",
				PublishedAt:    now.Add(-1 * time.Hour),
			},
		},
	}

	h, _, _ := round11NewHandler(t, cfg, state)
	h.repos.Account().SetEncryptor(noopEncryptor{})

	claims := &auth.Claims{Username: "agent", IsAgent: true, Scopes: []string{auth.ScopeRead, auth.ScopeWrite}}
	ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses", nil, nil, nil)
	require.NoError(t, err)

	req := &apimodels.CreateStatusRequest{
		Status: "correction",
		MemoryEvent: &apimodels.AgentMemoryEventRequest{
			EventType:  "correction",
			OriginalID: "orig1",
		},
	}

	resp, err := h.normalizeAgentMemoryEventRequest(ctx, claims, req)
	require.NoError(t, err)
	require.Nil(t, resp)
	require.Equal(t, "orig1", req.InReplyToID)

	req.AgentAttribution = &apimodels.AgentPostAttribution{
		TriggerType:    "mention",
		TriggerDetails: "hello",
		MemoryCitations: []string{
			"orig1",
			"orig1",
		},
	}

	attribution, resp, err := h.buildAgentStatusAttribution(ctx, claims, req)
	require.NoError(t, err)
	require.Nil(t, resp)
	require.NotNil(t, attribution)
	require.Equal(t, "mention", attribution.TriggerType)
	require.Equal(t, "@owner", attribution.DelegatedBy)
	require.Len(t, attribution.MemoryCitations, 1)
}

func TestAgentFeaturesRound12_MemorySearchTimeline(t *testing.T) {
	cfg := round10TestConfig()
	cfg.AllowAgents = true

	policy := storagemodels.NewAgentInstanceConfig()
	policy.AllowAgents = true

	now := time.Now().UTC()
	state := &round10QueryState{
		agentInstanceConfig: policy,
		statusByID: map[string]storagemodels.Status{
			"s1": {
				StatusID:       "s1",
				AuthorUsername: "agent",
				Content:        "hello world",
				PublishedAt:    now.Add(-10 * time.Minute),
				Hashtags:       []string{"tag1"},
			},
		},
		agentMemoryEventsByAgent: map[string][]storagemodels.AgentMemoryEvent{
			"agent": {{
				EventID:       "e1",
				EventType:     storagemodels.MemoryEventCreate,
				StatusID:      "s1",
				OriginalID:    "s1",
				AgentUsername: "agent",
				Timestamp:     now.Add(-9 * time.Minute),
				CreatedAt:     now.Add(-9 * time.Minute),
			}},
		},
	}

	h, _, _ := round11NewHandler(t, cfg, state)
	h.repos.Account().SetEncryptor(noopEncryptor{})

	agentToken := round12SignAgentAccessToken(t, cfg.JWTSecret, "agent", []string{auth.ScopeRead})
	headers := map[string]string{"Authorization": "Bearer " + agentToken}
	query := map[string]string{
		"query": "hello",
		"tags":  "tag1",
		"mode":  "timeline",
	}

	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/agents/memory/search", headers, query, nil)
	require.NoError(t, err)

	resp := requireStatus(t, http.StatusOK)(h.HandleAgentMemorySearchLift(ctx))
	var out apimodels.AgentMemorySearchResponse
	require.NoError(t, json.Unmarshal(resp.Body, &out))
	require.Len(t, out.Results, 1)
	require.Equal(t, 1, out.Total)
	require.NotNil(t, out.Results[0].Status)
	require.NotNil(t, out.Results[0].Context)
	require.Equal(t, "s1", out.Results[0].Context.OriginalID)
}

func TestAgentFeaturesRound12_SelfSovereignRegistration(t *testing.T) {
	cfg := round10TestConfig()
	cfg.AllowAgents = true
	cfg.AllowAgentRegistration = true

	policy := storagemodels.NewAgentInstanceConfig()
	policy.AllowAgents = true
	policy.AllowAgentRegistration = true
	policy.AgentMaxPostsPerHour = 25
	policy.DefaultQuarantineDays = 1

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	publicKey := base64.StdEncoding.EncodeToString(pub)

	now := time.Now().UTC()
	expiresAt := now.Add(10 * time.Minute)
	challengeID := "challenge-1"
	message := buildAgentKeyChallengeMessage(challengeID, agentKeyActionRegister, "agent", "nonce", now, expiresAt)
	signature := base64.StdEncoding.EncodeToString(ed25519.Sign(priv, []byte(message)))

	challenge := storagemodels.AgentKeyChallenge{
		PK:        "AGENT_KEY_CHALLENGE#" + challengeID,
		SK:        "CHALLENGE",
		TTL:       expiresAt.Unix(),
		ID:        challengeID,
		Username:  "agent",
		Action:    agentKeyActionRegister,
		Nonce:     "nonce",
		Message:   message,
		IssuedAt:  now,
		ExpiresAt: expiresAt,
		Used:      false,
	}

	state := &round10QueryState{
		agentInstanceConfig: policy,
		agentKeyChallengesByID: map[string]storagemodels.AgentKeyChallenge{
			challengeID: challenge,
		},
		usersByUsername: map[string]storagemodels.User{
			"agent": {
				PK:           "USER#agent",
				SK:           storagemodels.SKMetadata,
				Username:     "agent",
				Role:         "user",
				Approved:     true,
				Version:      1,
				CreatedAt:    now.Add(-24 * time.Hour),
				IsAgent:      true,
				AgentType:    agentTypeCustom,
				AgentVersion: "v1",
			},
		},
	}

	h, _, _ := round11NewHandler(t, cfg, state)
	h.repos.Account().SetEncryptor(noopEncryptor{})

	req := apimodels.AgentSelfRegistrationRequest{
		Username:    "agent",
		DisplayName: "Agent",
		PublicKey:   publicKey,
		KeyType:     "ed25519",
		ChallengeID: challengeID,
		Signature:   signature,
		Scopes:      []string{"read", "write:statuses"},
	}

	ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/register", nil, nil, req)
	require.NoError(t, err)

	resp := requireStatus(t, http.StatusOK)(h.HandleAgentRegisterLift(ctx))
	var out apimodels.AgentSelfRegistrationResponse
	require.NoError(t, json.Unmarshal(resp.Body, &out))
	require.NotEmpty(t, out.Token.AccessToken)
	require.Equal(t, "Bearer", out.Token.TokenType)
}
