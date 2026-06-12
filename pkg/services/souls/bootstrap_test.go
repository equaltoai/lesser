package souls

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

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
				"request_id":  "host-req-body",
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
	require.Equal(t, "HOST_INSTANCE_TRUST_REJECTED", hostErr.Code)
	require.Equal(t, http.StatusForbidden, hostErr.StatusCode)
	require.Equal(t, "host-req-body", hostErr.HostRequestID)
	require.NotContains(t, hostErr.Message, instanceKey)
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
