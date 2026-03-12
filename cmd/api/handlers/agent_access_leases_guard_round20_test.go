package handlers

import (
	"crypto/ed25519"
	crand "crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/config"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"
	dynamormerrors "github.com/theory-cloud/tabletheory/pkg/errors"
)

func TestAgentAccessLeaseHandlersRound20_GuardBranches(t *testing.T) {
	cfg := round10TestConfig()
	cfg.AllowAgents = true
	cfg.AllowAgentRegistration = true

	now := time.Now().UTC()
	ownerKey := mustGenerateWalletKey(t)
	agentKey := mustGenerateWalletKey(t)
	ownerAddr := strings.ToLower(crypto.PubkeyToAddress(ownerKey.PublicKey).Hex())
	agentAddr := strings.ToLower(crypto.PubkeyToAddress(agentKey.PublicKey).Hex())

	newState := func() *round10QueryState {
		policy := storagemodels.NewAgentInstanceConfig()
		policy.AllowAgents = true
		policy.AllowAgentRegistration = true

		return &round10QueryState{
			agentInstanceConfig: policy,
			usersByUsername: map[string]storagemodels.User{
				"owner": {
					PK:        "USER#owner",
					SK:        storagemodels.SKMetadata,
					Username:  "owner",
					Approved:  true,
					Version:   1,
					CreatedAt: now.Add(-24 * time.Hour),
				},
				"agent1": {
					PK:           "USER#agent1",
					SK:           storagemodels.SKMetadata,
					Username:     "agent1",
					Approved:     true,
					Version:      1,
					CreatedAt:    now.Add(-24 * time.Hour),
					IsAgent:      true,
					AgentOwner:   "@owner",
					AgentType:    agentTypeCustom,
					AgentVersion: "v1",
					Metadata: map[string]any{
						"agent_delegated_scopes": []any{"read", "write", "follow"},
					},
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

	ownerHeaders := map[string]string{
		"Authorization": "Bearer " + round11SignAccessToken(t, cfg.JWTSecret, "owner", []string{auth.ScopeWrite, auth.ScopeRead, "follow"}),
	}

	newActiveLease := func(leaseID string) storagemodels.AgentAccessLease {
		return storagemodels.AgentAccessLease{
			PK:                "AGENT_ACCESS_LEASE#agent1",
			SK:                "LEASE#" + leaseID,
			ID:                leaseID,
			Username:          "agent1",
			PrincipalUsername: "owner",
			PrincipalWallet:   ownerAddr,
			AgentWallet:       agentAddr,
			Scopes:            []string{"read"},
			DeviceLabel:       "local-agent",
			Status:            agentAccessLeaseStatusActive,
			IdleTimeoutHours:  24,
			IdleExpiresAt:     now.Add(24 * time.Hour),
			AbsoluteExpiresAt: now.Add(48 * time.Hour),
			LastUsedAt:        now,
			LeaseVersion:      1,
			CreatedAt:         now,
			UpdatedAt:         now,
		}
	}

	t.Run("create lease covers disabled invalid username malformed body and missing fields", func(t *testing.T) {
		disabledCfg := round10TestConfig()
		disabledCfg.AllowAgents = false
		hDisabled, _, _ := round11NewHandler(t, disabledCfg, &round10QueryState{})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent1/access-leases", ownerHeaders, nil, apimodels.CreateAgentAccessLeaseRequest{})
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"
		requireStatus(t, http.StatusForbidden)(hDisabled.HandleCreateAgentAccessLeaseLift(ctx))

		h, _, _ := round11NewHandler(t, cfg, newState())

		ctx, err = round10NewLiftContext(http.MethodPost, "/api/v1/agents/bad user/access-leases", ownerHeaders, nil, apimodels.CreateAgentAccessLeaseRequest{})
		require.NoError(t, err)
		ctx.Params["username"] = "bad user"
		requireStatus(t, http.StatusBadRequest)(h.HandleCreateAgentAccessLeaseLift(ctx))

		ctx = round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/agents/agent1/access-leases", ownerHeaders, nil, []byte("{bad"))
		ctx.Params["username"] = "agent1"
		requireStatus(t, http.StatusBadRequest)(h.HandleCreateAgentAccessLeaseLift(ctx))

		ctx, err = round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent1/access-leases", ownerHeaders, nil, apimodels.CreateAgentAccessLeaseRequest{
			PrincipalChallengeID: "principal",
		})
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"
		requireStatus(t, http.StatusBadRequest)(h.HandleCreateAgentAccessLeaseLift(ctx))

		ctx, err = round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent1/access-leases", ownerHeaders, nil, apimodels.CreateAgentAccessLeaseRequest{
			PrincipalChallengeID: "principal",
			PrincipalSignature:   "0x01",
		})
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"
		requireStatus(t, http.StatusBadRequest)(h.HandleCreateAgentAccessLeaseLift(ctx))

		ctx, err = round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent1/access-leases", ownerHeaders, nil, apimodels.CreateAgentAccessLeaseRequest{
			PrincipalChallengeID: "principal",
			PrincipalSignature:   "0x01",
			AgentChallengeID:     "agent",
		})
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"
		requireStatus(t, http.StatusBadRequest)(h.HandleCreateAgentAccessLeaseLift(ctx))
	})

	t.Run("create lease covers principal wallet missing and agent signature failure", func(t *testing.T) {
		principal := buildCreateLeaseChallengeRound20(now, "principal", "lease-a", ownerAddr, ownerAddr, agentAddr, agentAccessLeaseActionPrincipal, "owner")
		agent := buildCreateLeaseChallengeRound20(now, "agent", "lease-a", agentAddr, ownerAddr, agentAddr, agentAccessLeaseActionAgent, "owner")

		state := newState()
		state.walletCredentialsByUser["owner"] = nil
		state.agentAccessChallengesByID = map[string]storagemodels.AgentAccessLeaseChallenge{
			"principal": principal,
			"agent":     agent,
		}
		h, _, _ := round11NewHandler(t, cfg, state)

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent1/access-leases", ownerHeaders, nil, apimodels.CreateAgentAccessLeaseRequest{
			PrincipalChallengeID: "principal",
			PrincipalSignature:   signTypedDataRound20(t, ownerKey, buildAgentAccessLeaseTypedData(&principal)),
			AgentChallengeID:     "agent",
			AgentSignature:       signTypedDataRound20(t, agentKey, buildAgentAccessLeaseTypedData(&agent)),
		})
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"
		requireStatus(t, http.StatusForbidden)(h.HandleCreateAgentAccessLeaseLift(ctx))

		state = newState()
		state.agentAccessChallengesByID = map[string]storagemodels.AgentAccessLeaseChallenge{
			"principal": principal,
			"agent":     agent,
		}
		h, _, _ = round11NewHandler(t, cfg, state)

		ctx, err = round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent1/access-leases", ownerHeaders, nil, apimodels.CreateAgentAccessLeaseRequest{
			PrincipalChallengeID: "principal",
			PrincipalSignature:   signTypedDataRound20(t, ownerKey, buildAgentAccessLeaseTypedData(&principal)),
			AgentChallengeID:     "agent",
			AgentSignature:       "0xdeadbeef",
		})
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"
		requireStatus(t, http.StatusUnauthorized)(h.HandleCreateAgentAccessLeaseLift(ctx))
	})

	t.Run("list leases covers invalid username and storage error", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, newState())

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/agents/bad user/access-leases", ownerHeaders, nil, nil)
		require.NoError(t, err)
		ctx.Params["username"] = "bad user"
		requireStatus(t, http.StatusBadRequest)(h.HandleListAgentAccessLeasesLift(ctx))

		state := newState()
		state.allErrorOnce = errors.New("boom")
		h, _, _ = round11NewHandler(t, cfg, state)

		ctx, err = round10NewLiftContext(http.MethodGet, "/api/v1/agents/agent1/access-leases", ownerHeaders, nil, nil)
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"
		requireStatus(t, http.StatusInternalServerError)(h.HandleListAgentAccessLeasesLift(ctx))
	})

	t.Run("revoke covers invalid username missing lease id malformed body and update error", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, newState())

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/bad user/access-leases/lease-1/revoke", ownerHeaders, nil, apimodels.RevokeAgentAccessLeaseRequest{})
		require.NoError(t, err)
		ctx.Params["username"] = "bad user"
		ctx.Params["leaseID"] = "lease-1"
		requireStatus(t, http.StatusBadRequest)(h.HandleRevokeAgentAccessLeaseLift(ctx))

		ctx, err = round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent1/access-leases/revoke", ownerHeaders, nil, apimodels.RevokeAgentAccessLeaseRequest{})
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"
		requireStatus(t, http.StatusBadRequest)(h.HandleRevokeAgentAccessLeaseLift(ctx))

		state := newState()
		leaseID := "lease-revoke"
		state.agentAccessLeasesByKey = map[string]storagemodels.AgentAccessLease{
			"AGENT_ACCESS_LEASE#agent1#LEASE#" + leaseID: newActiveLease(leaseID),
		}
		h, _, _ = round11NewHandler(t, cfg, state)

		ctx = round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/agents/agent1/access-leases/"+leaseID+"/revoke", ownerHeaders, nil, []byte("{bad"))
		ctx.Params["username"] = "agent1"
		ctx.Params["leaseID"] = leaseID
		requireStatus(t, http.StatusBadRequest)(h.HandleRevokeAgentAccessLeaseLift(ctx))

		state = newState()
		state.executeErrorOnce = errors.New("boom")
		state.agentAccessLeasesByKey = map[string]storagemodels.AgentAccessLease{
			"AGENT_ACCESS_LEASE#agent1#LEASE#" + leaseID: newActiveLease(leaseID),
		}
		h, _, _ = round11NewHandler(t, cfg, state)

		ctx, err = round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent1/access-leases/"+leaseID+"/revoke", ownerHeaders, nil, apimodels.RevokeAgentAccessLeaseRequest{
			Reason: "rotated",
		})
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"
		ctx.Params["leaseID"] = leaseID
		requireStatus(t, http.StatusInternalServerError)(h.HandleRevokeAgentAccessLeaseLift(ctx))
	})

	t.Run("session key challenge covers invalid username missing lease id malformed body and create failure", func(t *testing.T) {
		pub, _, err := ed25519.GenerateKey(crand.Reader)
		require.NoError(t, err)
		sessionKey := base64.StdEncoding.EncodeToString(pub)
		leaseID := "lease-session"

		h, _, _ := round11NewHandler(t, cfg, newState())

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/bad user/access-leases/"+leaseID+"/session-key/challenge", nil, nil, apimodels.AgentAccessLeaseSessionKeyChallengeRequest{
			SessionPublicKey: sessionKey,
		})
		require.NoError(t, err)
		ctx.Params["username"] = "bad user"
		ctx.Params["leaseID"] = leaseID
		requireStatus(t, http.StatusBadRequest)(h.HandleCreateAgentAccessLeaseSessionKeyChallengeLift(ctx))

		ctx, err = round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent1/access-leases/session-key/challenge", nil, nil, apimodels.AgentAccessLeaseSessionKeyChallengeRequest{
			SessionPublicKey: sessionKey,
		})
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"
		requireStatus(t, http.StatusBadRequest)(h.HandleCreateAgentAccessLeaseSessionKeyChallengeLift(ctx))

		state := newState()
		state.agentAccessLeasesByKey = map[string]storagemodels.AgentAccessLease{
			"AGENT_ACCESS_LEASE#agent1#LEASE#" + leaseID: newActiveLease(leaseID),
		}
		h, _, _ = round11NewHandler(t, cfg, state)

		ctx = round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/agents/agent1/access-leases/"+leaseID+"/session-key/challenge", nil, nil, []byte("{bad"))
		ctx.Params["username"] = "agent1"
		ctx.Params["leaseID"] = leaseID
		requireStatus(t, http.StatusBadRequest)(h.HandleCreateAgentAccessLeaseSessionKeyChallengeLift(ctx))

		state = newState()
		state.createErrorOnce = errors.New("boom")
		state.agentAccessLeasesByKey = map[string]storagemodels.AgentAccessLease{
			"AGENT_ACCESS_LEASE#agent1#LEASE#" + leaseID: newActiveLease(leaseID),
		}
		h, _, _ = round11NewHandler(t, cfg, state)

		ctx, err = round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent1/access-leases/"+leaseID+"/session-key/challenge", nil, nil, apimodels.AgentAccessLeaseSessionKeyChallengeRequest{
			SessionPublicKey: sessionKey,
		})
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"
		ctx.Params["leaseID"] = leaseID
		requireStatus(t, http.StatusInternalServerError)(h.HandleCreateAgentAccessLeaseSessionKeyChallengeLift(ctx))
	})

	t.Run("authorize session key covers invalid username missing lease id malformed body missing fields and used challenge", func(t *testing.T) {
		pub, _, err := ed25519.GenerateKey(crand.Reader)
		require.NoError(t, err)
		sessionKey := base64.StdEncoding.EncodeToString(pub)
		leaseID := "lease-authorize"

		h, _, _ := round11NewHandler(t, cfg, newState())

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/bad user/access-leases/"+leaseID+"/session-key", nil, nil, apimodels.AuthorizeAgentAccessLeaseSessionKeyRequest{})
		require.NoError(t, err)
		ctx.Params["username"] = "bad user"
		ctx.Params["leaseID"] = leaseID
		requireStatus(t, http.StatusBadRequest)(h.HandleAuthorizeAgentAccessLeaseSessionKeyLift(ctx))

		ctx, err = round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent1/access-leases/session-key", nil, nil, apimodels.AuthorizeAgentAccessLeaseSessionKeyRequest{})
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"
		requireStatus(t, http.StatusBadRequest)(h.HandleAuthorizeAgentAccessLeaseSessionKeyLift(ctx))

		state := newState()
		state.agentAccessLeasesByKey = map[string]storagemodels.AgentAccessLease{
			"AGENT_ACCESS_LEASE#agent1#LEASE#" + leaseID: newActiveLease(leaseID),
		}
		h, _, _ = round11NewHandler(t, cfg, state)

		ctx = round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/agents/agent1/access-leases/"+leaseID+"/session-key", nil, nil, []byte("{bad"))
		ctx.Params["username"] = "agent1"
		ctx.Params["leaseID"] = leaseID
		requireStatus(t, http.StatusBadRequest)(h.HandleAuthorizeAgentAccessLeaseSessionKeyLift(ctx))

		ctx, err = round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent1/access-leases/"+leaseID+"/session-key", nil, nil, apimodels.AuthorizeAgentAccessLeaseSessionKeyRequest{})
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"
		ctx.Params["leaseID"] = leaseID
		requireStatus(t, http.StatusBadRequest)(h.HandleAuthorizeAgentAccessLeaseSessionKeyLift(ctx))

		ctx, err = round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent1/access-leases/"+leaseID+"/session-key", nil, nil, apimodels.AuthorizeAgentAccessLeaseSessionKeyRequest{
			ChallengeID: "challenge-1",
		})
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"
		ctx.Params["leaseID"] = leaseID
		requireStatus(t, http.StatusBadRequest)(h.HandleAuthorizeAgentAccessLeaseSessionKeyLift(ctx))

		challenge := buildSessionKeyChallengeRound20(now, "challenge-1", leaseID, ownerAddr, agentAddr, sessionKey)
		state = newState()
		state.executeErrorOnce = dynamormerrors.ErrConditionFailed
		state.agentAccessLeasesByKey = map[string]storagemodels.AgentAccessLease{
			"AGENT_ACCESS_LEASE#agent1#LEASE#" + leaseID: newActiveLease(leaseID),
		}
		state.agentAccessChallengesByID = map[string]storagemodels.AgentAccessLeaseChallenge{
			"challenge-1": challenge,
		}
		h, _, _ = round11NewHandler(t, cfg, state)

		ctx, err = round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent1/access-leases/"+leaseID+"/session-key", nil, nil, apimodels.AuthorizeAgentAccessLeaseSessionKeyRequest{
			ChallengeID: "challenge-1",
			Signature:   signTypedDataRound20(t, agentKey, buildAgentAccessLeaseTypedData(&challenge)),
		})
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"
		ctx.Params["leaseID"] = leaseID
		requireStatus(t, http.StatusUnauthorized)(h.HandleAuthorizeAgentAccessLeaseSessionKeyLift(ctx))
	})

	t.Run("renew challenge covers invalid username missing lease id and create failure", func(t *testing.T) {
		leaseID := "lease-renew"
		h, _, _ := round11NewHandler(t, cfg, newState())

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/bad user/access-leases/"+leaseID+"/renew/challenge", nil, nil, nil)
		require.NoError(t, err)
		ctx.Params["username"] = "bad user"
		ctx.Params["leaseID"] = leaseID
		requireStatus(t, http.StatusBadRequest)(h.HandleCreateAgentAccessLeaseRenewChallengeLift(ctx))

		ctx, err = round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent1/access-leases/renew/challenge", nil, nil, nil)
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"
		requireStatus(t, http.StatusBadRequest)(h.HandleCreateAgentAccessLeaseRenewChallengeLift(ctx))

		state := newState()
		state.createErrorOnce = errors.New("boom")
		state.agentAccessLeasesByKey = map[string]storagemodels.AgentAccessLease{
			"AGENT_ACCESS_LEASE#agent1#LEASE#" + leaseID: newActiveLease(leaseID),
		}
		h, _, _ = round11NewHandler(t, cfg, state)

		ctx, err = round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent1/access-leases/"+leaseID+"/renew/challenge", nil, nil, nil)
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"
		ctx.Params["leaseID"] = leaseID
		requireStatus(t, http.StatusInternalServerError)(h.HandleCreateAgentAccessLeaseRenewChallengeLift(ctx))
	})

	t.Run("exchange token covers invalid username missing lease id malformed body missing fields used challenge and empty jwt secret", func(t *testing.T) {
		leaseID := "lease-token"
		h, _, _ := round11NewHandler(t, cfg, newState())

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/bad user/access-leases/"+leaseID+"/token", nil, nil, apimodels.RenewAgentAccessLeaseTokenRequest{})
		require.NoError(t, err)
		ctx.Params["username"] = "bad user"
		ctx.Params["leaseID"] = leaseID
		requireStatus(t, http.StatusBadRequest)(h.HandleExchangeAgentAccessLeaseTokenLift(ctx))

		ctx, err = round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent1/access-leases/token", nil, nil, apimodels.RenewAgentAccessLeaseTokenRequest{})
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"
		requireStatus(t, http.StatusBadRequest)(h.HandleExchangeAgentAccessLeaseTokenLift(ctx))

		state := newState()
		state.agentAccessLeasesByKey = map[string]storagemodels.AgentAccessLease{
			"AGENT_ACCESS_LEASE#agent1#LEASE#" + leaseID: newActiveLease(leaseID),
		}
		h, _, _ = round11NewHandler(t, cfg, state)

		ctx = round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/agents/agent1/access-leases/"+leaseID+"/token", nil, nil, []byte("{bad"))
		ctx.Params["username"] = "agent1"
		ctx.Params["leaseID"] = leaseID
		requireStatus(t, http.StatusBadRequest)(h.HandleExchangeAgentAccessLeaseTokenLift(ctx))

		ctx, err = round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent1/access-leases/"+leaseID+"/token", nil, nil, apimodels.RenewAgentAccessLeaseTokenRequest{})
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"
		ctx.Params["leaseID"] = leaseID
		requireStatus(t, http.StatusBadRequest)(h.HandleExchangeAgentAccessLeaseTokenLift(ctx))

		ctx, err = round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent1/access-leases/"+leaseID+"/token", nil, nil, apimodels.RenewAgentAccessLeaseTokenRequest{
			ChallengeID: "challenge-1",
		})
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"
		ctx.Params["leaseID"] = leaseID
		requireStatus(t, http.StatusBadRequest)(h.HandleExchangeAgentAccessLeaseTokenLift(ctx))

		challenge := buildCreateLeaseChallengeRound20(now, "challenge-1", leaseID, agentAddr, ownerAddr, agentAddr, agentAccessLeaseActionRenewWallet, "owner")
		signature := signTypedDataRound20(t, agentKey, buildAgentAccessLeaseTypedData(&challenge))

		state = newState()
		state.executeErrorOnce = dynamormerrors.ErrConditionFailed
		state.agentAccessLeasesByKey = map[string]storagemodels.AgentAccessLease{
			"AGENT_ACCESS_LEASE#agent1#LEASE#" + leaseID: newActiveLease(leaseID),
		}
		state.agentAccessChallengesByID = map[string]storagemodels.AgentAccessLeaseChallenge{
			"challenge-1": challenge,
		}
		h, _, _ = round11NewHandler(t, cfg, state)

		ctx, err = round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent1/access-leases/"+leaseID+"/token", nil, nil, apimodels.RenewAgentAccessLeaseTokenRequest{
			ChallengeID: "challenge-1",
			Signature:   signature,
		})
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"
		ctx.Params["leaseID"] = leaseID
		requireStatus(t, http.StatusUnauthorized)(h.HandleExchangeAgentAccessLeaseTokenLift(ctx))

		noJWT := *cfg
		noJWT.JWTSecret = ""
		state = newState()
		state.agentAccessLeasesByKey = map[string]storagemodels.AgentAccessLease{
			"AGENT_ACCESS_LEASE#agent1#LEASE#" + leaseID: newActiveLease(leaseID),
		}
		state.agentAccessChallengesByID = map[string]storagemodels.AgentAccessLeaseChallenge{
			"challenge-1": challenge,
		}
		h, _, _ = round11NewHandler(t, &noJWT, state)

		ctx, err = round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent1/access-leases/"+leaseID+"/token", nil, nil, apimodels.RenewAgentAccessLeaseTokenRequest{
			ChallengeID: "challenge-1",
			Signature:   signature,
		})
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"
		ctx.Params["leaseID"] = leaseID
		requireStatus(t, http.StatusInternalServerError)(h.HandleExchangeAgentAccessLeaseTokenLift(ctx))
	})
}

func TestAgentAccessLeaseHelpersRound20_AdditionalBranches(t *testing.T) {
	config.ResetForTests()
	t.Cleanup(config.ResetForTests)

	require.Equal(t, "localhost", humanReadableAccessLeaseDomain())

	t.Setenv("BASE_URL", "https://leases.example.com")
	config.ResetForTests()
	require.Equal(t, "localhost", humanReadableAccessLeaseDomain())

	require.Nil(t, challengeTypedDataResponse(nil))
	require.Nil(t, challengeTypedDataResponse(&storagemodels.AgentAccessLeaseChallenge{Action: "unknown"}))
}
