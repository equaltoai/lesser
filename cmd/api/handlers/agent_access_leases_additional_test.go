package handlers

import (
	"crypto/ed25519"
	crand "crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/config"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"
)

func TestAgentAccessLeaseAdditionalHelpers_ValidationBranches(t *testing.T) {
	t.Run("normalize options covers required and invalid inputs", func(t *testing.T) {
		_, err := normalizeAgentAccessLeaseOptions(
			"",
			"agent",
			"owner",
			"0x1111111111111111111111111111111111111111",
			"0x2222222222222222222222222222222222222222",
			"",
			[]string{"read"},
			"",
			0,
			0,
			true,
		)
		require.ErrorContains(t, err, "lease_id is required")

		_, err = normalizeAgentAccessLeaseOptions(
			"lease-1",
			"agent",
			"owner",
			"bad",
			"0x2222222222222222222222222222222222222222",
			"",
			[]string{"read"},
			"",
			0,
			0,
			true,
		)
		require.ErrorContains(t, err, "principal_wallet")

		_, err = normalizeAgentAccessLeaseOptions(
			"lease-1",
			"agent",
			"owner",
			"0x1111111111111111111111111111111111111111",
			"bad",
			"",
			[]string{"read"},
			"",
			0,
			0,
			true,
		)
		require.ErrorContains(t, err, "agent_wallet")
	})

	t.Run("normalize options covers session keys and clamping", func(t *testing.T) {
		pub, _, err := ed25519.GenerateKey(crand.Reader)
		require.NoError(t, err)
		pubBase64 := base64.StdEncoding.EncodeToString(pub)

		opts, err := normalizeAgentAccessLeaseOptions(
			"lease-1",
			" Agent-0 ",
			" owner ",
			"0x1111111111111111111111111111111111111111",
			"0x2222222222222222222222222222222222222222",
			pubBase64,
			[]string{"write", "read"},
			" desktop ",
			agentAccessLeaseMaxIdleHrs+10,
			1,
			true,
		)
		require.NoError(t, err)
		require.Equal(t, "Agent-0", opts.Username)
		require.Equal(t, "owner", opts.PrincipalUsername)
		require.Equal(t, "desktop", opts.DeviceLabel)
		require.Equal(t, agentAccessLeaseMaxIdleHrs, opts.IdleTimeoutHours)
		require.Equal(t, agentAccessLeaseMaxIdleHrs, opts.AbsoluteTTLHours)
		require.Equal(t, agentAccessLeaseSessionKeyType, opts.SessionKeyType)
		require.Equal(t, pubBase64, opts.SessionPublicKey)
		require.Equal(t, []string{"read", "write"}, opts.Scopes)
	})

	t.Run("address validation covers bad prefix and bad hex", func(t *testing.T) {
		_, err := normalizeEthLeaseAddress("")
		require.ErrorContains(t, err, "is required")

		_, err = normalizeEthLeaseAddress("0x1234")
		require.ErrorContains(t, err, "20-byte ethereum address")

		_, err = normalizeEthLeaseAddress("0xzz11111111111111111111111111111111111111")
		require.ErrorContains(t, err, "hex encoded")
	})
}

func TestAgentAccessLeaseAdditionalHelpers_SignatureBranches(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(crand.Reader)
	require.NoError(t, err)
	pubBase64 := base64.StdEncoding.EncodeToString(pub)
	message := "session-authorize"
	rawURLSig := base64.RawURLEncoding.EncodeToString(ed25519.Sign(priv, []byte(message)))

	require.ErrorContains(t, verifyAgentAccessLeaseSessionSignature("", message, rawURLSig), "session key is not configured")
	require.ErrorContains(t, verifyAgentAccessLeaseSessionSignature("invalid", message, rawURLSig), "invalid session key")
	require.NoError(t, verifyAgentAccessLeaseSessionSignature(pubBase64, message, rawURLSig))

	var nilHandler *Handler
	require.ErrorContains(t, nilHandler.verifyLeaseChallengeSignature(nil, nil, rawURLSig), "signature verification unavailable")

	sessionChallenge := &storagemodels.AgentAccessLeaseChallenge{
		Action:           agentAccessLeaseActionRenewSession,
		SessionPublicKey: pubBase64,
		Message:          message,
	}
	require.NoError(t, nilHandler.verifyLeaseChallengeSignature(nil, sessionChallenge, rawURLSig))

	sessionChallenge.Action = "unsupported"
	require.ErrorContains(t, nilHandler.verifyLeaseChallengeSignature(nil, sessionChallenge, rawURLSig), "unsupported challenge action")

	key, err := crypto.GenerateKey()
	require.NoError(t, err)
	address := crypto.PubkeyToAddress(key.PublicKey).Hex()
	typedChallenge := &storagemodels.AgentAccessLeaseChallenge{
		ID:                "challenge-1",
		LeaseID:           "lease-1",
		Username:          "agent1",
		Action:            agentAccessLeaseActionPrincipal,
		Address:           address,
		PrincipalUsername: "owner",
		PrincipalWallet:   address,
		AgentWallet:       "0x2222222222222222222222222222222222222222",
		Scopes:            []string{"read"},
		DeviceLabel:       "local-agent",
		IdleTimeoutHours:  24,
		AbsoluteTTLHours:  48,
		Nonce:             "nonce",
		IssuedAt:          time.Now().UTC(),
		ExpiresAt:         time.Now().UTC().Add(time.Minute),
	}
	typedData := buildAgentAccessLeaseTypedData(typedChallenge)
	require.ErrorContains(t, verifyAgentAccessLeaseTypedDataSignature(address, typedData, "not-hex"), "invalid signature format")
	require.ErrorContains(t, verifyAgentAccessLeaseTypedDataSignature(address, typedData, "0x01"), "invalid signature length")
}

func TestAgentAccessLeaseAdditionalHelpers_StateAndDomainBranches(t *testing.T) {
	config.ResetForTests()
	t.Cleanup(config.ResetForTests)
	t.Setenv("DOMAIN", "leases.example.com")
	require.Equal(t, "leases.example.com", humanReadableAccessLeaseDomain())

	now := time.Now().UTC()
	require.Equal(t, "", effectiveAgentAccessLeaseStatus(nil, now))
	require.Equal(t, agentAccessLeaseStatusExpired, effectiveAgentAccessLeaseStatus(&storagemodels.AgentAccessLease{
		Status:            agentAccessLeaseStatusActive,
		IdleExpiresAt:     now.Add(time.Hour),
		AbsoluteExpiresAt: now.Add(-time.Minute),
	}, now))

	left := &storagemodels.AgentAccessLeaseChallenge{
		LeaseID:           "lease-1",
		Username:          "agent1",
		PrincipalUsername: "owner",
		PrincipalWallet:   "0x1111111111111111111111111111111111111111",
		AgentWallet:       "0x2222222222222222222222222222222222222222",
		Scopes:            []string{"read", "write"},
		DeviceLabel:       "desktop",
		IdleTimeoutHours:  24,
		AbsoluteTTLHours:  48,
	}
	right := *left
	require.True(t, agentAccessLeaseChallengesMatch(left, &right))
	require.False(t, agentAccessLeaseChallengesMatch(left, nil))
	right.AgentWallet = "0x3333333333333333333333333333333333333333"
	require.False(t, agentAccessLeaseChallengesMatch(left, &right))

	var nilHandler *Handler
	_, err := nilHandler.createAgentAccessLeaseChallenge(nil, agentAccessLeaseOptions{}, agentAccessLeaseActionPrincipal)
	require.ErrorContains(t, err, "storage not initialized")

	ok, err := nilHandler.userHasWallet(nil, "owner", "0x1111111111111111111111111111111111111111")
	require.False(t, ok)
	require.ErrorContains(t, err, "account repository unavailable")
}

func TestAgentAccessLeaseAdditionalHelpers_HandlerErrorBranches(t *testing.T) {
	cfg := round10TestConfig()
	cfg.AllowAgents = true
	cfg.AllowAgentRegistration = true

	now := time.Now().UTC()
	headers := map[string]string{"Authorization": "Bearer " + round11SignAccessToken(t, cfg.JWTSecret, "owner", []string{"write"})}

	t.Run("require helpers reject non-agents and unauthorized managers", func(t *testing.T) {
		state := &round10QueryState{
			agentInstanceConfig: policyLike(cfg),
			usersByUsername: map[string]storagemodels.User{
				"owner": {
					PK:        "USER#owner",
					SK:        storagemodels.SKMetadata,
					Username:  "owner",
					Approved:  true,
					Version:   1,
					CreatedAt: now.Add(-time.Hour),
				},
				"person": {
					PK:        "USER#person",
					SK:        storagemodels.SKMetadata,
					Username:  "person",
					Approved:  true,
					Version:   1,
					CreatedAt: now.Add(-time.Hour),
				},
				"other-agent": {
					PK:           "USER#other-agent",
					SK:           storagemodels.SKMetadata,
					Username:     "other-agent",
					Approved:     true,
					Version:      1,
					CreatedAt:    now.Add(-time.Hour),
					IsAgent:      true,
					AgentOwner:   "@someone-else",
					AgentType:    agentTypeCustom,
					AgentVersion: "v1",
				},
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state)

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/agents/person/access-leases", headers, nil, nil)
		require.NoError(t, err)
		ctx.Params["username"] = "person"

		_, _, resp, err := h.requireOwnedAgentAccount(ctx, "person")
		require.NotNil(t, resp)
		require.NoError(t, err)

		ctx.Params["username"] = "other-agent"
		_, _, resp, err = h.requireManagedAgentAccount(ctx, "other-agent")
		require.NotNil(t, resp)
		require.NoError(t, err)
	})

	t.Run("storage failures surface internal errors", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/agents/agent1/access-leases", headers, nil, nil)
		require.NoError(t, err)
		ctx.Params["username"] = "agent1"

		hLease, _, _ := round11NewHandler(t, cfg, &round10QueryState{
			agentInstanceConfig: policyLike(cfg),
			firstErrorOnce:      errors.New("boom"),
		})
		lease, resp, err := hLease.loadAgentAccessLease(ctx, "agent1", "lease-1")
		require.Nil(t, lease)
		require.NotNil(t, resp)
		require.NoError(t, err)

		hChallenge, _, _ := round11NewHandler(t, cfg, &round10QueryState{
			agentInstanceConfig: policyLike(cfg),
			firstErrorOnce:      errors.New("boom"),
		})
		challenge, resp, err := hChallenge.loadAgentAccessLeaseChallenge(ctx, "challenge-1")
		require.Nil(t, challenge)
		require.NotNil(t, resp)
		require.NoError(t, err)

		hCreate, _, _ := round11NewHandler(t, cfg, &round10QueryState{
			agentInstanceConfig: policyLike(cfg),
			createErrorOnce:     errors.New("boom"),
		})
		_, err = hCreate.createAgentAccessLeaseChallenge(ctx, agentAccessLeaseOptions{
			LeaseID:           "lease-1",
			Username:          "agent1",
			PrincipalUsername: "owner",
			PrincipalWallet:   "0x1111111111111111111111111111111111111111",
			AgentWallet:       "0x2222222222222222222222222222222222222222",
			Scopes:            []string{"read"},
			DeviceLabel:       "local-agent",
			IdleTimeoutHours:  24,
			AbsoluteTTLHours:  48,
		}, agentAccessLeaseActionRenewSession)
		require.ErrorContains(t, err, "boom")
	})
}
