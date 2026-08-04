package handlers

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	dynamormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
)

func TestAgentSelfSovereignRound16_RegisterChallengeErrors(t *testing.T) {
	cfg := round10TestConfig()
	cfg.AllowAgents = true
	cfg.AllowAgentRegistration = true

	policy := storagemodels.NewAgentInstanceConfig()
	policy.AllowAgents = true
	policy.AllowAgentRegistration = true

	t.Run("invalid JSON returns 400", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{agentInstanceConfig: policy})
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/agents/register/challenge", nil, nil, []byte("{bad"))
		requireStatus(t, http.StatusBadRequest)(h.HandleAgentRegisterChallengeLift(ctx))
	})

	t.Run("invalid username returns 400", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{agentInstanceConfig: policy})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/register/challenge", nil, nil, apimodels.AgentKeyChallengeRequest{
			Username: "not a username",
		})
		require.NoError(t, err)
		requireStatus(t, http.StatusBadRequest)(h.HandleAgentRegisterChallengeLift(ctx))
	})

	t.Run("create challenge failures return 500", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{
			agentInstanceConfig: policy,
			createErrorOnce:     errors.New("boom"),
		})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/register/challenge", nil, nil, apimodels.AgentKeyChallengeRequest{
			Username: "agent",
		})
		require.NoError(t, err)
		requireStatus(t, http.StatusInternalServerError)(h.HandleAgentRegisterChallengeLift(ctx))
	})
}

func TestAgentSelfSovereignRound16_RotateKeyChallengeErrors(t *testing.T) {
	cfg := round10TestConfig()
	cfg.AllowAgents = true

	policy := storagemodels.NewAgentInstanceConfig()
	policy.AllowAgents = true

	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	pubB64 := base64.StdEncoding.EncodeToString(pub)

	t.Run("missing token returns 401", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{agentInstanceConfig: policy})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent/rotate_key/challenge", nil, nil, nil)
		require.NoError(t, err)
		ctx.Params["username"] = "agent"

		requireStatus(t, http.StatusUnauthorized)(h.HandleAgentRotateKeyChallengeLift(ctx))
	})

	t.Run("insufficient scope returns 403", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{agentInstanceConfig: policy})

		token := round12SignAgentAccessToken(t, cfg.JWTSecret, "agent", []string{auth.ScopeWrite})
		headers := map[string]string{"Authorization": "Bearer " + token}

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent/rotate_key/challenge", headers, nil, nil)
		require.NoError(t, err)
		ctx.Params["username"] = "agent"

		requireStatus(t, http.StatusForbidden)(h.HandleAgentRotateKeyChallengeLift(ctx))
	})

	t.Run("non-agent tokens are forbidden", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{agentInstanceConfig: policy})

		token := round11SignAccessToken(t, cfg.JWTSecret, "agent", []string{auth.ScopeRead})
		headers := map[string]string{"Authorization": "Bearer " + token}

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent/rotate_key/challenge", headers, nil, nil)
		require.NoError(t, err)
		ctx.Params["username"] = "agent"

		requireStatus(t, http.StatusForbidden)(h.HandleAgentRotateKeyChallengeLift(ctx))
	})

	t.Run("agent username mismatch is forbidden", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{agentInstanceConfig: policy})

		token := round12SignAgentAccessToken(t, cfg.JWTSecret, "agent", []string{auth.ScopeRead})
		headers := map[string]string{"Authorization": "Bearer " + token}

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/other/rotate_key/challenge", headers, nil, nil)
		require.NoError(t, err)
		ctx.Params["username"] = "other"

		requireStatus(t, http.StatusForbidden)(h.HandleAgentRotateKeyChallengeLift(ctx))
	})

	t.Run("agent not found returns 404", func(t *testing.T) {
		state := &round10QueryState{agentInstanceConfig: policy}
		h, _, _ := round11NewHandler(t, cfg, state)

		token := round12SignAgentAccessToken(t, cfg.JWTSecret, "missing", []string{auth.ScopeRead})
		headers := map[string]string{"Authorization": "Bearer " + token}

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/missing/rotate_key/challenge", headers, nil, nil)
		require.NoError(t, err)
		ctx.Params["username"] = "missing"

		requireStatus(t, http.StatusNotFound)(h.HandleAgentRotateKeyChallengeLift(ctx))
	})

	t.Run("agents missing configured keys are forbidden", func(t *testing.T) {
		state := &round10QueryState{
			agentInstanceConfig: policy,
			usersByUsername: map[string]storagemodels.User{
				"agent": {
					PK:       "USER#agent",
					SK:       storagemodels.SKMetadata,
					Username: "agent",
					Role:     "user",
					Approved: true,
					Version:  1,
					IsAgent:  true,
				},
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state)

		token := round12SignAgentAccessToken(t, cfg.JWTSecret, "agent", []string{auth.ScopeRead})
		headers := map[string]string{"Authorization": "Bearer " + token}

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent/rotate_key/challenge", headers, nil, nil)
		require.NoError(t, err)
		ctx.Params["username"] = "agent"

		requireStatus(t, http.StatusForbidden)(h.HandleAgentRotateKeyChallengeLift(ctx))
	})

	_ = pubB64
}

func TestAgentSelfSovereignRound16_RotateKeyLiftErrorBranches(t *testing.T) {
	cfg := round10TestConfig()
	cfg.AllowAgents = true

	policy := storagemodels.NewAgentInstanceConfig()
	policy.AllowAgents = true

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	pubB64 := base64.StdEncoding.EncodeToString(pub)

	baseState := func() *round10QueryState {
		return &round10QueryState{
			agentInstanceConfig:    policy,
			agentKeyChallengesByID: map[string]storagemodels.AgentKeyChallenge{},
			usersByUsername: map[string]storagemodels.User{
				"agent": {
					PK:             "USER#agent",
					SK:             storagemodels.SKMetadata,
					Username:       "agent",
					Role:           "user",
					Approved:       true,
					Version:        1,
					CreatedAt:      time.Now().Add(-24 * time.Hour),
					IsAgent:        true,
					AgentType:      agentTypeCustom,
					AgentKeyType:   "ed25519",
					AgentPublicKey: pubB64,
				},
			},
		}
	}

	t.Run("invalid JSON returns 400", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, baseState())
		h.repos.Account().SetEncryptor(noopEncryptor{})

		token := round12SignAgentAccessToken(t, cfg.JWTSecret, "agent", []string{auth.ScopeWrite})
		headers := map[string]string{"Authorization": "Bearer " + token}

		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/agents/agent/rotate_key", headers, nil, []byte("{bad"))
		ctx.Params["username"] = "agent"

		requireStatus(t, http.StatusBadRequest)(h.HandleAgentRotateKeyLift(ctx))
	})

	t.Run("invalid signature returns 401", func(t *testing.T) {
		state := baseState()
		h, _, _ := round11NewHandler(t, cfg, state)
		h.repos.Account().SetEncryptor(noopEncryptor{})

		readToken := round12SignAgentAccessToken(t, cfg.JWTSecret, "agent", []string{auth.ScopeRead})
		writeToken := round12SignAgentAccessToken(t, cfg.JWTSecret, "agent", []string{auth.ScopeWrite})

		challengeCtx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent/rotate_key/challenge", map[string]string{
			"Authorization": "Bearer " + readToken,
		}, nil, nil)
		require.NoError(t, err)
		challengeCtx.Params["username"] = "agent"

		challengeResp := requireStatus(t, http.StatusOK)(h.HandleAgentRotateKeyChallengeLift(challengeCtx))
		var challenge apimodels.AgentKeyChallengeResponse
		require.NoError(t, json.Unmarshal(challengeResp.Body, &challenge))

		state.agentKeyChallengesByID[challenge.ID] = storagemodels.AgentKeyChallenge{
			PK:        "AGENT_KEY_CHALLENGE#" + challenge.ID,
			SK:        "CHALLENGE",
			ID:        challenge.ID,
			Username:  challenge.Username,
			Action:    challenge.Action,
			Message:   challenge.Message,
			IssuedAt:  challenge.IssuedAt,
			ExpiresAt: challenge.ExpiresAt,
			TTL:       challenge.ExpiresAt.Unix(),
			Used:      false,
		}

		rotateCtx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent/rotate_key", map[string]string{
			"Authorization": "Bearer " + writeToken,
		}, nil, apimodels.AgentRotateKeyRequest{
			PublicKey:   pubB64,
			KeyType:     "ed25519",
			ChallengeID: challenge.ID,
			Signature:   "not-base64",
		})
		require.NoError(t, err)
		rotateCtx.Params["username"] = "agent"

		requireStatus(t, http.StatusUnauthorized)(h.HandleAgentRotateKeyLift(rotateCtx))
	})

	t.Run("invalid new public key is rejected with 400", func(t *testing.T) {
		state := baseState()
		h, _, _ := round11NewHandler(t, cfg, state)
		h.repos.Account().SetEncryptor(noopEncryptor{})

		readToken := round12SignAgentAccessToken(t, cfg.JWTSecret, "agent", []string{auth.ScopeRead})
		writeToken := round12SignAgentAccessToken(t, cfg.JWTSecret, "agent", []string{auth.ScopeWrite})

		challengeCtx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent/rotate_key/challenge", map[string]string{
			"Authorization": "Bearer " + readToken,
		}, nil, nil)
		require.NoError(t, err)
		challengeCtx.Params["username"] = "agent"

		challengeResp := requireStatus(t, http.StatusOK)(h.HandleAgentRotateKeyChallengeLift(challengeCtx))
		var challenge apimodels.AgentKeyChallengeResponse
		require.NoError(t, json.Unmarshal(challengeResp.Body, &challenge))

		state.agentKeyChallengesByID[challenge.ID] = storagemodels.AgentKeyChallenge{
			PK:        "AGENT_KEY_CHALLENGE#" + challenge.ID,
			SK:        "CHALLENGE",
			ID:        challenge.ID,
			Username:  challenge.Username,
			Action:    challenge.Action,
			Message:   challenge.Message,
			IssuedAt:  challenge.IssuedAt,
			ExpiresAt: challenge.ExpiresAt,
			TTL:       challenge.ExpiresAt.Unix(),
			Used:      false,
		}

		rotateSig := ed25519.Sign(priv, []byte(challenge.Message))
		rotateCtx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent/rotate_key", map[string]string{
			"Authorization": "Bearer " + writeToken,
		}, nil, apimodels.AgentRotateKeyRequest{
			PublicKey:   "not-a-public-key",
			KeyType:     "ed25519",
			ChallengeID: challenge.ID,
			Signature:   base64.StdEncoding.EncodeToString(rotateSig),
		})
		require.NoError(t, err)
		rotateCtx.Params["username"] = "agent"

		requireStatus(t, http.StatusBadRequest)(h.HandleAgentRotateKeyLift(rotateCtx))
	})

	t.Run("condition failures map to invalid_challenge", func(t *testing.T) {
		state := baseState()
		state.executeErrorOnce = dynamormerrors.ErrConditionFailed

		h, _, _ := round11NewHandler(t, cfg, state)
		h.repos.Account().SetEncryptor(noopEncryptor{})

		readToken := round12SignAgentAccessToken(t, cfg.JWTSecret, "agent", []string{auth.ScopeRead})
		writeToken := round12SignAgentAccessToken(t, cfg.JWTSecret, "agent", []string{auth.ScopeWrite})

		challengeCtx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent/rotate_key/challenge", map[string]string{
			"Authorization": "Bearer " + readToken,
		}, nil, nil)
		require.NoError(t, err)
		challengeCtx.Params["username"] = "agent"

		challengeResp := requireStatus(t, http.StatusOK)(h.HandleAgentRotateKeyChallengeLift(challengeCtx))
		var challenge apimodels.AgentKeyChallengeResponse
		require.NoError(t, json.Unmarshal(challengeResp.Body, &challenge))

		state.agentKeyChallengesByID[challenge.ID] = storagemodels.AgentKeyChallenge{
			PK:        "AGENT_KEY_CHALLENGE#" + challenge.ID,
			SK:        "CHALLENGE",
			ID:        challenge.ID,
			Username:  challenge.Username,
			Action:    challenge.Action,
			Message:   challenge.Message,
			IssuedAt:  challenge.IssuedAt,
			ExpiresAt: challenge.ExpiresAt,
			TTL:       challenge.ExpiresAt.Unix(),
			Used:      false,
		}

		newPub, _, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)

		rotateSig := ed25519.Sign(priv, []byte(challenge.Message))
		rotateCtx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent/rotate_key", map[string]string{
			"Authorization": "Bearer " + writeToken,
		}, nil, apimodels.AgentRotateKeyRequest{
			PublicKey:   base64.StdEncoding.EncodeToString(newPub),
			KeyType:     "ed25519",
			ChallengeID: challenge.ID,
			Signature:   base64.StdEncoding.EncodeToString(rotateSig),
		})
		require.NoError(t, err)
		rotateCtx.Params["username"] = "agent"

		requireStatus(t, http.StatusUnauthorized)(h.HandleAgentRotateKeyLift(rotateCtx))
	})

	t.Run("missing governance returns 503", func(t *testing.T) {
		state := baseState()
		h, _, _ := round11NewHandler(t, cfg, state)
		h.repos.Account().SetEncryptor(noopEncryptor{})

		readToken := round12SignAgentAccessToken(t, cfg.JWTSecret, "agent", []string{auth.ScopeRead})
		writeToken := round12SignAgentAccessToken(t, cfg.JWTSecret, "agent", []string{auth.ScopeWrite})

		challengeCtx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent/rotate_key/challenge", map[string]string{
			"Authorization": "Bearer " + readToken,
		}, nil, nil)
		require.NoError(t, err)
		challengeCtx.Params["username"] = "agent"

		challengeResp := requireStatus(t, http.StatusOK)(h.HandleAgentRotateKeyChallengeLift(challengeCtx))
		var challenge apimodels.AgentKeyChallengeResponse
		require.NoError(t, json.Unmarshal(challengeResp.Body, &challenge))

		newPub, _, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)

		rotateSig := ed25519.Sign(priv, []byte(challenge.Message))
		rotateCtx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent/rotate_key", map[string]string{
			"Authorization": "Bearer " + writeToken,
		}, nil, apimodels.AgentRotateKeyRequest{
			PublicKey:   base64.StdEncoding.EncodeToString(newPub),
			KeyType:     "ed25519",
			ChallengeID: challenge.ID,
			Signature:   base64.StdEncoding.EncodeToString(rotateSig),
		})
		require.NoError(t, err)
		rotateCtx.Params["username"] = "agent"

		requireStatus(t, http.StatusServiceUnavailable)(h.HandleAgentRotateKeyLift(rotateCtx))
	})
}

func TestAgentSelfSovereignRound16_AgentKeyChallengeResponseNil(t *testing.T) {
	out := agentKeyChallengeResponse(nil)
	require.Empty(t, out.ID)
	require.Empty(t, out.Username)
}
