package handlers

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	dynamormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
)

func TestAgentSelfSovereignRound16_HandleAgentAuthTokenLift_ErrorBranches(t *testing.T) {
	cfg := round10TestConfig()
	cfg.AllowAgents = true

	policy := storagemodels.NewAgentInstanceConfig()
	policy.AllowAgents = true

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	pubB64 := base64.StdEncoding.EncodeToString(pub)

	now := time.Now().UTC()
	expiresAt := now.Add(10 * time.Minute)

	baseState := func() *round10QueryState {
		return &round10QueryState{
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
		}
	}

	t.Run("invalid JSON returns 400", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, baseState())
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/agents/auth/token", nil, nil, []byte("{bad"))
		requireStatus(t, http.StatusBadRequest)(h.HandleAgentAuthTokenLift(ctx))
	})

	t.Run("missing required fields returns 400", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, baseState())
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/auth/token", nil, nil, apimodels.AgentSelfAuthTokenRequest{
			Username: "agent",
		})
		require.NoError(t, err)
		requireStatus(t, http.StatusBadRequest)(h.HandleAgentAuthTokenLift(ctx))
	})

	t.Run("missing key configuration returns 403", func(t *testing.T) {
		state := baseState()
		state.usersByUsername["agent"] = storagemodels.User{
			PK:       "USER#agent",
			SK:       storagemodels.SKMetadata,
			Username: "agent",
			Role:     "user",
			Approved: true,
			Version:  1,
			IsAgent:  true,
		}
		h, _, _ := round11NewHandler(t, cfg, state)

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/auth/token", nil, nil, apimodels.AgentSelfAuthTokenRequest{
			Username:    "agent",
			ChallengeID: "c1",
			Signature:   "sig",
		})
		require.NoError(t, err)
		requireStatus(t, http.StatusForbidden)(h.HandleAgentAuthTokenLift(ctx))
	})

	t.Run("invalid signature returns 401", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, baseState())

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/auth/token", nil, nil, apimodels.AgentSelfAuthTokenRequest{
			Username:    "agent",
			ChallengeID: "c1",
			Signature:   "not-base64",
		})
		require.NoError(t, err)
		requireStatus(t, http.StatusUnauthorized)(h.HandleAgentAuthTokenLift(ctx))
	})

	t.Run("condition failures map to invalid_challenge", func(t *testing.T) {
		state := baseState()
		state.executeErrorOnce = dynamormerrors.ErrConditionFailed
		h, _, _ := round11NewHandler(t, cfg, state)

		sig := ed25519.Sign(priv, []byte(state.agentKeyChallengesByID["c1"].Message))

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/auth/token", nil, nil, apimodels.AgentSelfAuthTokenRequest{
			Username:    "agent",
			ChallengeID: "c1",
			Signature:   base64.StdEncoding.EncodeToString(sig),
		})
		require.NoError(t, err)
		requireStatus(t, http.StatusUnauthorized)(h.HandleAgentAuthTokenLift(ctx))
	})
}

func TestAgentSelfSovereignRound16_HandleAgentRegisterLift_ConflictBranch(t *testing.T) {
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
		createErrorOnce: dynamormerrors.ErrConditionFailed,
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
	})
	require.NoError(t, err)

	requireStatus(t, http.StatusConflict)(h.HandleAgentRegisterLift(ctx))
}
