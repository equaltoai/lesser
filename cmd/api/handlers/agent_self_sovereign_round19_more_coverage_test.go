package handlers

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v4/runtime"
	"go.uber.org/zap/zaptest"
)

func TestAgentSelfSovereignRound19_ValidateAgentSelfRegistrationRequest_MissingFields(t *testing.T) {
	cfg := round10TestConfig()

	h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

	base := apimodels.AgentSelfRegistrationRequest{
		Username:    "alice",
		DisplayName: "Alice",
		PublicKey:   "pk",
		KeyType:     "ed25519",
		ChallengeID: "c1",
		Signature:   "sig",
	}

	cases := []struct {
		name   string
		mutate func(*apimodels.AgentSelfRegistrationRequest)
	}{
		{name: "invalid username", mutate: func(r *apimodels.AgentSelfRegistrationRequest) { r.Username = "not a username" }},
		{name: "missing display_name", mutate: func(r *apimodels.AgentSelfRegistrationRequest) { r.DisplayName = "" }},
		{name: "missing public_key", mutate: func(r *apimodels.AgentSelfRegistrationRequest) { r.PublicKey = "" }},
		{name: "missing key_type", mutate: func(r *apimodels.AgentSelfRegistrationRequest) { r.KeyType = "" }},
		{name: "missing challenge_id", mutate: func(r *apimodels.AgentSelfRegistrationRequest) { r.ChallengeID = "" }},
		{name: "missing signature", mutate: func(r *apimodels.AgentSelfRegistrationRequest) { r.Signature = "" }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := base
			tc.mutate(&req)
			resp, err := h.validateAgentSelfRegistrationRequest(&apptheory.Context{}, &req)
			require.NoError(t, err)
			require.NotNil(t, resp)
			require.Equal(t, http.StatusBadRequest, resp.Status)
		})
	}
}

func TestAgentSelfSovereignRound19_ValidateAgentSelfRegistrationRequest_UsesMastodonBioRules(t *testing.T) {
	cfg := round10TestConfig()

	h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
	h.mastodonLogic = common.NewMastodonBusinessLogic(common.DefaultMastodonConfig(), zaptest.NewLogger(t))

	req := &apimodels.AgentSelfRegistrationRequest{
		Username:    "alice",
		DisplayName: "Alice",
		Bio:         strings.Repeat("a", 10000),
		PublicKey:   "pk",
		KeyType:     "ed25519",
		ChallengeID: "c1",
		Signature:   "sig",
	}

	resp, err := h.validateAgentSelfRegistrationRequest(&apptheory.Context{}, req)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, http.StatusBadRequest, resp.Status)
}

func TestAgentSelfSovereignRound19_HandleAgentRegisterLift_EarlyReturns(t *testing.T) {
	t.Run("registration disabled by config returns forbidden", func(t *testing.T) {
		cfg := round10TestConfig()
		cfg.AllowAgents = true
		cfg.AllowAgentRegistration = false

		policy := storagemodels.NewAgentInstanceConfig()
		policy.AllowAgents = true

		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{agentInstanceConfig: policy})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/register", nil, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusForbidden)(h.HandleAgentRegisterLift(ctx))
	})

	t.Run("invalid JSON returns 400", func(t *testing.T) {
		cfg := round10TestConfig()
		cfg.AllowAgents = true
		cfg.AllowAgentRegistration = true

		policy := storagemodels.NewAgentInstanceConfig()
		policy.AllowAgents = true
		policy.AllowAgentRegistration = true

		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{agentInstanceConfig: policy})

		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/agents/register", nil, nil, []byte("{bad"))
		requireStatus(t, http.StatusBadRequest)(h.HandleAgentRegisterLift(ctx))
	})

	t.Run("invalid agent_info returns 400", func(t *testing.T) {
		cfg := round10TestConfig()
		cfg.AllowAgents = true
		cfg.AllowAgentRegistration = true

		policy := storagemodels.NewAgentInstanceConfig()
		policy.AllowAgents = true
		policy.AllowAgentRegistration = true

		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)
		pubB64 := base64.StdEncoding.EncodeToString(pub)

		now := time.Now().UTC()
		expiresAt := now.Add(10 * time.Minute)

		state := &round10QueryState{
			agentInstanceConfig: policy,
			agentKeyChallengesByID: map[string]storagemodels.AgentKeyChallenge{
				"c1": {
					PK:        "AGENT_KEY_CHALLENGE#c1",
					SK:        "CHALLENGE",
					ID:        "c1",
					Username:  "agent",
					Action:    agentKeyActionRegister,
					Message:   "message-to-sign",
					IssuedAt:  now,
					ExpiresAt: expiresAt,
					TTL:       expiresAt.Unix(),
					Used:      false,
				},
			},
		}

		h, _, _ := round11NewHandler(t, cfg, state)
		h.repos.Account().SetEncryptor(noopEncryptor{})

		sig := ed25519.Sign(priv, []byte(state.agentKeyChallengesByID["c1"].Message))
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/register", nil, nil, apimodels.AgentSelfRegistrationRequest{
			Username:    "agent",
			DisplayName: "Agent",
			PublicKey:   pubB64,
			KeyType:     "ed25519",
			ChallengeID: "c1",
			Signature:   base64.StdEncoding.EncodeToString(sig),
			Scopes:      []string{auth.ScopeRead, auth.ScopeWrite, "follow"},
			AgentInfo:   "not-an-object",
		})
		require.NoError(t, err)

		requireStatus(t, http.StatusBadRequest)(h.HandleAgentRegisterLift(ctx))
	})
}

func TestAgentSelfSovereignRound19_CreateSelfSovereignAgentAccount_InternalError(t *testing.T) {
	cfg := round10TestConfig()

	state := &round10QueryState{
		createErrorOnce: errors.New("boom"),
	}

	h, _, _ := round11NewHandler(t, cfg, state)
	h.repos.Account().SetEncryptor(noopEncryptor{})

	user := &storage.User{
		Username:    "agent",
		DisplayName: "Agent",
		Approved:    true,
		Version:     1,
		IsAgent:     true,
	}

	ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/register", nil, nil, nil)
	require.NoError(t, err)

	_, resp, err := h.createSelfSovereignAgentAccount(ctx, user, "agent", "Agent", "", []string{auth.ScopeRead, auth.ScopeWrite, "follow"}, time.Now().UTC())
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, http.StatusInternalServerError, resp.Status)
}

func TestAgentSelfSovereignRound19_HandleAgentAuthChallengeLift_AdditionalBranches(t *testing.T) {
	t.Run("agents disabled returns forbidden", func(t *testing.T) {
		cfg := round10TestConfig()
		cfg.AllowAgents = false

		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/auth/challenge", nil, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusForbidden)(h.HandleAgentAuthChallengeLift(ctx))
	})

	t.Run("invalid JSON returns 400", func(t *testing.T) {
		cfg := round10TestConfig()
		cfg.AllowAgents = true

		policy := storagemodels.NewAgentInstanceConfig()
		policy.AllowAgents = true

		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{agentInstanceConfig: policy})

		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/agents/auth/challenge", nil, nil, []byte("{bad"))
		requireStatus(t, http.StatusBadRequest)(h.HandleAgentAuthChallengeLift(ctx))
	})

	t.Run("invalid username returns 400", func(t *testing.T) {
		cfg := round10TestConfig()
		cfg.AllowAgents = true

		policy := storagemodels.NewAgentInstanceConfig()
		policy.AllowAgents = true

		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{agentInstanceConfig: policy})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/auth/challenge", nil, nil, apimodels.AgentKeyChallengeRequest{Username: "not a username"})
		require.NoError(t, err)
		requireStatus(t, http.StatusBadRequest)(h.HandleAgentAuthChallengeLift(ctx))
	})

	t.Run("challenge creation errors return 500", func(t *testing.T) {
		cfg := round10TestConfig()
		cfg.AllowAgents = true

		policy := storagemodels.NewAgentInstanceConfig()
		policy.AllowAgents = true

		state := &round10QueryState{
			agentInstanceConfig: policy,
			usersByUsername: map[string]storagemodels.User{
				"agent": {
					PK:             "USER#agent",
					SK:             storagemodels.SKMetadata,
					Username:       "agent",
					Role:           "user",
					Approved:       true,
					Version:        1,
					IsAgent:        true,
					AgentKeyType:   "ed25519",
					AgentPublicKey: "pk",
				},
			},
			createErrorOnce: errors.New("boom"),
		}

		h, _, _ := round11NewHandler(t, cfg, state)

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/auth/challenge", nil, nil, apimodels.AgentKeyChallengeRequest{Username: "agent"})
		require.NoError(t, err)
		requireStatus(t, http.StatusInternalServerError)(h.HandleAgentAuthChallengeLift(ctx))
	})
}

func TestAgentSelfSovereignRound19_HandleAgentAuthTokenLift_MarkUsedErrorsReturn500(t *testing.T) {
	cfg := round10TestConfig()
	cfg.AllowAgents = true

	policy := storagemodels.NewAgentInstanceConfig()
	policy.AllowAgents = true

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	pubB64 := base64.StdEncoding.EncodeToString(pub)

	now := time.Now().UTC()
	expiresAt := now.Add(10 * time.Minute)

	state := &round10QueryState{
		agentInstanceConfig: policy,
		agentKeyChallengesByID: map[string]storagemodels.AgentKeyChallenge{
			"c1": {
				PK:        "AGENT_KEY_CHALLENGE#c1",
				SK:        "CHALLENGE",
				ID:        "c1",
				Username:  "agent",
				Action:    agentKeyActionAuth,
				Message:   "message-to-sign",
				IssuedAt:  now,
				ExpiresAt: expiresAt,
				TTL:       expiresAt.Unix(),
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
				IsAgent:        true,
				AgentKeyType:   "ed25519",
				AgentPublicKey: pubB64,
			},
		},
		executeErrorOnce: errors.New("boom"),
	}

	h, _, _ := round11NewHandler(t, cfg, state)

	sig := ed25519.Sign(priv, []byte(state.agentKeyChallengesByID["c1"].Message))
	ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/auth/token", nil, nil, apimodels.AgentSelfAuthTokenRequest{
		Username:    "agent",
		ChallengeID: "c1",
		Signature:   base64.StdEncoding.EncodeToString(sig),
	})
	require.NoError(t, err)

	requireStatus(t, http.StatusInternalServerError)(h.HandleAgentAuthTokenLift(ctx))
}

func TestAgentSelfSovereignRound19_HandleAgentRotateKeyLift_MissingRequiredFields(t *testing.T) {
	cfg := round10TestConfig()
	cfg.AllowAgents = true

	policy := storagemodels.NewAgentInstanceConfig()
	policy.AllowAgents = true

	h, _, _ := round11NewHandler(t, cfg, &round10QueryState{agentInstanceConfig: policy})

	token := round12SignAgentAccessToken(t, cfg.JWTSecret, "agent", []string{auth.ScopeWrite})
	headers := map[string]string{"Authorization": "Bearer " + token}

	cases := []struct {
		name string
		req  apimodels.AgentRotateKeyRequest
	}{
		{name: "missing public_key", req: apimodels.AgentRotateKeyRequest{PublicKey: " ", KeyType: "ed25519", ChallengeID: "c1", Signature: "sig"}},
		{name: "missing key_type", req: apimodels.AgentRotateKeyRequest{PublicKey: "pk", KeyType: " ", ChallengeID: "c1", Signature: "sig"}},
		{name: "missing challenge_id", req: apimodels.AgentRotateKeyRequest{PublicKey: "pk", KeyType: "ed25519", ChallengeID: " ", Signature: "sig"}},
		{name: "missing signature", req: apimodels.AgentRotateKeyRequest{PublicKey: "pk", KeyType: "ed25519", ChallengeID: "c1", Signature: " "}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent/rotate_key", headers, nil, tc.req)
			require.NoError(t, err)
			ctx.Params["username"] = "agent"

			requireStatus(t, http.StatusBadRequest)(h.HandleAgentRotateKeyLift(ctx))
		})
	}
}

func TestAgentSelfSovereignRound19_MarkAgentKeyChallengeUsed_RequiresStorage(t *testing.T) {
	require.Error(t, (&Handler{}).markAgentKeyChallengeUsed(&apptheory.Context{}, "c1"))
}
