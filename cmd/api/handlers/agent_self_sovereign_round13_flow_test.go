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
	"github.com/stretchr/testify/require"
)

func TestAgentSelfSovereignRound13_Flows(t *testing.T) {
	cfg := round10TestConfig()
	cfg.AllowAgents = true
	cfg.AllowAgentRegistration = true

	policy := storagemodels.NewAgentInstanceConfig()
	policy.AllowAgents = true
	policy.AllowAgentRegistration = true

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	pubB64 := base64.StdEncoding.EncodeToString(pub)

	t.Run("register challenge + register", func(t *testing.T) {
		state := &round10QueryState{
			agentInstanceConfig:    policy,
			agentKeyChallengesByID: map[string]storagemodels.AgentKeyChallenge{},
			usersByUsername: map[string]storagemodels.User{
				"owner": {PK: "USER#owner", SK: storagemodels.SKMetadata, Username: "owner", Role: "user", Approved: true, Version: 1},
			},
		}

		h, _, _ := round11NewHandler(t, cfg, state)
		h.repos.Account().SetEncryptor(noopEncryptor{})

		challengeCtx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/register/challenge", nil, nil, apimodels.AgentKeyChallengeRequest{
			Username: "agentreg",
		})
		require.NoError(t, err)

		challengeResp := requireStatus(t, http.StatusOK)(h.HandleAgentRegisterChallengeLift(challengeCtx))
		var challenge apimodels.AgentKeyChallengeResponse
		require.NoError(t, json.Unmarshal(challengeResp.Body, &challenge))
		require.NotEmpty(t, challenge.ID)
		require.NotEmpty(t, challenge.Message)

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

		sig := ed25519.Sign(priv, []byte(challenge.Message))
		registerCtx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/register", nil, nil, apimodels.AgentSelfRegistrationRequest{
			Username:    "agentreg",
			DisplayName: "Agent Reg",
			PublicKey:   pubB64,
			KeyType:     "ed25519",
			ChallengeID: challenge.ID,
			Signature:   base64.StdEncoding.EncodeToString(sig),
			Scopes:      []string{auth.ScopeRead, "write:statuses", "follow"},
		})
		require.NoError(t, err)

		requireStatus(t, http.StatusOK)(h.HandleAgentRegisterLift(registerCtx))
	})

	t.Run("auth challenge + token", func(t *testing.T) {
		state := &round10QueryState{
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
			agentGovernanceByUsername: map[string]storagemodels.AgentGovernanceState{
				"agent": {
					PK:            "USER#agent",
					SK:            storagemodels.SKAgentGovernance,
					Username:      "agent",
					SelfScopes:    []string{auth.ScopeRead, auth.ScopeWrite, "follow"},
					SelfSovereign: true,
					CreatedAt:     time.Now().Add(-24 * time.Hour),
					UpdatedAt:     time.Now().Add(-time.Hour),
					Version:       1,
				},
			},
		}

		h, _, _ := round11NewHandler(t, cfg, state)
		h.repos.Account().SetEncryptor(noopEncryptor{})

		challengeCtx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/auth/challenge", nil, nil, apimodels.AgentKeyChallengeRequest{
			Username: "agent",
		})
		require.NoError(t, err)

		challengeResp := requireStatus(t, http.StatusOK)(h.HandleAgentAuthChallengeLift(challengeCtx))
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

		sig := ed25519.Sign(priv, []byte(challenge.Message))
		tokenCtx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/auth/token", nil, nil, apimodels.AgentSelfAuthTokenRequest{
			Username:    "agent",
			ChallengeID: challenge.ID,
			Signature:   base64.StdEncoding.EncodeToString(sig),
		})
		require.NoError(t, err)

		requireStatus(t, http.StatusOK)(h.HandleAgentAuthTokenLift(tokenCtx))
	})

	t.Run("rotate key challenge + rotate key", func(t *testing.T) {
		state := &round10QueryState{
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
			agentGovernanceByUsername: map[string]storagemodels.AgentGovernanceState{
				"agent": {
					PK:            "USER#agent",
					SK:            storagemodels.SKAgentGovernance,
					Username:      "agent",
					SelfScopes:    []string{auth.ScopeRead, auth.ScopeWrite, "follow"},
					SelfSovereign: true,
					CreatedAt:     time.Now().Add(-24 * time.Hour),
					UpdatedAt:     time.Now().Add(-time.Hour),
					Version:       1,
				},
			},
		}

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

		newPub, newPriv, err := ed25519.GenerateKey(rand.Reader)
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

		requireStatus(t, http.StatusOK)(h.HandleAgentRotateKeyLift(rotateCtx))

		_ = newPriv
	})
}
