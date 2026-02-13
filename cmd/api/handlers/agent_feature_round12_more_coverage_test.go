package handlers

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/agents"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	dynamormerrors "github.com/theory-cloud/tabletheory/pkg/errors"
)

func TestAgentsRound12_DirectoryLifecycleAndActivity(t *testing.T) {
	cfg := round10TestConfig()
	cfg.AllowAgents = true

	now := time.Now().UTC()
	policy := storagemodels.NewAgentInstanceConfig()
	policy.AllowAgents = true
	policy.AgentMaxPostsPerHour = 50
	policy.VerifiedAgentMaxPostsPerHour = 200

	aliceMetadata := map[string]any{
		"agent_delegated_scopes": []any{"read", "write:statuses"},
		"agent_verified":         "true",
		"agent_verified_at":      now.Add(-1 * time.Hour).Format(time.RFC3339),
	}

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
			"admin": {
				PK:        "USER#admin",
				SK:        storagemodels.SKMetadata,
				Username:  "admin",
				Role:      "admin",
				Approved:  true,
				Version:   1,
				CreatedAt: now.Add(-24 * time.Hour),
				IsAgent:   true,
				Suspended: true,
			},
			"alice": {
				PK:           "USER#alice",
				SK:           storagemodels.SKMetadata,
				Username:     "alice",
				Role:         "user",
				Approved:     true,
				Version:      1,
				CreatedAt:    now.Add(-24 * time.Hour),
				IsAgent:      true,
				AgentOwner:   "@owner",
				AgentType:    agentTypeCustom,
				AgentVersion: "v1",
				Metadata:     aliceMetadata,
				AgentCapabilities: &agents.Capabilities{
					CanPost:           true,
					RestrictedDomains: []string{"example.org"},
					MaxPostsPerHour:   10,
				},
			},
		},
		auditLogsByUser: map[string][]*storagemodels.AuthAuditLog{
			"alice": {
				nil,
				{
					ID:        "a1",
					Timestamp: now.Add(-2 * time.Minute),
					EventType: "agent.key_rotated",
					Metadata:  `{"target_id":"s1","foo":"bar"}`,
				},
				{
					ID:        "a2",
					Timestamp: now.Add(-3 * time.Minute),
					EventType: "login",
					Metadata:  `{"target_id":"s2"}`,
				},
				{
					ID:        "a3",
					Timestamp: now.Add(-1 * time.Minute),
					EventType: "agent.badmeta",
					Metadata:  "not-json",
				},
			},
		},
	}

	h, repos, _ := round11NewHandler(t, cfg, state)
	h.repos.Account().SetEncryptor(noopEncryptor{})

	t.Run("lists_agents_and_filters", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/agents", nil, nil, nil)
		require.NoError(t, err)

		resp := requireStatus(t, http.StatusOK)(h.HandleListAgentsLift(ctx))
		var out []apimodels.Agent
		require.NoError(t, json.Unmarshal(resp.Body, &out))
		require.Len(t, out, 1)
		require.Equal(t, "alice", out[0].Username)
		require.Equal(t, 10, out[0].AgentCapabilities.MaxPostsPerHour)
	})

	t.Run("gets_agent_by_username", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/agents/alice", nil, nil, nil)
		require.NoError(t, err)
		ctx.Params["username"] = "alice"

		resp := requireStatus(t, http.StatusOK)(h.HandleGetAgentLift(ctx))
		var out apimodels.Agent
		require.NoError(t, json.Unmarshal(resp.Body, &out))
		require.Equal(t, "alice", out.Username)
		require.True(t, out.Verified)
	})

	t.Run("updates_agent_as_owner", func(t *testing.T) {
		ownerToken := round11SignAccessToken(t, cfg.JWTSecret, "owner", []string{auth.ScopeWrite, auth.ScopeRead})
		headers := map[string]string{"Authorization": "Bearer " + ownerToken}

		req := apimodels.UpdateAgentRequest{
			DisplayName:    "Alice Agent",
			Bio:            "updated bio",
			AgentType:      "CUSTOM",
			AgentVersion:   "v2",
			ExitQuarantine: true,
			AgentCapabilities: &apimodels.AgentCapabilities{
				CanPost:           true,
				CanReply:          true,
				RestrictedDomains: []string{"example.org", "example.net"},
				MaxPostsPerHour:   500,
				RequiresApproval:  true,
			},
		}

		ctx, err := round10NewLiftContext(http.MethodPatch, "/api/v1/agents/alice", headers, nil, req)
		require.NoError(t, err)
		ctx.Params["username"] = "alice"

		resp := requireStatus(t, http.StatusOK)(h.HandleUpdateAgentLift(ctx))
		var out apimodels.Agent
		require.NoError(t, json.Unmarshal(resp.Body, &out))
		require.Equal(t, "alice", out.Username)
		require.Equal(t, "Alice Agent", out.DisplayName)
		require.Equal(t, "updated bio", out.Bio)
		require.Equal(t, 200, out.AgentCapabilities.MaxPostsPerHour)
		require.True(t, out.AgentCapabilities.RequiresApproval)
	})

	t.Run("deletes_agent_as_owner", func(t *testing.T) {
		ownerToken := round11SignAccessToken(t, cfg.JWTSecret, "owner", []string{auth.ScopeWrite})
		headers := map[string]string{"Authorization": "Bearer " + ownerToken}

		ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/agents/alice", headers, nil, nil)
		require.NoError(t, err)
		ctx.Params["username"] = "alice"

		resp := requireStatus(t, http.StatusOK)(h.HandleDeleteAgentLift(ctx))
		var out apimodels.Agent
		require.NoError(t, json.Unmarshal(resp.Body, &out))
		require.True(t, out.Username == "alice")
	})

	t.Run("suspends_agent_as_admin", func(t *testing.T) {
		adminToken := round11SignAccessToken(t, cfg.JWTSecret, "admin", []string{auth.ScopeAdmin})
		headers := map[string]string{"Authorization": "Bearer " + adminToken}

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/alice/suspend", headers, nil, nil)
		require.NoError(t, err)
		ctx.Params["username"] = "alice"

		resp := requireStatus(t, http.StatusOK)(h.HandleSuspendAgentLift(ctx))
		var out apimodels.Agent
		require.NoError(t, json.Unmarshal(resp.Body, &out))
		require.Equal(t, "alice", out.Username)
	})

	t.Run("returns_activity_logs", func(t *testing.T) {
		readToken := round11SignAccessToken(t, cfg.JWTSecret, "owner", []string{auth.ScopeRead})
		headers := map[string]string{"Authorization": "Bearer " + readToken}

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/agents/alice/activity", headers, nil, nil)
		require.NoError(t, err)
		ctx.Params["username"] = "alice"

		resp := requireStatus(t, http.StatusOK)(h.HandleGetAgentActivityLift(ctx))
		var out apimodels.AgentActivityLogList
		require.NoError(t, json.Unmarshal(resp.Body, &out))
		require.Len(t, out, 2)
		require.Equal(t, "alice", out[0].AgentUsername)
	})

	t.Run("validates_access_token_ttl", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/delegate", nil, nil, nil)
		require.NoError(t, err)

		_, resp, err := validateAgentAccessTokenTTL(ctx, 30)
		require.NoError(t, err)
		require.NotNil(t, resp)
	})

	repos.AssertExpectations(t)
}

func TestAgentSelfSovereignRound12_ChallengeAuthAndRotateKey(t *testing.T) {
	cfg := round10TestConfig()
	cfg.AllowAgents = true
	cfg.AllowAgentRegistration = true

	now := time.Now().UTC()
	policy := storagemodels.NewAgentInstanceConfig()
	policy.AllowAgents = true
	policy.AllowAgentRegistration = true

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	publicKey := base64.StdEncoding.EncodeToString(pub)

	expiresAt := now.Add(10 * time.Minute)
	authChallengeID := "challenge-auth"
	authMessage := buildAgentKeyChallengeMessage(authChallengeID, agentKeyActionAuth, "agent", "nonce", now, expiresAt)
	authSignature := base64.StdEncoding.EncodeToString(ed25519.Sign(priv, []byte(authMessage)))

	rotateChallengeID := "challenge-rotate"
	rotateMessage := buildAgentKeyChallengeMessage(rotateChallengeID, agentKeyActionRotateKey, "agent", "nonce2", now, expiresAt)
	rotateSignature := base64.StdEncoding.EncodeToString(ed25519.Sign(priv, []byte(rotateMessage)))

	state := &round10QueryState{
		agentInstanceConfig: policy,
		agentKeyChallengesByID: map[string]storagemodels.AgentKeyChallenge{
			authChallengeID: {
				PK:        "AGENT_KEY_CHALLENGE#" + authChallengeID,
				SK:        "CHALLENGE",
				TTL:       expiresAt.Unix(),
				ID:        authChallengeID,
				Username:  "agent",
				Action:    agentKeyActionAuth,
				Nonce:     "nonce",
				Message:   authMessage,
				IssuedAt:  now,
				ExpiresAt: expiresAt,
				Used:      false,
			},
			rotateChallengeID: {
				PK:        "AGENT_KEY_CHALLENGE#" + rotateChallengeID,
				SK:        "CHALLENGE",
				TTL:       expiresAt.Unix(),
				ID:        rotateChallengeID,
				Username:  "agent",
				Action:    agentKeyActionRotateKey,
				Nonce:     "nonce2",
				Message:   rotateMessage,
				IssuedAt:  now,
				ExpiresAt: expiresAt,
				Used:      false,
			},
		},
		usersByUsername: map[string]storagemodels.User{
			"agent": {
				PK:             "USER#agent",
				SK:             storagemodels.SKMetadata,
				Username:       "agent",
				Role:           "user",
				Approved:       true,
				Version:        1,
				CreatedAt:      now.Add(-24 * time.Hour),
				IsAgent:        true,
				AgentType:      agentTypeCustom,
				AgentVersion:   "v1",
				AgentPublicKey: publicKey,
				AgentKeyType:   "ed25519",
				Metadata: map[string]any{
					"agent_self_scopes":    []any{"read", "write:statuses", "follow"},
					"agent_self_sovereign": true,
				},
			},
		},
	}

	h, repos, _ := round11NewHandler(t, cfg, state)
	h.repos.Account().SetEncryptor(noopEncryptor{})

	t.Run("issues_register_challenge", func(t *testing.T) {
		req := apimodels.AgentKeyChallengeRequest{Username: "agent"}
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/register/challenge", nil, nil, req)
		require.NoError(t, err)

		resp := requireStatus(t, http.StatusOK)(h.HandleAgentRegisterChallengeLift(ctx))
		var out apimodels.AgentKeyChallengeResponse
		require.NoError(t, json.Unmarshal(resp.Body, &out))
		require.NotEmpty(t, out.ID)
		require.Equal(t, "register", out.Action)
	})

	t.Run("issues_auth_challenge", func(t *testing.T) {
		req := apimodels.AgentKeyChallengeRequest{Username: "agent"}
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/auth/challenge", nil, nil, req)
		require.NoError(t, err)

		resp := requireStatus(t, http.StatusOK)(h.HandleAgentAuthChallengeLift(ctx))
		var out apimodels.AgentKeyChallengeResponse
		require.NoError(t, json.Unmarshal(resp.Body, &out))
		require.NotEmpty(t, out.ID)
		require.Equal(t, "auth", out.Action)
	})

	t.Run("mints_auth_token", func(t *testing.T) {
		req := apimodels.AgentSelfAuthTokenRequest{
			Username:    "agent",
			ChallengeID: authChallengeID,
			Signature:   authSignature,
		}

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/auth/token", nil, nil, req)
		require.NoError(t, err)

		resp := requireStatus(t, http.StatusOK)(h.HandleAgentAuthTokenLift(ctx))
		var out apimodels.OAuthTokenResponse
		require.NoError(t, json.Unmarshal(resp.Body, &out))
		require.NotEmpty(t, out.AccessToken)
		require.Equal(t, "Bearer", out.TokenType)
	})

	t.Run("issues_rotate_key_challenge", func(t *testing.T) {
		agentToken := round12SignAgentAccessToken(t, cfg.JWTSecret, "agent", []string{auth.ScopeRead})
		headers := map[string]string{"Authorization": "Bearer " + agentToken}

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent/rotate_key/challenge", headers, nil, nil)
		require.NoError(t, err)
		ctx.Params["username"] = "agent"

		resp := requireStatus(t, http.StatusOK)(h.HandleAgentRotateKeyChallengeLift(ctx))
		var out apimodels.AgentKeyChallengeResponse
		require.NoError(t, json.Unmarshal(resp.Body, &out))
		require.NotEmpty(t, out.ID)
		require.Equal(t, "rotate_key", out.Action)
	})

	t.Run("rotates_key", func(t *testing.T) {
		newPub, _, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)
		newPublicKey := base64.StdEncoding.EncodeToString(newPub)

		agentToken := round12SignAgentAccessToken(t, cfg.JWTSecret, "agent", []string{auth.ScopeWrite})
		headers := map[string]string{"Authorization": "Bearer " + agentToken}

		req := apimodels.AgentRotateKeyRequest{
			PublicKey:   newPublicKey,
			KeyType:     "ed25519",
			ChallengeID: rotateChallengeID,
			Signature:   rotateSignature,
		}
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent/rotate_key", headers, nil, req)
		require.NoError(t, err)
		ctx.Params["username"] = "agent"

		resp := requireStatus(t, http.StatusOK)(h.HandleAgentRotateKeyLift(ctx))
		var out apimodels.Agent
		require.NoError(t, json.Unmarshal(resp.Body, &out))
		require.Equal(t, "agent", out.Username)
	})

	repos.AssertExpectations(t)
}

func TestAgentMemorySearchRound12_HybridFallbackAndThread(t *testing.T) {
	cfg := round10TestConfig()
	cfg.AllowAgents = true

	now := time.Now().UTC()
	policy := storagemodels.NewAgentInstanceConfig()
	policy.AllowAgents = true
	policy.HybridRetrievalEnabled = true
	policy.HybridRetrievalMaxCandidates = 10

	state := &round10QueryState{
		agentInstanceConfig: policy,
		statusList: []storagemodels.Status{
			{
				StatusID:       "s1",
				AuthorUsername: "agent",
				Content:        "hello world",
				PublishedAt:    now.Add(-10 * time.Minute),
				ConversationID: "thread1",
				Hashtags:       []string{"tag1"},
			},
			{
				StatusID:       "s2",
				AuthorUsername: "agent",
				Content:        "hello again",
				PublishedAt:    now.Add(-9 * time.Minute),
				ConversationID: "thread1",
				Hashtags:       []string{"tag2"},
			},
			{
				StatusID:       "s3",
				AuthorUsername: "someoneelse",
				Content:        "hello from someone else",
				PublishedAt:    now.Add(-8 * time.Minute),
				ConversationID: "thread1",
			},
			{
				StatusID:       "s4",
				AuthorUsername: "agent",
				Content:        "goodbye",
				PublishedAt:    now.Add(-7 * time.Minute),
			},
		},
	}

	h, repos, _ := round11NewHandler(t, cfg, state)
	h.repos.Account().SetEncryptor(noopEncryptor{})

	agentToken := round12SignAgentAccessToken(t, cfg.JWTSecret, "agent", []string{auth.ScopeRead})
	headers := map[string]string{"Authorization": "Bearer " + agentToken}

	t.Run("hybrid_fallback_mode", func(t *testing.T) {
		query := map[string]string{
			"query":           "hello",
			"mode":            "hybrid",
			"include_threads": "true",
			"limit":           "3",
		}

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/agents/memory/search", headers, query, nil)
		require.NoError(t, err)

		resp := requireStatus(t, http.StatusOK)(h.HandleAgentMemorySearchLift(ctx))
		var out apimodels.AgentMemorySearchResponse
		require.NoError(t, json.Unmarshal(resp.Body, &out))
		require.Greater(t, out.Total, 0)
		require.NotNil(t, out.Results[0].Status)
		require.NotNil(t, out.Results[0].Thread)
	})

	t.Run("thread_lookup_mode", func(t *testing.T) {
		query := map[string]string{
			"thread_id": "thread1",
		}
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/agents/memory/search", headers, query, nil)
		require.NoError(t, err)

		resp := requireStatus(t, http.StatusOK)(h.HandleAgentMemorySearchLift(ctx))
		var out apimodels.AgentMemorySearchResponse
		require.NoError(t, json.Unmarshal(resp.Body, &out))
		require.Equal(t, 1, out.Total)
		require.NotNil(t, out.Results[0].Thread)
	})

	t.Run("parses_date_range_validation", func(t *testing.T) {
		_, _, err := parseDateRange(&apimodels.DateRange{Start: "not-a-date", End: "2020-01-01"})
		require.Error(t, err)
	})

	t.Run("caps_thread", func(t *testing.T) {
		items := []*storagemodels.Status{
			{StatusID: "root"},
			{StatusID: "a"},
			{StatusID: "b"},
			{StatusID: "c"},
		}
		capped := capThreadForAgent(items, 3)
		require.Len(t, capped, 3)
		require.Equal(t, "root", capped[0].StatusID)
	})

	repos.AssertExpectations(t)
}

func TestAgentRemotePolicyRound12_HideDecisions(t *testing.T) {
	cfg := round10TestConfig()
	cfg.AllowAgents = true

	policy := storagemodels.NewAgentInstanceConfig()
	policy.AllowAgents = true
	policy.AllowRemoteAgents = true
	policy.RemoteQuarantineDays = 7
	policy.BlockedAgentDomains = []string{"blocked.example"}
	policy.TrustedAgentDomains = []string{"trusted.example"}

	now := time.Now().UTC()
	state := &round10QueryState{
		agentInstanceConfig: policy,
		notFoundPKSK: map[string]bool{
			"REMOTE_ACTOR#alice@remote.example#PROFILE": true,
		},
		remoteActorsByPK: map[string]storagemodels.RemoteActor{
			"REMOTE_ACTOR#bob@remote.example": {
				PK:        "REMOTE_ACTOR#bob@remote.example",
				SK:        storagemodels.SKProfile,
				CachedAt:  now.Add(-10 * 24 * time.Hour),
				UpdatedAt: now.Add(-10 * 24 * time.Hour),
			},
		},
	}

	h, _, _ := round11NewHandler(t, cfg, state)

	require.Equal(t, "remote.example", extractDomainFromActorID("https://remote.example/users/alice"))
	require.Equal(t, "alice@remote.example", extractHandleFromActorID("https://remote.example/users/alice"))
	require.True(t, isLocalDomain("Example.COM", "example.com"))
	require.True(t, domainInList("trusted.example", []string{"trusted.example", "other.example"}))

	// trusted domain should not be hidden
	require.False(t, h.shouldHideRemoteAgentActor(contextBackground(t), "https://trusted.example/users/agent"))
	// blocked domain should be hidden
	require.True(t, h.shouldHideRemoteAgentActor(contextBackground(t), "https://blocked.example/users/agent"))
	// remote quarantine active when not found
	require.True(t, h.remoteAgentQuarantineActive(contextBackground(t), "https://remote.example/users/alice", policy))
	// remote quarantine inactive after grace period
	require.False(t, h.remoteAgentQuarantineActive(contextBackground(t), "https://remote.example/users/bob", policy))
}

func TestAgentSafetyRailsRound12_ValidationsAndHelpers(t *testing.T) {
	cfg := round10TestConfig()
	cfg.AllowAgents = true

	now := time.Now().UTC()
	state := &round10QueryState{
		usersByUsername: map[string]storagemodels.User{
			"agent": {
				PK:        "USER#agent",
				SK:        storagemodels.SKMetadata,
				Username:  "agent",
				Role:      "user",
				Approved:  true,
				Version:   1,
				CreatedAt: now.Add(-24 * time.Hour),
				IsAgent:   true,
				Metadata: map[string]any{
					"agent_quarantine_end": now.Add(24 * time.Hour).Format(time.RFC3339),
				},
			},
		},
	}

	h, _, _ := round11NewHandler(t, cfg, state)

	ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/statuses", nil, nil, nil)
	claims := &auth.Claims{Username: "agent", IsAgent: true}

	t.Run("too_many_hashtags", func(t *testing.T) {
		req := &apimodels.CreateStatusRequest{Status: "#a #b #c #d #e #f"}
		resp, err := h.enforceAgentStatusCreateRails(ctx, claims, req)
		require.NoError(t, err)
		require.NotNil(t, resp)
	})

	t.Run("too_many_chars", func(t *testing.T) {
		req := &apimodels.CreateStatusRequest{Status: strings.Repeat("a", 501)}
		resp, err := h.enforceAgentStatusCreateRails(ctx, claims, req)
		require.NoError(t, err)
		require.NotNil(t, resp)
	})

	t.Run("quarantine_active_helper", func(t *testing.T) {
		user := &storage.User{
			IsAgent: true,
			Metadata: map[string]any{
				"agent_quarantine_end": now.Add(24 * time.Hour).Format(time.RFC3339),
			},
		}
		quarantined, until := agentQuarantineActive(user)
		require.True(t, quarantined)
		require.False(t, until.IsZero())
	})

	require.Equal(t, []string{"Tag"}, uniqueHashtags("#Tag #tag"))
	require.NotEmpty(t, hashAgentContent("  Hello   World "))
	require.Equal(t, "agent:alice", agentLockoutIdentifier("Alice"))
}

func TestAgentMemoryEventsRound12_RecordHelpers(t *testing.T) {
	cfg := round10TestConfig()
	cfg.AllowAgents = true

	h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

	ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/statuses", nil, nil, nil)
	h.recordAgentMemoryEvent(ctx, "agent", "s1", &apimodels.CreateStatusRequest{
		Status: "correction",
		MemoryEvent: &apimodels.AgentMemoryEventRequest{
			EventType:  "correction",
			OriginalID: "orig1",
			Reason:     "fix",
		},
	})

	h.recordAgentMemoryTombstone(ctx.Context(), "agent", "s1", "delete")
}

func TestAgentGovernanceRound12_GetPolicy(t *testing.T) {
	cfg := round10TestConfig()
	cfg.AllowAgents = true

	policy := storagemodels.NewAgentInstanceConfig()
	policy.AllowAgents = true
	policy.AllowRemoteAgents = true
	policy.RemoteQuarantineDays = 3

	state := &round10QueryState{agentInstanceConfig: policy}
	h, _, _ := round11NewHandler(t, cfg, state)

	adminToken := round11SignAccessToken(t, cfg.JWTSecret, "admin", []string{auth.ScopeAdmin})
	headers := map[string]string{"Authorization": "Bearer " + adminToken}

	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/agents/policy", headers, nil, nil)
	require.NoError(t, err)

	resp := requireStatus(t, http.StatusOK)(h.HandleAdminGetAgentPolicyLift(ctx))
	var out apimodels.AdminAgentPolicy
	require.NoError(t, json.Unmarshal(resp.Body, &out))
	require.True(t, out.AllowAgents)
}

func TestAgentSelfSovereignScopesRound12_MetadataShapes(t *testing.T) {
	user := &storage.User{
		IsAgent: true,
		Metadata: map[string]any{
			"agent_self_scopes": "read write:statuses follow admin",
		},
	}
	scopes := agentSelfSovereignScopes(user)
	require.Subset(t, scopes, []string{auth.ScopeRead, auth.ScopeWrite, "follow"})
}

func contextBackground(t *testing.T) context.Context {
	t.Helper()
	return context.Background()
}

func TestAgentKeyChallengeConsumeRound12_ConditionFailed(t *testing.T) {
	cfg := round10TestConfig()
	cfg.AllowAgents = true
	cfg.AllowAgentRegistration = true

	now := time.Now().UTC()
	policy := storagemodels.NewAgentInstanceConfig()
	policy.AllowAgents = true
	policy.AllowAgentRegistration = true

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	publicKey := base64.StdEncoding.EncodeToString(pub)

	expiresAt := now.Add(10 * time.Minute)
	challengeID := "challenge-used"
	message := buildAgentKeyChallengeMessage(challengeID, agentKeyActionRegister, "agent", "nonce", now, expiresAt)
	signature := base64.StdEncoding.EncodeToString(ed25519.Sign(priv, []byte(message)))

	state := &round10QueryState{
		agentInstanceConfig: policy,
		agentKeyChallengesByID: map[string]storagemodels.AgentKeyChallenge{
			challengeID: {
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
			},
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
		executeErrorOnce: dynamormerrors.ErrConditionFailed,
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

	resp := requireStatus(t, http.StatusUnauthorized)(h.HandleAgentRegisterLift(ctx))
	require.Contains(t, string(resp.Body), "invalid_challenge")
}
