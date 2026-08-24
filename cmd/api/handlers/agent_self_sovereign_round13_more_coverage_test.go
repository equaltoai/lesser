package handlers

import (
	"net/http"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v4/runtime"
)

func TestAgentSelfSovereignRound13_ParseAndScopesHelpers(t *testing.T) {
	t.Run("parseAgentSelfRegistrationRequest rejects invalid JSON", func(t *testing.T) {
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/agents/register", nil, nil, []byte("{bad"))
		_, resp, err := parseAgentSelfRegistrationRequest(ctx)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusBadRequest, resp.Status)
	})

	t.Run("parseAgentSelfRegistrationRequest trims fields", func(t *testing.T) {
		req := apimodels.AgentSelfRegistrationRequest{
			Username:    "  alice ",
			DisplayName: "  Alice ",
			PublicKey:   "  pk ",
			KeyType:     " ed25519 ",
			ChallengeID: "  c1 ",
			Signature:   "  sig ",
		}

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/register", nil, nil, req)
		require.NoError(t, err)

		parsed, resp, parseErr := parseAgentSelfRegistrationRequest(ctx)
		require.NoError(t, parseErr)
		require.Nil(t, resp)
		require.Equal(t, "alice", parsed.Username)
		require.Equal(t, "Alice", parsed.DisplayName)
		require.Equal(t, "pk", parsed.PublicKey)
		require.Equal(t, "ed25519", parsed.KeyType)
		require.Equal(t, "c1", parsed.ChallengeID)
		require.Equal(t, "sig", parsed.Signature)
	})

	t.Run("agentSelfSovereignScopes normalizes governance variants", func(t *testing.T) {
		require.Equal(t, []string{auth.ScopeRead, auth.ScopeWrite, "follow"}, agentSelfSovereignScopes(nil))
		require.Equal(t, []string{auth.ScopeRead, auth.ScopeWrite, "follow"}, agentSelfSovereignScopes(&storage.AgentGovernanceState{}))

		governance := &storage.AgentGovernanceState{
			SelfScopes: []string{"read", "write:statuses", "follow"},
		}
		require.Equal(t, []string{auth.ScopeRead, auth.ScopeWrite, "follow"}, agentSelfSovereignScopes(governance))

		governance.SelfScopes = []string{"read", "write", "follow", ""}
		require.Equal(t, []string{auth.ScopeRead, auth.ScopeWrite, "follow"}, agentSelfSovereignScopes(governance))
	})
}

func TestAgentSelfSovereignRound13_ValidateAndLoadChallengeBranches(t *testing.T) {
	cfg := round10TestConfig()

	t.Run("validateAgentSelfRegistrationRequest enforces required fields", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		resp, err := h.validateAgentSelfRegistrationRequest(&apptheory.Context{}, &apimodels.AgentSelfRegistrationRequest{
			Username: "alice",
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusBadRequest, resp.Status)
	})

	t.Run("loadAndValidateAgentKeyChallenge rejects missing storage", func(t *testing.T) {
		h := &Handler{}
		_, resp, err := h.loadAndValidateAgentKeyChallenge(&apptheory.Context{}, "c1", "alice", agentKeyActionRegister)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusServiceUnavailable, resp.Status)
	})

	t.Run("loadAndValidateAgentKeyChallenge rejects missing fields", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		_, resp, err := h.loadAndValidateAgentKeyChallenge(&apptheory.Context{}, "", "alice", agentKeyActionRegister)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusBadRequest, resp.Status)
	})

	t.Run("loadAndValidateAgentKeyChallenge returns unauthorized on not found", func(t *testing.T) {
		state := &round10QueryState{
			notFoundPKSK: map[string]bool{
				"AGENT_KEY_CHALLENGE#missing#CHALLENGE": true,
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state)

		_, resp, err := h.loadAndValidateAgentKeyChallenge(&apptheory.Context{}, "missing", "alice", agentKeyActionRegister)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusUnauthorized, resp.Status)
	})

	t.Run("loadAndValidateAgentKeyChallenge rejects mismatched username/action", func(t *testing.T) {
		now := time.Now().UTC()
		expiresAt := now.Add(10 * time.Minute)
		state := &round10QueryState{
			agentKeyChallengesByID: map[string]storagemodels.AgentKeyChallenge{
				"c1": {
					PK:        "AGENT_KEY_CHALLENGE#c1",
					SK:        "CHALLENGE",
					ID:        "c1",
					Username:  "alice",
					Action:    agentKeyActionRegister,
					Message:   "m",
					IssuedAt:  now,
					ExpiresAt: expiresAt,
					TTL:       expiresAt.Unix(),
				},
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state)

		_, resp, err := h.loadAndValidateAgentKeyChallenge(&apptheory.Context{}, "c1", "bob", agentKeyActionRegister)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusUnauthorized, resp.Status)
	})

	t.Run("loadAndValidateAgentKeyChallenge rejects expired challenges", func(t *testing.T) {
		now := time.Now().UTC()
		expiresAt := now.Add(-1 * time.Minute)
		state := &round10QueryState{
			agentKeyChallengesByID: map[string]storagemodels.AgentKeyChallenge{
				"c1": {
					PK:        "AGENT_KEY_CHALLENGE#c1",
					SK:        "CHALLENGE",
					ID:        "c1",
					Username:  "alice",
					Action:    agentKeyActionRegister,
					Message:   "m",
					IssuedAt:  now.Add(-10 * time.Minute),
					ExpiresAt: expiresAt,
					TTL:       expiresAt.Unix(),
				},
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state)

		_, resp, err := h.loadAndValidateAgentKeyChallenge(&apptheory.Context{}, "c1", "alice", agentKeyActionRegister)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusUnauthorized, resp.Status)
	})

	t.Run("loadAndValidateAgentKeyChallenge rejects already used challenges", func(t *testing.T) {
		now := time.Now().UTC()
		expiresAt := now.Add(10 * time.Minute)
		state := &round10QueryState{
			agentKeyChallengesByID: map[string]storagemodels.AgentKeyChallenge{
				"c1": {
					PK:        "AGENT_KEY_CHALLENGE#c1",
					SK:        "CHALLENGE",
					ID:        "c1",
					Username:  "alice",
					Action:    agentKeyActionRegister,
					Message:   "m",
					IssuedAt:  now,
					ExpiresAt: expiresAt,
					TTL:       expiresAt.Unix(),
					Used:      true,
				},
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state)

		_, resp, err := h.loadAndValidateAgentKeyChallenge(&apptheory.Context{}, "c1", "alice", agentKeyActionRegister)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusUnauthorized, resp.Status)
	})
}
