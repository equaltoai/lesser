package handlers

import (
	"crypto/ed25519"
	crand "crypto/rand"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"
	"github.com/stretchr/testify/require"
)

func TestAgentAccessLeaseCryptoHelpers_TypedDataSignature(t *testing.T) {
	key, err := crypto.GenerateKey()
	require.NoError(t, err)
	address := crypto.PubkeyToAddress(key.PublicKey).Hex()

	challenge := &storageModels.AgentAccessLeaseChallenge{
		ID:                "challenge-1",
		LeaseID:           "lease-1",
		Username:          "agent1",
		Action:            agentAccessLeaseActionPrincipal,
		Address:           address,
		PrincipalUsername: "owner",
		PrincipalWallet:   address,
		AgentWallet:       "0x2222222222222222222222222222222222222222",
		Scopes:            []string{"read", "write"},
		DeviceLabel:       "local-agent",
		IdleTimeoutHours:  168,
		AbsoluteTTLHours:  720,
		Nonce:             "nonce",
		Message:           "display",
		IssuedAt:          time.Now().UTC(),
		ExpiresAt:         time.Now().UTC().Add(time.Minute),
	}

	typedData := buildAgentAccessLeaseTypedData(challenge)
	digest, _, err := apitypes.TypedDataAndHash(typedData)
	require.NoError(t, err)
	sig, err := crypto.Sign(digest, key)
	require.NoError(t, err)
	require.NoError(t, verifyAgentAccessLeaseTypedDataSignature(address, typedData, hexutil.Encode(sig)))
	require.Error(t, verifyAgentAccessLeaseTypedDataSignature("0x3333333333333333333333333333333333333333", typedData, hexutil.Encode(sig)))
}

func TestAgentAccessLeaseCryptoHelpers_SessionSignature(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(crand.Reader)
	require.NoError(t, err)
	pubBase64 := base64.StdEncoding.EncodeToString(pub)
	message := "session-renewal"
	sig := base64.StdEncoding.EncodeToString(ed25519.Sign(priv, []byte(message)))

	require.NoError(t, verifyAgentAccessLeaseSessionSignature(pubBase64, message, sig))
	require.Error(t, verifyAgentAccessLeaseSessionSignature(pubBase64, message, "not-base64"))
	require.Error(t, verifyAgentAccessLeaseSessionSignature(pubBase64, "wrong-message", sig))
}

func TestAgentAccessLeaseCryptoHelpers_ChallengeTypedDataResponse(t *testing.T) {
	key, err := crypto.GenerateKey()
	require.NoError(t, err)
	address := crypto.PubkeyToAddress(key.PublicKey).Hex()
	challenge := &storageModels.AgentAccessLeaseChallenge{
		ID:                "challenge-1",
		LeaseID:           "lease-1",
		Username:          "agent1",
		Action:            agentAccessLeaseActionRenewWallet,
		Address:           address,
		PrincipalUsername: "owner",
		PrincipalWallet:   address,
		AgentWallet:       address,
		Scopes:            []string{"read"},
		DeviceLabel:       "local-agent",
		IdleTimeoutHours:  24,
		AbsoluteTTLHours:  48,
		Nonce:             "nonce",
		Message:           "display",
		IssuedAt:          time.Now().UTC(),
		ExpiresAt:         time.Now().UTC().Add(time.Minute),
	}

	resp := challengeTypedDataResponse(challenge)
	raw, err := json.Marshal(resp)
	require.NoError(t, err)
	require.Contains(t, string(raw), "AgentAccessLeaseChallenge")

	challenge.Action = agentAccessLeaseActionRenewSession
	require.Nil(t, challengeTypedDataResponse(challenge))
}

func TestAgentAccessLeaseCryptoHelpers_ResponseMappers(t *testing.T) {
	now := time.Now().UTC()
	sessionKeyCreated := now.Add(-2 * time.Hour)
	sessionKeyLastUsed := now.Add(-time.Hour)
	revokedAt := now.Add(-30 * time.Minute)

	lease := &storageModels.AgentAccessLease{
		ID:                   "lease-1",
		Username:             "agent1",
		PrincipalUsername:    "owner",
		PrincipalWallet:      "0x1111111111111111111111111111111111111111",
		AgentWallet:          "0x2222222222222222222222222222222222222222",
		Scopes:               []string{"read", "write"},
		DeviceLabel:          "local-agent",
		Status:               agentAccessLeaseStatusRevoked,
		IdleTimeoutHours:     24,
		IdleExpiresAt:        now.Add(time.Hour),
		AbsoluteExpiresAt:    now.Add(24 * time.Hour),
		LastUsedAt:           now,
		LeaseVersion:         2,
		SessionPublicKey:     "pubkey",
		SessionKeyType:       "ed25519",
		SessionKeyCreatedAt:  sessionKeyCreated,
		SessionKeyLastUsedAt: sessionKeyLastUsed,
		CreatedAt:            now.Add(-3 * time.Hour),
		UpdatedAt:            now,
		RevokedAt:            revokedAt,
		RevokedBy:            "owner",
		RevokedReason:        "cleanup",
	}

	modelLease := agentAccessLeaseResponse(lease, now)
	require.Equal(t, "pubkey", modelLease.SessionPublicKey)
	require.Equal(t, "ed25519", modelLease.SessionKeyType)
	require.NotNil(t, modelLease.SessionKeyCreatedAt)
	require.NotNil(t, modelLease.SessionKeyLastUsedAt)
	require.NotNil(t, modelLease.RevokedAt)

	challenge := &storageModels.AgentAccessLeaseChallenge{
		ID:                "challenge-1",
		LeaseID:           "lease-1",
		Username:          "agent1",
		Action:            agentAccessLeaseActionSessionKeyAuth,
		Address:           "0x1111111111111111111111111111111111111111",
		PrincipalUsername: "owner",
		PrincipalWallet:   "0x1111111111111111111111111111111111111111",
		AgentWallet:       "0x2222222222222222222222222222222222222222",
		SessionPublicKey:  "pubkey",
		SessionKeyType:    "ed25519",
		Scopes:            []string{"read"},
		DeviceLabel:       "local-agent",
		IdleTimeoutHours:  24,
		AbsoluteTTLHours:  48,
		Message:           "challenge",
		IssuedAt:          now,
		ExpiresAt:         now.Add(time.Minute),
	}
	modelChallenge := agentAccessLeaseChallengeResponse(challenge)
	require.Equal(t, "pubkey", modelChallenge.SessionPublicKey)
	require.Equal(t, "ed25519", modelChallenge.SessionKeyType)
	require.NotNil(t, modelChallenge.TypedData)
}

func TestAgentAccessLeaseCryptoHelpers_NormalizeSessionPublicKey(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(crand.Reader)
	require.NoError(t, err)
	pubBase64 := base64.StdEncoding.EncodeToString(pub)
	normalized, err := normalizeAgentAccessSessionPublicKey(pubBase64)
	require.NoError(t, err)
	require.Equal(t, pubBase64, normalized)
	_, err = normalizeAgentAccessSessionPublicKey("")
	require.Error(t, err)
	_, err = normalizeAgentAccessSessionPublicKey("invalid")
	require.Error(t, err)
}

func TestAgentAccessLeaseCryptoHelpers_EffectiveStatusExpired(t *testing.T) {
	now := time.Now().UTC()
	lease := &storageModels.AgentAccessLease{
		Status:            agentAccessLeaseStatusActive,
		IdleExpiresAt:     now.Add(-time.Minute),
		AbsoluteExpiresAt: now.Add(time.Hour),
	}
	require.Equal(t, agentAccessLeaseStatusExpired, effectiveAgentAccessLeaseStatus(lease, now))

	var zero apimodels.AgentAccessLease
	require.Equal(t, zero, agentAccessLeaseResponse(nil, now))
	var zeroChallenge apimodels.AgentAccessLeaseChallengeResponse
	require.Equal(t, zeroChallenge, agentAccessLeaseChallengeResponse(nil))
}
