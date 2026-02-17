package handlers

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/require"
)

func TestHandleGetInstanceV2Lift_TrustConfig(t *testing.T) {
	now := time.Now()

	t.Run("disabled by default", func(t *testing.T) {
		cfg := round11TestConfig()
		state := &round10QueryState{
			vapidKeys: &storage.VAPIDKeys{
				PublicKey:  "pubkey",
				PrivateKey: "privkey",
				Subject:    "mailto:admin@example.com",
				CreatedAt:  now.Add(-time.Hour),
				UpdatedAt:  now,
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v2/instance", nil, nil, nil)
		require.NoError(t, err)

		resp := requireStatus(t, http.StatusOK)(handler.HandleGetInstanceV2Lift(ctx))

		var body map[string]any
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		cfgAny, ok := body["configuration"].(map[string]any)
		require.True(t, ok)

		trustAny, ok := cfgAny["trust"].(map[string]any)
		require.True(t, ok)
		require.Equal(t, false, trustAny["enabled"])
		_, hasBase := trustAny["base_url"]
		require.False(t, hasBase)
	})

	t.Run("enabled when explicitly configured", func(t *testing.T) {
		t.Setenv("LESSER_HOST_URL", "https://lab.lesser.host")

		cfg := round11TestConfig()
		cfg.LesserHostInstanceKey = "instance-key"

		state := &round10QueryState{
			vapidKeys: &storage.VAPIDKeys{
				PublicKey:  "pubkey",
				PrivateKey: "privkey",
				Subject:    "mailto:admin@example.com",
				CreatedAt:  now.Add(-time.Hour),
				UpdatedAt:  now,
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v2/instance", nil, nil, nil)
		require.NoError(t, err)

		resp := requireStatus(t, http.StatusOK)(handler.HandleGetInstanceV2Lift(ctx))

		var body map[string]any
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		cfgAny, ok := body["configuration"].(map[string]any)
		require.True(t, ok)

		trustAny, ok := cfgAny["trust"].(map[string]any)
		require.True(t, ok)
		require.Equal(t, true, trustAny["enabled"])
		require.Equal(t, "https://lab.lesser.host", trustAny["base_url"])
		require.Equal(t, "https://lab.lesser.host/.well-known/jwks.json", trustAny["jwks_url"])
		require.Equal(t, "https://lab.lesser.host/attestations", trustAny["attestations_url"])

		proxyAny, ok := trustAny["proxy"].(map[string]any)
		require.True(t, ok)
		require.Equal(t, "/api/v1/trust/jwks.json", proxyAny["jwks_path"])
		require.Equal(t, "/api/v1/trust/attestations", proxyAny["attestations_path"])
		require.Equal(t, "/api/v1/trust/attestations/{id}", proxyAny["attestation_path_template"])
	})

	t.Run("disabled for Lambda Function URL hosts", func(t *testing.T) {
		t.Setenv("LESSER_HOST_URL", "https://abc.lambda-url.us-east-1.on.aws")

		cfg := round11TestConfig()
		cfg.LesserHostInstanceKey = "instance-key"

		state := &round10QueryState{
			vapidKeys: &storage.VAPIDKeys{
				PublicKey:  "pubkey",
				PrivateKey: "privkey",
				Subject:    "mailto:admin@example.com",
				CreatedAt:  now.Add(-time.Hour),
				UpdatedAt:  now,
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v2/instance", nil, nil, nil)
		require.NoError(t, err)

		resp := requireStatus(t, http.StatusOK)(handler.HandleGetInstanceV2Lift(ctx))

		var body map[string]any
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		cfgAny, ok := body["configuration"].(map[string]any)
		require.True(t, ok)

		trustAny, ok := cfgAny["trust"].(map[string]any)
		require.True(t, ok)
		require.Equal(t, false, trustAny["enabled"])
		_, hasBase := trustAny["base_url"]
		require.False(t, hasBase)
	})
}
