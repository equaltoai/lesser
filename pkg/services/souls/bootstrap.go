package souls

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
)

const (
	hostBootstrapMaxResponseBytes = 256 * 1024

	hostBootstrapSigningMethodEIP191 = "eip191_personal_sign"
	hostBootstrapEncodingHexBytes    = "hex_bytes"
	hostBootstrapVersion1            = "1"
)

var (
	// ErrHostTrustNotConfigured indicates the effective instance trust base URL is unavailable.
	ErrHostTrustNotConfigured = errors.New("host trust not configured")
	// ErrHostInstanceKeyMissing indicates no server-side Host instance key can be resolved.
	ErrHostInstanceKeyMissing = errors.New("host instance key missing")
	// ErrHostInstanceKeyUnavailable indicates the configured Host instance key secret could not be resolved.
	ErrHostInstanceKeyUnavailable = errors.New("host instance key unavailable")
	// ErrHostUnavailable indicates the Host instance-key API is unreachable or returned an unusable response.
	ErrHostUnavailable = errors.New("host unavailable")
	// ErrHostSigningPayloadUnsupported indicates Host returned signing metadata Lesser does not support.
	ErrHostSigningPayloadUnsupported = errors.New("host signing payload unsupported")
)

// HostBootstrapError describes a bounded, client-safe failure returned while
// talking to Host's instance-key bootstrap API.
type HostBootstrapError struct {
	Code          string
	Message       string
	Source        string
	StatusCode    int
	HostRequestID string
	Err           error
}

func (e *HostBootstrapError) Error() string {
	if e == nil {
		return ""
	}
	if strings.TrimSpace(e.Code) != "" {
		return strings.TrimSpace(e.Code)
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return "host bootstrap error"
}

func (e *HostBootstrapError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// BootstrapBeginInput is the Lesser-local request for Host registration begin.
type BootstrapBeginInput struct {
	// Username is the Lesser-local body handle Host receives as local_id.
	Username      string
	BodyID        string // Lesser-local state/UI identity; never serialized as Host local_id.
	WalletAddress string
	Capabilities  []string
}

// BootstrapBeginResult contains Host begin output needed by the GraphQL state.
type BootstrapBeginResult struct {
	RegistrationID  string
	HostSoulAgentID string
	WalletAddress   string
	WalletChallenge BootstrapWalletChallenge
	HostRequestID   string
}

// BootstrapWalletChallenge contains the Host-issued wallet signing message.
type BootstrapWalletChallenge struct {
	ID        string
	Address   string
	ChainID   int
	Nonce     string
	Message   string
	IssuedAt  *time.Time
	ExpiresAt *time.Time
}

// BootstrapPrincipalPreflightInput is the Lesser-local request for Host
// principal declaration preflight.
type BootstrapPrincipalPreflightInput struct {
	RegistrationID       string
	PrincipalAddress     string
	PrincipalDeclaration string
	DeclaredAt           time.Time
}

// BootstrapPrincipalPreflightResult contains Host-owned signing material.
type BootstrapPrincipalPreflightResult struct {
	Version          string
	PrincipalAddress string
	SignerAddress    string
	SigningMethod    string
	MessageEncoding  string
	MessageHex       string
	DigestHex        string
	CanonicalJSON    string
	DeclaredAt       *time.Time
	HostRequestID    string
}

// BootstrapPrincipalVerifyInput is the Lesser-local request for Host proof
// verification.
type BootstrapPrincipalVerifyInput struct {
	RegistrationID       string
	WalletSignature      string
	PrincipalAddress     string
	PrincipalDeclaration string
	PrincipalSignature   string
	DeclaredAt           time.Time
}

// BootstrapPrincipalVerifyResult contains Host verification output needed by
// the GraphQL state.
type BootstrapPrincipalVerifyResult struct {
	RegistrationID   string
	HostSoulAgentID  string
	WalletAddress    string
	PrincipalAddress string
	OperationID      string
	PromotionStage   string
	HostRequestID    string
}

type hostInstanceKeyResolver func(ctx context.Context, cfg *config.Config, secretARN string) (string, error)

func defaultHostInstanceKeyResolver(_ context.Context, cfg *config.Config, secretARN string) (string, error) {
	secretARN = strings.TrimSpace(secretARN)
	if secretARN != "" {
		return config.ResolveOptionalSecretValue("", secretARN)
	}
	if cfg == nil {
		return "", nil
	}
	return cfg.ResolveLesserHostInstanceKey()
}

// WithHostInstanceKeyResolver overrides instance-key resolution for tests.
func (s *Service) WithHostInstanceKeyResolver(resolver func(context.Context, *config.Config, string) (string, error)) *Service {
	if s == nil || resolver == nil {
		return s
	}
	s.hostInstanceKeyResolver = resolver
	return s
}

// BeginBootstrapRegistration calls Host's instance-key registration begin route.
func (s *Service) BeginBootstrapRegistration(ctx context.Context, input BootstrapBeginInput) (*BootstrapBeginResult, error) {
	baseURL, instanceDomain, instanceKey, err := s.hostBootstrapInputs(ctx)
	if err != nil {
		return nil, err
	}

	payload := map[string]any{
		"domain":         instanceDomain,
		"local_id":       bootstrapHostLocalID(input),
		"wallet_address": strings.TrimSpace(input.WalletAddress),
	}
	if len(input.Capabilities) > 0 {
		payload["capabilities"] = normalizeBootstrapCapabilities(input.Capabilities)
	}

	var out hostRegistrationBeginResponse
	requestID, err := s.doHostBootstrapJSON(ctx, http.MethodPost, baseURL, "/api/v1/soul/instance/agents/register/begin", instanceKey, payload, http.StatusCreated, &out)
	if err != nil {
		return nil, err
	}

	return &BootstrapBeginResult{
		RegistrationID:  strings.TrimSpace(out.Registration.ID),
		HostSoulAgentID: strings.ToLower(strings.TrimSpace(out.Registration.AgentID)),
		WalletAddress:   strings.ToLower(strings.TrimSpace(firstNonEmpty(out.Wallet.Address, out.Registration.WalletAddress))),
		WalletChallenge: BootstrapWalletChallenge{
			ID:        strings.TrimSpace(out.Wallet.ID),
			Address:   strings.ToLower(strings.TrimSpace(out.Wallet.Address)),
			ChainID:   out.Wallet.ChainID,
			Nonce:     strings.TrimSpace(out.Wallet.Nonce),
			Message:   strings.TrimSpace(out.Wallet.Message),
			IssuedAt:  parseHostTimePtr(out.Wallet.IssuedAt),
			ExpiresAt: parseHostTimePtr(out.Wallet.ExpiresAt),
		},
		HostRequestID: requestID,
	}, nil
}

func bootstrapHostLocalID(input BootstrapBeginInput) string {
	return strings.ToLower(strings.TrimSpace(input.Username))
}

// PrepareBootstrapPrincipalDeclaration calls Host's principal declaration
// preflight route and fails closed on unsupported signing metadata.
func (s *Service) PrepareBootstrapPrincipalDeclaration(ctx context.Context, input BootstrapPrincipalPreflightInput) (*BootstrapPrincipalPreflightResult, error) {
	baseURL, _, instanceKey, err := s.hostBootstrapInputs(ctx)
	if err != nil {
		return nil, err
	}

	registrationID := strings.TrimSpace(input.RegistrationID)
	if registrationID == "" {
		return nil, &HostBootstrapError{Code: "HOST_REGISTRATION_ID_REQUIRED", Message: "registration id is required", Source: "lesser", Err: ErrHostSigningPayloadUnsupported}
	}

	payload := map[string]any{
		"principal_address":     strings.TrimSpace(input.PrincipalAddress),
		"principal_declaration": input.PrincipalDeclaration,
		"declared_at":           input.DeclaredAt.UTC().Format(time.RFC3339),
	}

	var out hostPrincipalPreflightResponse
	requestID, err := s.doHostBootstrapJSON(ctx, http.MethodPost, baseURL, "/api/v1/soul/instance/agents/register/"+url.PathEscape(registrationID)+"/principal-declaration/preflight", instanceKey, payload, http.StatusOK, &out)
	if err != nil {
		return nil, err
	}
	if err := validateHostPrincipalSigningPayload(out); err != nil {
		return nil, err
	}

	return &BootstrapPrincipalPreflightResult{
		Version:          out.Version,
		PrincipalAddress: out.PrincipalAddress,
		SignerAddress:    out.SignerAddress,
		SigningMethod:    out.SigningMethod,
		MessageEncoding:  out.MessageEncoding,
		MessageHex:       out.MessageHex,
		DigestHex:        out.DigestHex,
		CanonicalJSON:    out.CanonicalJSON,
		DeclaredAt:       parseHostTimePtr(out.DeclaredAt),
		HostRequestID:    requestID,
	}, nil
}

// VerifyBootstrapPrincipalDeclaration calls Host's combined wallet/proof/
// principal declaration verification route.
func (s *Service) VerifyBootstrapPrincipalDeclaration(ctx context.Context, input BootstrapPrincipalVerifyInput) (*BootstrapPrincipalVerifyResult, error) {
	baseURL, _, instanceKey, err := s.hostBootstrapInputs(ctx)
	if err != nil {
		return nil, err
	}

	registrationID := strings.TrimSpace(input.RegistrationID)
	if registrationID == "" {
		return nil, &HostBootstrapError{Code: "HOST_REGISTRATION_ID_REQUIRED", Message: "registration id is required", Source: "lesser", Err: ErrHostSigningPayloadUnsupported}
	}

	payload := map[string]any{
		"signature":             strings.TrimSpace(input.WalletSignature),
		"principal_address":     strings.TrimSpace(input.PrincipalAddress),
		"principal_declaration": input.PrincipalDeclaration,
		"principal_signature":   strings.TrimSpace(input.PrincipalSignature),
		"declared_at":           input.DeclaredAt.UTC().Format(time.RFC3339),
	}

	var out hostRegistrationVerifyResponse
	requestID, err := s.doHostBootstrapJSON(ctx, http.MethodPost, baseURL, "/api/v1/soul/instance/agents/register/"+url.PathEscape(registrationID)+"/verify", instanceKey, payload, http.StatusOK, &out)
	if err != nil {
		return nil, err
	}

	return &BootstrapPrincipalVerifyResult{
		RegistrationID:   strings.TrimSpace(out.Registration.ID),
		HostSoulAgentID:  strings.ToLower(strings.TrimSpace(firstNonEmpty(out.Registration.AgentID, out.Operation.AgentID))),
		WalletAddress:    strings.ToLower(strings.TrimSpace(out.Registration.WalletAddress)),
		PrincipalAddress: strings.ToLower(strings.TrimSpace(firstNonEmpty(out.Promotion.PrincipalAddress, input.PrincipalAddress))),
		OperationID:      strings.TrimSpace(out.Operation.OperationID),
		PromotionStage:   strings.TrimSpace(out.Promotion.Stage),
		HostRequestID:    requestID,
	}, nil
}

func (s *Service) hostBootstrapInputs(ctx context.Context) (string, string, string, error) {
	if s == nil || s.instanceRepo == nil || s.cfg == nil {
		return "", "", "", fmt.Errorf("soul service misconfigured")
	}

	effectiveTrust, err := s.instanceRepo.EffectiveTrustConfig(ctx)
	if err != nil {
		return "", "", "", err
	}
	if effectiveTrust == nil || strings.TrimSpace(effectiveTrust.TrustBaseURL) == "" {
		return "", "", "", &HostBootstrapError{Code: "HOST_TRUST_NOT_CONFIGURED", Message: "Host instance trust base URL is not configured.", Source: "lesser", Err: ErrHostTrustNotConfigured}
	}
	baseURL := strings.TrimRight(strings.TrimSpace(effectiveTrust.TrustBaseURL), "/")
	if err := validateHostBootstrapBaseURL(baseURL); err != nil {
		return "", "", "", &HostBootstrapError{Code: "HOST_TRUST_NOT_CONFIGURED", Message: "Host instance trust base URL is invalid.", Source: "lesser", Err: ErrHostTrustNotConfigured}
	}

	instanceDomain := strings.ToLower(strings.TrimSpace(s.cfg.Domain))
	if instanceDomain == "" {
		return "", "", "", fmt.Errorf("instance domain is required")
	}

	resolver := s.hostInstanceKeyResolver
	if resolver == nil {
		resolver = defaultHostInstanceKeyResolver
	}
	instanceKey, err := resolver(ctx, s.cfg, strings.TrimSpace(effectiveTrust.InstanceKeySecretARN))
	if err != nil {
		return "", "", "", &HostBootstrapError{Code: "HOST_INSTANCE_KEY_UNAVAILABLE", Message: "Host instance key could not be resolved.", Source: "lesser", Err: fmt.Errorf("%w: %v", ErrHostInstanceKeyUnavailable, err)}
	}
	if strings.TrimSpace(instanceKey) == "" {
		return "", "", "", &HostBootstrapError{Code: "HOST_INSTANCE_KEY_MISSING", Message: "Host instance key is not configured.", Source: "lesser", Err: ErrHostInstanceKeyMissing}
	}

	return baseURL, instanceDomain, strings.TrimSpace(instanceKey), nil
}

func (s *Service) doHostBootstrapJSON(ctx context.Context, method string, baseURL string, path string, instanceKey string, payload any, expectedStatus int, out any) (string, error) {
	endpoint, err := hostBootstrapURL(baseURL, path)
	if err != nil {
		return "", &HostBootstrapError{Code: "HOST_UNAVAILABLE", Message: "Host bootstrap endpoint is unavailable.", Source: "host", Err: fmt.Errorf("%w: %v", ErrHostUnavailable, err)}
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", &HostBootstrapError{Code: "HOST_UNAVAILABLE", Message: "Host bootstrap endpoint is unavailable.", Source: "host", Err: fmt.Errorf("%w: %v", ErrHostUnavailable, err)}
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+instanceKey)

	client := s.httpClient
	if client == nil {
		client = &http.Client{Timeout: defaultSoulHTTPTimeout}
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", &HostBootstrapError{Code: "HOST_UNAVAILABLE", Message: "Host bootstrap endpoint is unavailable.", Source: "host", Err: fmt.Errorf("%w: %v", ErrHostUnavailable, err)}
	}
	defer resp.Body.Close()

	responseBody, truncated, err := common.ReadUntrustedHTTPResponseBody(resp.Body, hostBootstrapMaxResponseBytes)
	if err != nil {
		return requestIDFromHeaders(resp.Header), &HostBootstrapError{Code: "HOST_UNAVAILABLE", Message: "Host bootstrap response is unavailable.", Source: "host", Err: fmt.Errorf("%w: %v", ErrHostUnavailable, err)}
	}
	requestID := requestIDFromHeaders(resp.Header)

	if resp.StatusCode != expectedStatus {
		return requestID, hostBootstrapHTTPError(resp.StatusCode, resp.Header, responseBody, truncated)
	}
	if truncated {
		return requestID, &HostBootstrapError{Code: "HOST_UNAVAILABLE", Message: "Host bootstrap response is too large.", Source: "host", StatusCode: resp.StatusCode, HostRequestID: requestID, Err: ErrHostUnavailable}
	}
	if out != nil {
		if err := json.Unmarshal(responseBody, out); err != nil {
			return requestID, &HostBootstrapError{Code: "HOST_RESPONSE_INVALID", Message: "Host bootstrap response is invalid.", Source: "host", StatusCode: resp.StatusCode, HostRequestID: requestID, Err: fmt.Errorf("%w: %v", ErrHostUnavailable, err)}
		}
	}

	return requestID, nil
}

func hostBootstrapHTTPError(status int, headers http.Header, body []byte, truncated bool) error {
	requestID := requestIDFromHeaders(headers)
	envelope := hostBootstrapErrorEnvelope{}
	if !truncated && len(body) > 0 {
		_ = json.Unmarshal(body, &envelope)
	}
	if envelope.Error.RequestID != "" {
		requestID = strings.TrimSpace(envelope.Error.RequestID)
	}

	code, message := mapHostBootstrapStatus(status)
	return &HostBootstrapError{
		Code:          code,
		Message:       message,
		Source:        "host",
		StatusCode:    status,
		HostRequestID: requestID,
		Err:           ErrHostUnavailable,
	}
}

func mapHostBootstrapStatus(status int) (string, string) {
	switch status {
	case http.StatusBadRequest:
		return "HOST_INVALID_REQUEST", "Host rejected the bootstrap request."
	case http.StatusUnauthorized, http.StatusForbidden:
		return "HOST_INSTANCE_TRUST_REJECTED", "Host rejected the instance trust credential."
	case http.StatusNotFound:
		return "HOST_REGISTRATION_NOT_FOUND", "Host registration was not found."
	case http.StatusConflict:
		return "HOST_BOOTSTRAP_CONFLICT", "Host reported a bootstrap conflict."
	case http.StatusTooManyRequests:
		return "HOST_RATE_LIMITED", "Host bootstrap request was rate limited."
	default:
		return "HOST_UNAVAILABLE", "Host bootstrap endpoint is unavailable."
	}
}

func validateHostPrincipalSigningPayload(out hostPrincipalPreflightResponse) error {
	if strings.TrimSpace(out.Version) != hostBootstrapVersion1 {
		return &HostBootstrapError{Code: "HOST_SIGNING_PAYLOAD_UNSUPPORTED", Message: "Host returned an unsupported signing payload version.", Source: "lesser", Err: ErrHostSigningPayloadUnsupported}
	}
	if strings.TrimSpace(out.SigningMethod) != hostBootstrapSigningMethodEIP191 {
		return &HostBootstrapError{Code: "HOST_SIGNING_PAYLOAD_UNSUPPORTED", Message: "Host returned an unsupported signing method.", Source: "lesser", Err: ErrHostSigningPayloadUnsupported}
	}
	if strings.TrimSpace(out.MessageEncoding) != hostBootstrapEncodingHexBytes {
		return &HostBootstrapError{Code: "HOST_SIGNING_PAYLOAD_UNSUPPORTED", Message: "Host returned an unsupported message encoding.", Source: "lesser", Err: ErrHostSigningPayloadUnsupported}
	}
	if strings.TrimSpace(out.MessageHex) == "" || strings.TrimSpace(out.DigestHex) == "" {
		return &HostBootstrapError{Code: "HOST_SIGNING_PAYLOAD_UNSUPPORTED", Message: "Host returned incomplete signing material.", Source: "lesser", Err: ErrHostSigningPayloadUnsupported}
	}
	return nil
}

func hostBootstrapURL(baseURL string, path string) (string, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	path = strings.TrimSpace(path)
	if baseURL == "" || !strings.HasPrefix(path, "/") {
		return "", errors.New("invalid host bootstrap URL")
	}
	return baseURL + path, nil
}

func validateHostBootstrapBaseURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return err
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return errors.New("base URL must include http(s) scheme")
	}
	if strings.TrimSpace(parsed.Hostname()) == "" {
		return errors.New("base URL missing hostname")
	}
	return nil
}

func requestIDFromHeaders(headers http.Header) string {
	if headers == nil {
		return ""
	}
	for _, key := range []string{"X-Request-Id", "X-Request-ID", "Request-Id", "Request-ID"} {
		if v := strings.TrimSpace(headers.Get(key)); v != "" {
			return v
		}
	}
	return ""
}

func normalizeBootstrapCapabilities(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func parseHostTimePtr(value string) *time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		parsed, err = time.Parse(time.RFC3339, value)
		if err != nil {
			return nil
		}
	}
	parsed = parsed.UTC()
	return &parsed
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

type hostRegistrationBeginResponse struct {
	Registration hostRegistration      `json:"registration"`
	Wallet       hostWalletChallenge   `json:"wallet"`
	Promotion    hostPromotionResponse `json:"promotion,omitempty"`
}

type hostRegistrationVerifyResponse struct {
	Registration hostRegistration      `json:"registration"`
	Operation    hostOperation         `json:"operation"`
	Promotion    hostPromotionResponse `json:"promotion,omitempty"`
}

type hostRegistration struct {
	ID            string `json:"id"`
	AgentID       string `json:"agent_id"`
	WalletAddress string `json:"wallet_address"`
	Status        string `json:"status"`
}

type hostWalletChallenge struct {
	ID        string `json:"id"`
	Address   string `json:"address"`
	ChainID   int    `json:"chainId"`
	Nonce     string `json:"nonce"`
	Message   string `json:"message"`
	IssuedAt  string `json:"issuedAt"`
	ExpiresAt string `json:"expiresAt"`
}

type hostPrincipalPreflightResponse struct {
	Version          string `json:"version"`
	PrincipalAddress string `json:"principal_address"`
	SignerAddress    string `json:"signer_address"`
	SigningMethod    string `json:"signing_method"`
	MessageEncoding  string `json:"message_encoding"`
	MessageHex       string `json:"message_hex"`
	DigestHex        string `json:"digest_hex"`
	CanonicalJSON    string `json:"canonical_json"`
	DeclaredAt       string `json:"declared_at"`
}

type hostOperation struct {
	OperationID string `json:"operation_id"`
	AgentID     string `json:"agent_id"`
	Status      string `json:"status"`
}

type hostPromotionResponse struct {
	AgentID          string `json:"agent_id"`
	RegistrationID   string `json:"registration_id"`
	Stage            string `json:"stage"`
	PrincipalAddress string `json:"principal_address"`
}

type hostBootstrapErrorEnvelope struct {
	Error struct {
		Code       string `json:"code"`
		Message    string `json:"message"`
		StatusCode int    `json:"status_code"`
		RequestID  string `json:"request_id"`
	} `json:"error"`
}
