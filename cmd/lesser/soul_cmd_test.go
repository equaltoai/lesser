package main

import (
	"crypto/ecdsa"
	"encoding/json"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"
)

func TestBuildSignedSoulENSRegistration_UpgradesV2ToV3(t *testing.T) {
	t.Parallel()

	signingKey, wallet := mustSoulSigningKey(t)
	cfg := &models.InstanceSoulENSChannel{
		AgentID:         "0x8db124b1d48e366002db4e61cc1501eeb8561e1ef06fd6f9abf9f984501d13ab",
		Name:            "agent-alice.lesserlab.eth",
		ResolverAddress: "0x000000000000000000000000000000000000cAFe",
		Chain:           "sepolia",
	}
	current := mustSoulJSON(t, map[string]any{
		"version":  "2",
		"agentId":  cfg.AgentID,
		"domain":   "example.com",
		"localId":  "agent-alice",
		"wallet":   wallet,
		"channels": map[string]any{"email": map[string]any{"address": "alice@example.com"}},
		"attestations": map[string]any{
			"selfAttestation": "0xdeadbeef",
		},
		"created": "2026-03-01T00:00:00Z",
		"updated": "2026-03-01T00:00:00Z",
	})

	out, err := buildSignedSoulENSRegistration(
		current,
		soulLatestVersion{
			VersionNumber:   4,
			RegistrationURI: "s3://bucket/registry/v1/agents/" + cfg.AgentID + "/versions/4/registration.json",
		},
		cfg,
		&soulSigningMaterial{Address: wallet, PrivateKey: signingKey, Source: "test"},
		"publish ENS channel",
	)
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(out, &parsed))
	require.Equal(t, "3", parsed["version"])
	require.Equal(t, "publish ENS channel", parsed["changeSummary"])
	require.Equal(t, "s3://bucket/registry/v1/agents/"+cfg.AgentID+"/versions/4/registration.json", parsed["previousVersionUri"])

	channels := parsed["channels"].(map[string]any)
	ens := channels["ens"].(map[string]any)
	require.Equal(t, cfg.Name, ens["name"])
	require.Equal(t, cfg.Chain, ens["chain"])
	require.Equal(t, cfg.ResolverAddress, ens["resolverAddress"])

	attestations := parsed["attestations"].(map[string]any)
	require.NotEmpty(t, attestations["selfAttestation"])
}

func TestBuildSignedSoulENSRegistration_RejectsLegacyV1(t *testing.T) {
	t.Parallel()

	signingKey, wallet := mustSoulSigningKey(t)
	cfg := &models.InstanceSoulENSChannel{
		AgentID: "0x8db124b1d48e366002db4e61cc1501eeb8561e1ef06fd6f9abf9f984501d13ab",
		Name:    "agent-alice.lesserlab.eth",
		Chain:   "sepolia",
	}

	_, err := buildSignedSoulENSRegistration(
		mustSoulJSON(t, map[string]any{
			"version": "1",
			"agentId": cfg.AgentID,
			"wallet":  wallet,
		}),
		soulLatestVersion{VersionNumber: 1, RegistrationURI: "s3://bucket/current.json"},
		cfg,
		&soulSigningMaterial{Address: wallet, PrivateKey: signingKey, Source: "test"},
		"",
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "legacy v1")
}

func TestBuildSoulRegistrationUpdatePayload_WrapsExpectedVersion(t *testing.T) {
	t.Parallel()

	body, err := buildSoulRegistrationUpdatePayload([]byte(`{"version":"3"}`), 7)
	require.NoError(t, err)

	var parsed struct {
		Registration    map[string]any `json:"registration"`
		ExpectedVersion int            `json:"expected_version"`
	}
	require.NoError(t, json.Unmarshal(body, &parsed))
	require.Equal(t, 7, parsed.ExpectedVersion)
	require.Equal(t, "3", parsed.Registration["version"])
}

func mustSoulSigningKey(t *testing.T) (*ecdsa.PrivateKey, string) {
	t.Helper()

	key, err := crypto.GenerateKey()
	require.NoError(t, err)
	return key, crypto.PubkeyToAddress(key.PublicKey).Hex()
}

func mustSoulJSON(t *testing.T, value any) []byte {
	t.Helper()

	body, err := json.Marshal(value)
	require.NoError(t, err)
	return body
}
