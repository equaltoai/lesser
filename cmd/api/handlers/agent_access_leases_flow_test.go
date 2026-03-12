package handlers

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	crand "crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"
	"github.com/stretchr/testify/require"
)

func TestAgentAccessLeasesRound20_FlowAndRenewal(t *testing.T) {
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
	baseUsers := map[string]storagemodels.User{
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
			Metadata: map[string]any{
				"agent_delegated_scopes": []any{"read", "write", "follow"},
			},
		},
	}

	newState := func() *round10QueryState {
		return &round10QueryState{
			agentInstanceConfig: policy,
			usersByUsername:     cloneRound20Users(baseUsers),
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

	t.Run("full_wallet_backed_lease_lifecycle", func(t *testing.T) {
		state := newState()
		h, _, _ := round11NewHandler(t, cfg, state)
		headers := map[string]string{"Authorization": "Bearer " + round11SignAccessToken(t, cfg.JWTSecret, "owner", []string{auth.ScopeWrite, auth.ScopeRead, "follow"})}

		principalChallengeReq := apimodels.AgentAccessLeaseChallengeRequest{
			PrincipalWallet:  ownerAddr,
			AgentWallet:      agentAddr,
			Scopes:           []string{"read", "write", "follow"},
			DeviceLabel:      "local-agent",
			IdleTimeoutHours: 168,
			AbsoluteTTLHours: 720,
		}
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent1/access-leases/challenge/principal", headers, nil, principalChallengeReq)
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"
		resp := requireStatus(t, http.StatusOK)(h.HandleCreateAgentAccessLeasePrincipalChallengeLift(ctx))

		var principalChallengeResp apimodels.AgentAccessLeaseChallengeResponse
		require.NoError(t, json.Unmarshal(resp.Body, &principalChallengeResp))
		require.NotEmpty(t, principalChallengeResp.LeaseID)

		agentChallengeReq := principalChallengeReq
		agentChallengeReq.LeaseID = principalChallengeResp.LeaseID
		ctx, err = round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent1/access-leases/challenge/agent", headers, nil, agentChallengeReq)
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"
		resp = requireStatus(t, http.StatusOK)(h.HandleCreateAgentAccessLeaseAgentChallengeLift(ctx))

		var agentChallengeResp apimodels.AgentAccessLeaseChallengeResponse
		require.NoError(t, json.Unmarshal(resp.Body, &agentChallengeResp))

		principalChallenge := state.agentAccessChallengesByID[principalChallengeResp.ID]
		agentChallenge := state.agentAccessChallengesByID[agentChallengeResp.ID]
		principalSig := signTypedDataRound20(t, ownerKey, buildAgentAccessLeaseTypedData(&principalChallenge))
		agentSig := signTypedDataRound20(t, agentKey, buildAgentAccessLeaseTypedData(&agentChallenge))

		createLeaseReq := apimodels.CreateAgentAccessLeaseRequest{
			PrincipalChallengeID: principalChallengeResp.ID,
			PrincipalSignature:   principalSig,
			AgentChallengeID:     agentChallengeResp.ID,
			AgentSignature:       agentSig,
		}
		ctx, err = round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent1/access-leases", headers, nil, createLeaseReq)
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"
		resp = requireStatus(t, http.StatusOK)(h.HandleCreateAgentAccessLeaseLift(ctx))

		var lease apimodels.AgentAccessLease
		require.NoError(t, json.Unmarshal(resp.Body, &lease))
		require.Equal(t, "agent1", lease.Username)
		require.Equal(t, "active", lease.Status)
		require.Equal(t, ownerAddr, strings.ToLower(lease.PrincipalWallet))
		require.Equal(t, agentAddr, strings.ToLower(lease.AgentWallet))

		ctx, err = round10NewLiftContext(http.MethodGet, "/api/v1/agents/agent1/access-leases", headers, nil, nil)
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"
		resp = requireStatus(t, http.StatusOK)(h.HandleListAgentAccessLeasesLift(ctx))

		var listResp apimodels.AgentAccessLeaseListResponse
		require.NoError(t, json.Unmarshal(resp.Body, &listResp))
		require.Len(t, listResp.Leases, 1)

		sessionPub, _, err := ed25519.GenerateKey(crand.Reader)
		require.NoError(t, err)
		sessionPubBase64 := base64.StdEncoding.EncodeToString(sessionPub)

		ctx, err = round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent1/access-leases/"+lease.ID+"/session-key/challenge", nil, nil, apimodels.AgentAccessLeaseSessionKeyChallengeRequest{
			SessionPublicKey: sessionPubBase64,
		})
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"
		ctx.Params["leaseID"] = lease.ID
		resp = requireStatus(t, http.StatusOK)(h.HandleCreateAgentAccessLeaseSessionKeyChallengeLift(ctx))

		var sessionChallengeResp apimodels.AgentAccessLeaseChallengeResponse
		require.NoError(t, json.Unmarshal(resp.Body, &sessionChallengeResp))
		sessionChallenge := state.agentAccessChallengesByID[sessionChallengeResp.ID]
		sessionAuthSig := signTypedDataRound20(t, agentKey, buildAgentAccessLeaseTypedData(&sessionChallenge))

		ctx, err = round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent1/access-leases/"+lease.ID+"/session-key", nil, nil, apimodels.AuthorizeAgentAccessLeaseSessionKeyRequest{
			ChallengeID: sessionChallengeResp.ID,
			Signature:   sessionAuthSig,
		})
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"
		ctx.Params["leaseID"] = lease.ID
		resp = requireStatus(t, http.StatusOK)(h.HandleAuthorizeAgentAccessLeaseSessionKeyLift(ctx))

		var leaseWithSession apimodels.AgentAccessLease
		require.NoError(t, json.Unmarshal(resp.Body, &leaseWithSession))
		require.Equal(t, sessionPubBase64, leaseWithSession.SessionPublicKey)

		state.agentAccessLeasesByKey["AGENT_ACCESS_LEASE#agent1#LEASE#"+lease.ID] = storagemodels.AgentAccessLease{
			PK:                  "AGENT_ACCESS_LEASE#agent1",
			SK:                  "LEASE#" + lease.ID,
			ID:                  lease.ID,
			Username:            "agent1",
			PrincipalUsername:   "owner",
			PrincipalWallet:     ownerAddr,
			AgentWallet:         agentAddr,
			Scopes:              []string{"read", "write", "follow"},
			DeviceLabel:         "local-agent",
			Status:              "active",
			IdleTimeoutHours:    168,
			IdleExpiresAt:       now.Add(168 * time.Hour),
			AbsoluteExpiresAt:   now.Add(720 * time.Hour),
			LastUsedAt:          now,
			LeaseVersion:        1,
			SessionPublicKey:    sessionPubBase64,
			SessionKeyType:      "ed25519",
			SessionKeyCreatedAt: now,
			CreatedAt:           now,
			UpdatedAt:           now,
		}

		ctx, err = round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent1/access-leases/"+lease.ID+"/renew/challenge", nil, nil, nil)
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"
		ctx.Params["leaseID"] = lease.ID
		resp = requireStatus(t, http.StatusOK)(h.HandleCreateAgentAccessLeaseRenewChallengeLift(ctx))

		var renewChallengeResp apimodels.AgentAccessLeaseChallengeResponse
		require.NoError(t, json.Unmarshal(resp.Body, &renewChallengeResp))
		require.Equal(t, "renew_session", renewChallengeResp.Action)

		sessionPub2, sessionPriv2, err := ed25519.GenerateKey(crand.Reader)
		require.NoError(t, err)
		_ = sessionPub2
		renewChallenge := state.agentAccessChallengesByID[renewChallengeResp.ID]
		renewSig := base64.StdEncoding.EncodeToString(ed25519.Sign(sessionPriv2, []byte(renewChallenge.Message)))
		renewChallenge.SessionPublicKey = base64.StdEncoding.EncodeToString(sessionPub2)
		state.agentAccessChallengesByID[renewChallenge.ID] = renewChallenge
		state.agentAccessLeasesByKey["AGENT_ACCESS_LEASE#agent1#LEASE#"+lease.ID] = storagemodels.AgentAccessLease{
			PK:                  "AGENT_ACCESS_LEASE#agent1",
			SK:                  "LEASE#" + lease.ID,
			ID:                  lease.ID,
			Username:            "agent1",
			PrincipalUsername:   "owner",
			PrincipalWallet:     ownerAddr,
			AgentWallet:         agentAddr,
			Scopes:              []string{"read", "write", "follow"},
			DeviceLabel:         "local-agent",
			Status:              "active",
			IdleTimeoutHours:    168,
			IdleExpiresAt:       now.Add(168 * time.Hour),
			AbsoluteExpiresAt:   now.Add(720 * time.Hour),
			LastUsedAt:          now,
			LeaseVersion:        1,
			SessionPublicKey:    renewChallenge.SessionPublicKey,
			SessionKeyType:      "ed25519",
			SessionKeyCreatedAt: now,
			CreatedAt:           now,
			UpdatedAt:           now,
		}

		ctx, err = round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent1/access-leases/"+lease.ID+"/token", nil, nil, apimodels.RenewAgentAccessLeaseTokenRequest{
			ChallengeID: renewChallengeResp.ID,
			Signature:   renewSig,
		})
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"
		ctx.Params["leaseID"] = lease.ID
		resp = requireStatus(t, http.StatusOK)(h.HandleExchangeAgentAccessLeaseTokenLift(ctx))

		var tokenResp apimodels.AgentAccessLeaseTokenResponse
		require.NoError(t, json.Unmarshal(resp.Body, &tokenResp))
		require.NotEmpty(t, tokenResp.Token.AccessToken)
		require.Empty(t, tokenResp.Token.RefreshToken)

		ctx, err = round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent1/access-leases/"+lease.ID+"/revoke", headers, nil, apimodels.RevokeAgentAccessLeaseRequest{
			Reason: "operator request",
		})
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"
		ctx.Params["leaseID"] = lease.ID
		resp = requireStatus(t, http.StatusOK)(h.HandleRevokeAgentAccessLeaseLift(ctx))

		var revoked apimodels.AgentAccessLease
		require.NoError(t, json.Unmarshal(resp.Body, &revoked))
		require.Equal(t, "revoked", revoked.Status)
	})

	t.Run("wallet_renewal_path_without_session_key", func(t *testing.T) {
		state := newState()
		leaseID := "lease-wallet"
		state.agentAccessLeasesByKey = map[string]storagemodels.AgentAccessLease{
			"AGENT_ACCESS_LEASE#agent1#LEASE#" + leaseID: {
				PK:                "AGENT_ACCESS_LEASE#agent1",
				SK:                "LEASE#" + leaseID,
				ID:                leaseID,
				Username:          "agent1",
				PrincipalUsername: "owner",
				PrincipalWallet:   ownerAddr,
				AgentWallet:       agentAddr,
				Scopes:            []string{"read", "write"},
				DeviceLabel:       "local-agent",
				Status:            "active",
				IdleTimeoutHours:  168,
				IdleExpiresAt:     now.Add(168 * time.Hour),
				AbsoluteExpiresAt: now.Add(720 * time.Hour),
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
		resp := requireStatus(t, http.StatusOK)(h.HandleCreateAgentAccessLeaseRenewChallengeLift(ctx))

		var renewChallengeResp apimodels.AgentAccessLeaseChallengeResponse
		require.NoError(t, json.Unmarshal(resp.Body, &renewChallengeResp))
		require.Equal(t, "renew_wallet", renewChallengeResp.Action)

		challenge := state.agentAccessChallengesByID[renewChallengeResp.ID]
		sig := signTypedDataRound20(t, agentKey, buildAgentAccessLeaseTypedData(&challenge))

		ctx, err = round10NewLiftContext(http.MethodPost, "/api/v1/agents/agent1/access-leases/"+leaseID+"/token", nil, nil, apimodels.RenewAgentAccessLeaseTokenRequest{
			ChallengeID: renewChallengeResp.ID,
			Signature:   sig,
		})
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"
		ctx.Params["leaseID"] = leaseID
		requireStatus(t, http.StatusOK)(h.HandleExchangeAgentAccessLeaseTokenLift(ctx))
	})
}

func mustGenerateWalletKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := crypto.GenerateKey()
	require.NoError(t, err)
	return key
}

func signTypedDataRound20(t *testing.T, key *ecdsa.PrivateKey, typedData apitypes.TypedData) string {
	t.Helper()
	digest, _, err := apitypes.TypedDataAndHash(typedData)
	require.NoError(t, err)
	sig, err := crypto.Sign(digest, key)
	require.NoError(t, err)
	return hexutil.Encode(sig)
}

func cloneRound20Users(in map[string]storagemodels.User) map[string]storagemodels.User {
	out := make(map[string]storagemodels.User, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
