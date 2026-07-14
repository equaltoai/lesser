package souls

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func project51HostConversationFixture(t *testing.T, name string) []byte {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "hosted-genesis", "v1.0.6", name))
	require.NoError(t, err)
	return data
}

func TestService_BeginBootstrapRegistrationUsesInstanceKeyAndNotUserBearer(t *testing.T) {
	t.Parallel()

	const (
		instanceKey = "host-instance-key"
		userBearer  = "user-oauth-token"
		wallet      = "0x1111111111111111111111111111111111111111"
		agentID     = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	)

	var sawAuth string
	var sawBody map[string]any
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v1/soul/instance/agents/register/begin", r.URL.Path)
		sawAuth = r.Header.Get("Authorization")
		require.NotEqual(t, "Bearer "+userBearer, sawAuth)
		require.Equal(t, "application/json", r.Header.Get("Accept"))
		require.NoError(t, json.NewDecoder(r.Body).Decode(&sawBody))

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-Id", "host-req-begin")
		w.WriteHeader(http.StatusCreated)
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"registration": map[string]any{
				"id":             "reg_123",
				"agent_id":       agentID,
				"wallet_address": wallet,
				"status":         "pending",
			},
			"wallet": map[string]any{
				"id":        "wallet_chal_1",
				"address":   wallet,
				"chainId":   1,
				"nonce":     "nonce-1",
				"message":   "Sign this exact Host wallet challenge",
				"issuedAt":  "2026-06-12T18:00:00Z",
				"expiresAt": "2026-06-12T18:10:00Z",
			},
			"proofs": []map[string]any{},
		}))
	}))
	defer host.Close()

	service := NewService(
		&fakeAccountRepo{},
		&fakeInstanceRepo{trust: &storageModels.EffectiveTrustConfig{TrustBaseURL: host.URL}},
		&config.Config{Domain: "example.com", LesserHostInstanceKey: instanceKey},
		zap.NewNop(),
	).WithHTTPClient(host.Client())

	result, err := service.BeginBootstrapRegistration(context.Background(), BootstrapBeginInput{
		Username:      " drone-alpha ",
		BodyID:        common.GenerateNumericID("drone-alpha"),
		WalletAddress: wallet,
		Capabilities:  []string{"post", "post", " reply "},
	})
	require.NoError(t, err)
	require.Equal(t, "Bearer "+instanceKey, sawAuth)
	require.Equal(t, "example.com", sawBody["domain"])
	require.Equal(t, "drone-alpha", sawBody["local_id"])
	require.NotEqual(t, common.GenerateNumericID("drone-alpha"), sawBody["local_id"])
	require.Equal(t, wallet, sawBody["wallet_address"])
	require.Equal(t, []any{"post", "reply"}, sawBody["capabilities"])
	require.Equal(t, "reg_123", result.RegistrationID)
	require.Equal(t, agentID, result.HostSoulAgentID)
	require.Equal(t, "Sign this exact Host wallet challenge", result.WalletChallenge.Message)
	require.Equal(t, "host-req-begin", result.HostRequestID)
	require.NotNil(t, result.WalletChallenge.IssuedAt)
	require.NotNil(t, result.WalletChallenge.ExpiresAt)
}

func TestService_BootstrapConfigErrorsAreTyped(t *testing.T) {
	t.Parallel()

	t.Run("missing effective trust base URL", func(t *testing.T) {
		t.Parallel()
		service := NewService(
			&fakeAccountRepo{},
			&fakeInstanceRepo{trust: &storageModels.EffectiveTrustConfig{}},
			&config.Config{Domain: "example.com", LesserHostInstanceKey: "key"},
			zap.NewNop(),
		)
		_, err := service.BeginBootstrapRegistration(context.Background(), BootstrapBeginInput{})
		var hostErr *HostBootstrapError
		require.ErrorAs(t, err, &hostErr)
		require.ErrorIs(t, err, ErrHostTrustNotConfigured)
		require.Equal(t, "HOST_TRUST_NOT_CONFIGURED", hostErr.Code)
	})

	t.Run("missing instance key", func(t *testing.T) {
		t.Parallel()
		service := NewService(
			&fakeAccountRepo{},
			&fakeInstanceRepo{trust: &storageModels.EffectiveTrustConfig{TrustBaseURL: "https://host.example"}},
			&config.Config{Domain: "example.com"},
			zap.NewNop(),
		)
		_, err := service.BeginBootstrapRegistration(context.Background(), BootstrapBeginInput{})
		var hostErr *HostBootstrapError
		require.ErrorAs(t, err, &hostErr)
		require.ErrorIs(t, err, ErrHostInstanceKeyMissing)
		require.Equal(t, "HOST_INSTANCE_KEY_MISSING", hostErr.Code)
	})

	t.Run("unresolvable instance key", func(t *testing.T) {
		t.Parallel()
		service := NewService(
			&fakeAccountRepo{},
			&fakeInstanceRepo{trust: &storageModels.EffectiveTrustConfig{TrustBaseURL: "https://host.example", InstanceKeySecretARN: "arn:host:key"}},
			&config.Config{Domain: "example.com"},
			zap.NewNop(),
		).WithHostInstanceKeyResolver(func(context.Context, *config.Config, string) (string, error) {
			return "", errors.New("secrets manager unavailable")
		})
		_, err := service.BeginBootstrapRegistration(context.Background(), BootstrapBeginInput{})
		var hostErr *HostBootstrapError
		require.ErrorAs(t, err, &hostErr)
		require.ErrorIs(t, err, ErrHostInstanceKeyUnavailable)
		require.Equal(t, "HOST_INSTANCE_KEY_UNAVAILABLE", hostErr.Code)
		require.NotContains(t, hostErr.Message, "arn:host:key")
	})
}

func TestService_PrepareBootstrapPrincipalDeclarationRelaysSigningPayloadAndFailsClosed(t *testing.T) {
	t.Parallel()

	const (
		instanceKey = "host-instance-key"
		principal   = "0x2222222222222222222222222222222222222222"
	)
	declaredAt := time.Date(2026, 6, 12, 19, 0, 0, 0, time.UTC)
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer "+instanceKey, r.Header.Get("Authorization"))
		require.Equal(t, "/api/v1/soul/instance/agents/register/reg_123/principal-declaration/preflight", r.URL.Path)

		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Equal(t, principal, body["principal_address"])
		require.Contains(t, []string{"I declare the principal boundary.", "bad method"}, body["principal_declaration"])
		require.Equal(t, declaredAt.Format(time.RFC3339), body["declared_at"])

		method := "eip191_personal_sign"
		if body["principal_declaration"] == "bad method" || strings.Contains(r.URL.RawQuery, "bad_method=1") {
			method = "unknown"
		}
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"version":           "1",
			"principal_address": principal,
			"signer_address":    principal,
			"signing_method":    method,
			"message_encoding":  "hex_bytes",
			"message_hex":       "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			"digest_hex":        "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			"canonical_json":    `{"host":"owned"}`,
			"declared_at":       declaredAt.Format(time.RFC3339),
		}))
	}))
	defer host.Close()

	service := NewService(
		&fakeAccountRepo{},
		&fakeInstanceRepo{trust: &storageModels.EffectiveTrustConfig{TrustBaseURL: host.URL}},
		&config.Config{Domain: "example.com", LesserHostInstanceKey: instanceKey},
		zap.NewNop(),
	).WithHTTPClient(host.Client())

	result, err := service.PrepareBootstrapPrincipalDeclaration(context.Background(), BootstrapPrincipalPreflightInput{
		RegistrationID:       "reg_123",
		PrincipalAddress:     principal,
		PrincipalDeclaration: "I declare the principal boundary.",
		DeclaredAt:           declaredAt,
	})
	require.NoError(t, err)
	require.Equal(t, "1", result.Version)
	require.Equal(t, "eip191_personal_sign", result.SigningMethod)
	require.Equal(t, "hex_bytes", result.MessageEncoding)
	require.Equal(t, "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", result.MessageHex)
	require.Equal(t, "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", result.DigestHex)
	require.Equal(t, `{"host":"owned"}`, result.CanonicalJSON)
	require.Equal(t, declaredAt, *result.DeclaredAt)

	_, err = service.PrepareBootstrapPrincipalDeclaration(context.Background(), BootstrapPrincipalPreflightInput{
		RegistrationID:       "reg_123",
		PrincipalAddress:     principal,
		PrincipalDeclaration: "bad method",
		DeclaredAt:           declaredAt,
	})
	var hostErr *HostBootstrapError
	require.ErrorAs(t, err, &hostErr)
	require.ErrorIs(t, err, ErrHostSigningPayloadUnsupported)
	require.Equal(t, "HOST_SIGNING_PAYLOAD_UNSUPPORTED", hostErr.Code)
}

func TestService_HostNon2xxIsBoundedTypedAndSanitized(t *testing.T) {
	t.Parallel()

	const instanceKey = "host-instance-key"
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer "+instanceKey, r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-Id", "host-req-denied")
		w.WriteHeader(http.StatusForbidden)
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"code":        "soul_instance.boundary_violation",
				"message":     "do not leak " + instanceKey,
				"status_code": 403,
				"details": map[string]any{
					"auth_token":        "microvm-token-value",
					"boundary":          "instance_domain",
					"host_route":        "https://host.internal/api/v1/soul/instance/agents/register/reg/mint-conversation",
					"microvm_endpoint":  "https://microvm.internal/session/token",
					"provider_response": "raw model response",
					"reason":            "tenant_domain_mismatch",
					"nested": map[string]any{
						"safe_hint": "retry from GraphQL",
						"ssm_value": "parameter-value",
					},
				},
				"request_id": "host-req-body",
			},
		}))
	}))
	defer host.Close()

	service := NewService(
		&fakeAccountRepo{},
		&fakeInstanceRepo{trust: &storageModels.EffectiveTrustConfig{TrustBaseURL: host.URL}},
		&config.Config{Domain: "example.com", LesserHostInstanceKey: instanceKey},
		zap.NewNop(),
	).WithHTTPClient(host.Client())

	_, err := service.BeginBootstrapRegistration(context.Background(), BootstrapBeginInput{})
	var hostErr *HostBootstrapError
	require.ErrorAs(t, err, &hostErr)
	require.Equal(t, "soul_instance.boundary_violation", hostErr.Code)
	require.Equal(t, http.StatusForbidden, hostErr.StatusCode)
	require.Equal(t, "host-req-body", hostErr.HostRequestID)
	require.NotContains(t, hostErr.Message, instanceKey)
	require.Contains(t, hostErr.Message, "[redacted]")
	require.JSONEq(t, `{
		"auth_token":"[redacted]",
		"boundary":"instance_domain",
		"host_route":"[redacted]",
		"microvm_endpoint":"[redacted]",
		"provider_response":"[redacted]",
		"reason":"tenant_domain_mismatch",
		"nested":{"safe_hint":"retry from GraphQL","ssm_value":"[redacted]"}
	}`, hostErr.DetailsJSON)
	require.NotContains(t, hostErr.DetailsJSON, "microvm-token-value")
	require.NotContains(t, hostErr.DetailsJSON, "https://host.internal")
	require.NotContains(t, hostErr.DetailsJSON, "parameter-value")
}

func TestProject51SanitizeHostBootstrapDetailsRedactsNestedCredentialSurfaces(t *testing.T) {
	t.Parallel()

	require.Empty(t, sanitizeHostBootstrapDetails(nil, "instance-secret"))

	sanitized := sanitizeHostBootstrapDetails(json.RawMessage(`{
		"authorization":"Bearer instance-secret",
		"safe":"keep me",
		"nested":{
			"host_route":"https://host.internal/path",
			"safe_hint":"operator refreshes state"
		},
		"items":[
			{"provider_key":"pk-live"},
			"raw transcript fragment",
			"plain value"
		]
	}`), "instance-secret")
	require.JSONEq(t, `{
		"authorization":"[redacted]",
		"safe":"keep me",
		"nested":{
			"host_route":"[redacted]",
			"safe_hint":"operator refreshes state"
		},
		"items":[
			{"provider_key":"[redacted]"},
			"[redacted]",
			"plain value"
		]
	}`, sanitized)
	require.NotContains(t, sanitized, "instance-secret")
	require.NotContains(t, sanitized, "https://host.internal")
	require.NotContains(t, sanitized, "raw transcript")

	fallback := sanitizeHostBootstrapDetails(json.RawMessage(`not-json instance-secret`), "instance-secret")
	require.Equal(t, `not-json [redacted]`, fallback)
}

func TestProject51HostedDeclarationHashValidation(t *testing.T) {
	t.Parallel()

	require.True(t, validHostedDeclarationHash("sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"))
	require.False(t, validHostedDeclarationHash(""))
	require.False(t, validHostedDeclarationHash("sha256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"))
	require.False(t, validHostedDeclarationHash("md5:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"))
	require.False(t, validHostedDeclarationHash("sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"))
}

func TestProject51HostRawJSONValueNormalizesStringsAndObjects(t *testing.T) {
	t.Parallel()

	require.Empty(t, hostRawJSONValue(nil))
	require.Empty(t, hostRawJSONValue(json.RawMessage(`null`)))
	require.Equal(t, "ready", hostRawJSONValue(json.RawMessage(`" ready "`)))
	require.Equal(t, `{"safe":true}`, hostRawJSONValue(json.RawMessage(`{ "safe": true }`)))
}

func TestService_BeginHostedBootstrapUsesInstanceTrustWithoutWallet(t *testing.T) {
	t.Parallel()

	const (
		instanceKey = "host-instance-key"
		agentID     = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	)
	var sawAuth string
	var sawBody map[string]any
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v1/soul/instance/agents/register/begin", r.URL.Path)
		sawAuth = r.Header.Get("Authorization")
		require.NoError(t, json.NewDecoder(r.Body).Decode(&sawBody))
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-Id", "host-req-hosted-begin")
		w.WriteHeader(http.StatusCreated)
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"registration": map[string]any{
				"id":                "reg_hosted",
				"agent_id":          agentID,
				"domain_normalized": "example.com",
				"local_id":          "drone-hosted",
				"authority_model":   "instance_trust",
				"status":            "pending",
			},
			"proofs": []map[string]any{},
			"promotion": map[string]any{
				"agent_id":         agentID,
				"registration_id":  "reg_hosted",
				"domain":           "example.com",
				"local_id":         "drone-hosted",
				"authority_model":  "instance_trust",
				"anchor_state":     "hosted_offchain",
				"readiness_status": "ready_for_conversation",
			},
		}))
	}))
	defer host.Close()

	service := NewService(
		&fakeAccountRepo{},
		&fakeInstanceRepo{trust: &storageModels.EffectiveTrustConfig{TrustBaseURL: host.URL}},
		&config.Config{Domain: "example.com", LesserHostInstanceKey: instanceKey},
		zap.NewNop(),
	).WithHTTPClient(host.Client())

	result, err := service.BeginHostedBootstrapRegistration(context.Background(), BootstrapBeginInput{
		Username:     "drone-hosted",
		BodyID:       common.GenerateNumericID("drone-hosted"),
		Capabilities: []string{"post"},
	})
	require.NoError(t, err)
	require.Equal(t, "Bearer "+instanceKey, sawAuth)
	require.Equal(t, "example.com", sawBody["domain"])
	require.Equal(t, "drone-hosted", sawBody["local_id"])
	require.Equal(t, "instance_trust", sawBody["authority_model"])
	require.NotContains(t, sawBody, "wallet_address")
	require.Equal(t, []any{"post"}, sawBody["capabilities"])
	require.Equal(t, "reg_hosted", result.RegistrationID)
	require.Equal(t, agentID, result.HostSoulAgentID)
	require.Equal(t, "instance_trust", result.AuthorityModel)
	require.Equal(t, "hosted_offchain", result.AnchorState)
	require.Empty(t, result.WalletAddress)
	require.Empty(t, result.WalletChallenge.Message)
	require.Equal(t, "host-req-hosted-begin", result.HostRequestID)
}

func TestService_VerifyBootstrapPrincipalDeclarationSendsCombinedHostVerifyRequest(t *testing.T) {
	t.Parallel()

	const (
		instanceKey = "host-instance-key"
		wallet      = "0x1111111111111111111111111111111111111111"
		principal   = "0x2222222222222222222222222222222222222222"
		agentID     = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	)
	declaredAt := time.Date(2026, 6, 12, 20, 0, 0, 0, time.UTC)

	var sawBody map[string]any
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v1/soul/instance/agents/register/reg_123/verify", r.URL.Path)
		require.Equal(t, "Bearer "+instanceKey, r.Header.Get("Authorization"))
		require.NoError(t, json.NewDecoder(r.Body).Decode(&sawBody))
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-Id", "host-req-verify")
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"registration": map[string]any{
				"id":             "reg_123",
				"agent_id":       agentID,
				"wallet_address": wallet,
				"status":         "completed",
			},
			"operation": map[string]any{
				"operation_id": "op_123",
				"agent_id":     agentID,
				"status":       "proposed",
			},
			"promotion": map[string]any{
				"agent_id":          agentID,
				"registration_id":   "reg_123",
				"stage":             "approved",
				"principal_address": principal,
			},
		}))
	}))
	defer host.Close()

	service := NewService(
		&fakeAccountRepo{},
		&fakeInstanceRepo{trust: &storageModels.EffectiveTrustConfig{TrustBaseURL: host.URL}},
		&config.Config{Domain: "example.com", LesserHostInstanceKey: instanceKey},
		zap.NewNop(),
	).WithHTTPClient(host.Client())

	result, err := service.VerifyBootstrapPrincipalDeclaration(context.Background(), BootstrapPrincipalVerifyInput{
		RegistrationID:       "reg_123",
		WalletSignature:      "wallet-signature",
		PrincipalAddress:     principal,
		PrincipalDeclaration: "I declare.",
		PrincipalSignature:   "principal-signature",
		DeclaredAt:           declaredAt,
	})
	require.NoError(t, err)
	require.Equal(t, "wallet-signature", sawBody["signature"])
	require.Equal(t, principal, sawBody["principal_address"])
	require.Equal(t, "I declare.", sawBody["principal_declaration"])
	require.Equal(t, "principal-signature", sawBody["principal_signature"])
	require.Equal(t, declaredAt.Format(time.RFC3339), sawBody["declared_at"])
	require.Equal(t, "reg_123", result.RegistrationID)
	require.Equal(t, agentID, result.HostSoulAgentID)
	require.Equal(t, wallet, result.WalletAddress)
	require.Equal(t, principal, result.PrincipalAddress)
	require.Equal(t, "op_123", result.OperationID)
	require.Equal(t, "approved", result.PromotionStage)
	require.Equal(t, "host-req-verify", result.HostRequestID)
}

func TestService_BootstrapConversationFinalizeRelaysInstanceRoutes(t *testing.T) {
	t.Parallel()

	const (
		instanceKey = "host-instance-key"
		userBearer  = "user-oauth-token"
		agentID     = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		principal   = "0x2222222222222222222222222222222222222222"
	)
	issuedAt := time.Date(2026, 6, 12, 21, 15, 0, 0, time.UTC)

	var sawSendBody map[string]any
	var sawPreflightBody map[string]map[string]string
	var sawFinalizeBody map[string]any
	var sawAuthHeaders []string
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuthHeaders = append(sawAuthHeaders, r.Header.Get("Authorization"))
		require.Equal(t, "Bearer "+instanceKey, r.Header.Get("Authorization"))
		require.NotEqual(t, "Bearer "+userBearer, r.Header.Get("Authorization"))
		w.Header().Set("X-Request-Id", "host-req-"+strings.Trim(strings.ReplaceAll(r.URL.Path, "/", "-"), "-"))

		switch r.URL.Path {
		case "/api/v1/soul/instance/agents/register/reg_123/mint-conversation":
			require.Equal(t, "application/json", r.Header.Get("Accept"))
			require.NoError(t, json.NewDecoder(r.Body).Decode(&sawSendBody))
			w.Header().Set("Content-Type", "application/json")
			require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
				"registration_id": "reg_123",
				"agent_id":        agentID,
				"conversation_id": "conv_123",
				"model":           "claude",
				"status":          "assistant_turn_ready",
				"latest_turn_id":  "turn_assistant_001",
				"message_count":   2,
				"request_id":      "host-req-conversation-json",
			}))
		case "/api/v1/soul/instance/agents/register/reg_123/mint-conversation/conv_123/complete":
			w.Header().Set("Content-Type", "application/json")
			require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
				"agent_id":              agentID,
				"conversation_id":       "conv_123",
				"model":                 "claude",
				"status":                "completed",
				"produced_declarations": `{"selfDescription":{"summary":"ready"},"capabilities":[],"boundaries":[],"transparency":{}}`,
				"completed_at":          "2026-06-12T21:16:00Z",
			}))
		case "/api/v1/soul/instance/agents/register/reg_123/mint-conversation/conv_123/finalize/preflight":
			require.NoError(t, json.NewDecoder(r.Body).Decode(&sawPreflightBody))
			w.Header().Set("Content-Type", "application/json")
			require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
				"version":          "1",
				"digest_hex":       "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
				"issued_at":        issuedAt.Format(time.RFC3339),
				"expected_version": 0,
				"next_version":     1,
				"declarations_preview": map[string]any{
					"selfDescription": map[string]any{"summary": "ready"},
					"capabilities":    []any{},
					"boundaries":      []any{},
					"transparency":    map[string]any{},
				},
				"boundary_requirements": []map[string]any{
					{
						"boundary_id":      "b1",
						"category":         "safety",
						"statement":        "Respect boundaries.",
						"signer_wallet":    principal,
						"signing_method":   "eip191_personal_sign",
						"message_encoding": "utf8",
						"message":          "Sign boundary",
						"digest_hex":       "0xcccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
					},
				},
				"self_attestation_signing": map[string]any{
					"signer_wallet":    principal,
					"signing_method":   "eip191_personal_sign",
					"message_encoding": "hex_bytes",
					"message_hex":      "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
					"digest_hex":       "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
					"canonical_json":   `{"host":"finalize"}`,
				},
				"finalize_request_template": map[string]any{
					"boundary_signatures": map[string]string{"b1": ""},
					"issued_at":           issuedAt.Format(time.RFC3339),
					"expected_version":    0,
					"self_attestation":    "",
				},
				"registration_preview": map[string]any{"agent_id": agentID},
			}))
		case "/api/v1/soul/instance/agents/register/reg_123/mint-conversation/conv_123/finalize":
			require.NoError(t, json.NewDecoder(r.Body).Decode(&sawFinalizeBody))
			w.Header().Set("Content-Type", "application/json")
			require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
				"version":           "1",
				"agent_id":          agentID,
				"published_version": 1,
				"agent": map[string]any{
					"agent_id":          agentID,
					"principal_address": principal,
					"wallet":            principal,
					"status":            "active",
					"lifecycle_status":  "active",
				},
				"publication": map[string]any{
					"agent_id":                      agentID,
					"published_version":             1,
					"registration_uri":              "s3://bucket/registry/v1/agents/" + agentID + "/registration.json",
					"registration_s3_key":           "registry/v1/agents/" + agentID + "/registration.json",
					"versioned_registration_uri":    "s3://bucket/registry/v1/agents/" + agentID + "/versions/1/registration.json",
					"versioned_registration_s3_key": "registry/v1/agents/" + agentID + "/versions/1/registration.json",
					"anchor_state":                  "hosted_offchain",
					"published_at":                  "2026-06-12T21:17:00Z",
				},
				"promotion": map[string]any{
					"agent_id":                   agentID,
					"registration_id":            "reg_123",
					"stage":                      "graduated",
					"request_status":             "graduated",
					"review_status":              "published",
					"readiness_status":           "graduated",
					"anchor_state":               "hosted_offchain",
					"latest_conversation_id":     "conv_123",
					"latest_conversation_status": "completed",
					"published_version":          1,
					"graduated_at":               "2026-06-12T21:17:00Z",
				},
			}))
		default:
			http.NotFound(w, r)
		}
	}))
	defer host.Close()

	service := NewService(
		&fakeAccountRepo{},
		&fakeInstanceRepo{trust: &storageModels.EffectiveTrustConfig{TrustBaseURL: host.URL}},
		&config.Config{Domain: "example.com", LesserHostInstanceKey: instanceKey},
		zap.NewNop(),
	).WithHTTPClient(host.Client())

	sent, err := service.SendBootstrapConversationMessage(context.Background(), BootstrapConversationMessageInput{
		RegistrationID: "reg_123",
		Message:        "Review my declaration.",
		Model:          "claude",
	})
	require.NoError(t, err)
	require.Equal(t, "Review my declaration.", sawSendBody["message"])
	require.Equal(t, "claude", sawSendBody["model"])
	require.Equal(t, "conv_123", sent.ConversationID)
	require.Equal(t, agentID, sent.HostSoulAgentID)
	require.Equal(t, "assistant_turn_ready", sent.Status)
	require.Equal(t, "host-req-conversation-json", sent.HostRequestID)

	completed, err := service.CompleteBootstrapConversation(context.Background(), BootstrapConversationCompleteInput{
		RegistrationID: "reg_123",
		ConversationID: "conv_123",
	})
	require.NoError(t, err)
	require.Equal(t, "declaration_ready", completed.Status)
	require.Equal(t, agentID, completed.HostSoulAgentID)
	require.NotNil(t, completed.CompletedAt)

	preflight, err := service.PrepareBootstrapFinalize(context.Background(), BootstrapFinalizePreflightInput{
		RegistrationID:     "reg_123",
		ConversationID:     "conv_123",
		BoundarySignatures: map[string]string{"b1": "0xsig"},
	})
	require.NoError(t, err)
	require.Equal(t, "0xsig", sawPreflightBody["boundary_signatures"]["b1"])
	require.Equal(t, "eip191_personal_sign", preflight.SelfAttestationSigning.SigningMethod)
	require.Equal(t, "hex_bytes", preflight.SelfAttestationSigning.MessageEncoding)
	require.Contains(t, preflight.BoundaryRequirementsJSON, `"boundary_id":"b1"`)
	require.Contains(t, preflight.FinalizeRequestTemplateJSON, `"expected_version":0`)

	finalized, err := service.FinalizeBootstrap(context.Background(), BootstrapFinalizeInput{
		RegistrationID:     "reg_123",
		ConversationID:     "conv_123",
		BoundarySignatures: map[string]string{"b1": "0xsig"},
		IssuedAt:           issuedAt,
		ExpectedVersion:    0,
		SelfAttestation:    "0xself",
	})
	require.NoError(t, err)
	require.Equal(t, "0xself", sawFinalizeBody["self_attestation"])
	require.Equal(t, agentID, finalized.HostSoulAgentID)
	require.Equal(t, principal, finalized.PrincipalAddress)
	require.Equal(t, "hosted_offchain", finalized.Publication.AnchorState)
	require.Equal(t, "graduated", finalized.Promotion.Stage)
	require.Len(t, sawAuthHeaders, 4)
	for _, authHeader := range sawAuthHeaders {
		require.Equal(t, "Bearer "+instanceKey, authHeader)
		require.NotEqual(t, "Bearer "+userBearer, authHeader)
	}
}

func TestProject49ServiceSendConversationAcceptsDurableJSONInProgress(t *testing.T) {
	t.Parallel()

	const (
		instanceKey = "host-instance-key"
		agentID     = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	)

	var sawBody map[string]any
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v1/soul/instance/agents/register/reg_hosted/mint-conversation", r.URL.Path)
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "Bearer "+instanceKey, r.Header.Get("Authorization"))
		require.Equal(t, "application/json", r.Header.Get("Accept"))
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))
		require.NoError(t, json.NewDecoder(r.Body).Decode(&sawBody))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"registration_id":        "reg_hosted",
			"conversation_id":        "hconv_p49_001",
			"agent_id":               agentID,
			"status":                 "in_progress",
			"latest_turn_id":         "turn_user_001",
			"message_count":          1,
			"request_id":             "host-req-p49-json",
			"produced_declarations":  nil,
			"declarations_completed": false,
		}))
	}))
	defer host.Close()

	service := NewService(
		&fakeAccountRepo{},
		&fakeInstanceRepo{trust: &storageModels.EffectiveTrustConfig{TrustBaseURL: host.URL}},
		&config.Config{Domain: "example.com", LesserHostInstanceKey: instanceKey},
		zap.NewNop(),
	).WithHTTPClient(host.Client())

	result, err := service.SendBootstrapConversationMessage(context.Background(), BootstrapConversationMessageInput{
		RegistrationID: "reg_hosted",
		Message:        "start hosted genesis",
		Model:          "claude",
	})
	require.NoError(t, err)
	require.Equal(t, "start hosted genesis", sawBody["message"])
	require.Equal(t, "claude", sawBody["model"])
	require.Equal(t, "reg_hosted", result.RegistrationID)
	require.Equal(t, agentID, result.HostSoulAgentID)
	require.Equal(t, "hconv_p49_001", result.ConversationID)
	require.Equal(t, "in_progress", result.Status)
	require.Equal(t, 1, result.MessageCount)
	require.Equal(t, "host-req-p49-json", result.HostRequestID)
	require.Empty(t, result.ProducedDeclarations)
}

func TestProject49ServiceSendConversationAcceptsHostWrapperInProgress(t *testing.T) {
	t.Parallel()

	const (
		instanceKey = "host-instance-key"
		agentID     = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	)

	var sawBody map[string]any
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v1/soul/instance/agents/register/reg_hosted/mint-conversation", r.URL.Path)
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "Bearer "+instanceKey, r.Header.Get("Authorization"))
		require.Equal(t, "application/json", r.Header.Get("Accept"))
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))
		require.NoError(t, json.NewDecoder(r.Body).Decode(&sawBody))

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-Id", "host-req-header-ignored")
		w.WriteHeader(http.StatusOK)
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"version":    "1",
			"request_id": "host-req-p49-wrapper",
			"conversation": map[string]any{
				"registration_id":        "reg_hosted",
				"conversation_id":        "hconv_p49_001",
				"agent_id":               agentID,
				"status":                 "in_progress",
				"latest_turn_id":         "turn_user_001",
				"message_count":          1,
				"produced_declarations":  nil,
				"declarations_completed": false,
			},
		}))
	}))
	defer host.Close()

	service := NewService(
		&fakeAccountRepo{},
		&fakeInstanceRepo{trust: &storageModels.EffectiveTrustConfig{TrustBaseURL: host.URL}},
		&config.Config{Domain: "example.com", LesserHostInstanceKey: instanceKey},
		zap.NewNop(),
	).WithHTTPClient(host.Client())

	result, err := service.SendBootstrapConversationMessage(context.Background(), BootstrapConversationMessageInput{
		RegistrationID: "reg_hosted",
		Message:        "start hosted genesis",
		Model:          "claude",
	})
	require.NoError(t, err)
	require.Equal(t, "start hosted genesis", sawBody["message"])
	require.Equal(t, "claude", sawBody["model"])
	require.Equal(t, "reg_hosted", result.RegistrationID)
	require.Equal(t, agentID, result.HostSoulAgentID)
	require.Equal(t, "hconv_p49_001", result.ConversationID)
	require.Equal(t, "in_progress", result.Status)
	require.Equal(t, 1, result.MessageCount)
	require.Equal(t, "host-req-p49-wrapper", result.HostRequestID)
	require.Empty(t, result.ProducedDeclarations)
}

func TestProject51ServiceReplaysHostV106ConversationFixtures(t *testing.T) {
	t.Parallel()

	const (
		instanceKey    = "host-instance-key"
		registrationID = "reg_01jzhostedgenesis"
		conversationID = "conv_01jzhostedgenesis"
		agentID        = "0x2222222222222222222222222222222222222222222222222222222222222222"
	)

	expectedDeclarations := `{
		"selfDescription": {
			"summary": "Demo hosted soul for a managed Lesser drone.",
			"version": "v2"
		},
		"capabilities": [
			{
				"id": "chat",
				"statement": "Can answer operator questions using its configured Lesser tools."
			}
		],
		"boundaries": [
			{
				"id": "no-credential-disclosure",
				"category": "security",
				"statement": "Will not disclose raw credentials, seed phrases, or Instance API keys."
			}
		],
		"transparency": {
			"authority_model": "instance_trust",
			"anchor_state": "hosted_offchain"
		}
	}`

	var sawPostAuth string
	var sawGetAuth string
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/soul/instance/agents/register/" + registrationID + "/mint-conversation":
			require.Equal(t, http.MethodPost, r.Method)
			sawPostAuth = r.Header.Get("Authorization")
			w.WriteHeader(http.StatusAccepted)
			_, err := w.Write(project51HostConversationFixture(t, "hosted-genesis.conversation.in-progress.example.json"))
			require.NoError(t, err)
		case "/api/v1/soul/instance/agents/register/" + registrationID + "/mint-conversation/" + conversationID:
			require.Equal(t, http.MethodGet, r.Method)
			sawGetAuth = r.Header.Get("Authorization")
			_, err := w.Write(project51HostConversationFixture(t, "hosted-genesis.conversation.completed-declaration-ready.example.json"))
			require.NoError(t, err)
		default:
			http.NotFound(w, r)
		}
	}))
	defer host.Close()

	service := NewService(
		&fakeAccountRepo{},
		&fakeInstanceRepo{trust: &storageModels.EffectiveTrustConfig{TrustBaseURL: host.URL}},
		&config.Config{Domain: "example.com", LesserHostInstanceKey: instanceKey},
		zap.NewNop(),
	).WithHTTPClient(host.Client())

	sent, err := service.SendBootstrapConversationMessage(context.Background(), BootstrapConversationMessageInput{
		RegistrationID: registrationID,
		Message:        "start hosted genesis",
	})
	require.NoError(t, err)
	require.Equal(t, "Bearer "+instanceKey, sawPostAuth)
	// P52 L3.1: a 202 Accepted-Pending is transport success. The 202 body is
	// NOT parsed as a full snapshot (G12) and never carries inline assistant
	// messages (G13): MessageCount is 0 and Messages is empty. Host's
	// conversation/agent ids are extracted best-effort so Lesser persists
	// host_conversation_id early; status is the canonical pending status.
	require.Equal(t, registrationID, sent.RegistrationID)
	require.Equal(t, agentID, sent.HostSoulAgentID)
	require.Equal(t, conversationID, sent.ConversationID)
	require.Equal(t, "in_progress", sent.Status)
	require.Equal(t, 0, sent.MessageCount)
	require.Empty(t, sent.Messages)
	require.Equal(t, "req_hosted_genesis_01", sent.HostRequestID)
	require.Empty(t, sent.ProducedDeclarations)

	completed, err := service.ReadBootstrapConversation(context.Background(), BootstrapConversationCompleteInput{
		RegistrationID: registrationID,
		ConversationID: conversationID,
	})
	require.NoError(t, err)
	require.Equal(t, "Bearer "+instanceKey, sawGetAuth)
	require.Equal(t, registrationID, completed.RegistrationID)
	require.Equal(t, agentID, completed.HostSoulAgentID)
	require.Equal(t, conversationID, completed.ConversationID)
	require.Equal(t, "declaration_ready", completed.Status)
	require.Equal(t, 2, completed.MessageCount)
	require.Equal(t, "req_hosted_genesis_02", completed.HostRequestID)
	require.JSONEq(t, expectedDeclarations, completed.ProducedDeclarations)
	require.NoError(t, ValidateHostedBootstrapCompletionEvidence(completed, conversationID))
	require.NotContains(t, completed.ProducedDeclarations, "declaration_id")
	require.NotContains(t, completed.ProducedDeclarations, "raw_transcript")
}

func TestProject51ServiceAssistantTurnReadyFixtureIncludesTranscript(t *testing.T) {
	t.Parallel()

	const (
		instanceKey    = "host-instance-key"
		registrationID = "reg_01jzhostedgenesis"
		conversationID = "conv_01jzhostedgenesis"
		agentID        = "0x2222222222222222222222222222222222222222222222222222222222222222"
	)

	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v1/soul/instance/agents/register/"+registrationID+"/mint-conversation/"+conversationID, r.URL.Path)
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "Bearer "+instanceKey, r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write(project51HostConversationFixture(t, "hosted-genesis.conversation.assistant-turn-ready.example.json"))
		require.NoError(t, err)
	}))
	defer host.Close()

	service := NewService(
		&fakeAccountRepo{},
		&fakeInstanceRepo{trust: &storageModels.EffectiveTrustConfig{TrustBaseURL: host.URL}},
		&config.Config{Domain: "example.com", LesserHostInstanceKey: instanceKey},
		zap.NewNop(),
	).WithHTTPClient(host.Client())

	result, err := service.ReadBootstrapConversation(context.Background(), BootstrapConversationCompleteInput{
		RegistrationID: registrationID,
		ConversationID: conversationID,
	})
	require.NoError(t, err)
	require.Equal(t, registrationID, result.RegistrationID)
	require.Equal(t, agentID, result.HostSoulAgentID)
	require.Equal(t, conversationID, result.ConversationID)
	require.Equal(t, "assistant_turn_ready", result.Status)
	require.Equal(t, "turn_01jzhostedgenesis_user", result.LatestTurnID)
	require.Equal(t, 2, result.MessageCount)
	require.Equal(t, "req_hosted_genesis_04", result.HostRequestID)
	require.NotNil(t, result.UpdatedAt)
	require.Equal(t, "2026-06-18T13:10:30Z", result.UpdatedAt.Format(time.RFC3339))
	require.False(t, result.MessagesTruncated)
	require.Len(t, result.Messages, 2)
	require.Equal(t, BootstrapConversationMessage{
		ID:      "msg_000001",
		Role:    "user",
		Content: "Describe the managed Lesser agent you are becoming.",
		Order:   1,
		CreatedAt: func() *time.Time {
			createdAt := time.Date(2026, 6, 18, 13, 10, 1, 0, time.UTC)
			return &createdAt
		}(),
	}, result.Messages[0])
	require.Equal(t, "msg_000002", result.Messages[1].ID)
	require.Equal(t, "assistant", result.Messages[1].Role)
	require.Contains(t, result.Messages[1].Content, "bounded tools")
	require.Empty(t, result.ProducedDeclarations)
}

func TestBootstrapConversationMessagesFromHostHandlesLegacyStringAndUnsafeContent(t *testing.T) {
	t.Parallel()

	legacyTranscript := `[{"role":"USER","content":" hello lesser ","order":0},{"id":"tool_ignored","role":"tool","content":"internal"},{"role":"assistant","content":"reply","order":2,"truncated":true}]`
	encodedLegacyTranscript, err := json.Marshal(legacyTranscript)
	require.NoError(t, err)

	messages := bootstrapConversationMessagesFromHost(json.RawMessage(encodedLegacyTranscript))
	require.Len(t, messages, 2)
	require.Equal(t, "msg_000001", messages[0].ID)
	require.Equal(t, hostConversationMessageRoleUser, messages[0].Role)
	require.Equal(t, "hello lesser", messages[0].Content)
	require.Equal(t, 1, messages[0].Order)
	require.Nil(t, messages[0].CreatedAt)
	require.Equal(t, "msg_000002", messages[1].ID)
	require.Equal(t, hostConversationMessageRoleAssistant, messages[1].Role)
	require.True(t, messages[1].Truncated)

	require.Nil(t, bootstrapConversationMessagesFromHost(json.RawMessage(``)))
	require.Nil(t, bootstrapConversationMessagesFromHost(json.RawMessage(`null`)))
	require.Nil(t, bootstrapConversationMessagesFromHost(json.RawMessage(`"not-json"`)))
	require.Nil(t, bootstrapConversationMessagesFromHost(json.RawMessage(`[{"role":"assistant","content":"Bearer host-token"}]`)))
}

func TestProject51ServiceFailedFixtureProjectsHostRecoveryTruth(t *testing.T) {
	t.Parallel()

	const (
		instanceKey    = "host-instance-key"
		registrationID = "reg_01jzhostedgenesis"
		conversationID = "conv_01jzhostedgenesis"
		agentID        = "0x2222222222222222222222222222222222222222222222222222222222222222"
	)

	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v1/soul/instance/agents/register/"+registrationID+"/mint-conversation/"+conversationID, r.URL.Path)
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "Bearer "+instanceKey, r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write(project51HostConversationFixture(t, "hosted-genesis.conversation.failed.example.json"))
		require.NoError(t, err)
	}))
	defer host.Close()

	service := NewService(
		&fakeAccountRepo{},
		&fakeInstanceRepo{trust: &storageModels.EffectiveTrustConfig{TrustBaseURL: host.URL}},
		&config.Config{Domain: "example.com", LesserHostInstanceKey: instanceKey},
		zap.NewNop(),
	).WithHTTPClient(host.Client())

	result, err := service.ReadBootstrapConversation(context.Background(), BootstrapConversationCompleteInput{
		RegistrationID: registrationID,
		ConversationID: conversationID,
	})
	require.NoError(t, err)
	require.Equal(t, registrationID, result.RegistrationID)
	require.Equal(t, agentID, result.HostSoulAgentID)
	require.Equal(t, conversationID, result.ConversationID)
	require.Equal(t, "failed", result.Status)
	require.Equal(t, "llm_unavailable", result.FailureCode)
	require.Equal(t, "Assistant turn failed before declaration extraction.", result.FailureMessage)
	require.True(t, result.FailureRetryable)
	require.Equal(t, "retry_same_step", result.FailureRecoveryAction)
	require.Empty(t, result.ProducedDeclarations)
}

func TestProject51FailedConversationRequiresLockedRecoveryAction(t *testing.T) {
	t.Parallel()

	raw := project51HostConversationFixture(t, "hosted-genesis.conversation.failed.example.json")
	makeSnapshot := func(t *testing.T, mutate func(map[string]any)) hostMintConversationResponse {
		t.Helper()
		var envelope map[string]any
		require.NoError(t, json.Unmarshal(raw, &envelope))
		conversation := envelope["conversation"].(map[string]any)
		if mutate != nil {
			mutate(conversation)
		}
		encoded, err := json.Marshal(envelope)
		require.NoError(t, err)
		out, version, err := parseHostConversationReadEnvelope(encoded, "host-req-failed")
		require.NoError(t, err)
		require.Equal(t, hostBootstrapVersion1, version)
		return out
	}

	require.NoError(t, validateHostConversationSnapshot(makeSnapshot(t, nil), "reg_01jzhostedgenesis", true, "host-req-failed"))

	tests := []struct {
		name       string
		mutate     func(map[string]any)
		wantErrMsg string
	}{
		{
			name: "missing failure code",
			mutate: func(conversation map[string]any) {
				failure := conversation["failure"].(map[string]any)
				delete(failure, "code")
			},
			wantErrMsg: "failure code",
		},
		{
			name: "missing failure message",
			mutate: func(conversation map[string]any) {
				failure := conversation["failure"].(map[string]any)
				delete(failure, "message")
			},
			wantErrMsg: "failure message",
		},
		{
			name: "missing recovery action",
			mutate: func(conversation map[string]any) {
				failure := conversation["failure"].(map[string]any)
				recovery := failure["recovery"].(map[string]any)
				delete(recovery, "action")
			},
			wantErrMsg: "locked recovery action",
		},
		{
			name: "unknown recovery action",
			mutate: func(conversation map[string]any) {
				failure := conversation["failure"].(map[string]any)
				recovery := failure["recovery"].(map[string]any)
				recovery["action"] = "resume_microvm"
			},
			wantErrMsg: "locked recovery action",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateHostConversationSnapshot(makeSnapshot(t, tt.mutate), "reg_01jzhostedgenesis", true, "host-req-failed")
			var hostErr *HostBootstrapError
			require.ErrorAs(t, err, &hostErr)
			require.Equal(t, "HOST_RESPONSE_INVALID", hostErr.Code)
			require.Equal(t, "host", hostErr.Source)
			require.Contains(t, hostErr.Message, tt.wantErrMsg)
		})
	}
}

func TestProject51CompleteResultFromMessageResultCarriesRecoveryTruth(t *testing.T) {
	t.Parallel()

	completedAt := time.Date(2026, 6, 26, 14, 30, 0, 0, time.UTC)
	require.Nil(t, completeResultFromMessageResult(nil))

	got := completeResultFromMessageResult(&BootstrapConversationMessageResult{
		RegistrationID:        "reg_p51",
		HostSoulAgentID:       "0xcccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
		ConversationID:        "conv_p51",
		Status:                "failed",
		LatestTurnID:          "turn_p51",
		MessageCount:          3,
		ProducedDeclarations:  `{"safe":true}`,
		CompletedAt:           &completedAt,
		HostRequestID:         "req_p51",
		FailureCode:           "operator_action_required",
		FailureMessage:        "operator recovery required",
		FailureRetryable:      false,
		FailureRecoveryAction: "operator_action",
	})

	require.NotNil(t, got)
	require.Equal(t, "reg_p51", got.RegistrationID)
	require.Equal(t, "0xcccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", got.HostSoulAgentID)
	require.Equal(t, "conv_p51", got.ConversationID)
	require.Equal(t, "failed", got.Status)
	require.Equal(t, "turn_p51", got.LatestTurnID)
	require.Equal(t, 3, got.MessageCount)
	require.Equal(t, `{"safe":true}`, got.ProducedDeclarations)
	require.Equal(t, &completedAt, got.CompletedAt)
	require.Equal(t, "req_p51", got.HostRequestID)
	require.Equal(t, "operator_action_required", got.FailureCode)
	require.Equal(t, "operator recovery required", got.FailureMessage)
	require.False(t, got.FailureRetryable)
	require.Equal(t, "operator_action", got.FailureRecoveryAction)
}

func TestProject51DeclarationEvidenceEnvelopeAcceptsHostMessageCountDrift(t *testing.T) {
	t.Parallel()

	const conversationID = "conv_01jzhostedgenesis"
	raw := project51HostConversationFixture(t, "hosted-genesis.conversation.completed-declaration-ready.example.json")

	var envelope map[string]any
	require.NoError(t, json.Unmarshal(raw, &envelope))
	conversation := envelope["conversation"].(map[string]any)
	produced := conversation["produced_declarations"].(map[string]any)
	evidence := produced["evidence"].(map[string]any)

	conversation["message_count"] = float64(10)
	evidence["message_count"] = float64(11)

	encoded, err := json.Marshal(envelope)
	require.NoError(t, err)
	out, version, err := parseHostConversationResponseEnvelope(encoded, "host-req-test")
	require.NoError(t, err)
	require.Equal(t, hostBootstrapVersion1, version)
	result := bootstrapConversationCompleteResultFromHost("reg_01jzhostedgenesis", out, hostConversationRequestID("host-req-test", out.RequestID))

	require.NoError(t, ValidateHostedBootstrapCompletionEvidence(result, conversationID))
}

func TestProject51DeclarationEvidenceEnvelopeFailsClosed(t *testing.T) {
	t.Parallel()

	const conversationID = "conv_01jzhostedgenesis"
	raw := project51HostConversationFixture(t, "hosted-genesis.conversation.completed-declaration-ready.example.json")

	makeResult := func(t *testing.T, mutate func(map[string]any)) *BootstrapConversationCompleteResult {
		t.Helper()
		var envelope map[string]any
		require.NoError(t, json.Unmarshal(raw, &envelope))
		conversation := envelope["conversation"].(map[string]any)
		produced := conversation["produced_declarations"].(map[string]any)
		if mutate != nil {
			mutate(produced)
		}
		encoded, err := json.Marshal(envelope)
		require.NoError(t, err)
		out, version, err := parseHostConversationResponseEnvelope(encoded, "host-req-test")
		require.NoError(t, err)
		require.Equal(t, hostBootstrapVersion1, version)
		return bootstrapConversationCompleteResultFromHost("reg_01jzhostedgenesis", out, hostConversationRequestID("host-req-test", out.RequestID))
	}

	require.NoError(t, ValidateHostedBootstrapCompletionEvidence(makeResult(t, nil), conversationID))

	tests := []struct {
		name       string
		mutate     func(map[string]any)
		wantErrMsg string
	}{
		{
			name: "stale evidence conversation",
			mutate: func(produced map[string]any) {
				evidence := produced["evidence"].(map[string]any)
				evidence["conversation_id"] = "conv_stale"
			},
			wantErrMsg: "conversation id",
		},
		{
			name: "mismatched evidence request",
			mutate: func(produced map[string]any) {
				evidence := produced["evidence"].(map[string]any)
				evidence["request_id"] = "req_stale"
			},
			wantErrMsg: "request id",
		},
		{
			name: "invalid evidence source",
			mutate: func(produced map[string]any) {
				evidence := produced["evidence"].(map[string]any)
				evidence["source"] = "raw_transcript"
			},
			wantErrMsg: "source",
		},
		{
			name: "missing declarations",
			mutate: func(produced map[string]any) {
				delete(produced, "declarations")
			},
			wantErrMsg: "missing selfDescription",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateHostedBootstrapCompletionEvidence(makeResult(t, tt.mutate), conversationID)
			var hostErr *HostBootstrapError
			require.ErrorAs(t, err, &hostErr)
			require.Equal(t, "HOST_RESPONSE_INVALID", hostErr.Code)
			require.Equal(t, "host", hostErr.Source)
			require.Contains(t, hostErr.Message, tt.wantErrMsg)
		})
	}
}

func TestService_SendBootstrapConversationRejectsUnsupportedHostWrapperVersion(t *testing.T) {
	t.Parallel()

	const (
		instanceKey = "host-instance-key"
		agentID     = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	)

	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v1/soul/instance/agents/register/reg_hosted/mint-conversation", r.URL.Path)
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "Bearer "+instanceKey, r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-Id", "host-req-version-header")
		w.WriteHeader(http.StatusOK)
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"version":    "2",
			"request_id": "host-req-version-wrapper",
			"conversation": map[string]any{
				"registration_id":        "reg_hosted",
				"conversation_id":        "hconv_p49_001",
				"agent_id":               agentID,
				"status":                 "in_progress",
				"latest_turn_id":         "turn_user_001",
				"message_count":          1,
				"produced_declarations":  nil,
				"declarations_completed": false,
			},
		}))
	}))
	defer host.Close()

	service := NewService(
		&fakeAccountRepo{},
		&fakeInstanceRepo{trust: &storageModels.EffectiveTrustConfig{TrustBaseURL: host.URL}},
		&config.Config{Domain: "example.com", LesserHostInstanceKey: instanceKey},
		zap.NewNop(),
	).WithHTTPClient(host.Client())

	_, err := service.SendBootstrapConversationMessage(context.Background(), BootstrapConversationMessageInput{
		RegistrationID: "reg_hosted",
		Message:        "start hosted genesis",
	})
	var hostErr *HostBootstrapError
	require.ErrorAs(t, err, &hostErr)
	require.ErrorIs(t, err, ErrHostUnavailable)
	require.Equal(t, "HOST_RESPONSE_INVALID", hostErr.Code)
	require.Equal(t, "host-req-version-wrapper", hostErr.HostRequestID)
	require.Contains(t, hostErr.Message, "unsupported version")
}

func TestService_CompleteBootstrapConversationAcceptsHostWrapperAcceptedInProgress(t *testing.T) {
	t.Parallel()

	const (
		instanceKey = "host-instance-key"
		agentID     = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	)

	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v1/soul/instance/agents/register/reg_hosted/mint-conversation/conv_hosted/complete", r.URL.Path)
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "Bearer "+instanceKey, r.Header.Get("Authorization"))
		require.Equal(t, "application/json", r.Header.Get("Accept"))
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-Id", "host-req-complete-header")
		w.WriteHeader(http.StatusAccepted)
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"version":    "1",
			"request_id": "host-req-complete-wrapper",
			"conversation": map[string]any{
				"registration_id":        "reg_hosted",
				"conversation_id":        "conv_hosted",
				"agent_id":               agentID,
				"status":                 "in_progress",
				"latest_turn_id":         "turn_user_002",
				"message_count":          2,
				"produced_declarations":  nil,
				"declarations_completed": false,
			},
		}))
	}))
	defer host.Close()

	service := NewService(
		&fakeAccountRepo{},
		&fakeInstanceRepo{trust: &storageModels.EffectiveTrustConfig{TrustBaseURL: host.URL}},
		&config.Config{Domain: "example.com", LesserHostInstanceKey: instanceKey},
		zap.NewNop(),
	).WithHTTPClient(host.Client())

	result, err := service.CompleteBootstrapConversation(context.Background(), BootstrapConversationCompleteInput{
		RegistrationID: "reg_hosted",
		ConversationID: "conv_hosted",
	})
	require.NoError(t, err)
	require.Equal(t, "reg_hosted", result.RegistrationID)
	require.Equal(t, agentID, result.HostSoulAgentID)
	require.Equal(t, "conv_hosted", result.ConversationID)
	require.Equal(t, "in_progress", result.Status)
	require.Equal(t, 2, result.MessageCount)
	require.Equal(t, "host-req-complete-wrapper", result.HostRequestID)
	require.Empty(t, result.ProducedDeclarations)
}

func TestService_CompleteBootstrapConversationRequiresTerminalDeclarationEvidence(t *testing.T) {
	t.Parallel()

	const instanceKey = "host-instance-key"
	validDeclarations := `{"selfDescription":{"summary":"ready"},"capabilities":[],"boundaries":[],"transparency":{}}`

	tests := []struct {
		name         string
		response     map[string]any
		wrap         bool
		wrapperReqID string
		wantErr      bool
		wantContains string
	}{
		{
			name: "in progress is progress",
			response: map[string]any{
				"conversation_id":       "conv_hosted",
				"status":                "in_progress",
				"produced_declarations": validDeclarations,
			},
			wantErr: false,
		},
		{
			name: "declaration ready wrapper with empty declarations",
			response: map[string]any{
				"conversation_id":       "conv_hosted",
				"status":                "declaration_ready",
				"produced_declarations": "",
			},
			wrap:         true,
			wrapperReqID: "host-req-complete-wrapper-invalid",
			wantErr:      true,
			wantContains: "produced declarations",
		},
		{
			name: "conversation mismatch",
			response: map[string]any{
				"conversation_id":       "conv_other",
				"status":                "completed",
				"produced_declarations": validDeclarations,
			},
			wantErr:      true,
			wantContains: "conversation id",
		},
		{
			name: "missing declaration shape",
			response: map[string]any{
				"conversation_id":       "conv_hosted",
				"status":                "completed",
				"produced_declarations": `{"selfDescription":{"summary":"ready"}}`,
			},
			wantErr:      true,
			wantContains: "capabilities",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "/api/v1/soul/instance/agents/register/reg_hosted/mint-conversation/conv_hosted/complete", r.URL.Path)
				require.Equal(t, "Bearer "+instanceKey, r.Header.Get("Authorization"))
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("X-Request-Id", "host-req-complete-invalid")
				response := any(tt.response)
				if tt.wrap {
					response = map[string]any{
						"version":      "1",
						"request_id":   tt.wrapperReqID,
						"conversation": tt.response,
					}
				}
				require.NoError(t, json.NewEncoder(w).Encode(response))
			}))
			defer host.Close()

			service := NewService(
				&fakeAccountRepo{},
				&fakeInstanceRepo{trust: &storageModels.EffectiveTrustConfig{TrustBaseURL: host.URL}},
				&config.Config{Domain: "example.com", LesserHostInstanceKey: instanceKey},
				zap.NewNop(),
			).WithHTTPClient(host.Client())

			result, err := service.CompleteBootstrapConversation(context.Background(), BootstrapConversationCompleteInput{
				RegistrationID: "reg_hosted",
				ConversationID: "conv_hosted",
			})
			if !tt.wantErr {
				require.NoError(t, err)
				require.Equal(t, "in_progress", result.Status)
				return
			}
			var hostErr *HostBootstrapError
			require.ErrorAs(t, err, &hostErr)
			require.ErrorIs(t, err, ErrHostUnavailable)
			require.Equal(t, "HOST_RESPONSE_INVALID", hostErr.Code)
			require.Equal(t, "host", hostErr.Source)
			wantHostRequestID := "host-req-complete-invalid"
			if tt.wrapperReqID != "" {
				wantHostRequestID = tt.wrapperReqID
			}
			require.Equal(t, wantHostRequestID, hostErr.HostRequestID)
			require.Contains(t, hostErr.Message, tt.wantContains)
		})
	}
}

func TestService_CompleteBootstrapConversationRecoversCompletedConflictFromReadRoute(t *testing.T) {
	t.Parallel()

	const (
		instanceKey = "host-instance-key"
		agentID     = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	)
	validDeclarations := `{"selfDescription":{"summary":"ready"},"capabilities":[],"boundaries":[],"transparency":{}}`
	completedAt := "2026-06-17T14:15:00Z"

	completeCalls := 0
	readCalls := 0
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer "+instanceKey, r.Header.Get("Authorization"))
		require.Equal(t, "application/json", r.Header.Get("Accept"))
		switch r.URL.Path {
		case "/api/v1/soul/instance/agents/register/reg_hosted/mint-conversation/conv_hosted/complete":
			completeCalls++
			require.Equal(t, http.MethodPost, r.Method)
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Request-Id", "host-req-complete-conflict-header")
			w.WriteHeader(http.StatusConflict)
			require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{
					"code":        "soul_instance.conflict",
					"message":     "conversation is not in progress",
					"status_code": 409,
					"request_id":  "host-req-complete-conflict",
				},
			}))
		case "/api/v1/soul/instance/agents/register/reg_hosted/mint-conversation/conv_hosted":
			readCalls++
			require.Equal(t, http.MethodGet, r.Method)
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Request-Id", "host-req-read-completed")
			require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
				"version": "1",
				"conversation": map[string]any{
					"agent_id":              agentID,
					"conversation_id":       "conv_hosted",
					"model":                 "claude",
					"messages":              `[{"role":"assistant","content":"ready"}]`,
					"status":                "completed",
					"produced_declarations": validDeclarations,
					"completed_at":          completedAt,
					"created_at":            "2026-06-17T14:00:00Z",
				},
			}))
		default:
			http.NotFound(w, r)
		}
	}))
	defer host.Close()

	service := NewService(
		&fakeAccountRepo{},
		&fakeInstanceRepo{trust: &storageModels.EffectiveTrustConfig{TrustBaseURL: host.URL}},
		&config.Config{Domain: "example.com", LesserHostInstanceKey: instanceKey},
		zap.NewNop(),
	).WithHTTPClient(host.Client())

	result, err := service.CompleteBootstrapConversation(context.Background(), BootstrapConversationCompleteInput{
		RegistrationID: "reg_hosted",
		ConversationID: "conv_hosted",
	})
	require.NoError(t, err)
	require.Equal(t, 1, completeCalls)
	require.Equal(t, 1, readCalls)
	require.Equal(t, "reg_hosted", result.RegistrationID)
	require.Equal(t, agentID, result.HostSoulAgentID)
	require.Equal(t, "conv_hosted", result.ConversationID)
	require.Equal(t, "declaration_ready", result.Status)
	require.JSONEq(t, validDeclarations, result.ProducedDeclarations)
	require.Equal(t, "host-req-read-completed", result.HostRequestID)
	require.NotNil(t, result.CompletedAt)
	require.Equal(t, completedAt, result.CompletedAt.Format(time.RFC3339))
}

func TestService_CompleteBootstrapConversationConflictReadFailsClosedWithoutRefreshEvidence(t *testing.T) {
	t.Parallel()

	const instanceKey = "host-instance-key"
	validDeclarations := `{"selfDescription":{"summary":"ready"},"capabilities":[],"boundaries":[],"transparency":{}}`

	tests := []struct {
		name         string
		conversation map[string]any
		wantErr      bool
		wantContains string
	}{
		{
			name: "failed terminal conversation",
			conversation: map[string]any{
				"conversation_id": "conv_hosted",
				"model":           "claude",
				"status":          "failed",
				"failure": map[string]any{
					"code":      "HOST_CONVERSATION_FAILED",
					"message":   "assistant failed",
					"retryable": true,
					"recovery": map[string]any{
						"action": "retry_same_step",
					},
				},
				"created_at": "2026-06-17T14:00:00Z",
			},
			wantErr: false,
		},
		{
			name: "completed without declarations",
			conversation: map[string]any{
				"conversation_id":       "conv_hosted",
				"model":                 "claude",
				"status":                "completed",
				"produced_declarations": "",
				"created_at":            "2026-06-17T14:00:00Z",
			},
			wantErr:      true,
			wantContains: "produced declarations",
		},
		{
			name: "completed with stale conversation id",
			conversation: map[string]any{
				"conversation_id":       "conv_other",
				"model":                 "claude",
				"status":                "completed",
				"produced_declarations": validDeclarations,
				"created_at":            "2026-06-17T14:00:00Z",
			},
			wantErr:      true,
			wantContains: "conversation id",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "Bearer "+instanceKey, r.Header.Get("Authorization"))
				switch r.URL.Path {
				case "/api/v1/soul/instance/agents/register/reg_hosted/mint-conversation/conv_hosted/complete":
					require.Equal(t, http.MethodPost, r.Method)
					w.Header().Set("Content-Type", "application/json")
					w.Header().Set("X-Request-Id", "host-req-conflict-header")
					w.WriteHeader(http.StatusConflict)
					require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
						"error": map[string]any{
							"code":        "HOST_BOOTSTRAP_CONFLICT",
							"message":     "Host reported a bootstrap conflict.",
							"status_code": 409,
							"request_id":  "host-req-conflict",
						},
					}))
				case "/api/v1/soul/instance/agents/register/reg_hosted/mint-conversation/conv_hosted":
					require.Equal(t, http.MethodGet, r.Method)
					w.Header().Set("Content-Type", "application/json")
					w.Header().Set("X-Request-Id", "host-req-read-no-evidence")
					require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
						"version":      "1",
						"conversation": tt.conversation,
					}))
				default:
					http.NotFound(w, r)
				}
			}))
			defer host.Close()

			service := NewService(
				&fakeAccountRepo{},
				&fakeInstanceRepo{trust: &storageModels.EffectiveTrustConfig{TrustBaseURL: host.URL}},
				&config.Config{Domain: "example.com", LesserHostInstanceKey: instanceKey},
				zap.NewNop(),
			).WithHTTPClient(host.Client())

			result, err := service.CompleteBootstrapConversation(context.Background(), BootstrapConversationCompleteInput{
				RegistrationID: "reg_hosted",
				ConversationID: "conv_hosted",
			})
			if !tt.wantErr {
				require.NoError(t, err)
				require.Equal(t, "failed", result.Status)
				require.Equal(t, "HOST_CONVERSATION_FAILED", result.FailureCode)
				return
			}
			var hostErr *HostBootstrapError
			require.ErrorAs(t, err, &hostErr)
			require.Equal(t, "HOST_RESPONSE_INVALID", hostErr.Code)
			require.Equal(t, "host", hostErr.Source)
			require.Equal(t, "host-req-read-no-evidence", hostErr.HostRequestID)
			require.Contains(t, hostErr.Message, tt.wantContains)
		})
	}
}

func TestService_ReadBootstrapConversationUsesInstanceReadRoute(t *testing.T) {
	t.Parallel()

	const (
		instanceKey = "host-instance-key"
		agentID     = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	)
	validDeclarations := `{"selfDescription":{"summary":"ready"},"capabilities":[],"boundaries":[],"transparency":{}}`

	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v1/soul/instance/agents/register/reg_hosted/mint-conversation/conv_hosted", r.URL.Path)
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "Bearer "+instanceKey, r.Header.Get("Authorization"))
		require.Equal(t, "application/json", r.Header.Get("Accept"))
		require.Empty(t, r.Header.Get("Content-Type"))
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-Id", "host-req-read-direct")
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"version": "1",
			"conversation": map[string]any{
				"agent_id":              agentID,
				"conversation_id":       "conv_hosted",
				"model":                 "claude",
				"status":                "completed",
				"produced_declarations": validDeclarations,
				"created_at":            "2026-06-17T14:00:00Z",
				"completed_at":          "2026-06-17T14:15:00Z",
			},
		}))
	}))
	defer host.Close()

	service := NewService(
		&fakeAccountRepo{},
		&fakeInstanceRepo{trust: &storageModels.EffectiveTrustConfig{TrustBaseURL: host.URL}},
		&config.Config{Domain: "example.com", LesserHostInstanceKey: instanceKey},
		zap.NewNop(),
	).WithHTTPClient(host.Client())

	result, err := service.ReadBootstrapConversation(context.Background(), BootstrapConversationCompleteInput{
		RegistrationID: "reg_hosted",
		ConversationID: "conv_hosted",
	})
	require.NoError(t, err)
	require.Equal(t, "reg_hosted", result.RegistrationID)
	require.Equal(t, agentID, result.HostSoulAgentID)
	require.Equal(t, "conv_hosted", result.ConversationID)
	require.Equal(t, "declaration_ready", result.Status)
	require.Equal(t, "host-req-read-direct", result.HostRequestID)
	require.NoError(t, ValidateHostedBootstrapCompletionEvidence(result, "conv_hosted"))
}

func TestService_ReadBootstrapConversationRejectsUnsupportedEnvelopeVersion(t *testing.T) {
	t.Parallel()

	const instanceKey = "host-instance-key"
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v1/soul/instance/agents/register/reg_hosted/mint-conversation/conv_hosted", r.URL.Path)
		require.Equal(t, http.MethodGet, r.Method)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-Id", "host-req-read-version")
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"version": "2",
			"conversation": map[string]any{
				"conversation_id": "conv_hosted",
				"status":          "completed",
			},
		}))
	}))
	defer host.Close()

	service := NewService(
		&fakeAccountRepo{},
		&fakeInstanceRepo{trust: &storageModels.EffectiveTrustConfig{TrustBaseURL: host.URL}},
		&config.Config{Domain: "example.com", LesserHostInstanceKey: instanceKey},
		zap.NewNop(),
	).WithHTTPClient(host.Client())

	_, err := service.ReadBootstrapConversation(context.Background(), BootstrapConversationCompleteInput{
		RegistrationID: "reg_hosted",
		ConversationID: "conv_hosted",
	})
	var hostErr *HostBootstrapError
	require.ErrorAs(t, err, &hostErr)
	require.Equal(t, "HOST_RESPONSE_INVALID", hostErr.Code)
	require.Equal(t, "host-req-read-version", hostErr.HostRequestID)
	require.Contains(t, hostErr.Message, "unsupported version")
}

func TestService_CompleteBootstrapConversationConflictRecoveryFallsBackToConflictRequestID(t *testing.T) {
	t.Parallel()

	const instanceKey = "host-instance-key"
	validDeclarations := `{"selfDescription":{"summary":"ready"},"capabilities":[],"boundaries":[],"transparency":{}}`

	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer "+instanceKey, r.Header.Get("Authorization"))
		switch r.URL.Path {
		case "/api/v1/soul/instance/agents/register/reg_hosted/mint-conversation/conv_hosted/complete":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{
					"code":        "soul_instance.conflict",
					"message":     "conversation is not in progress",
					"status_code": 409,
					"request_id":  "host-req-conflict-fallback",
				},
			}))
		case "/api/v1/soul/instance/agents/register/reg_hosted/mint-conversation/conv_hosted":
			w.Header().Set("Content-Type", "application/json")
			require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
				"version": "1",
				"conversation": map[string]any{
					"conversation_id":       "conv_hosted",
					"model":                 "claude",
					"status":                "completed",
					"produced_declarations": validDeclarations,
					"created_at":            "2026-06-17T14:00:00Z",
				},
			}))
		default:
			http.NotFound(w, r)
		}
	}))
	defer host.Close()

	service := NewService(
		&fakeAccountRepo{},
		&fakeInstanceRepo{trust: &storageModels.EffectiveTrustConfig{TrustBaseURL: host.URL}},
		&config.Config{Domain: "example.com", LesserHostInstanceKey: instanceKey},
		zap.NewNop(),
	).WithHTTPClient(host.Client())

	result, err := service.CompleteBootstrapConversation(context.Background(), BootstrapConversationCompleteInput{
		RegistrationID: "reg_hosted",
		ConversationID: "conv_hosted",
	})
	require.NoError(t, err)
	require.Equal(t, "host-req-conflict-fallback", result.HostRequestID)
}

func TestHostBootstrapConversationConflictClassifier(t *testing.T) {
	t.Parallel()

	require.False(t, isHostBootstrapConversationConflict(errors.New("plain")))
	require.True(t, isHostBootstrapConversationConflict(&HostBootstrapError{Code: "soul_instance.conflict"}))
	require.True(t, isHostBootstrapConversationConflict(&HostBootstrapError{Code: "HOST_BOOTSTRAP_CONFLICT"}))
	require.True(t, isHostBootstrapConversationConflict(&HostBootstrapError{StatusCode: http.StatusConflict}))
	require.True(t, isHostBootstrapConversationConflict(&HostBootstrapError{Message: "conversation is not in progress"}))
	require.Equal(t, "host-req", hostBootstrapRequestIDFromError(&HostBootstrapError{HostRequestID: " host-req "}))
	require.Empty(t, hostBootstrapRequestIDFromError(errors.New("plain")))
}

func TestValidateHostedBootstrapCompletionEvidenceCoversDeclarationShape(t *testing.T) {
	t.Parallel()

	validDeclarations := `{
		"selfDescription":{"summary":"ready"},
		"capabilities":[{"name":"post"}],
		"boundaries":[],
		"transparency":{}
	}`

	tests := []struct {
		name           string
		result         *BootstrapConversationCompleteResult
		expected       string
		wantErr        bool
		wantCode       string
		wantSource     string
		wantContains   string
		wantHostReqID  string
		wantErrMatches error
	}{
		{
			name:         "nil result",
			result:       nil,
			expected:     "conv_hosted",
			wantErr:      true,
			wantCode:     "HOST_RESPONSE_INVALID",
			wantSource:   "host",
			wantContains: "missing",
		},
		{
			name: "missing expected conversation id is local invalid request",
			result: &BootstrapConversationCompleteResult{
				ConversationID:       "conv_hosted",
				Status:               "completed",
				ProducedDeclarations: validDeclarations,
				HostRequestID:        "host-req-complete",
			},
			expected:       " ",
			wantErr:        true,
			wantCode:       "HOST_CONVERSATION_ID_REQUIRED",
			wantSource:     "lesser",
			wantContains:   "conversation id",
			wantErrMatches: ErrHostSigningPayloadUnsupported,
		},
		{
			name: "invalid declarations json",
			result: &BootstrapConversationCompleteResult{
				ConversationID:       "conv_hosted",
				Status:               "completed",
				ProducedDeclarations: `{`,
				HostRequestID:        "host-req-invalid-json",
			},
			expected:      "conv_hosted",
			wantErr:       true,
			wantCode:      "HOST_RESPONSE_INVALID",
			wantSource:    "host",
			wantContains:  "not valid JSON",
			wantHostReqID: "host-req-invalid-json",
		},
		{
			name: "empty declarations object",
			result: &BootstrapConversationCompleteResult{
				ConversationID:       "conv_hosted",
				Status:               "completed",
				ProducedDeclarations: `{}`,
				HostRequestID:        "host-req-empty",
			},
			expected:      "conv_hosted",
			wantErr:       true,
			wantCode:      "HOST_RESPONSE_INVALID",
			wantSource:    "host",
			wantContains:  "empty",
			wantHostReqID: "host-req-empty",
		},
		{
			name: "empty self description fails closed",
			result: &BootstrapConversationCompleteResult{
				ConversationID:       "conv_hosted",
				Status:               "completed",
				ProducedDeclarations: `{"selfDescription":{},"capabilities":[],"boundaries":[],"transparency":{}}`,
				HostRequestID:        "host-req-empty-self",
			},
			expected:      "conv_hosted",
			wantErr:       true,
			wantCode:      "HOST_RESPONSE_INVALID",
			wantSource:    "host",
			wantContains:  "empty selfDescription",
			wantHostReqID: "host-req-empty-self",
		},
		{
			name: "missing self description fails closed",
			result: &BootstrapConversationCompleteResult{
				ConversationID:       "conv_hosted",
				Status:               "completed",
				ProducedDeclarations: `{"capabilities":[],"boundaries":[],"transparency":{}}`,
				HostRequestID:        "host-req-missing-self",
			},
			expected:      "conv_hosted",
			wantErr:       true,
			wantCode:      "HOST_RESPONSE_INVALID",
			wantSource:    "host",
			wantContains:  "missing selfDescription",
			wantHostReqID: "host-req-missing-self",
		},
		{
			name: "invalid boundaries type fails closed",
			result: &BootstrapConversationCompleteResult{
				ConversationID:       "conv_hosted",
				Status:               "completed",
				ProducedDeclarations: `{"selfDescription":{"summary":"ready"},"capabilities":[],"boundaries":{},"transparency":{}}`,
				HostRequestID:        "host-req-boundaries",
			},
			expected:      "conv_hosted",
			wantErr:       true,
			wantCode:      "HOST_RESPONSE_INVALID",
			wantSource:    "host",
			wantContains:  "invalid boundaries",
			wantHostReqID: "host-req-boundaries",
		},
		{
			name: "invalid transparency type fails closed",
			result: &BootstrapConversationCompleteResult{
				ConversationID:       "conv_hosted",
				Status:               "completed",
				ProducedDeclarations: `{"selfDescription":{"summary":"ready"},"capabilities":[],"boundaries":[],"transparency":[]}`,
				HostRequestID:        "host-req-transparency",
			},
			expected:      "conv_hosted",
			wantErr:       true,
			wantCode:      "HOST_RESPONSE_INVALID",
			wantSource:    "host",
			wantContains:  "invalid transparency",
			wantHostReqID: "host-req-transparency",
		},
		{
			name: "complete terminal declarations accepted",
			result: &BootstrapConversationCompleteResult{
				ConversationID:       " conv_hosted ",
				Status:               "COMPLETED",
				ProducedDeclarations: validDeclarations,
				HostRequestID:        "host-req-valid",
			},
			expected: "conv_hosted",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateHostedBootstrapCompletionEvidence(tt.result, tt.expected)
			if !tt.wantErr {
				require.NoError(t, err)
				return
			}

			var hostErr *HostBootstrapError
			require.ErrorAs(t, err, &hostErr)
			require.Equal(t, tt.wantCode, hostErr.Code)
			require.Equal(t, tt.wantSource, hostErr.Source)
			require.Contains(t, hostErr.Message, tt.wantContains)
			require.Equal(t, tt.wantHostReqID, hostErr.HostRequestID)
			if tt.wantErrMatches != nil {
				require.ErrorIs(t, err, tt.wantErrMatches)
			} else {
				require.ErrorIs(t, err, ErrHostUnavailable)
			}
		})
	}
}

func TestService_PublishHostedBootstrapOmitsWalletSigningMaterial(t *testing.T) {
	t.Parallel()

	const (
		instanceKey = "host-instance-key"
		userBearer  = "user-oauth-token"
		agentID     = "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	)
	var sawBody map[string]any
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v1/soul/instance/agents/register/reg_hosted/mint-conversation/conv_hosted/finalize", r.URL.Path)
		require.Equal(t, "Bearer "+instanceKey, r.Header.Get("Authorization"))
		require.NotEqual(t, "Bearer "+userBearer, r.Header.Get("Authorization"))
		require.NoError(t, json.NewDecoder(r.Body).Decode(&sawBody))
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-Id", "host-req-hosted-publish")
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"version":           "1",
			"agent_id":          agentID,
			"published_version": 1,
			"agent": map[string]any{
				"agent_id":              agentID,
				"domain":                "example.com",
				"local_id":              "drone-hosted",
				"authority_model":       "instance_trust",
				"anchor_state":          "hosted_offchain",
				"operational_binding":   "hosted_bound_soul",
				"status":                "active",
				"lifecycle_status":      "active",
				"principal_address":     "",
				"principal_declaration": "",
			},
			"publication": map[string]any{
				"agent_id":                      agentID,
				"published_version":             1,
				"authority_model":               "instance_trust",
				"registration_uri":              "s3://bucket/registry/v1/agents/" + agentID + "/registration.json",
				"registration_s3_key":           "registry/v1/agents/" + agentID + "/registration.json",
				"versioned_registration_uri":    "s3://bucket/registry/v1/agents/" + agentID + "/versions/1/registration.json",
				"versioned_registration_s3_key": "registry/v1/agents/" + agentID + "/versions/1/registration.json",
				"anchor_state":                  "hosted_offchain",
				"published_at":                  "2026-06-14T02:00:00Z",
			},
			"promotion": map[string]any{
				"agent_id":                   agentID,
				"registration_id":            "reg_hosted",
				"stage":                      "graduated",
				"request_status":             "graduated",
				"review_status":              "published",
				"readiness_status":           "graduated",
				"authority_model":            "instance_trust",
				"anchor_state":               "hosted_offchain",
				"latest_conversation_id":     "conv_hosted",
				"latest_conversation_status": "completed",
				"published_version":          1,
				"graduated_at":               "2026-06-14T02:00:00Z",
			},
		}))
	}))
	defer host.Close()

	service := NewService(
		&fakeAccountRepo{},
		&fakeInstanceRepo{trust: &storageModels.EffectiveTrustConfig{TrustBaseURL: host.URL}},
		&config.Config{Domain: "example.com", LesserHostInstanceKey: instanceKey},
		zap.NewNop(),
	).WithHTTPClient(host.Client())

	result, err := service.PublishHostedBootstrap(context.Background(), HostedBootstrapPublishInput{
		RegistrationID: "reg_hosted",
		ConversationID: "conv_hosted",
		LocalID:        "drone-hosted",
	})
	require.NoError(t, err)
	require.Empty(t, sawBody)
	require.NotContains(t, sawBody, "boundary_signatures")
	require.NotContains(t, sawBody, "self_attestation")
	require.Equal(t, agentID, result.HostSoulAgentID)
	require.Equal(t, "example.com", result.AgentDomain)
	require.Equal(t, "drone-hosted", result.AgentLocalID)
	require.Equal(t, "instance_trust", result.AgentAuthorityModel)
	require.Equal(t, "hosted_offchain", result.AgentAnchorState)
	require.Equal(t, "hosted_bound_soul", result.AgentOperationalBinding)
	require.Equal(t, "instance_trust", result.Publication.AuthorityModel)
	require.Equal(t, "hosted_offchain", result.Publication.AnchorState)
	require.Equal(t, "instance_trust", result.Promotion.AuthorityModel)
	require.Equal(t, "host-req-hosted-publish", result.HostRequestID)
}

func TestValidateHostHostedFinalizeResponse_Guardrails(t *testing.T) {
	t.Parallel()

	const agentID = "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	valid := validHostedFinalizeResponse(agentID)
	require.NoError(t, validateHostHostedFinalizeResponse(valid, "example.com", "drone-hosted"))
	mixedCaseIDs := valid
	mixedCaseIDs.AgentID = strings.ToUpper(agentID)
	mixedCaseIDs.Agent.AgentID = strings.ToUpper(agentID)
	mixedCaseIDs.Publication.AgentID = strings.ToUpper(agentID)
	mixedCaseIDs.Promotion.AgentID = strings.ToUpper(agentID)
	require.NoError(t, validateHostHostedFinalizeResponse(mixedCaseIDs, "example.com", "drone-hosted"))

	testCases := []struct {
		name   string
		mutate func(*hostFinalizeResponse)
	}{
		{
			name: "unsupported version",
			mutate: func(out *hostFinalizeResponse) {
				out.Version = "2"
			},
		},
		{
			name: "invalid soul id",
			mutate: func(out *hostFinalizeResponse) {
				out.AgentID = "not-a-soul-id"
				out.Agent.AgentID = ""
				out.Publication.AgentID = ""
				out.Promotion.AgentID = ""
			},
		},
		{
			name: "top-level agent id mismatch",
			mutate: func(out *hostFinalizeResponse) {
				out.AgentID = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
			},
		},
		{
			name: "nested agent id mismatch",
			mutate: func(out *hostFinalizeResponse) {
				out.Agent.AgentID = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
			},
		},
		{
			name: "publication agent id mismatch",
			mutate: func(out *hostFinalizeResponse) {
				out.Publication.AgentID = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
			},
		},
		{
			name: "promotion agent id mismatch",
			mutate: func(out *hostFinalizeResponse) {
				out.Promotion.AgentID = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
			},
		},
		{
			name: "domain mismatch",
			mutate: func(out *hostFinalizeResponse) {
				out.Agent.Domain = "other.example"
			},
		},
		{
			name: "local id mismatch",
			mutate: func(out *hostFinalizeResponse) {
				out.Agent.LocalID = "other-drone"
			},
		},
		{
			name: "agent authority mismatch",
			mutate: func(out *hostFinalizeResponse) {
				out.Agent.AuthorityModel = SoulAuthorityModelWalletPrincipal
			},
		},
		{
			name: "publication authority mismatch",
			mutate: func(out *hostFinalizeResponse) {
				out.Publication.AuthorityModel = SoulAuthorityModelWalletPrincipal
			},
		},
		{
			name: "promotion authority mismatch",
			mutate: func(out *hostFinalizeResponse) {
				out.Promotion.AuthorityModel = SoulAuthorityModelWalletPrincipal
			},
		},
		{
			name: "agent anchor mismatch",
			mutate: func(out *hostFinalizeResponse) {
				out.Agent.AnchorState = SoulAnchorStateImmutableOnchain
			},
		},
		{
			name: "publication anchor mismatch",
			mutate: func(out *hostFinalizeResponse) {
				out.Publication.AnchorState = SoulAnchorStateImmutableOnchain
			},
		},
		{
			name: "promotion anchor mismatch",
			mutate: func(out *hostFinalizeResponse) {
				out.Promotion.AnchorState = SoulAnchorStateImmutableOnchain
			},
		},
		{
			name: "operational binding mismatch",
			mutate: func(out *hostFinalizeResponse) {
				out.Agent.OperationalBinding = "wallet_bound_soul"
			},
		},
		{
			name: "inactive status",
			mutate: func(out *hostFinalizeResponse) {
				out.Agent.Status = "suspended"
			},
		},
		{
			name: "missing published version",
			mutate: func(out *hostFinalizeResponse) {
				out.PublishedVersion = 0
				out.Publication.PublishedVersion = 0
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			out := valid
			tc.mutate(&out)
			var hostErr *HostBootstrapError
			require.ErrorAs(t, validateHostHostedFinalizeResponse(out, "example.com", "drone-hosted"), &hostErr)
			require.Equal(t, "HOST_RESPONSE_INVALID", hostErr.Code)
		})
	}
}

func TestValidateHostedBeginResponse_Guardrails(t *testing.T) {
	t.Parallel()

	valid := hostRegistrationBeginResponse{
		Registration: hostRegistration{
			AuthorityModel:   SoulAuthorityModelInstanceTrust,
			DomainNormalized: "example.com",
			LocalID:          "drone-hosted",
		},
		Promotion: hostPromotionResponse{
			AuthorityModel: SoulAuthorityModelInstanceTrust,
			Domain:         "example.com",
			LocalID:        "drone-hosted",
		},
	}
	require.NoError(t, validateHostedBeginResponse(valid, "example.com", "drone-hosted"))

	testCases := []struct {
		name   string
		mutate func(*hostRegistrationBeginResponse)
	}{
		{
			name: "authority mismatch",
			mutate: func(out *hostRegistrationBeginResponse) {
				out.Registration.AuthorityModel = SoulAuthorityModelWalletPrincipal
			},
		},
		{
			name: "domain mismatch",
			mutate: func(out *hostRegistrationBeginResponse) {
				out.Registration.DomainNormalized = "other.example"
			},
		},
		{
			name: "local id mismatch",
			mutate: func(out *hostRegistrationBeginResponse) {
				out.Registration.LocalID = "other-drone"
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			out := valid
			tc.mutate(&out)
			var hostErr *HostBootstrapError
			require.ErrorAs(t, validateHostedBeginResponse(out, "example.com", "drone-hosted"), &hostErr)
			require.Equal(t, "HOST_RESPONSE_INVALID", hostErr.Code)
		})
	}
}

func TestHostedBootstrapRejectsInvalidLocalInputs(t *testing.T) {
	t.Parallel()

	service := NewService(
		&fakeAccountRepo{},
		&fakeInstanceRepo{trust: &storageModels.EffectiveTrustConfig{TrustBaseURL: "https://host.example"}},
		&config.Config{Domain: "example.com", LesserHostInstanceKey: "instance-key"},
		zap.NewNop(),
	)

	var hostErr *HostBootstrapError
	_, err := service.PublishHostedBootstrap(context.Background(), HostedBootstrapPublishInput{})
	require.ErrorAs(t, err, &hostErr)
	require.Equal(t, "HOST_REGISTRATION_ID_REQUIRED", hostErr.Code)

	hostErr = nil
	_, err = service.PublishHostedBootstrap(context.Background(), HostedBootstrapPublishInput{RegistrationID: "reg_hosted"})
	require.ErrorAs(t, err, &hostErr)
	require.Equal(t, "HOST_CONVERSATION_ID_REQUIRED", hostErr.Code)

	noKeyService := NewService(
		&fakeAccountRepo{},
		&fakeInstanceRepo{trust: &storageModels.EffectiveTrustConfig{TrustBaseURL: "https://host.example"}},
		&config.Config{Domain: "example.com"},
		zap.NewNop(),
	)
	hostErr = nil
	_, err = noKeyService.BeginHostedBootstrapRegistration(context.Background(), BootstrapBeginInput{Username: "drone-hosted"})
	require.ErrorAs(t, err, &hostErr)
	require.Equal(t, "HOST_INSTANCE_KEY_MISSING", hostErr.Code)
}

func TestHostBootstrapInputsRejectInvalidTrustConfiguration(t *testing.T) {
	t.Parallel()

	_, err := defaultHostInstanceKeyResolver(context.Background(), nil, "")
	require.NoError(t, err)

	var nilService *Service
	_, _, _, err = nilService.hostBootstrapInputs(context.Background())
	require.Error(t, err)

	service := NewService(
		&fakeAccountRepo{},
		&fakeInstanceRepo{trust: &storageModels.EffectiveTrustConfig{}},
		&config.Config{Domain: "example.com", LesserHostInstanceKey: "instance-key"},
		zap.NewNop(),
	)
	service.WithHostInstanceKeyResolver(nil)
	var hostErr *HostBootstrapError
	_, _, _, err = service.hostBootstrapInputs(context.Background())
	require.ErrorAs(t, err, &hostErr)
	require.Equal(t, "HOST_TRUST_NOT_CONFIGURED", hostErr.Code)

	service = NewService(
		&fakeAccountRepo{},
		&fakeInstanceRepo{trust: &storageModels.EffectiveTrustConfig{TrustBaseURL: "host.example"}},
		&config.Config{Domain: "example.com", LesserHostInstanceKey: "instance-key"},
		zap.NewNop(),
	)
	hostErr = nil
	_, _, _, err = service.hostBootstrapInputs(context.Background())
	require.ErrorAs(t, err, &hostErr)
	require.Equal(t, "HOST_TRUST_NOT_CONFIGURED", hostErr.Code)

	service = NewService(
		&fakeAccountRepo{},
		&fakeInstanceRepo{trust: &storageModels.EffectiveTrustConfig{TrustBaseURL: "https://host.example"}},
		&config.Config{Domain: ""},
		zap.NewNop(),
	)
	_, _, _, err = service.hostBootstrapInputs(context.Background())
	require.ErrorContains(t, err, "instance domain is required")

	service = NewService(
		&fakeAccountRepo{},
		&fakeInstanceRepo{trust: &storageModels.EffectiveTrustConfig{TrustBaseURL: "https://host.example"}},
		&config.Config{Domain: "example.com"},
		zap.NewNop(),
	).WithHostInstanceKeyResolver(func(context.Context, *config.Config, string) (string, error) {
		return "", errors.New("kms unavailable")
	})
	hostErr = nil
	_, _, _, err = service.hostBootstrapInputs(context.Background())
	require.ErrorAs(t, err, &hostErr)
	require.Equal(t, "HOST_INSTANCE_KEY_UNAVAILABLE", hostErr.Code)
}

func TestHostBootstrapSecretRedactionHelpers(t *testing.T) {
	t.Parallel()

	require.Empty(t, sanitizeHostBootstrapDetails(nil, "secret-token"))
	require.Empty(t, sanitizeHostBootstrapDetails(json.RawMessage(`   `), "secret-token"))
	require.JSONEq(t, `{"token":"[redacted]"}`, sanitizeHostBootstrapDetails(json.RawMessage(`{"token":"secret-token"}`), "secret-token"))
	require.Equal(t, "no secret", redactHostBootstrapSecret("no secret", ""))
}

func validHostedFinalizeResponse(agentID string) hostFinalizeResponse {
	return hostFinalizeResponse{
		Version:          hostBootstrapVersion1,
		AgentID:          agentID,
		PublishedVersion: 1,
		Agent: hostFinalizeAgent{
			AgentID:            agentID,
			Domain:             "example.com",
			LocalID:            "drone-hosted",
			AuthorityModel:     SoulAuthorityModelInstanceTrust,
			AnchorState:        SoulAnchorStateHostedOffchain,
			OperationalBinding: SoulOperationalBindingHostedBound,
			Status:             "active",
			LifecycleStatus:    "active",
		},
		Publication: hostFinalizePublication{
			AgentID:          agentID,
			PublishedVersion: 1,
			AuthorityModel:   SoulAuthorityModelInstanceTrust,
			AnchorState:      SoulAnchorStateHostedOffchain,
		},
		Promotion: hostFinalizePromotion{
			AgentID:          agentID,
			Stage:            "graduated",
			AuthorityModel:   SoulAuthorityModelInstanceTrust,
			AnchorState:      SoulAnchorStateHostedOffchain,
			PublishedVersion: 1,
		},
	}
}

func TestService_BootstrapRejectsInvalidLocalInputsAndResponses(t *testing.T) {
	t.Parallel()

	service := NewService(
		&fakeAccountRepo{},
		&fakeInstanceRepo{trust: &storageModels.EffectiveTrustConfig{TrustBaseURL: "https://host.example"}},
		&config.Config{Domain: "example.com", LesserHostInstanceKey: "instance-key"},
		zap.NewNop(),
	)

	_, err := service.PrepareBootstrapPrincipalDeclaration(context.Background(), BootstrapPrincipalPreflightInput{})
	var hostErr *HostBootstrapError
	require.ErrorAs(t, err, &hostErr)
	require.ErrorIs(t, err, ErrHostSigningPayloadUnsupported)
	require.Equal(t, "HOST_REGISTRATION_ID_REQUIRED", hostErr.Code)

	_, err = service.VerifyBootstrapPrincipalDeclaration(context.Background(), BootstrapPrincipalVerifyInput{})
	hostErr = nil
	require.ErrorAs(t, err, &hostErr)
	require.ErrorIs(t, err, ErrHostSigningPayloadUnsupported)
	require.Equal(t, "HOST_REGISTRATION_ID_REQUIRED", hostErr.Code)

	_, err = service.SendBootstrapConversationMessage(context.Background(), BootstrapConversationMessageInput{
		RegistrationID: "reg_123",
		Message:        " ",
	})
	hostErr = nil
	require.ErrorAs(t, err, &hostErr)
	require.ErrorIs(t, err, ErrHostSigningPayloadUnsupported)
	require.Equal(t, "HOST_INVALID_REQUEST", hostErr.Code)

	_, err = service.CompleteBootstrapConversation(context.Background(), BootstrapConversationCompleteInput{})
	hostErr = nil
	require.ErrorAs(t, err, &hostErr)
	require.ErrorIs(t, err, ErrHostSigningPayloadUnsupported)
	require.Equal(t, "HOST_REGISTRATION_ID_REQUIRED", hostErr.Code)

	_, err = service.PrepareBootstrapFinalize(context.Background(), BootstrapFinalizePreflightInput{
		RegistrationID: "reg_123",
	})
	hostErr = nil
	require.ErrorAs(t, err, &hostErr)
	require.ErrorIs(t, err, ErrHostSigningPayloadUnsupported)
	require.Equal(t, "HOST_CONVERSATION_ID_REQUIRED", hostErr.Code)

	_, err = service.FinalizeBootstrap(context.Background(), BootstrapFinalizeInput{
		RegistrationID:  "reg_123",
		ConversationID:  "conv_123",
		ExpectedVersion: 0,
		SelfAttestation: "0xself",
	})
	hostErr = nil
	require.ErrorAs(t, err, &hostErr)
	require.ErrorIs(t, err, ErrHostSigningPayloadUnsupported)
	require.Equal(t, "HOST_INVALID_REQUEST", hostErr.Code)

	_, err = service.FinalizeBootstrap(context.Background(), BootstrapFinalizeInput{
		RegistrationID:  "reg_123",
		ConversationID:  "conv_123",
		IssuedAt:        time.Date(2026, 6, 12, 22, 0, 0, 0, time.UTC),
		ExpectedVersion: -1,
		SelfAttestation: "0xself",
	})
	hostErr = nil
	require.ErrorAs(t, err, &hostErr)
	require.ErrorIs(t, err, ErrHostSigningPayloadUnsupported)
	require.Equal(t, "HOST_INVALID_REQUEST", hostErr.Code)

	_, err = service.FinalizeBootstrap(context.Background(), BootstrapFinalizeInput{
		RegistrationID:  "reg_123",
		ConversationID:  "conv_123",
		IssuedAt:        time.Date(2026, 6, 12, 22, 0, 0, 0, time.UTC),
		ExpectedVersion: 0,
	})
	hostErr = nil
	require.ErrorAs(t, err, &hostErr)
	require.ErrorIs(t, err, ErrHostSigningPayloadUnsupported)
	require.Equal(t, "HOST_INVALID_REQUEST", hostErr.Code)
}

func TestBootstrapHelperBranches(t *testing.T) {
	t.Parallel()

	require.Equal(t, "", (*HostBootstrapError)(nil).Error())
	require.Equal(t, "HOST_UNAVAILABLE", (&HostBootstrapError{Code: "HOST_UNAVAILABLE"}).Error())
	require.Equal(t, "host unavailable", (&HostBootstrapError{Err: ErrHostUnavailable}).Error())
	require.Equal(t, "host bootstrap error", (&HostBootstrapError{}).Error())
	require.Nil(t, (*HostBootstrapError)(nil).Unwrap())
	require.ErrorIs(t, (&HostBootstrapError{Err: ErrHostUnavailable}).Unwrap(), ErrHostUnavailable)

	for status, wantCode := range map[int]string{
		http.StatusBadRequest:          "HOST_INVALID_REQUEST",
		http.StatusUnauthorized:        "HOST_INSTANCE_TRUST_REJECTED",
		http.StatusForbidden:           "HOST_INSTANCE_TRUST_REJECTED",
		http.StatusNotFound:            "HOST_REGISTRATION_NOT_FOUND",
		http.StatusConflict:            "HOST_BOOTSTRAP_CONFLICT",
		http.StatusTooManyRequests:     "HOST_RATE_LIMITED",
		http.StatusInternalServerError: "HOST_UNAVAILABLE",
	} {
		code, message := mapHostBootstrapStatus(status)
		require.Equal(t, wantCode, code)
		require.NotEmpty(t, message)
	}

	headers := http.Header{}
	headers.Set("Request-ID", "req-header")
	require.Equal(t, "req-header", requestIDFromHeaders(headers))
	require.Empty(t, requestIDFromHeaders(nil))

	err := hostBootstrapHTTPError(http.StatusBadRequest, headers, []byte(`{"error":{"request_id":"req-body"}}`), false)
	var hostErr *HostBootstrapError
	require.ErrorAs(t, err, &hostErr)
	require.Equal(t, "req-body", hostErr.HostRequestID)
	require.Equal(t, "HOST_INVALID_REQUEST", hostErr.Code)

	require.NoError(t, validateHostBootstrapBaseURL("https://host.example"))
	require.NoError(t, validateHostBootstrapBaseURL("http://127.0.0.1:8080"))
	require.Error(t, validateHostBootstrapBaseURL("ftp://host.example"))
	require.Error(t, validateHostBootstrapBaseURL("https:///missing-host"))
	require.Error(t, validateHostBootstrapBaseURL("://bad"))

	built, err := hostBootstrapURL("https://host.example/", "/path")
	require.NoError(t, err)
	require.Equal(t, "https://host.example/path", built)
	_, err = hostBootstrapURL("", "/path")
	require.Error(t, err)
	_, err = hostBootstrapURL("https://host.example", "missing-leading-slash")
	require.Error(t, err)

	require.Nil(t, parseHostTimePtr(""))
	require.Nil(t, parseHostTimePtr("not-a-time"))
	require.NotNil(t, parseHostTimePtr("2026-06-12T20:30:00.123Z"))
	require.Equal(t, " first ", firstNonEmpty("", " first ", "second"))
	require.Empty(t, firstNonEmpty("", " "))
	require.Nil(t, normalizeBootstrapCapabilities(nil))
	require.Equal(t, []string{"A", "b"}, normalizeBootstrapCapabilities([]string{" A ", "b", "a", ""}))
	require.Equal(t, map[string]string{"b1": "0xsig"}, normalizeBootstrapSignatureMap(map[string]string{" b1 ": " 0xsig ", " ": "ignored"}))

	registrationID, conversationID, err := requireBootstrapRegistrationConversation(" reg_123 ", " conv_123 ")
	require.NoError(t, err)
	require.Equal(t, "reg_123", registrationID)
	require.Equal(t, "conv_123", conversationID)
	_, _, err = requireBootstrapRegistrationConversation("reg_123", "")
	require.ErrorIs(t, err, ErrHostSigningPayloadUnsupported)

	require.Equal(t, `{"a":1}`, compactHostJSON(json.RawMessage(` { "a" : 1 } `)))
	require.Empty(t, compactHostJSON(nil))
	require.Equal(t, "{bad", compactHostJSON(json.RawMessage(` {bad `)))

	require.NoError(t, validateHostPrincipalSigningPayload(hostPrincipalPreflightResponse{
		Version:         "1",
		SigningMethod:   "eip191_personal_sign",
		MessageEncoding: "hex_bytes",
		MessageHex:      "0xabc",
		DigestHex:       "0xabc",
	}))
	for _, payload := range []hostPrincipalPreflightResponse{
		{Version: "2", SigningMethod: "eip191_personal_sign", MessageEncoding: "hex_bytes", MessageHex: "0xabc", DigestHex: "0xabc"},
		{Version: "1", SigningMethod: "unknown", MessageEncoding: "hex_bytes", MessageHex: "0xabc", DigestHex: "0xabc"},
		{Version: "1", SigningMethod: "eip191_personal_sign", MessageEncoding: "utf8", MessageHex: "0xabc", DigestHex: "0xabc"},
		{Version: "1", SigningMethod: "eip191_personal_sign", MessageEncoding: "hex_bytes"},
	} {
		err := validateHostPrincipalSigningPayload(payload)
		require.ErrorIs(t, err, ErrHostSigningPayloadUnsupported)
	}

	validFinalize := hostFinalizePreflightResponse{
		Version:         "1",
		ExpectedVersion: 0,
		NextVersion:     1,
		SelfAttestationSigning: hostFinalizeSigningInput{
			SigningMethod:   "eip191_personal_sign",
			MessageEncoding: "hex_bytes",
			MessageHex:      "0xabc",
			DigestHex:       "0xabc",
		},
		BoundaryRequirementsRaw: json.RawMessage(`[{"boundary_id":"b1","signing_method":"eip191_personal_sign","message_encoding":"utf8","message":"Sign","digest_hex":"0xabc"}]`),
	}
	require.NoError(t, validateHostFinalizePreflightPayload(validFinalize))
	badFinalize := validFinalize
	badFinalize.Version = "2"
	require.ErrorIs(t, validateHostFinalizePreflightPayload(badFinalize), ErrHostSigningPayloadUnsupported)
	badFinalize = validFinalize
	badFinalize.SelfAttestationSigning.SigningMethod = "unknown"
	require.ErrorIs(t, validateHostFinalizePreflightPayload(badFinalize), ErrHostSigningPayloadUnsupported)
	badFinalize = validFinalize
	badFinalize.SelfAttestationSigning.MessageEncoding = "utf8"
	require.ErrorIs(t, validateHostFinalizePreflightPayload(badFinalize), ErrHostSigningPayloadUnsupported)
	badFinalize = validFinalize
	badFinalize.SelfAttestationSigning.MessageHex = ""
	require.ErrorIs(t, validateHostFinalizePreflightPayload(badFinalize), ErrHostSigningPayloadUnsupported)
	badFinalize = validFinalize
	badFinalize.ExpectedVersion = -1
	require.ErrorIs(t, validateHostFinalizePreflightPayload(badFinalize), ErrHostSigningPayloadUnsupported)
	badFinalize = validFinalize
	badFinalize.NextVersion = 0
	require.ErrorIs(t, validateHostFinalizePreflightPayload(badFinalize), ErrHostSigningPayloadUnsupported)
	badFinalize = validFinalize
	badFinalize.BoundaryRequirementsRaw = json.RawMessage(`{bad`)
	require.ErrorIs(t, validateHostFinalizePreflightPayload(badFinalize), ErrHostSigningPayloadUnsupported)
	badFinalize = validFinalize
	badFinalize.BoundaryRequirementsRaw = json.RawMessage(`[{"boundary_id":"b1","signing_method":"unknown","message_encoding":"utf8","message":"Sign","digest_hex":"0xabc"}]`)
	require.ErrorIs(t, validateHostFinalizePreflightPayload(badFinalize), ErrHostSigningPayloadUnsupported)
	badFinalize = validFinalize
	badFinalize.BoundaryRequirementsRaw = json.RawMessage(`[{"boundary_id":"b1","signing_method":"eip191_personal_sign","message_encoding":"hex_bytes","message":"Sign","digest_hex":"0xabc"}]`)
	require.ErrorIs(t, validateHostFinalizePreflightPayload(badFinalize), ErrHostSigningPayloadUnsupported)
	badFinalize = validFinalize
	badFinalize.BoundaryRequirementsRaw = json.RawMessage(`[{"boundary_id":"b1","signing_method":"eip191_personal_sign","message_encoding":"utf8","message":"","digest_hex":"0xabc"}]`)
	require.ErrorIs(t, validateHostFinalizePreflightPayload(badFinalize), ErrHostSigningPayloadUnsupported)

	validFinalizeResponse := hostFinalizeResponse{
		Version:          "1",
		AgentID:          "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		PublishedVersion: 1,
	}
	require.NoError(t, validateHostFinalizeResponse(validFinalizeResponse))
	badFinalizeResponse := validFinalizeResponse
	badFinalizeResponse.Version = "2"
	require.ErrorIs(t, validateHostFinalizeResponse(badFinalizeResponse), ErrHostUnavailable)
	badFinalizeResponse = validFinalizeResponse
	badFinalizeResponse.AgentID = "not-an-agent-id"
	require.ErrorIs(t, validateHostFinalizeResponse(badFinalizeResponse), ErrHostUnavailable)
	badFinalizeResponse = validFinalizeResponse
	badFinalizeResponse.PublishedVersion = 0
	require.ErrorIs(t, validateHostFinalizeResponse(badFinalizeResponse), ErrHostUnavailable)
}

func TestService_BootstrapInvalidHostResponseIsTyped(t *testing.T) {
	t.Parallel()

	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-ID", "host-req-invalid-json")
		_, _ = w.Write([]byte(`{"registration":`))
	}))
	defer host.Close()

	service := NewService(
		&fakeAccountRepo{},
		&fakeInstanceRepo{trust: &storageModels.EffectiveTrustConfig{TrustBaseURL: host.URL}},
		&config.Config{Domain: "example.com", LesserHostInstanceKey: "instance-key"},
		zap.NewNop(),
	).WithHTTPClient(host.Client())

	_, err := service.VerifyBootstrapPrincipalDeclaration(context.Background(), BootstrapPrincipalVerifyInput{
		RegistrationID: "reg_123",
		DeclaredAt:     time.Date(2026, 6, 12, 21, 0, 0, 0, time.UTC),
	})
	var hostErr *HostBootstrapError
	require.ErrorAs(t, err, &hostErr)
	require.ErrorIs(t, err, ErrHostUnavailable)
	require.Equal(t, "HOST_RESPONSE_INVALID", hostErr.Code)
	require.Equal(t, "host-req-invalid-json", hostErr.HostRequestID)
}

// TestP52_SendBootstrapConversation202AcceptedPendingNotParsedAsSnapshot proves
// L3.1 G12: a 202 Accepted-Pending response is never parsed as a full
// conversation snapshot, and G13: it never surfaces inline assistant messages.
// Host may carry transcript fields in the 202 body; Lesser must ignore them for
// a pending accept and project MessageCount=0 / Messages=nil while still
// persisting the conversation id early.
func TestP52_SendBootstrapConversation202AcceptedPendingNotParsedAsSnapshot(t *testing.T) {
	t.Parallel()

	const (
		instanceKey    = "host-instance-key"
		registrationID = "reg_p52_accept"
		conversationID = "conv_p52_accept"
		agentID        = "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	)

	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-Id", "host-req-accept-header")
		w.WriteHeader(http.StatusAccepted)
		// Host nests ids under a durable conversation envelope and — crucially
		// — includes transcript fields Lesser must NOT project from a 202.
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"version":    "1",
			"request_id": "host-req-accept-body",
			"conversation": map[string]any{
				"registration_id": registrationID,
				"conversation_id": conversationID,
				"agent_id":        agentID,
				"status":          "in_progress",
				"latest_turn_id":  "turn_should_not_project",
				"message_count":   3,
				"messages": []map[string]any{
					{"role": "assistant", "content": "assistant message that must NOT be projected from a 202", "order": 1},
					{"role": "user", "content": "user message that must NOT be projected from a 202", "order": 2},
				},
				"produced_declarations": map[string]any{"selfDescription": "must not appear"},
			},
		}))
	}))
	defer host.Close()

	service := NewService(
		&fakeAccountRepo{},
		&fakeInstanceRepo{trust: &storageModels.EffectiveTrustConfig{TrustBaseURL: host.URL}},
		&config.Config{Domain: "example.com", LesserHostInstanceKey: instanceKey},
		zap.NewNop(),
	).WithHTTPClient(host.Client())

	sent, err := service.SendBootstrapConversationMessage(context.Background(), BootstrapConversationMessageInput{
		RegistrationID: registrationID,
		Message:        "start hosted genesis",
	})
	require.NoError(t, err, "202 Accepted-Pending is transport success")
	// G12: 202 is not a snapshot. G13: no inline assistant messages.
	require.Equal(t, "in_progress", sent.Status)
	require.Equal(t, 0, sent.MessageCount, "202 must not project a snapshot message_count")
	require.Empty(t, sent.Messages, "202 must not project inline assistant messages")
	require.Empty(t, sent.LatestTurnID, "202 must not project a latest turn id")
	require.Empty(t, sent.ProducedDeclarations, "202 must not project produced declarations")
	// Ids are extracted best-effort so host_conversation_id persists early.
	require.Equal(t, registrationID, sent.RegistrationID)
	require.Equal(t, conversationID, sent.ConversationID)
	require.Equal(t, agentID, sent.HostSoulAgentID)
	require.Equal(t, "host-req-accept-body", sent.HostRequestID)
}

// TestP52_SendBootstrapConversation202EmptyBodyAcceptedPending proves a 202
// with no body is still a valid accepted-pending: Lesser falls back to the
// caller's existing conversation id and never errors with HOST_RESPONSE_INVALID.
func TestP52_SendBootstrapConversation202EmptyBodyAcceptedPending(t *testing.T) {
	t.Parallel()

	const (
		instanceKey    = "host-instance-key"
		registrationID = "reg_p52_empty"
		conversationID = "conv_p52_existing"
	)

	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-Id", "host-req-empty-202")
		w.WriteHeader(http.StatusAccepted)
	}))
	defer host.Close()

	service := NewService(
		&fakeAccountRepo{},
		&fakeInstanceRepo{trust: &storageModels.EffectiveTrustConfig{TrustBaseURL: host.URL}},
		&config.Config{Domain: "example.com", LesserHostInstanceKey: instanceKey},
		zap.NewNop(),
	).WithHTTPClient(host.Client())

	sent, err := service.SendBootstrapConversationMessage(context.Background(), BootstrapConversationMessageInput{
		RegistrationID: registrationID,
		ConversationID: conversationID,
		Message:        "start hosted genesis",
	})
	require.NoError(t, err)
	require.Equal(t, "in_progress", sent.Status)
	require.Equal(t, 0, sent.MessageCount)
	require.Empty(t, sent.Messages)
	require.Equal(t, registrationID, sent.RegistrationID)
	require.Equal(t, conversationID, sent.ConversationID, "empty 202 body preserves the caller conversation id")
	require.Equal(t, "host-req-empty-202", sent.HostRequestID)
}

// TestP52_SendBootstrapConversation200ParsesFullSnapshot proves the legacy
// synchronous 200 path still parses a complete snapshot with inline messages —
// the 202-not-a-snapshot change is scoped to 202 only.
func TestP52_SendBootstrapConversation200ParsesFullSnapshot(t *testing.T) {
	t.Parallel()

	const (
		instanceKey    = "host-instance-key"
		registrationID = "reg_p52_200"
		conversationID = "conv_p52_200"
		agentID        = "0xcccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	)

	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"version": "1",
			"conversation": map[string]any{
				"registration_id": registrationID,
				"conversation_id": conversationID,
				"agent_id":        agentID,
				"status":          "assistant_turn_ready",
				"latest_turn_id":  "turn_200",
				"message_count":   2,
				"messages": []map[string]any{
					{"role": "user", "content": "hello", "order": 1},
					{"role": "assistant", "content": "ready", "order": 2},
				},
			},
		}))
	}))
	defer host.Close()

	service := NewService(
		&fakeAccountRepo{},
		&fakeInstanceRepo{trust: &storageModels.EffectiveTrustConfig{TrustBaseURL: host.URL}},
		&config.Config{Domain: "example.com", LesserHostInstanceKey: instanceKey},
		zap.NewNop(),
	).WithHTTPClient(host.Client())

	sent, err := service.SendBootstrapConversationMessage(context.Background(), BootstrapConversationMessageInput{
		RegistrationID: registrationID,
		Message:        "start hosted genesis",
	})
	require.NoError(t, err)
	require.Equal(t, "assistant_turn_ready", sent.Status)
	require.Equal(t, 2, sent.MessageCount, "200 full snapshot still carries message_count")
	require.Len(t, sent.Messages, 2, "200 full snapshot still carries inline messages")
	require.Equal(t, conversationID, sent.ConversationID)
}

// TestP52_SendBootstrapConversationAcceptTimeoutRoutesHostUnavailable proves
// L3.1 G11 + L3.2 G15 setup: the send POST is bounded by the short accept
// timeout (not the 10s turn wait). When Host does not acknowledge the turn in
// time, Lesser surfaces HOST_UNAVAILABLE — which L3.2 routes to REFRESH_STATE
// (poll), never to RETRY_SAME_STEP (re-issue the blocking call).
func TestP52_SendBootstrapConversationAcceptTimeoutRoutesHostUnavailable(t *testing.T) {
	t.Parallel()

	const (
		instanceKey    = "host-instance-key"
		registrationID = "reg_p52_timeout"
	)

	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Sleep past the accept timeout. Host never acknowledges the turn.
		time.Sleep(defaultSoulBootstrapAcceptTimeout + 500*time.Millisecond)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer host.Close()

	service := NewService(
		&fakeAccountRepo{},
		&fakeInstanceRepo{trust: &storageModels.EffectiveTrustConfig{TrustBaseURL: host.URL}},
		&config.Config{Domain: "example.com", LesserHostInstanceKey: instanceKey},
		zap.NewNop(),
	).WithHTTPClient(&http.Client{Transport: http.DefaultTransport})

	start := time.Now()
	_, err := service.SendBootstrapConversationMessage(context.Background(), BootstrapConversationMessageInput{
		RegistrationID: registrationID,
		Message:        "start hosted genesis",
	})
	elapsed := time.Since(start)
	require.Error(t, err)
	var hostErr *HostBootstrapError
	require.ErrorAs(t, err, &hostErr)
	require.Equal(t, "HOST_UNAVAILABLE", hostErr.Code)
	// The call must be bounded by the accept timeout, not the 10s turn wait.
	require.Less(t, elapsed, defaultSoulHTTPTimeout, "send must use the short accept timeout, not the 10s turn wait")
}
