package handlers

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
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
	"go.uber.org/zap/zaptest"
)

func TestAgentSelfSovereignRound14_NormalizeScopesDefaultsWhenInvalid(t *testing.T) {
	require.Equal(t, []string{auth.ScopeRead, auth.ScopeWrite, "follow"}, normalizeSelfSovereignScopes(nil))
	require.Equal(t, []string{auth.ScopeRead, auth.ScopeWrite, "follow"}, normalizeSelfSovereignScopes([]string{}))
	require.Equal(t, []string{auth.ScopeRead, auth.ScopeWrite, "follow"}, normalizeSelfSovereignScopes([]string{"profile", "  "}))
	require.Equal(t, []string{auth.ScopeRead, auth.ScopeWrite, "follow"}, normalizeSelfSovereignScopes([]string{"read", "read", "write:statuses", "follow"}))
}

func TestAgentSelfSovereignRound14_ValidateUsesMastodonLogic(t *testing.T) {
	cfg := round10TestConfig()
	cfg.AllowAgents = true
	cfg.AllowAgentRegistration = true

	policy := storagemodels.NewAgentInstanceConfig()
	policy.AllowAgents = true
	policy.AllowAgentRegistration = true

	state := &round10QueryState{agentInstanceConfig: policy}
	h, _, _ := round11NewHandler(t, cfg, state)
	h.mastodonLogic = common.NewMastodonBusinessLogic(common.DefaultMastodonConfig(), zaptest.NewLogger(t))

	resp, err := h.validateAgentSelfRegistrationRequest(&apptheory.Context{}, &apimodels.AgentSelfRegistrationRequest{
		Username:    "alice",
		DisplayName: "this display name is far too long for policy enforcement",
		PublicKey:   "pk",
		KeyType:     "ed25519",
		ChallengeID: "c1",
		Signature:   "sig",
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, http.StatusBadRequest, resp.Status)
}

func TestAgentSelfSovereignRound14_CreateAndConsumeChallengeBranches(t *testing.T) {
	cfg := round10TestConfig()
	cfg.AllowAgents = true
	cfg.AllowAgentRegistration = true

	policy := storagemodels.NewAgentInstanceConfig()
	policy.AllowAgents = true
	policy.AllowAgentRegistration = true

	state := &round10QueryState{agentInstanceConfig: policy}
	h, _, _ := round11NewHandler(t, cfg, state)

	t.Run("createAgentKeyChallenge requires storage and fields", func(t *testing.T) {
		_, err := (&Handler{}).createAgentKeyChallenge(&apptheory.Context{}, "alice", agentKeyActionRegister, time.Minute)
		require.Error(t, err)

		_, err = h.createAgentKeyChallenge(&apptheory.Context{}, "", agentKeyActionRegister, time.Minute)
		require.Error(t, err)
	})

	t.Run("createAgentKeyChallenge returns populated model", func(t *testing.T) {
		challenge, err := h.createAgentKeyChallenge(&apptheory.Context{}, "alice", agentKeyActionRegister, time.Minute)
		require.NoError(t, err)
		require.NotNil(t, challenge)
		require.NotEmpty(t, challenge.ID)
		require.NotEmpty(t, challenge.Message)
		require.Equal(t, "alice", challenge.Username)
		require.Equal(t, agentKeyActionRegister, challenge.Action)
		require.False(t, challenge.Used)
		require.False(t, challenge.ExpiresAt.IsZero())
		require.Equal(t, "AGENT_KEY_CHALLENGE#"+challenge.ID, challenge.PK)
		require.Equal(t, "CHALLENGE", challenge.SK)
		require.Equal(t, challenge.ExpiresAt.Unix(), challenge.TTL)
	})

	t.Run("markAgentKeyChallengeUsed rejects empty id", func(t *testing.T) {
		require.Error(t, h.markAgentKeyChallengeUsed(&apptheory.Context{}, ""))
	})

	t.Run("verifyAndConsumeSelfRegistrationChallenge handles invalid signature and condition failures", func(t *testing.T) {
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)
		pubB64 := base64.StdEncoding.EncodeToString(pub)

		now := time.Now().UTC()
		expiresAt := now.Add(10 * time.Minute)
		challenge := storagemodels.AgentKeyChallenge{
			PK:        "AGENT_KEY_CHALLENGE#c1",
			SK:        "CHALLENGE",
			ID:        "c1",
			Username:  "alice",
			Action:    agentKeyActionRegister,
			Message:   "message-to-sign",
			IssuedAt:  now,
			ExpiresAt: expiresAt,
			TTL:       expiresAt.Unix(),
			Used:      false,
		}

		newHandler := func(t *testing.T, execErr error) *Handler {
			t.Helper()
			state := &round10QueryState{
				agentInstanceConfig:    policy,
				agentKeyChallengesByID: map[string]storagemodels.AgentKeyChallenge{"c1": challenge},
				executeErrorOnce:       execErr,
			}
			h, _, _ := round11NewHandler(t, cfg, state)
			return h
		}

		req := &apimodels.AgentSelfRegistrationRequest{
			Username:    "alice",
			DisplayName: "Alice",
			PublicKey:   pubB64,
			KeyType:     "ed25519",
			ChallengeID: "c1",
			Signature:   "not-base64",
		}

		resp, err := newHandler(t, nil).verifyAndConsumeSelfRegistrationChallenge(&apptheory.Context{}, req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusUnauthorized, resp.Status)

		sig := ed25519.Sign(priv, []byte(challenge.Message))
		req.Signature = base64.StdEncoding.EncodeToString(sig)

		resp, err = newHandler(t, dynamormerrors.ErrConditionFailed).verifyAndConsumeSelfRegistrationChallenge(&apptheory.Context{}, req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusUnauthorized, resp.Status)

		resp, err = newHandler(t, errors.New("boom")).verifyAndConsumeSelfRegistrationChallenge(&apptheory.Context{}, req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusInternalServerError, resp.Status)
	})
}

func TestAgentSelfSovereignRound14_AgentAuthErrorBranches(t *testing.T) {
	cfg := round10TestConfig()
	cfg.AllowAgents = true

	policy := storagemodels.NewAgentInstanceConfig()
	policy.AllowAgents = true

	t.Run("auth challenge returns not found when agent missing", func(t *testing.T) {
		state := &round10QueryState{
			agentInstanceConfig: policy,
			notFoundPKSK: map[string]bool{
				"USER#missing#METADATA": true,
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state)

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/auth/challenge", nil, nil, apimodels.AgentKeyChallengeRequest{
			Username: "missing",
		})
		require.NoError(t, err)

		resp := requireStatus(t, http.StatusNotFound)(h.HandleAgentAuthChallengeLift(ctx))
		require.NotNil(t, resp)
	})

	t.Run("auth challenge forbids agents missing self-sovereign key", func(t *testing.T) {
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

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/auth/challenge", nil, nil, apimodels.AgentKeyChallengeRequest{
			Username: "agent",
		})
		require.NoError(t, err)

		resp := requireStatus(t, http.StatusForbidden)(h.HandleAgentAuthChallengeLift(ctx))
		require.NotNil(t, resp)
	})
}
