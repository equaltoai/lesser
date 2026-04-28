package handlers

import (
	"crypto/ed25519"
	crand "crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"
	dynamormerrors "github.com/theory-cloud/tabletheory/pkg/errors"
)

func TestAgentAccessLeasesRound20_ErrorPaths(t *testing.T) {
	cfg := round10TestConfig()
	cfg.AllowAgents = true
	cfg.AllowAgentRegistration = true

	policy := storagemodels.NewAgentInstanceConfig()
	policy.AllowAgents = true
	policy.AllowAgentRegistration = true

	ownerKey := mustGenerateWalletKey(t)
	agentKey := mustGenerateWalletKey(t)
	ownerAddr := strings.ToLower(crypto.PubkeyToAddress(ownerKey.PublicKey).Hex())
	agentAddr := strings.ToLower(crypto.PubkeyToAddress(agentKey.PublicKey).Hex())
	now := time.Now().UTC()

	baseState := func() *round10QueryState {
		return &round10QueryState{
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
			agentGovernanceByUsername: map[string]storagemodels.AgentGovernanceState{
				"agent1": {
					PK:              "USER#agent1",
					SK:              storagemodels.SKAgentGovernance,
					Username:        "agent1",
					DelegatedScopes: []string{auth.ScopeRead, auth.ScopeWrite, "follow"},
					CreatedAt:       now.Add(-24 * time.Hour),
					UpdatedAt:       now.Add(-time.Hour),
				},
			},
			walletCredentialsByUser: map[string][]storagemodels.WalletCredential{
				"owner": {{
					Username: "owner",
					Address:  ownerAddr,
					Type:     "ethereum",
					ChainID:  1,
					LinkedAt: now.Add(-24 * time.Hour),
					LastUsed: now.Add(-time.Hour),
				}},
				"agent1": {{
					Username: "agent1",
					Address:  agentAddr,
					Type:     "ethereum",
					ChainID:  1,
					LinkedAt: now.Add(-24 * time.Hour),
					LastUsed: now.Add(-time.Hour),
				}},
			},
		}
	}

	headers := map[string]string{"Authorization": "Bearer " + round11SignAccessToken(t, cfg.JWTSecret, "owner", []string{auth.ScopeWrite, auth.ScopeRead, "follow"})}

	t.Run("principal challenge rejects malformed body", func(t *testing.T) {
		state := baseState()
		h, _, _ := round11NewHandler(t, cfg, state)
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/agents/agent1/access-leases/challenge/principal", headers, nil, []byte("{bad"))
		ctx.Params["username"] = "agent1"
		requireStatus(t, http.StatusBadRequest)(h.HandleCreateAgentAccessLeasePrincipalChallengeLift(ctx))
	})

	t.Run("principal challenge rejects missing principal wallet binding", func(t *testing.T) {
		state := baseState()
		state.walletCredentialsByUser["owner"] = nil
		h, _, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent1/access-leases/challenge/principal", headers, nil, apimodels.AgentAccessLeaseChallengeRequest{
			PrincipalWallet: ownerAddr,
			AgentWallet:     agentAddr,
			Scopes:          []string{"read"},
			DeviceLabel:     "local-agent",
		})
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"
		requireStatus(t, http.StatusForbidden)(h.HandleCreateAgentAccessLeasePrincipalChallengeLift(ctx))
	})

	t.Run("create lease rejects invalid signature", func(t *testing.T) {
		state := baseState()
		h, _, _ := round11NewHandler(t, cfg, state)

		req := apimodels.AgentAccessLeaseChallengeRequest{
			PrincipalWallet:  ownerAddr,
			AgentWallet:      agentAddr,
			Scopes:           []string{"read"},
			DeviceLabel:      "local-agent",
			IdleTimeoutHours: 24,
			AbsoluteTTLHours: 48,
		}
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent1/access-leases/challenge/principal", headers, nil, req)
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"
		resp := requireStatus(t, http.StatusOK)(h.HandleCreateAgentAccessLeasePrincipalChallengeLift(ctx))
		var principalResp apimodels.AgentAccessLeaseChallengeResponse
		require.NoError(t, json.Unmarshal(resp.Body, &principalResp))

		req.LeaseID = principalResp.LeaseID
		ctx, err = round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent1/access-leases/challenge/agent", headers, nil, req)
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"
		resp = requireStatus(t, http.StatusOK)(h.HandleCreateAgentAccessLeaseAgentChallengeLift(ctx))
		var agentResp apimodels.AgentAccessLeaseChallengeResponse
		require.NoError(t, json.Unmarshal(resp.Body, &agentResp))

		ctx, err = round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent1/access-leases", headers, nil, apimodels.CreateAgentAccessLeaseRequest{
			PrincipalChallengeID: principalResp.ID,
			PrincipalSignature:   "0xdeadbeef",
			AgentChallengeID:     agentResp.ID,
			AgentSignature:       "0xdeadbeef",
		})
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"
		requireStatus(t, http.StatusUnauthorized)(h.HandleCreateAgentAccessLeaseLift(ctx))
	})

	t.Run("create lease rejects missing required fields", func(t *testing.T) {
		state := baseState()
		h, _, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent1/access-leases", headers, nil, apimodels.CreateAgentAccessLeaseRequest{})
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"
		requireStatus(t, http.StatusBadRequest)(h.HandleCreateAgentAccessLeaseLift(ctx))
	})

	t.Run("create lease rejects used challenge", func(t *testing.T) {
		state := baseState()
		state.agentAccessChallengesByID = map[string]storagemodels.AgentAccessLeaseChallenge{
			"principal": {
				PK:                "AGENT_ACCESS_CHALLENGE#principal",
				SK:                "CHALLENGE",
				ID:                "principal",
				LeaseID:           "lease-a",
				Username:          "agent1",
				Action:            agentAccessLeaseActionPrincipal,
				Address:           ownerAddr,
				PrincipalUsername: "owner",
				PrincipalWallet:   ownerAddr,
				AgentWallet:       agentAddr,
				Scopes:            []string{"read"},
				DeviceLabel:       "local-agent",
				IdleTimeoutHours:  24,
				AbsoluteTTLHours:  48,
				Message:           "principal",
				IssuedAt:          now,
				ExpiresAt:         now.Add(time.Minute),
				TTL:               now.Add(time.Minute).Unix(),
				Used:              true,
			},
			"agent": {
				PK:                "AGENT_ACCESS_CHALLENGE#agent",
				SK:                "CHALLENGE",
				ID:                "agent",
				LeaseID:           "lease-a",
				Username:          "agent1",
				Action:            agentAccessLeaseActionAgent,
				Address:           agentAddr,
				PrincipalUsername: "owner",
				PrincipalWallet:   ownerAddr,
				AgentWallet:       agentAddr,
				Scopes:            []string{"read"},
				DeviceLabel:       "local-agent",
				IdleTimeoutHours:  24,
				AbsoluteTTLHours:  48,
				Message:           "agent",
				IssuedAt:          now,
				ExpiresAt:         now.Add(time.Minute),
				TTL:               now.Add(time.Minute).Unix(),
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent1/access-leases", headers, nil, apimodels.CreateAgentAccessLeaseRequest{
			PrincipalChallengeID: "principal",
			PrincipalSignature:   "0xdeadbeef",
			AgentChallengeID:     "agent",
			AgentSignature:       "0xdeadbeef",
		})
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"
		requireStatus(t, http.StatusUnauthorized)(h.HandleCreateAgentAccessLeaseLift(ctx))
	})

	t.Run("create lease rejects missing agent wallet binding", func(t *testing.T) {
		state := baseState()
		delete(state.walletCredentialsByUser, "agent1")
		state.agentAccessChallengesByID = map[string]storagemodels.AgentAccessLeaseChallenge{
			"principal": {
				PK:                "AGENT_ACCESS_CHALLENGE#principal",
				SK:                "CHALLENGE",
				ID:                "principal",
				LeaseID:           "lease-a",
				Username:          "agent1",
				Action:            agentAccessLeaseActionPrincipal,
				Address:           ownerAddr,
				PrincipalUsername: "owner",
				PrincipalWallet:   ownerAddr,
				AgentWallet:       agentAddr,
				Scopes:            []string{"read"},
				DeviceLabel:       "local-agent",
				IdleTimeoutHours:  24,
				AbsoluteTTLHours:  48,
				Message:           "principal",
				IssuedAt:          now,
				ExpiresAt:         now.Add(time.Minute),
				TTL:               now.Add(time.Minute).Unix(),
			},
			"agent": {
				PK:                "AGENT_ACCESS_CHALLENGE#agent",
				SK:                "CHALLENGE",
				ID:                "agent",
				LeaseID:           "lease-a",
				Username:          "agent1",
				Action:            agentAccessLeaseActionAgent,
				Address:           agentAddr,
				PrincipalUsername: "owner",
				PrincipalWallet:   ownerAddr,
				AgentWallet:       agentAddr,
				Scopes:            []string{"read"},
				DeviceLabel:       "local-agent",
				IdleTimeoutHours:  24,
				AbsoluteTTLHours:  48,
				Message:           "agent",
				IssuedAt:          now,
				ExpiresAt:         now.Add(time.Minute),
				TTL:               now.Add(time.Minute).Unix(),
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent1/access-leases", headers, nil, apimodels.CreateAgentAccessLeaseRequest{
			PrincipalChallengeID: "principal",
			PrincipalSignature:   "0xdeadbeef",
			AgentChallengeID:     "agent",
			AgentSignature:       "0xdeadbeef",
		})
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"
		requireStatus(t, http.StatusForbidden)(h.HandleCreateAgentAccessLeaseLift(ctx))
	})

	t.Run("create lease rejects mismatched challenges", func(t *testing.T) {
		state := baseState()
		state.agentAccessChallengesByID = map[string]storagemodels.AgentAccessLeaseChallenge{
			"principal": {
				PK:                "AGENT_ACCESS_CHALLENGE#principal",
				SK:                "CHALLENGE",
				ID:                "principal",
				LeaseID:           "lease-a",
				Username:          "agent1",
				Action:            agentAccessLeaseActionPrincipal,
				Address:           ownerAddr,
				PrincipalUsername: "owner",
				PrincipalWallet:   ownerAddr,
				AgentWallet:       agentAddr,
				Scopes:            []string{"read"},
				DeviceLabel:       "local-agent",
				IdleTimeoutHours:  24,
				AbsoluteTTLHours:  48,
				Message:           "principal",
				IssuedAt:          now,
				ExpiresAt:         now.Add(time.Minute),
				TTL:               now.Add(time.Minute).Unix(),
			},
			"agent": {
				PK:                "AGENT_ACCESS_CHALLENGE#agent",
				SK:                "CHALLENGE",
				ID:                "agent",
				LeaseID:           "lease-b",
				Username:          "agent1",
				Action:            agentAccessLeaseActionAgent,
				Address:           agentAddr,
				PrincipalUsername: "owner",
				PrincipalWallet:   ownerAddr,
				AgentWallet:       agentAddr,
				Scopes:            []string{"read"},
				DeviceLabel:       "local-agent",
				IdleTimeoutHours:  24,
				AbsoluteTTLHours:  48,
				Message:           "agent",
				IssuedAt:          now,
				ExpiresAt:         now.Add(time.Minute),
				TTL:               now.Add(time.Minute).Unix(),
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent1/access-leases", headers, nil, apimodels.CreateAgentAccessLeaseRequest{
			PrincipalChallengeID: "principal",
			PrincipalSignature:   "0xdeadbeef",
			AgentChallengeID:     "agent",
			AgentSignature:       "0xdeadbeef",
		})
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"
		requireStatus(t, http.StatusUnauthorized)(h.HandleCreateAgentAccessLeaseLift(ctx))
	})

	t.Run("create lease rejects challenge action mismatch", func(t *testing.T) {
		state := baseState()
		state.agentAccessChallengesByID = map[string]storagemodels.AgentAccessLeaseChallenge{
			"principal": {
				PK:                "AGENT_ACCESS_CHALLENGE#principal",
				SK:                "CHALLENGE",
				ID:                "principal",
				LeaseID:           "lease-a",
				Username:          "agent1",
				Action:            agentAccessLeaseActionAgent,
				Address:           ownerAddr,
				PrincipalUsername: "owner",
				PrincipalWallet:   ownerAddr,
				AgentWallet:       agentAddr,
				Scopes:            []string{"read"},
				DeviceLabel:       "local-agent",
				IdleTimeoutHours:  24,
				AbsoluteTTLHours:  48,
				Message:           "principal",
				IssuedAt:          now,
				ExpiresAt:         now.Add(time.Minute),
				TTL:               now.Add(time.Minute).Unix(),
			},
			"agent": {
				PK:                "AGENT_ACCESS_CHALLENGE#agent",
				SK:                "CHALLENGE",
				ID:                "agent",
				LeaseID:           "lease-a",
				Username:          "agent1",
				Action:            agentAccessLeaseActionAgent,
				Address:           agentAddr,
				PrincipalUsername: "owner",
				PrincipalWallet:   ownerAddr,
				AgentWallet:       agentAddr,
				Scopes:            []string{"read"},
				DeviceLabel:       "local-agent",
				IdleTimeoutHours:  24,
				AbsoluteTTLHours:  48,
				Message:           "agent",
				IssuedAt:          now,
				ExpiresAt:         now.Add(time.Minute),
				TTL:               now.Add(time.Minute).Unix(),
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent1/access-leases", headers, nil, apimodels.CreateAgentAccessLeaseRequest{
			PrincipalChallengeID: "principal",
			PrincipalSignature:   "0xdeadbeef",
			AgentChallengeID:     "agent",
			AgentSignature:       "0xdeadbeef",
		})
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"
		requireStatus(t, http.StatusUnauthorized)(h.HandleCreateAgentAccessLeaseLift(ctx))
	})

	t.Run("create lease rejects principal username mismatch", func(t *testing.T) {
		state := baseState()
		state.agentAccessChallengesByID = map[string]storagemodels.AgentAccessLeaseChallenge{
			"principal": {
				PK:                "AGENT_ACCESS_CHALLENGE#principal",
				SK:                "CHALLENGE",
				ID:                "principal",
				LeaseID:           "lease-a",
				Username:          "agent1",
				Action:            agentAccessLeaseActionPrincipal,
				Address:           ownerAddr,
				PrincipalUsername: "other-owner",
				PrincipalWallet:   ownerAddr,
				AgentWallet:       agentAddr,
				Scopes:            []string{"read"},
				DeviceLabel:       "local-agent",
				IdleTimeoutHours:  24,
				AbsoluteTTLHours:  48,
				Message:           "principal",
				IssuedAt:          now,
				ExpiresAt:         now.Add(time.Minute),
				TTL:               now.Add(time.Minute).Unix(),
			},
			"agent": {
				PK:                "AGENT_ACCESS_CHALLENGE#agent",
				SK:                "CHALLENGE",
				ID:                "agent",
				LeaseID:           "lease-a",
				Username:          "agent1",
				Action:            agentAccessLeaseActionAgent,
				Address:           agentAddr,
				PrincipalUsername: "other-owner",
				PrincipalWallet:   ownerAddr,
				AgentWallet:       agentAddr,
				Scopes:            []string{"read"},
				DeviceLabel:       "local-agent",
				IdleTimeoutHours:  24,
				AbsoluteTTLHours:  48,
				Message:           "agent",
				IssuedAt:          now,
				ExpiresAt:         now.Add(time.Minute),
				TTL:               now.Add(time.Minute).Unix(),
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent1/access-leases", headers, nil, apimodels.CreateAgentAccessLeaseRequest{
			PrincipalChallengeID: "principal",
			PrincipalSignature:   "0xdeadbeef",
			AgentChallengeID:     "agent",
			AgentSignature:       "0xdeadbeef",
		})
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"
		requireStatus(t, http.StatusUnauthorized)(h.HandleCreateAgentAccessLeaseLift(ctx))
	})

	t.Run("create lease surfaces mark-used condition failure", func(t *testing.T) {
		state := baseState()
		principal := buildCreateLeaseChallengeRound20(now, "principal", "lease-a", ownerAddr, ownerAddr, agentAddr, agentAccessLeaseActionPrincipal, "owner")
		agent := buildCreateLeaseChallengeRound20(now, "agent", "lease-a", agentAddr, ownerAddr, agentAddr, agentAccessLeaseActionAgent, "owner")
		state.agentAccessChallengesByID = map[string]storagemodels.AgentAccessLeaseChallenge{
			"principal": principal,
			"agent":     agent,
		}
		state.executeErrorOnce = dynamormerrors.ErrConditionFailed
		h, _, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent1/access-leases", headers, nil, apimodels.CreateAgentAccessLeaseRequest{
			PrincipalChallengeID: "principal",
			PrincipalSignature:   signTypedDataRound20(t, ownerKey, buildAgentAccessLeaseTypedData(&principal)),
			AgentChallengeID:     "agent",
			AgentSignature:       signTypedDataRound20(t, agentKey, buildAgentAccessLeaseTypedData(&agent)),
		})
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"
		requireStatus(t, http.StatusUnauthorized)(h.HandleCreateAgentAccessLeaseLift(ctx))
	})

	t.Run("create lease surfaces create conflict", func(t *testing.T) {
		state := baseState()
		principal := buildCreateLeaseChallengeRound20(now, "principal", "lease-a", ownerAddr, ownerAddr, agentAddr, agentAccessLeaseActionPrincipal, "owner")
		agent := buildCreateLeaseChallengeRound20(now, "agent", "lease-a", agentAddr, ownerAddr, agentAddr, agentAccessLeaseActionAgent, "owner")
		state.agentAccessChallengesByID = map[string]storagemodels.AgentAccessLeaseChallenge{
			"principal": principal,
			"agent":     agent,
		}
		state.createErrorOnce = dynamormerrors.ErrConditionFailed
		h, _, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent1/access-leases", headers, nil, apimodels.CreateAgentAccessLeaseRequest{
			PrincipalChallengeID: "principal",
			PrincipalSignature:   signTypedDataRound20(t, ownerKey, buildAgentAccessLeaseTypedData(&principal)),
			AgentChallengeID:     "agent",
			AgentSignature:       signTypedDataRound20(t, agentKey, buildAgentAccessLeaseTypedData(&agent)),
		})
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"
		requireStatus(t, http.StatusConflict)(h.HandleCreateAgentAccessLeaseLift(ctx))
	})

	t.Run("list leases requires auth", func(t *testing.T) {
		state := baseState()
		h, _, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/agents/agent1/access-leases", nil, nil, nil)
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"
		requireStatus(t, http.StatusUnauthorized)(h.HandleListAgentAccessLeasesLift(ctx))
	})

	t.Run("list leases forbids wrong owner", func(t *testing.T) {
		state := baseState()
		h, _, _ := round11NewHandler(t, cfg, state)
		badHeaders := map[string]string{"Authorization": "Bearer " + round11SignAccessToken(t, cfg.JWTSecret, "intruder", []string{auth.ScopeWrite})}
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/agents/agent1/access-leases", badHeaders, nil, nil)
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"
		requireStatus(t, http.StatusForbidden)(h.HandleListAgentAccessLeasesLift(ctx))
	})

	t.Run("session key challenge rejects invalid public key", func(t *testing.T) {
		state := baseState()
		leaseID := "lease-1"
		state.agentAccessLeasesByKey = map[string]storagemodels.AgentAccessLease{
			"AGENT_ACCESS_LEASE#agent1#LEASE#" + leaseID: {
				PK:                "AGENT_ACCESS_LEASE#agent1",
				SK:                "LEASE#" + leaseID,
				ID:                leaseID,
				Username:          "agent1",
				PrincipalUsername: "owner",
				PrincipalWallet:   ownerAddr,
				AgentWallet:       agentAddr,
				Scopes:            []string{"read"},
				DeviceLabel:       "local-agent",
				Status:            "active",
				IdleTimeoutHours:  24,
				IdleExpiresAt:     now.Add(24 * time.Hour),
				AbsoluteExpiresAt: now.Add(48 * time.Hour),
				LastUsedAt:        now,
				LeaseVersion:      1,
				CreatedAt:         now,
				UpdatedAt:         now,
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent1/access-leases/"+leaseID+"/session-key/challenge", nil, nil, apimodels.AgentAccessLeaseSessionKeyChallengeRequest{
			SessionPublicKey: "invalid",
		})
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"
		ctx.Params["leaseID"] = leaseID
		requireStatus(t, http.StatusBadRequest)(h.HandleCreateAgentAccessLeaseSessionKeyChallengeLift(ctx))
	})

	t.Run("session key authorization rejects mismatched challenge", func(t *testing.T) {
		state := baseState()
		leaseID := "lease-sess"
		state.agentAccessLeasesByKey = map[string]storagemodels.AgentAccessLease{
			"AGENT_ACCESS_LEASE#agent1#LEASE#" + leaseID: {
				PK:                "AGENT_ACCESS_LEASE#agent1",
				SK:                "LEASE#" + leaseID,
				ID:                leaseID,
				Username:          "agent1",
				PrincipalUsername: "owner",
				PrincipalWallet:   ownerAddr,
				AgentWallet:       agentAddr,
				Scopes:            []string{"read"},
				DeviceLabel:       "local-agent",
				Status:            "active",
				IdleTimeoutHours:  24,
				IdleExpiresAt:     now.Add(24 * time.Hour),
				AbsoluteExpiresAt: now.Add(48 * time.Hour),
				LastUsedAt:        now,
				LeaseVersion:      1,
				CreatedAt:         now,
				UpdatedAt:         now,
			},
		}
		state.agentAccessChallengesByID = map[string]storagemodels.AgentAccessLeaseChallenge{
			"challenge-1": {
				PK:                "AGENT_ACCESS_CHALLENGE#challenge-1",
				SK:                "CHALLENGE",
				ID:                "challenge-1",
				LeaseID:           "other-lease",
				Username:          "agent1",
				Action:            agentAccessLeaseActionSessionKeyAuth,
				Address:           agentAddr,
				PrincipalUsername: "owner",
				PrincipalWallet:   ownerAddr,
				AgentWallet:       agentAddr,
				SessionPublicKey:  "pub",
				SessionKeyType:    "ed25519",
				Scopes:            []string{"read"},
				DeviceLabel:       "local-agent",
				IdleTimeoutHours:  24,
				AbsoluteTTLHours:  48,
				Message:           "challenge",
				IssuedAt:          now,
				ExpiresAt:         now.Add(time.Minute),
				TTL:               now.Add(time.Minute).Unix(),
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent1/access-leases/"+leaseID+"/session-key", nil, nil, apimodels.AuthorizeAgentAccessLeaseSessionKeyRequest{
			ChallengeID: "challenge-1",
			Signature:   "bad",
		})
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"
		ctx.Params["leaseID"] = leaseID
		requireStatus(t, http.StatusUnauthorized)(h.HandleAuthorizeAgentAccessLeaseSessionKeyLift(ctx))
	})

	t.Run("session key authorization rejects invalid signature", func(t *testing.T) {
		state := baseState()
		leaseID := "lease-sig"
		state.agentAccessLeasesByKey = map[string]storagemodels.AgentAccessLease{
			"AGENT_ACCESS_LEASE#agent1#LEASE#" + leaseID: {
				PK:                "AGENT_ACCESS_LEASE#agent1",
				SK:                "LEASE#" + leaseID,
				ID:                leaseID,
				Username:          "agent1",
				PrincipalUsername: "owner",
				PrincipalWallet:   ownerAddr,
				AgentWallet:       agentAddr,
				Scopes:            []string{"read"},
				DeviceLabel:       "local-agent",
				Status:            "active",
				IdleTimeoutHours:  24,
				IdleExpiresAt:     now.Add(24 * time.Hour),
				AbsoluteExpiresAt: now.Add(48 * time.Hour),
				LastUsedAt:        now,
				LeaseVersion:      1,
				CreatedAt:         now,
				UpdatedAt:         now,
			},
		}
		state.agentAccessChallengesByID = map[string]storagemodels.AgentAccessLeaseChallenge{
			"challenge-1": {
				PK:                "AGENT_ACCESS_CHALLENGE#challenge-1",
				SK:                "CHALLENGE",
				ID:                "challenge-1",
				LeaseID:           leaseID,
				Username:          "agent1",
				Action:            agentAccessLeaseActionSessionKeyAuth,
				Address:           agentAddr,
				PrincipalUsername: "owner",
				PrincipalWallet:   ownerAddr,
				AgentWallet:       agentAddr,
				SessionPublicKey:  "pub",
				SessionKeyType:    "ed25519",
				Scopes:            []string{"read"},
				DeviceLabel:       "local-agent",
				IdleTimeoutHours:  24,
				AbsoluteTTLHours:  48,
				Message:           "challenge",
				IssuedAt:          now,
				ExpiresAt:         now.Add(time.Minute),
				TTL:               now.Add(time.Minute).Unix(),
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent1/access-leases/"+leaseID+"/session-key", nil, nil, apimodels.AuthorizeAgentAccessLeaseSessionKeyRequest{
			ChallengeID: "challenge-1",
			Signature:   "0xdeadbeef",
		})
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"
		ctx.Params["leaseID"] = leaseID
		requireStatus(t, http.StatusUnauthorized)(h.HandleAuthorizeAgentAccessLeaseSessionKeyLift(ctx))
	})

	t.Run("session key authorization surfaces update failure", func(t *testing.T) {
		state := baseState()
		leaseID := "lease-update"
		pubKey := strings.Repeat("a", 44)
		state.agentAccessLeasesByKey = map[string]storagemodels.AgentAccessLease{
			"AGENT_ACCESS_LEASE#agent1#LEASE#" + leaseID: {
				PK:                "AGENT_ACCESS_LEASE#agent1",
				SK:                "LEASE#" + leaseID,
				ID:                leaseID,
				Username:          "agent1",
				PrincipalUsername: "owner",
				PrincipalWallet:   ownerAddr,
				AgentWallet:       agentAddr,
				Scopes:            []string{"read"},
				DeviceLabel:       "local-agent",
				Status:            "active",
				IdleTimeoutHours:  24,
				IdleExpiresAt:     now.Add(24 * time.Hour),
				AbsoluteExpiresAt: now.Add(48 * time.Hour),
				LastUsedAt:        now,
				LeaseVersion:      1,
				CreatedAt:         now,
				UpdatedAt:         now,
			},
		}
		challenge := buildSessionKeyChallengeRound20(now, "challenge-1", leaseID, ownerAddr, agentAddr, pubKey)
		state.agentAccessChallengesByID = map[string]storagemodels.AgentAccessLeaseChallenge{"challenge-1": challenge}
		state.executeErrorOnce = errors.New("boom")
		h, _, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent1/access-leases/"+leaseID+"/session-key", nil, nil, apimodels.AuthorizeAgentAccessLeaseSessionKeyRequest{
			ChallengeID: "challenge-1",
			Signature:   signTypedDataRound20(t, agentKey, buildAgentAccessLeaseTypedData(&challenge)),
		})
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"
		ctx.Params["leaseID"] = leaseID
		requireStatus(t, http.StatusInternalServerError)(h.HandleAuthorizeAgentAccessLeaseSessionKeyLift(ctx))
	})

	t.Run("renew challenge rejects inactive lease", func(t *testing.T) {
		state := baseState()
		leaseID := "lease-2"
		state.agentAccessLeasesByKey = map[string]storagemodels.AgentAccessLease{
			"AGENT_ACCESS_LEASE#agent1#LEASE#" + leaseID: {
				PK:                "AGENT_ACCESS_LEASE#agent1",
				SK:                "LEASE#" + leaseID,
				ID:                leaseID,
				Username:          "agent1",
				PrincipalUsername: "owner",
				PrincipalWallet:   ownerAddr,
				AgentWallet:       agentAddr,
				Scopes:            []string{"read"},
				DeviceLabel:       "local-agent",
				Status:            "revoked",
				IdleTimeoutHours:  24,
				IdleExpiresAt:     now.Add(24 * time.Hour),
				AbsoluteExpiresAt: now.Add(48 * time.Hour),
				LastUsedAt:        now,
				LeaseVersion:      1,
				CreatedAt:         now,
				UpdatedAt:         now,
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent1/access-leases/"+leaseID+"/renew/challenge", nil, nil, nil)
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"
		ctx.Params["leaseID"] = leaseID
		requireStatus(t, http.StatusUnauthorized)(h.HandleCreateAgentAccessLeaseRenewChallengeLift(ctx))
	})

	t.Run("renew challenge rejects suspended agent", func(t *testing.T) {
		state := baseState()
		agent := state.usersByUsername["agent1"]
		agent.Suspended = true
		state.usersByUsername["agent1"] = agent
		leaseID := "lease-suspended"
		state.agentAccessLeasesByKey = map[string]storagemodels.AgentAccessLease{
			"AGENT_ACCESS_LEASE#agent1#LEASE#" + leaseID: {
				PK:                "AGENT_ACCESS_LEASE#agent1",
				SK:                "LEASE#" + leaseID,
				ID:                leaseID,
				Username:          "agent1",
				PrincipalUsername: "owner",
				PrincipalWallet:   ownerAddr,
				AgentWallet:       agentAddr,
				Scopes:            []string{"read"},
				DeviceLabel:       "local-agent",
				Status:            "active",
				IdleTimeoutHours:  24,
				IdleExpiresAt:     now.Add(24 * time.Hour),
				AbsoluteExpiresAt: now.Add(48 * time.Hour),
				LastUsedAt:        now,
				LeaseVersion:      1,
				CreatedAt:         now,
				UpdatedAt:         now,
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent1/access-leases/"+leaseID+"/renew/challenge", nil, nil, nil)
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"
		ctx.Params["leaseID"] = leaseID
		requireStatus(t, http.StatusNotFound)(h.HandleCreateAgentAccessLeaseRenewChallengeLift(ctx))
	})

	t.Run("exchange token rejects suspended agent before renewal proof", func(t *testing.T) {
		state := baseState()
		agent := state.usersByUsername["agent1"]
		agent.Suspended = true
		state.usersByUsername["agent1"] = agent
		leaseID := "lease-suspended-token"
		state.agentAccessLeasesByKey = map[string]storagemodels.AgentAccessLease{
			"AGENT_ACCESS_LEASE#agent1#LEASE#" + leaseID: {
				PK:                "AGENT_ACCESS_LEASE#agent1",
				SK:                "LEASE#" + leaseID,
				ID:                leaseID,
				Username:          "agent1",
				PrincipalUsername: "owner",
				PrincipalWallet:   ownerAddr,
				AgentWallet:       agentAddr,
				Scopes:            []string{"read"},
				DeviceLabel:       "local-agent",
				Status:            "active",
				IdleTimeoutHours:  24,
				IdleExpiresAt:     now.Add(24 * time.Hour),
				AbsoluteExpiresAt: now.Add(48 * time.Hour),
				LastUsedAt:        now,
				LeaseVersion:      1,
				CreatedAt:         now,
				UpdatedAt:         now,
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent1/access-leases/"+leaseID+"/token", nil, nil, apimodels.RenewAgentAccessLeaseTokenRequest{
			ChallengeID: "challenge-suspended",
			Signature:   "0xdeadbeef",
		})
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"
		ctx.Params["leaseID"] = leaseID
		requireStatus(t, http.StatusNotFound)(h.HandleExchangeAgentAccessLeaseTokenLift(ctx))
	})

	t.Run("exchange token rejects mismatched challenge action", func(t *testing.T) {
		state := baseState()
		leaseID := "lease-3"
		state.agentAccessLeasesByKey = map[string]storagemodels.AgentAccessLease{
			"AGENT_ACCESS_LEASE#agent1#LEASE#" + leaseID: {
				PK:                "AGENT_ACCESS_LEASE#agent1",
				SK:                "LEASE#" + leaseID,
				ID:                leaseID,
				Username:          "agent1",
				PrincipalUsername: "owner",
				PrincipalWallet:   ownerAddr,
				AgentWallet:       agentAddr,
				Scopes:            []string{"read"},
				DeviceLabel:       "local-agent",
				Status:            "active",
				IdleTimeoutHours:  24,
				IdleExpiresAt:     now.Add(24 * time.Hour),
				AbsoluteExpiresAt: now.Add(48 * time.Hour),
				LastUsedAt:        now,
				LeaseVersion:      1,
				CreatedAt:         now,
				UpdatedAt:         now,
			},
		}
		state.agentAccessChallengesByID = map[string]storagemodels.AgentAccessLeaseChallenge{
			"challenge-1": {
				PK:                "AGENT_ACCESS_CHALLENGE#challenge-1",
				SK:                "CHALLENGE",
				ID:                "challenge-1",
				LeaseID:           leaseID,
				Username:          "agent1",
				Action:            "principal_approve",
				Address:           ownerAddr,
				PrincipalUsername: "owner",
				PrincipalWallet:   ownerAddr,
				AgentWallet:       agentAddr,
				Scopes:            []string{"read"},
				DeviceLabel:       "local-agent",
				IdleTimeoutHours:  24,
				AbsoluteTTLHours:  48,
				Message:           "challenge",
				IssuedAt:          now,
				ExpiresAt:         now.Add(time.Minute),
				TTL:               now.Add(time.Minute).Unix(),
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent1/access-leases/"+leaseID+"/token", nil, nil, apimodels.RenewAgentAccessLeaseTokenRequest{
			ChallengeID: "challenge-1",
			Signature:   "bad",
		})
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"
		ctx.Params["leaseID"] = leaseID
		requireStatus(t, http.StatusUnauthorized)(h.HandleExchangeAgentAccessLeaseTokenLift(ctx))
	})

	t.Run("exchange token rejects expired challenge", func(t *testing.T) {
		state := baseState()
		leaseID := "lease-expired"
		state.agentAccessLeasesByKey = map[string]storagemodels.AgentAccessLease{
			"AGENT_ACCESS_LEASE#agent1#LEASE#" + leaseID: {
				PK:                "AGENT_ACCESS_LEASE#agent1",
				SK:                "LEASE#" + leaseID,
				ID:                leaseID,
				Username:          "agent1",
				PrincipalUsername: "owner",
				PrincipalWallet:   ownerAddr,
				AgentWallet:       agentAddr,
				Scopes:            []string{"read"},
				DeviceLabel:       "local-agent",
				Status:            "active",
				IdleTimeoutHours:  24,
				IdleExpiresAt:     now.Add(24 * time.Hour),
				AbsoluteExpiresAt: now.Add(48 * time.Hour),
				LastUsedAt:        now,
				LeaseVersion:      1,
				CreatedAt:         now,
				UpdatedAt:         now,
			},
		}
		state.agentAccessChallengesByID = map[string]storagemodels.AgentAccessLeaseChallenge{
			"challenge-1": {
				PK:                "AGENT_ACCESS_CHALLENGE#challenge-1",
				SK:                "CHALLENGE",
				ID:                "challenge-1",
				LeaseID:           leaseID,
				Username:          "agent1",
				Action:            agentAccessLeaseActionRenewWallet,
				Address:           agentAddr,
				PrincipalUsername: "owner",
				PrincipalWallet:   ownerAddr,
				AgentWallet:       agentAddr,
				Scopes:            []string{"read"},
				DeviceLabel:       "local-agent",
				IdleTimeoutHours:  24,
				AbsoluteTTLHours:  48,
				Message:           "challenge",
				IssuedAt:          now.Add(-2 * time.Minute),
				ExpiresAt:         now.Add(-time.Minute),
				TTL:               now.Add(-time.Minute).Unix(),
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent1/access-leases/"+leaseID+"/token", nil, nil, apimodels.RenewAgentAccessLeaseTokenRequest{
			ChallengeID: "challenge-1",
			Signature:   "bad",
		})
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"
		ctx.Params["leaseID"] = leaseID
		requireStatus(t, http.StatusUnauthorized)(h.HandleExchangeAgentAccessLeaseTokenLift(ctx))
	})

	t.Run("exchange token surfaces update failure after successful signature", func(t *testing.T) {
		state := baseState()
		leaseID := "lease-update"
		pub, priv, err := ed25519.GenerateKey(crand.Reader)
		require.NoError(t, err)
		pubBase64 := base64.StdEncoding.EncodeToString(pub)
		challenge := buildRenewSessionChallengeRound20(now, "challenge-1", leaseID, ownerAddr, agentAddr, pubBase64)
		state.agentAccessLeasesByKey = map[string]storagemodels.AgentAccessLease{
			"AGENT_ACCESS_LEASE#agent1#LEASE#" + leaseID: {
				PK:                  "AGENT_ACCESS_LEASE#agent1",
				SK:                  "LEASE#" + leaseID,
				ID:                  leaseID,
				Username:            "agent1",
				PrincipalUsername:   "owner",
				PrincipalWallet:     ownerAddr,
				AgentWallet:         agentAddr,
				Scopes:              []string{"read"},
				DeviceLabel:         "local-agent",
				Status:              "active",
				IdleTimeoutHours:    24,
				IdleExpiresAt:       now.Add(24 * time.Hour),
				AbsoluteExpiresAt:   now.Add(48 * time.Hour),
				LastUsedAt:          now,
				LeaseVersion:        1,
				SessionPublicKey:    pubBase64,
				SessionKeyType:      "ed25519",
				SessionKeyCreatedAt: now,
				CreatedAt:           now,
				UpdatedAt:           now,
			},
		}
		state.agentAccessChallengesByID = map[string]storagemodels.AgentAccessLeaseChallenge{"challenge-1": challenge}
		state.executeErrorOnce = errors.New("boom")
		h, _, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent1/access-leases/"+leaseID+"/token", nil, nil, apimodels.RenewAgentAccessLeaseTokenRequest{
			ChallengeID: "challenge-1",
			Signature:   base64.StdEncoding.EncodeToString(ed25519.Sign(priv, []byte(challenge.Message))),
		})
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"
		ctx.Params["leaseID"] = leaseID
		requireStatus(t, http.StatusInternalServerError)(h.HandleExchangeAgentAccessLeaseTokenLift(ctx))
	})

	t.Run("revoke returns current lease when already revoked", func(t *testing.T) {
		state := baseState()
		leaseID := "lease-4"
		state.agentAccessLeasesByKey = map[string]storagemodels.AgentAccessLease{
			"AGENT_ACCESS_LEASE#agent1#LEASE#" + leaseID: {
				PK:                "AGENT_ACCESS_LEASE#agent1",
				SK:                "LEASE#" + leaseID,
				ID:                leaseID,
				Username:          "agent1",
				PrincipalUsername: "owner",
				PrincipalWallet:   ownerAddr,
				AgentWallet:       agentAddr,
				Scopes:            []string{"read"},
				DeviceLabel:       "local-agent",
				Status:            "revoked",
				IdleTimeoutHours:  24,
				IdleExpiresAt:     now.Add(24 * time.Hour),
				AbsoluteExpiresAt: now.Add(48 * time.Hour),
				LastUsedAt:        now,
				LeaseVersion:      1,
				CreatedAt:         now,
				UpdatedAt:         now,
				RevokedAt:         now.Add(-time.Minute),
				RevokedBy:         "owner",
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent1/access-leases/"+leaseID+"/revoke", headers, nil, apimodels.RevokeAgentAccessLeaseRequest{})
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"
		ctx.Params["leaseID"] = leaseID
		requireStatus(t, http.StatusOK)(h.HandleRevokeAgentAccessLeaseLift(ctx))
	})
}

func buildCreateLeaseChallengeRound20(now time.Time, id, leaseID, address, principalWallet, agentWallet, action, principalUsername string) storagemodels.AgentAccessLeaseChallenge {
	return storagemodels.AgentAccessLeaseChallenge{
		PK:                "AGENT_ACCESS_CHALLENGE#" + id,
		SK:                "CHALLENGE",
		ID:                id,
		LeaseID:           leaseID,
		Username:          "agent1",
		Action:            action,
		Address:           address,
		PrincipalUsername: principalUsername,
		PrincipalWallet:   principalWallet,
		AgentWallet:       agentWallet,
		Scopes:            []string{"read"},
		DeviceLabel:       "local-agent",
		IdleTimeoutHours:  24,
		AbsoluteTTLHours:  48,
		Message:           id,
		IssuedAt:          now,
		ExpiresAt:         now.Add(time.Minute),
		TTL:               now.Add(time.Minute).Unix(),
	}
}

func buildSessionKeyChallengeRound20(now time.Time, id, leaseID, ownerAddr, agentAddr, sessionPublicKey string) storagemodels.AgentAccessLeaseChallenge {
	return storagemodels.AgentAccessLeaseChallenge{
		PK:                "AGENT_ACCESS_CHALLENGE#" + id,
		SK:                "CHALLENGE",
		ID:                id,
		LeaseID:           leaseID,
		Username:          "agent1",
		Action:            agentAccessLeaseActionSessionKeyAuth,
		Address:           agentAddr,
		PrincipalUsername: "owner",
		PrincipalWallet:   ownerAddr,
		AgentWallet:       agentAddr,
		SessionPublicKey:  sessionPublicKey,
		SessionKeyType:    "ed25519",
		Scopes:            []string{"read"},
		DeviceLabel:       "local-agent",
		IdleTimeoutHours:  24,
		AbsoluteTTLHours:  48,
		Message:           id,
		IssuedAt:          now,
		ExpiresAt:         now.Add(time.Minute),
		TTL:               now.Add(time.Minute).Unix(),
	}
}

func buildRenewSessionChallengeRound20(now time.Time, id, leaseID, ownerAddr, agentAddr, sessionPublicKey string) storagemodels.AgentAccessLeaseChallenge {
	return storagemodels.AgentAccessLeaseChallenge{
		PK:                "AGENT_ACCESS_CHALLENGE#" + id,
		SK:                "CHALLENGE",
		ID:                id,
		LeaseID:           leaseID,
		Username:          "agent1",
		Action:            agentAccessLeaseActionRenewSession,
		PrincipalUsername: "owner",
		PrincipalWallet:   ownerAddr,
		AgentWallet:       agentAddr,
		SessionPublicKey:  sessionPublicKey,
		SessionKeyType:    "ed25519",
		Scopes:            []string{"read"},
		DeviceLabel:       "local-agent",
		IdleTimeoutHours:  24,
		AbsoluteTTLHours:  48,
		Message:           id,
		IssuedAt:          now,
		ExpiresAt:         now.Add(time.Minute),
		TTL:               now.Add(time.Minute).Unix(),
	}
}
