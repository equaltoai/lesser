package souls

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
)

const (
	hostBootstrapMaxResponseBytes    = 256 * 1024
	hostBootstrapSigningMethodEIP191 = "eip191_personal_sign"
	hostBootstrapEncodingHexBytes    = "hex_bytes"
	hostBootstrapVersion1            = "1"

	hostConversationStatusDeclarationReady = "declaration_ready"
	hostConversationMessageRoleUser        = "user"
	hostConversationMessageRoleAssistant   = "assistant"
	hostedGenesisConversationListMaxResults = 50

	// SoulAuthorityModelWalletPrincipal is Host's wallet/principal authority model.
	SoulAuthorityModelWalletPrincipal = "wallet_principal"
	// SoulAuthorityModelInstanceTrust is Host's managed instance-key authority model.
	SoulAuthorityModelInstanceTrust = "instance_trust"
	// SoulAnchorStateHostedOffchain is Host's hosted/off-chain anchor state.
	SoulAnchorStateHostedOffchain = "hosted_offchain"
	// SoulAnchorStateImmutableOnchain is Host's on-chain anchor state.
	SoulAnchorStateImmutableOnchain = "immutable_onchain"
	// SoulOperationalBindingHostedBound is Host's hosted-bound operational binding marker.
	SoulOperationalBindingHostedBound = "hosted_bound_soul"
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
	DetailsJSON   string
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
	RegistrationID     string
	HostSoulAgentID    string
	WalletAddress      string
	AuthorityModel     string
	AnchorState        string
	RegistrationStatus string
	WalletChallenge    BootstrapWalletChallenge
	HostRequestID      string
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

// BootstrapConversationMessageInput is the Lesser-local request for sending a
// Host mint-conversation turn through the server-side instance-key route.
type BootstrapConversationMessageInput struct {
	RegistrationID string
	ConversationID string
	Message        string
	Model          string
}

// BootstrapConversationMessageResult contains the Host conversation ids and
// durable conversation status returned by Host's JSON instance-key route.
type BootstrapConversationMessageResult struct {
	RegistrationID        string
	HostSoulAgentID       string
	ConversationID        string
	Status                string
	Model                 string
	LatestTurnID          string
	MessageCount          int
	Messages              []BootstrapConversationMessage
	MessagesTruncated     bool
	FullResponse          string
	ProducedDeclarations  string
	UpdatedAt             *time.Time
	CompletedAt           *time.Time
	HostRequestID         string
	FailureCode           string
	FailureMessage        string
	FailureRetryable      bool
	FailureRecoveryAction string
}

// BootstrapConversationMessage is Host's bounded, client-safe hosted genesis
// transcript turn projection. It intentionally carries no Host credentials,
// provider secrets, signing material, or infrastructure state.
type BootstrapConversationMessage struct {
	ID        string
	Role      string
	Content   string
	Order     int
	CreatedAt *time.Time
	Truncated bool
}

// HostedGenesisConversationSummary is a bounded summary of one Host mint
// conversation for the genesis conversation list query. It carries no
// transcript messages, signing material, or Host credentials.
type HostedGenesisConversationSummary struct {
	ConversationID  string
	RegistrationID  string
	Status          string
	MessageCount    int
	LatestTurnID    string
	CreatedAt       *time.Time
	UpdatedAt       *time.Time
}

// BootstrapConversationCompleteInput is the Lesser-local request for completing
// a Host mint conversation.
type BootstrapConversationCompleteInput struct {
	RegistrationID string
	ConversationID string
}

// BootstrapConversationRecoverInput is the Lesser-local request for recovering a
// stuck Host mint conversation turn without sending a new user message.
type BootstrapConversationRecoverInput struct {
	RegistrationID  string
	ConversationID  string
	CorrelationID   string
	IdempotencyKey  string
	LesserRequestID string
}

// BootstrapConversationCompleteResult contains Host completion state.
type BootstrapConversationCompleteResult struct {
	RegistrationID        string
	HostSoulAgentID       string
	ConversationID        string
	Status                string
	LatestTurnID          string
	MessageCount          int
	Messages              []BootstrapConversationMessage
	MessagesTruncated     bool
	ProducedDeclarations  string
	UpdatedAt             *time.Time
	CompletedAt           *time.Time
	HostRequestID         string
	FailureCode           string
	FailureMessage        string
	FailureRetryable      bool
	FailureRecoveryAction string
}

// BootstrapFinalizePreflightInput is the Lesser-local request for Host finalize
// preflight/signing material.
type BootstrapFinalizePreflightInput struct {
	RegistrationID     string
	ConversationID     string
	BoundarySignatures map[string]string
}

// BootstrapFinalizeSigningInput contains the Host-owned self-attestation
// signing payload.
type BootstrapFinalizeSigningInput struct {
	SignerWallet    string
	SigningMethod   string
	MessageEncoding string
	MessageHex      string
	DigestHex       string
	CanonicalJSON   string
}

// BootstrapFinalizePreflightResult contains Host-owned finalize signing
// material. Lesser relays these fields unchanged; it never reconstructs the
// signing payload locally.
type BootstrapFinalizePreflightResult struct {
	Version                     string
	DigestHex                   string
	IssuedAt                    *time.Time
	ExpectedVersion             int
	NextVersion                 int
	SelfAttestationSigning      BootstrapFinalizeSigningInput
	BoundaryRequirementsJSON    string
	FinalizeRequestTemplateJSON string
	RegistrationPreviewJSON     string
	HostRequestID               string
}

// BootstrapFinalizeInput is the Lesser-local request for Host hosted/off-chain
// finalize and publication.
type BootstrapFinalizeInput struct {
	RegistrationID     string
	ConversationID     string
	BoundarySignatures map[string]string
	IssuedAt           time.Time
	ExpectedVersion    int
	SelfAttestation    string
}

// HostedBootstrapPublishInput is the hosted-first no-wallet publish request.
type HostedBootstrapPublishInput struct {
	RegistrationID string
	ConversationID string
	LocalID        string
}

// BootstrapPublicationEvidence contains Host publication evidence for the
// versioned hosted/off-chain registration artifact.
type BootstrapPublicationEvidence struct {
	AgentID                    string
	PublishedVersion           int
	AuthorityModel             string
	RegistrationURI            string
	RegistrationS3Key          string
	VersionedRegistrationURI   string
	VersionedRegistrationS3Key string
	AnchorState                string
	PublishedAt                *time.Time
}

// BootstrapPromotionEvidence contains Host promotion continuity fields.
type BootstrapPromotionEvidence struct {
	AgentID                  string
	RegistrationID           string
	Stage                    string
	RequestStatus            string
	ReviewStatus             string
	ReadinessStatus          string
	AuthorityModel           string
	AnchorState              string
	LatestConversationID     string
	LatestConversationStatus string
	PublishedVersion         int
	GraduatedAt              *time.Time
}

// BootstrapFinalizeResult contains Host finalize/publication output needed for
// Lesser local soul binding and workflow projection.
type BootstrapFinalizeResult struct {
	Version                 string
	HostSoulAgentID         string
	PublishedVersion        int
	AgentDomain             string
	AgentLocalID            string
	AgentAuthorityModel     string
	AgentAnchorState        string
	AgentOperationalBinding string
	AgentStatus             string
	AgentLifecycleStatus    string
	PrincipalAddress        string
	Publication             BootstrapPublicationEvidence
	Promotion               BootstrapPromotionEvidence
	HostRequestID           string
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
		"domain":          instanceDomain,
		"local_id":        bootstrapHostLocalID(input),
		"wallet_address":  strings.TrimSpace(input.WalletAddress),
		"authority_model": SoulAuthorityModelWalletPrincipal,
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
		RegistrationID:     strings.TrimSpace(out.Registration.ID),
		HostSoulAgentID:    strings.ToLower(strings.TrimSpace(out.Registration.AgentID)),
		WalletAddress:      strings.ToLower(strings.TrimSpace(firstNonEmpty(out.Wallet.Address, out.Registration.WalletAddress))),
		AuthorityModel:     strings.TrimSpace(out.Registration.AuthorityModel),
		AnchorState:        strings.TrimSpace(out.Promotion.AnchorState),
		RegistrationStatus: strings.TrimSpace(out.Registration.Status),
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

// BeginHostedBootstrapRegistration calls Host's M7.1 instance-trust begin route.
func (s *Service) BeginHostedBootstrapRegistration(ctx context.Context, input BootstrapBeginInput) (*BootstrapBeginResult, error) {
	baseURL, instanceDomain, instanceKey, err := s.hostBootstrapInputs(ctx)
	if err != nil {
		return nil, err
	}

	localID := bootstrapHostLocalID(input)
	payload := map[string]any{
		"domain":          instanceDomain,
		"local_id":        localID,
		"authority_model": SoulAuthorityModelInstanceTrust,
	}
	if len(input.Capabilities) > 0 {
		payload["capabilities"] = normalizeBootstrapCapabilities(input.Capabilities)
	}

	var out hostRegistrationBeginResponse
	requestID, err := s.doHostBootstrapJSON(ctx, http.MethodPost, baseURL, "/api/v1/soul/instance/agents/register/begin", instanceKey, payload, http.StatusCreated, &out)
	if err != nil {
		return nil, err
	}
	if err := validateHostedBeginResponse(out, instanceDomain, localID); err != nil {
		return nil, err
	}

	return &BootstrapBeginResult{
		RegistrationID:     strings.TrimSpace(out.Registration.ID),
		HostSoulAgentID:    strings.ToLower(strings.TrimSpace(out.Registration.AgentID)),
		AuthorityModel:     firstNonEmpty(strings.TrimSpace(out.Registration.AuthorityModel), strings.TrimSpace(out.Promotion.AuthorityModel), SoulAuthorityModelInstanceTrust),
		AnchorState:        strings.TrimSpace(out.Promotion.AnchorState),
		RegistrationStatus: strings.TrimSpace(out.Registration.Status),
		HostRequestID:      requestID,
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

// SendBootstrapConversationMessage relays one mint-conversation turn to Host's
// durable JSON instance-key route. HTTP 200/202 is transport success only; the
// returned Host status drives Lesser's local projection.
func (s *Service) SendBootstrapConversationMessage(ctx context.Context, input BootstrapConversationMessageInput) (*BootstrapConversationMessageResult, error) {
	baseURL, _, instanceKey, err := s.hostBootstrapInputs(ctx)
	if err != nil {
		return nil, err
	}

	registrationID := strings.TrimSpace(input.RegistrationID)
	if registrationID == "" {
		return nil, &HostBootstrapError{Code: "HOST_REGISTRATION_ID_REQUIRED", Message: "registration id is required", Source: "lesser", Err: ErrHostSigningPayloadUnsupported}
	}
	if strings.TrimSpace(input.Message) == "" {
		return nil, &HostBootstrapError{Code: "HOST_INVALID_REQUEST", Message: "conversation message is required", Source: "lesser", Err: ErrHostSigningPayloadUnsupported}
	}

	payload := map[string]any{
		"message": strings.TrimSpace(input.Message),
	}
	if conversationID := strings.TrimSpace(input.ConversationID); conversationID != "" {
		payload["conversation_id"] = conversationID
	}
	if model := strings.TrimSpace(input.Model); model != "" {
		payload["model"] = model
	}

	var raw json.RawMessage
	requestID, err := s.doHostBootstrapJSON(ctx, http.MethodPost, baseURL, "/api/v1/soul/instance/agents/register/"+url.PathEscape(registrationID)+"/mint-conversation", instanceKey, payload, http.StatusOK, http.StatusAccepted, &raw)
	if err != nil {
		return nil, err
	}
	out, version, err := parseHostConversationResponseEnvelope(raw, requestID)
	if err != nil {
		return nil, err
	}
	if version != "" && version != hostBootstrapVersion1 {
		return nil, unsupportedHostConversationVersionError(
			"Host conversation response",
			requestID,
			out.RequestID,
		)
	}
	if err := validateHostConversationSnapshot(out, registrationID, false, requestID); err != nil {
		return nil, err
	}
	result := bootstrapConversationMessageResultFromHost(registrationID, out, hostConversationRequestID(requestID, out.RequestID))
	if isHostedBootstrapTerminalDeclarationStatus(result.Status) {
		if err := ValidateHostedBootstrapCompletionEvidence(completeResultFromMessageResult(result), result.ConversationID); err != nil {
			return nil, err
		}
		if compact, err := compactHostedBootstrapProducedDeclarations(result.ProducedDeclarations, result.HostRequestID); err == nil {
			result.ProducedDeclarations = compact
		}
	}
	return result, nil
}

// CompleteBootstrapConversation completes a Host mint conversation and returns
// the Host-owned declaration output checkpoint.
func (s *Service) CompleteBootstrapConversation(ctx context.Context, input BootstrapConversationCompleteInput) (*BootstrapConversationCompleteResult, error) {
	baseURL, _, instanceKey, err := s.hostBootstrapInputs(ctx)
	if err != nil {
		return nil, err
	}
	registrationID, conversationID, err := requireBootstrapRegistrationConversation(input.RegistrationID, input.ConversationID)
	if err != nil {
		return nil, err
	}

	var raw json.RawMessage
	requestID, err := s.doHostBootstrapJSON(ctx, http.MethodPost, baseURL, "/api/v1/soul/instance/agents/register/"+url.PathEscape(registrationID)+"/mint-conversation/"+url.PathEscape(conversationID)+"/complete", instanceKey, map[string]any{}, http.StatusOK, http.StatusAccepted, &raw)
	if err != nil {
		if isHostBootstrapConversationConflict(err) {
			return s.recoverCompletedBootstrapConversation(ctx, baseURL, instanceKey, registrationID, conversationID, err)
		}
		return nil, err
	}
	out, version, err := parseHostConversationResponseEnvelope(raw, requestID)
	if err != nil {
		return nil, err
	}
	if version != "" && version != hostBootstrapVersion1 {
		return nil, unsupportedHostConversationVersionError(
			"Host conversation response",
			requestID,
			out.RequestID,
		)
	}
	if err := validateHostConversationSnapshot(out, registrationID, true, requestID); err != nil {
		return nil, err
	}
	result := bootstrapConversationCompleteResultFromHost(registrationID, out, hostConversationRequestID(requestID, out.RequestID))
	if isHostedBootstrapTerminalDeclarationStatus(result.Status) {
		if err := ValidateHostedBootstrapCompletionEvidence(result, conversationID); err != nil {
			return nil, err
		}
		if compact, err := compactHostedBootstrapProducedDeclarations(result.ProducedDeclarations, result.HostRequestID); err == nil {
			result.ProducedDeclarations = compact
		}
	}
	return result, nil
}

// RecoverHostedGenesisTurn calls Host's POST /recover endpoint to re-run the
// assistant turn for a stuck mint conversation without sending a new user
// message. If the session is not stuck, Host returns the current conversation
// state (idempotent). The response uses the same hostedGenesisConversationResponse
// envelope as the GET conversation read route.
func (s *Service) RecoverHostedGenesisTurn(ctx context.Context, input BootstrapConversationRecoverInput) (*BootstrapConversationCompleteResult, error) {
	baseURL, _, instanceKey, err := s.hostBootstrapInputs(ctx)
	if err != nil {
		return nil, err
	}
	registrationID, conversationID, err := requireBootstrapRegistrationConversation(input.RegistrationID, input.ConversationID)
	if err != nil {
		return nil, err
	}

	payload := map[string]any{}
	if correlationID := strings.TrimSpace(input.CorrelationID); correlationID != "" {
		payload["correlation_id"] = correlationID
	}
	if idempotencyKey := strings.TrimSpace(input.IdempotencyKey); idempotencyKey != "" {
		payload["idempotency_key"] = idempotencyKey
	}
	if lesserRequestID := strings.TrimSpace(input.LesserRequestID); lesserRequestID != "" {
		payload["lesser_request_id"] = lesserRequestID
	}

	var raw json.RawMessage
	requestID, err := s.doHostBootstrapJSON(ctx, http.MethodPost, baseURL, "/api/v1/soul/instance/agents/register/"+url.PathEscape(registrationID)+"/mint-conversation/"+url.PathEscape(conversationID)+"/recover", instanceKey, payload, http.StatusOK, http.StatusAccepted, &raw)
	if err != nil {
		return nil, err
	}
	out, version, err := parseHostConversationResponseEnvelope(raw, requestID)
	if err != nil {
		return nil, err
	}
	if version != "" && version != hostBootstrapVersion1 {
		return nil, unsupportedHostConversationVersionError(
			"Host conversation recovery response",
			requestID,
			out.RequestID,
		)
	}
	if err := validateHostConversationSnapshot(out, registrationID, true, requestID); err != nil {
		return nil, err
	}
	result := bootstrapConversationCompleteResultFromHost(registrationID, out, hostConversationRequestID(requestID, out.RequestID))
	if isHostedBootstrapTerminalDeclarationStatus(result.Status) {
		if err := ValidateHostedBootstrapCompletionEvidence(result, conversationID); err != nil {
			return nil, err
		}
		if compact, err := compactHostedBootstrapProducedDeclarations(result.ProducedDeclarations, result.HostRequestID); err == nil {
			result.ProducedDeclarations = compact
		}
	}
	return result, nil
}

// ListHostedGenesisConversations calls Host's GET mint-conversations list
// endpoint to return bounded conversation summaries for a given agent.
//
// TODO(host-dependency): Host's GET /api/v1/soul/instance/agents/{agentId}/
// mint-conversations endpoint does not exist yet. It needs to be implemented on
// lesser-host as a separate issue/PR. This Lesser service method is written to
// call the expected endpoint shape and is tested with mocked Host responses.
func (s *Service) ListHostedGenesisConversations(ctx context.Context, agentID string) ([]HostedGenesisConversationSummary, error) {
	baseURL, _, instanceKey, err := s.hostBootstrapInputs(ctx)
	if err != nil {
		return nil, err
	}
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return nil, &HostBootstrapError{Code: "HOST_AGENT_ID_REQUIRED", Message: "agent id is required", Source: "lesser", Err: ErrHostSigningPayloadUnsupported}
	}

	var raw json.RawMessage
	_, err = s.doHostBootstrapJSON(ctx, http.MethodGet, baseURL, "/api/v1/soul/instance/agents/"+url.PathEscape(agentID)+"/mint-conversations", instanceKey, nil, http.StatusOK, &raw)
	if err != nil {
		return nil, err
	}

	var listResp hostMintConversationsListResponse
	if err := json.Unmarshal(raw, &listResp); err != nil {
		return nil, &HostBootstrapError{Code: "HOST_RESPONSE_INVALID", Message: "Host conversation list response is invalid.", Source: "host", Err: fmt.Errorf("%w: %v", ErrHostUnavailable, err)}
	}

	summaries := make([]HostedGenesisConversationSummary, 0, len(listResp.Conversations))
	for _, conv := range listResp.Conversations {
		summaries = append(summaries, HostedGenesisConversationSummary{
			ConversationID:  strings.TrimSpace(conv.ConversationID),
			RegistrationID:  strings.TrimSpace(conv.RegistrationID),
			Status:          NormalizeHostedBootstrapConversationStatus(conv.Status),
			MessageCount:    conv.MessageCount,
			LatestTurnID:    strings.TrimSpace(conv.LatestTurnID),
			CreatedAt:       parseHostTimePtr(conv.CreatedAt),
			UpdatedAt:       parseHostTimePtr(conv.UpdatedAt),
		})
	}

	sort.Slice(summaries, func(i, j int) bool {
		if summaries[i].UpdatedAt == nil && summaries[j].UpdatedAt == nil {
			return summaries[i].ConversationID < summaries[j].ConversationID
		}
		if summaries[i].UpdatedAt == nil {
			return false
		}
		if summaries[j].UpdatedAt == nil {
			return true
		}
		return summaries[i].UpdatedAt.After(*summaries[j].UpdatedAt)
	})

	if len(summaries) > hostedGenesisConversationListMaxResults {
		summaries = summaries[:hostedGenesisConversationListMaxResults]
	}

	return summaries, nil
}

// ReadBootstrapConversation reads Host's private instance mint-conversation
// record for a registration/conversation pair. Callers must validate terminal
// declaration evidence before treating the result as publish-ready.
func (s *Service) ReadBootstrapConversation(ctx context.Context, input BootstrapConversationCompleteInput) (*BootstrapConversationCompleteResult, error) {
	baseURL, _, instanceKey, err := s.hostBootstrapInputs(ctx)
	if err != nil {
		return nil, err
	}
	registrationID, conversationID, err := requireBootstrapRegistrationConversation(input.RegistrationID, input.ConversationID)
	if err != nil {
		return nil, err
	}
	return s.readBootstrapConversation(ctx, baseURL, instanceKey, registrationID, conversationID)
}

func (s *Service) recoverCompletedBootstrapConversation(ctx context.Context, baseURL string, instanceKey string, registrationID string, conversationID string, conflictErr error) (*BootstrapConversationCompleteResult, error) {
	result, err := s.readBootstrapConversation(ctx, baseURL, instanceKey, registrationID, conversationID)
	if err != nil {
		return nil, err
	}
	if result.HostRequestID == "" {
		result.HostRequestID = hostBootstrapRequestIDFromError(conflictErr)
	}
	if isHostedBootstrapTerminalDeclarationStatus(result.Status) {
		if err := ValidateHostedBootstrapCompletionEvidence(result, conversationID); err != nil {
			return nil, err
		}
		if compact, err := compactHostedBootstrapProducedDeclarations(result.ProducedDeclarations, result.HostRequestID); err == nil {
			result.ProducedDeclarations = compact
		}
	}
	return result, nil
}

func (s *Service) readBootstrapConversation(ctx context.Context, baseURL string, instanceKey string, registrationID string, conversationID string) (*BootstrapConversationCompleteResult, error) {
	var raw json.RawMessage
	requestID, err := s.doHostBootstrapJSON(ctx, http.MethodGet, baseURL, "/api/v1/soul/instance/agents/register/"+url.PathEscape(registrationID)+"/mint-conversation/"+url.PathEscape(conversationID), instanceKey, nil, http.StatusOK, &raw)
	if err != nil {
		return nil, err
	}
	out, version, err := parseHostConversationReadEnvelope(raw, requestID)
	if err != nil {
		return nil, err
	}
	if version != "" && version != hostBootstrapVersion1 {
		return nil, unsupportedHostConversationVersionError(
			"Host conversation read response",
			requestID,
			out.RequestID,
		)
	}
	if err := validateHostConversationSnapshot(out, registrationID, true, requestID); err != nil {
		return nil, err
	}
	result := bootstrapConversationCompleteResultFromHost(registrationID, out, hostConversationRequestID(requestID, out.RequestID))
	if isHostedBootstrapTerminalDeclarationStatus(result.Status) {
		if err := ValidateHostedBootstrapCompletionEvidence(result, conversationID); err != nil {
			return nil, err
		}
		if compact, err := compactHostedBootstrapProducedDeclarations(result.ProducedDeclarations, result.HostRequestID); err == nil {
			result.ProducedDeclarations = compact
		}
	}
	return result, nil
}

func bootstrapConversationCompleteResultFromHost(registrationID string, out hostMintConversationResponse, requestID string) *BootstrapConversationCompleteResult {
	return &BootstrapConversationCompleteResult{
		RegistrationID:        firstNonEmpty(strings.TrimSpace(out.RegistrationID), registrationID),
		HostSoulAgentID:       strings.ToLower(strings.TrimSpace(out.AgentID)),
		ConversationID:        strings.TrimSpace(out.ConversationID),
		Status:                NormalizeHostedBootstrapConversationStatus(out.Status),
		LatestTurnID:          strings.TrimSpace(out.LatestTurnID),
		MessageCount:          out.MessageCount,
		Messages:              bootstrapConversationMessagesFromHost(out.MessagesRaw),
		MessagesTruncated:     out.MessagesTruncated,
		ProducedDeclarations:  hostRawJSONValue(out.ProducedDeclarationsRaw),
		UpdatedAt:             parseHostTimePtr(out.UpdatedAt),
		CompletedAt:           parseHostTimePtr(out.CompletedAt),
		HostRequestID:         strings.TrimSpace(requestID),
		FailureCode:           strings.TrimSpace(out.Failure.Code),
		FailureMessage:        strings.TrimSpace(out.Failure.Message),
		FailureRetryable:      out.Failure.Retryable,
		FailureRecoveryAction: strings.TrimSpace(out.Failure.Recovery.Action),
	}
}

func bootstrapConversationMessageResultFromHost(registrationID string, out hostMintConversationResponse, requestID string) *BootstrapConversationMessageResult {
	return &BootstrapConversationMessageResult{
		RegistrationID:        firstNonEmpty(strings.TrimSpace(out.RegistrationID), registrationID),
		HostSoulAgentID:       strings.ToLower(strings.TrimSpace(out.AgentID)),
		ConversationID:        strings.TrimSpace(out.ConversationID),
		Status:                NormalizeHostedBootstrapConversationStatus(out.Status),
		Model:                 strings.TrimSpace(out.Model),
		LatestTurnID:          strings.TrimSpace(out.LatestTurnID),
		MessageCount:          out.MessageCount,
		Messages:              bootstrapConversationMessagesFromHost(out.MessagesRaw),
		MessagesTruncated:     out.MessagesTruncated,
		FullResponse:          strings.TrimSpace(out.FullResponse),
		ProducedDeclarations:  hostRawJSONValue(out.ProducedDeclarationsRaw),
		UpdatedAt:             parseHostTimePtr(out.UpdatedAt),
		CompletedAt:           parseHostTimePtr(out.CompletedAt),
		HostRequestID:         strings.TrimSpace(requestID),
		FailureCode:           strings.TrimSpace(out.Failure.Code),
		FailureMessage:        strings.TrimSpace(out.Failure.Message),
		FailureRetryable:      out.Failure.Retryable,
		FailureRecoveryAction: strings.TrimSpace(out.Failure.Recovery.Action),
	}
}

func completeResultFromMessageResult(in *BootstrapConversationMessageResult) *BootstrapConversationCompleteResult {
	if in == nil {
		return nil
	}
	return &BootstrapConversationCompleteResult{
		RegistrationID:        in.RegistrationID,
		HostSoulAgentID:       in.HostSoulAgentID,
		ConversationID:        in.ConversationID,
		Status:                in.Status,
		LatestTurnID:          in.LatestTurnID,
		MessageCount:          in.MessageCount,
		Messages:              cloneBootstrapConversationMessages(in.Messages),
		MessagesTruncated:     in.MessagesTruncated,
		ProducedDeclarations:  in.ProducedDeclarations,
		UpdatedAt:             in.UpdatedAt,
		CompletedAt:           in.CompletedAt,
		HostRequestID:         in.HostRequestID,
		FailureCode:           in.FailureCode,
		FailureMessage:        in.FailureMessage,
		FailureRetryable:      in.FailureRetryable,
		FailureRecoveryAction: in.FailureRecoveryAction,
	}
}

func cloneBootstrapConversationMessages(in []BootstrapConversationMessage) []BootstrapConversationMessage {
	if len(in) == 0 {
		return nil
	}
	out := make([]BootstrapConversationMessage, len(in))
	copy(out, in)
	return out
}

// PrepareBootstrapFinalize calls Host finalize preflight and fails closed on
// unsupported signing metadata.
func (s *Service) PrepareBootstrapFinalize(ctx context.Context, input BootstrapFinalizePreflightInput) (*BootstrapFinalizePreflightResult, error) {
	baseURL, _, instanceKey, err := s.hostBootstrapInputs(ctx)
	if err != nil {
		return nil, err
	}
	registrationID, conversationID, err := requireBootstrapRegistrationConversation(input.RegistrationID, input.ConversationID)
	if err != nil {
		return nil, err
	}

	payload := map[string]any{
		"boundary_signatures": normalizeBootstrapSignatureMap(input.BoundarySignatures),
	}

	var out hostFinalizePreflightResponse
	requestID, err := s.doHostBootstrapJSON(ctx, http.MethodPost, baseURL, "/api/v1/soul/instance/agents/register/"+url.PathEscape(registrationID)+"/mint-conversation/"+url.PathEscape(conversationID)+"/finalize/preflight", instanceKey, payload, http.StatusOK, &out)
	if err != nil {
		return nil, err
	}
	if err := validateHostFinalizePreflightPayload(out); err != nil {
		return nil, err
	}

	return &BootstrapFinalizePreflightResult{
		Version:         strings.TrimSpace(out.Version),
		DigestHex:       strings.TrimSpace(out.DigestHex),
		IssuedAt:        parseHostTimePtr(out.IssuedAt),
		ExpectedVersion: out.ExpectedVersion,
		NextVersion:     out.NextVersion,
		SelfAttestationSigning: BootstrapFinalizeSigningInput{
			SignerWallet:    strings.ToLower(strings.TrimSpace(out.SelfAttestationSigning.SignerWallet)),
			SigningMethod:   strings.TrimSpace(out.SelfAttestationSigning.SigningMethod),
			MessageEncoding: strings.TrimSpace(out.SelfAttestationSigning.MessageEncoding),
			MessageHex:      strings.TrimSpace(out.SelfAttestationSigning.MessageHex),
			DigestHex:       strings.TrimSpace(out.SelfAttestationSigning.DigestHex),
			CanonicalJSON:   strings.TrimSpace(out.SelfAttestationSigning.CanonicalJSON),
		},
		BoundaryRequirementsJSON:    compactHostJSON(out.BoundaryRequirementsRaw),
		FinalizeRequestTemplateJSON: compactHostJSON(out.FinalizeRequestTemplateRaw),
		RegistrationPreviewJSON:     compactHostJSON(out.RegistrationPreviewRaw),
		HostRequestID:               requestID,
	}, nil
}

// FinalizeBootstrap relays Host finalize and publication. Hosted/off-chain
// success does not require on-chain mint transaction fields.
func (s *Service) FinalizeBootstrap(ctx context.Context, input BootstrapFinalizeInput) (*BootstrapFinalizeResult, error) {
	baseURL, _, instanceKey, err := s.hostBootstrapInputs(ctx)
	if err != nil {
		return nil, err
	}
	registrationID, conversationID, err := requireBootstrapRegistrationConversation(input.RegistrationID, input.ConversationID)
	if err != nil {
		return nil, err
	}
	if input.IssuedAt.IsZero() {
		return nil, &HostBootstrapError{Code: "HOST_INVALID_REQUEST", Message: "issued_at is required", Source: "lesser", Err: ErrHostSigningPayloadUnsupported}
	}
	if input.ExpectedVersion < 0 {
		return nil, &HostBootstrapError{Code: "HOST_INVALID_REQUEST", Message: "expected_version is invalid", Source: "lesser", Err: ErrHostSigningPayloadUnsupported}
	}
	if strings.TrimSpace(input.SelfAttestation) == "" {
		return nil, &HostBootstrapError{Code: "HOST_INVALID_REQUEST", Message: "self_attestation is required", Source: "lesser", Err: ErrHostSigningPayloadUnsupported}
	}

	payload := map[string]any{
		"boundary_signatures": normalizeBootstrapSignatureMap(input.BoundarySignatures),
		"issued_at":           input.IssuedAt.UTC().Format(time.RFC3339),
		"expected_version":    input.ExpectedVersion,
		"self_attestation":    strings.TrimSpace(input.SelfAttestation),
	}

	var out hostFinalizeResponse
	requestID, err := s.doHostBootstrapJSON(ctx, http.MethodPost, baseURL, "/api/v1/soul/instance/agents/register/"+url.PathEscape(registrationID)+"/mint-conversation/"+url.PathEscape(conversationID)+"/finalize", instanceKey, payload, http.StatusOK, &out)
	if err != nil {
		return nil, err
	}
	if err := validateHostFinalizeResponse(out); err != nil {
		return nil, err
	}

	return bootstrapFinalizeResultFromHost(out, requestID), nil
}

// PublishHostedBootstrap relays the hosted-first no-wallet finalize request.
func (s *Service) PublishHostedBootstrap(ctx context.Context, input HostedBootstrapPublishInput) (*BootstrapFinalizeResult, error) {
	baseURL, instanceDomain, instanceKey, err := s.hostBootstrapInputs(ctx)
	if err != nil {
		return nil, err
	}
	registrationID, conversationID, err := requireBootstrapRegistrationConversation(input.RegistrationID, input.ConversationID)
	if err != nil {
		return nil, err
	}

	var out hostFinalizeResponse
	requestID, err := s.doHostBootstrapJSON(ctx, http.MethodPost, baseURL, "/api/v1/soul/instance/agents/register/"+url.PathEscape(registrationID)+"/mint-conversation/"+url.PathEscape(conversationID)+"/finalize", instanceKey, map[string]any{}, http.StatusOK, &out)
	if err != nil {
		return nil, err
	}
	if err := validateHostHostedFinalizeResponse(out, instanceDomain, bootstrapHostLocalID(BootstrapBeginInput{Username: input.LocalID})); err != nil {
		return nil, err
	}

	return bootstrapFinalizeResultFromHost(out, requestID), nil
}

func bootstrapFinalizeResultFromHost(out hostFinalizeResponse, requestID string) *BootstrapFinalizeResult {
	agentID := strings.ToLower(strings.TrimSpace(firstNonEmpty(out.AgentID, out.Agent.AgentID, out.Publication.AgentID, out.Promotion.AgentID)))
	return &BootstrapFinalizeResult{
		Version:                 strings.TrimSpace(out.Version),
		HostSoulAgentID:         agentID,
		PublishedVersion:        out.PublishedVersion,
		AgentDomain:             strings.ToLower(strings.TrimSpace(out.Agent.Domain)),
		AgentLocalID:            strings.ToLower(strings.TrimSpace(out.Agent.LocalID)),
		AgentAuthorityModel:     strings.TrimSpace(out.Agent.AuthorityModel),
		AgentAnchorState:        strings.TrimSpace(out.Agent.AnchorState),
		AgentOperationalBinding: strings.TrimSpace(out.Agent.OperationalBinding),
		AgentStatus:             strings.TrimSpace(out.Agent.Status),
		AgentLifecycleStatus:    strings.TrimSpace(out.Agent.LifecycleStatus),
		PrincipalAddress:        strings.ToLower(strings.TrimSpace(out.Agent.PrincipalAddress)),
		Publication: BootstrapPublicationEvidence{
			AgentID:                    strings.ToLower(strings.TrimSpace(out.Publication.AgentID)),
			PublishedVersion:           out.Publication.PublishedVersion,
			AuthorityModel:             strings.TrimSpace(out.Publication.AuthorityModel),
			RegistrationURI:            strings.TrimSpace(out.Publication.RegistrationURI),
			RegistrationS3Key:          strings.TrimSpace(out.Publication.RegistrationS3Key),
			VersionedRegistrationURI:   strings.TrimSpace(out.Publication.VersionedRegistrationURI),
			VersionedRegistrationS3Key: strings.TrimSpace(out.Publication.VersionedRegistrationS3Key),
			AnchorState:                strings.TrimSpace(out.Publication.AnchorState),
			PublishedAt:                parseHostTimePtr(out.Publication.PublishedAt),
		},
		Promotion: BootstrapPromotionEvidence{
			AgentID:                  strings.ToLower(strings.TrimSpace(out.Promotion.AgentID)),
			RegistrationID:           strings.TrimSpace(out.Promotion.RegistrationID),
			Stage:                    strings.TrimSpace(out.Promotion.Stage),
			RequestStatus:            strings.TrimSpace(out.Promotion.RequestStatus),
			ReviewStatus:             strings.TrimSpace(out.Promotion.ReviewStatus),
			ReadinessStatus:          strings.TrimSpace(out.Promotion.ReadinessStatus),
			AuthorityModel:           strings.TrimSpace(out.Promotion.AuthorityModel),
			AnchorState:              strings.TrimSpace(out.Promotion.AnchorState),
			LatestConversationID:     strings.TrimSpace(out.Promotion.LatestConversationID),
			LatestConversationStatus: strings.TrimSpace(out.Promotion.LatestConversationStatus),
			PublishedVersion:         out.Promotion.PublishedVersion,
			GraduatedAt:              parseHostTimePtr(out.Promotion.GraduatedAt),
		},
		HostRequestID: requestID,
	}
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

func (s *Service) doHostBootstrapJSON(ctx context.Context, method string, baseURL string, path string, instanceKey string, payload any, expectedStatuses ...any) (string, error) {
	endpoint, err := hostBootstrapURL(baseURL, path)
	if err != nil {
		return "", &HostBootstrapError{Code: "HOST_UNAVAILABLE", Message: "Host bootstrap endpoint is unavailable.", Source: "host", Err: fmt.Errorf("%w: %v", ErrHostUnavailable, err)}
	}
	expected, out := splitHostBootstrapJSONExpectedStatuses(expectedStatuses...)

	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return "", err
		}
		body = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return "", &HostBootstrapError{Code: "HOST_UNAVAILABLE", Message: "Host bootstrap endpoint is unavailable.", Source: "host", Err: fmt.Errorf("%w: %v", ErrHostUnavailable, err)}
	}
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
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

	if !hostBootstrapStatusAllowed(resp.StatusCode, expected) {
		return requestID, hostBootstrapHTTPError(resp.StatusCode, resp.Header, responseBody, truncated, instanceKey)
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

func splitHostBootstrapJSONExpectedStatuses(values ...any) ([]int, any) {
	expected := make([]int, 0, len(values))
	var out any
	for _, value := range values {
		switch typed := value.(type) {
		case int:
			expected = append(expected, typed)
		default:
			out = value
		}
	}
	if len(expected) == 0 {
		expected = append(expected, http.StatusOK)
	}
	return expected, out
}

func hostBootstrapStatusAllowed(status int, expected []int) bool {
	for _, want := range expected {
		if status == want {
			return true
		}
	}
	return false
}

func hostBootstrapHTTPError(status int, headers http.Header, body []byte, truncated bool, secrets ...string) error {
	requestID := requestIDFromHeaders(headers)
	envelope := hostBootstrapErrorEnvelope{}
	if !truncated && len(body) > 0 {
		_ = json.Unmarshal(body, &envelope)
	}
	if strings.TrimSpace(envelope.Error.RequestID) != "" {
		requestID = strings.TrimSpace(envelope.Error.RequestID)
	}
	statusCode := status
	if envelope.Error.StatusCode != 0 {
		statusCode = envelope.Error.StatusCode
	}
	if strings.TrimSpace(envelope.Error.Code) != "" {
		message := strings.TrimSpace(envelope.Error.Message)
		if message == "" {
			_, message = mapHostBootstrapStatus(status)
		}
		return &HostBootstrapError{
			Code:          strings.TrimSpace(envelope.Error.Code),
			Message:       redactHostBootstrapSecret(message, secrets...),
			Source:        "host",
			StatusCode:    statusCode,
			HostRequestID: requestID,
			DetailsJSON:   sanitizeHostBootstrapDetails(envelope.Error.DetailsRaw, secrets...),
			Err:           ErrHostUnavailable,
		}
	}

	code, message := mapHostBootstrapStatus(status)
	return &HostBootstrapError{
		Code:          code,
		Message:       redactHostBootstrapSecret(message, secrets...),
		Source:        "host",
		StatusCode:    statusCode,
		HostRequestID: requestID,
		Err:           ErrHostUnavailable,
	}
}

func sanitizeHostBootstrapDetails(raw json.RawMessage, secrets ...string) string {
	if len(bytes.TrimSpace(raw)) == 0 {
		return ""
	}
	var value any
	if err := json.Unmarshal(raw, &value); err == nil {
		sanitized := sanitizeHostBootstrapDetailValue("", value, secrets...)
		encoded, err := json.Marshal(sanitized)
		if err == nil {
			return string(encoded)
		}
	}
	compact := compactHostJSON(raw)
	if compact == "" {
		return ""
	}
	return redactHostBootstrapSecret(compact, secrets...)
}

func sanitizeHostBootstrapDetailValue(key string, value any, secrets ...string) any {
	if sensitiveHostBootstrapDetailKey(key) {
		return "[redacted]"
	}
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for childKey, childValue := range typed {
			out[childKey] = sanitizeHostBootstrapDetailValue(childKey, childValue, secrets...)
		}
		return out
	case []any:
		out := make([]any, 0, len(typed))
		for _, childValue := range typed {
			out = append(out, sanitizeHostBootstrapDetailValue("", childValue, secrets...))
		}
		return out
	case string:
		redacted := redactHostBootstrapSecret(typed, secrets...)
		if sensitiveHostBootstrapDetailString(redacted) {
			return "[redacted]"
		}
		return redacted
	default:
		return value
	}
}

func sensitiveHostBootstrapDetailKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" {
		return false
	}
	for _, marker := range []string{
		"authorization",
		"bearer",
		"credential",
		"endpoint",
		"host_route",
		"instance_key",
		"microvm",
		"private_key",
		"prompt",
		"provider_key",
		"provider_response",
		"raw_response",
		"raw_transcript",
		"route",
		"secret",
		"seed",
		"shell_auth",
		"ssm",
		"token",
		"transcript",
		"url",
		"wallet_signature",
	} {
		if strings.Contains(key, marker) {
			return true
		}
	}
	return false
}

func sensitiveHostBootstrapDetailString(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	if lower == "" {
		return false
	}
	if strings.Contains(lower, "bearer ") ||
		strings.Contains(lower, "http://") ||
		strings.Contains(lower, "https://") ||
		strings.Contains(lower, "microvm") ||
		strings.Contains(lower, "provider response") ||
		strings.Contains(lower, "raw transcript") {
		return true
	}
	return false
}

func redactHostBootstrapSecret(value string, secrets ...string) string {
	out := value
	for _, secret := range secrets {
		secret = strings.TrimSpace(secret)
		if secret == "" {
			continue
		}
		out = strings.ReplaceAll(out, secret, "[redacted]")
	}
	return out
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

func isHostBootstrapConversationConflict(err error) bool {
	var hostErr *HostBootstrapError
	if !errors.As(err, &hostErr) {
		return false
	}
	code := strings.TrimSpace(hostErr.Code)
	if code == "soul_instance.conflict" || code == "HOST_BOOTSTRAP_CONFLICT" {
		return true
	}
	if hostErr.StatusCode == http.StatusConflict {
		return true
	}
	return strings.Contains(strings.ToLower(strings.TrimSpace(hostErr.Message)), "conversation is not in progress")
}

func hostBootstrapRequestIDFromError(err error) string {
	var hostErr *HostBootstrapError
	if errors.As(err, &hostErr) {
		return strings.TrimSpace(hostErr.HostRequestID)
	}
	return ""
}

func validateHostedBeginResponse(out hostRegistrationBeginResponse, instanceDomain string, localID string) error {
	if authority := firstNonEmpty(out.Registration.AuthorityModel, out.Promotion.AuthorityModel); strings.TrimSpace(authority) != "" && strings.TrimSpace(authority) != SoulAuthorityModelInstanceTrust {
		return &HostBootstrapError{Code: "HOST_RESPONSE_INVALID", Message: "Host begin response did not preserve hosted authority.", Source: "host", Err: ErrHostUnavailable}
	}
	if domain := firstNonEmpty(out.Registration.DomainNormalized, out.Registration.DomainRaw, out.Promotion.Domain); strings.TrimSpace(domain) != "" && !domainMatches(domain, instanceDomain) {
		return &HostBootstrapError{Code: "HOST_RESPONSE_INVALID", Message: "Host begin response domain does not match this instance.", Source: "host", Err: ErrHostUnavailable}
	}
	if hostLocalID := firstNonEmpty(out.Registration.LocalID, out.Registration.LocalIDRaw, out.Promotion.LocalID); strings.TrimSpace(hostLocalID) != "" && !strings.EqualFold(strings.TrimSpace(hostLocalID), strings.TrimSpace(localID)) {
		return &HostBootstrapError{Code: "HOST_RESPONSE_INVALID", Message: "Host begin response local id does not match this body.", Source: "host", Err: ErrHostUnavailable}
	}
	return nil
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

func requireBootstrapRegistrationConversation(registrationID string, conversationID string) (string, string, error) {
	registrationID = strings.TrimSpace(registrationID)
	if registrationID == "" {
		return "", "", &HostBootstrapError{Code: "HOST_REGISTRATION_ID_REQUIRED", Message: "registration id is required", Source: "lesser", Err: ErrHostSigningPayloadUnsupported}
	}
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return "", "", &HostBootstrapError{Code: "HOST_CONVERSATION_ID_REQUIRED", Message: "conversation id is required", Source: "lesser", Err: ErrHostSigningPayloadUnsupported}
	}
	return registrationID, conversationID, nil
}

func normalizeBootstrapSignatureMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for key, value := range in {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		out[key] = strings.TrimSpace(value)
	}
	return out
}

func parseHostConversationEnvelope(raw json.RawMessage, hostRequestID string) (hostMintConversationResponse, error) {
	var out hostMintConversationResponse
	if len(bytes.TrimSpace(raw)) == 0 {
		return out, hostBootstrapInvalidResponse("Host conversation response is missing.", hostRequestID)
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return out, &HostBootstrapError{Code: "HOST_RESPONSE_INVALID", Message: "Host conversation response is invalid.", Source: "host", HostRequestID: strings.TrimSpace(hostRequestID), Err: fmt.Errorf("%w: %v", ErrHostUnavailable, err)}
	}
	return out, nil
}

func parseHostConversationResponseEnvelope(raw json.RawMessage, hostRequestID string) (hostMintConversationResponse, string, error) {
	return parseHostConversationDurableEnvelope(raw, hostRequestID, "Host conversation response")
}

func parseHostConversationReadEnvelope(raw json.RawMessage, hostRequestID string) (hostMintConversationResponse, string, error) {
	return parseHostConversationDurableEnvelope(raw, hostRequestID, "Host conversation read response")
}

func parseHostConversationDurableEnvelope(raw json.RawMessage, hostRequestID string, responseName string) (hostMintConversationResponse, string, error) {
	var wrapper hostMintConversationReadResponse
	if len(bytes.TrimSpace(raw)) == 0 {
		return hostMintConversationResponse{}, "", hostBootstrapInvalidResponse(responseName+" is missing.", hostRequestID)
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return hostMintConversationResponse{}, "", &HostBootstrapError{
			Code:          "HOST_RESPONSE_INVALID",
			Message:       responseName + " is invalid.",
			Source:        "host",
			HostRequestID: strings.TrimSpace(hostRequestID),
			Err:           fmt.Errorf("%w: %v", ErrHostUnavailable, err),
		}
	}
	if len(bytes.TrimSpace(wrapper.ConversationRaw)) > 0 {
		wrapperRequestID := hostConversationRequestID(hostRequestID, wrapper.RequestID)
		conversation, err := parseHostConversationEnvelope(wrapper.ConversationRaw, wrapperRequestID)
		if err != nil {
			return hostMintConversationResponse{}, strings.TrimSpace(wrapper.Version), err
		}
		if strings.TrimSpace(conversation.RequestID) == "" {
			conversation.RequestID = strings.TrimSpace(wrapper.RequestID)
		}
		return conversation, strings.TrimSpace(wrapper.Version), nil
	}
	if strings.TrimSpace(wrapper.Version) != "" {
		return hostMintConversationResponse{}, strings.TrimSpace(wrapper.Version), hostBootstrapInvalidResponse(
			responseName+" did not include a conversation.",
			hostConversationRequestID(hostRequestID, wrapper.RequestID),
		)
	}
	conversation, err := parseHostConversationEnvelope(raw, hostRequestID)
	return conversation, "", err
}

func validateHostConversationSnapshot(out hostMintConversationResponse, fallbackRegistrationID string, requireConversationID bool, headerRequestID string) error {
	requestID := hostConversationRequestID(headerRequestID, out.RequestID)
	status := NormalizeHostedBootstrapConversationStatus(out.Status)
	if status == "" {
		return hostBootstrapInvalidResponse("Host conversation response did not include a valid status.", requestID)
	}
	if strings.TrimSpace(out.RegistrationID) == "" && strings.TrimSpace(fallbackRegistrationID) == "" {
		return hostBootstrapInvalidResponse("Host conversation response did not include a registration id.", requestID)
	}
	if requireConversationID && strings.TrimSpace(out.ConversationID) == "" {
		return hostBootstrapInvalidResponse("Host conversation response did not include a conversation id.", requestID)
	}
	if strings.TrimSpace(out.ConversationID) == "" {
		return hostBootstrapInvalidResponse("Host conversation response did not include a conversation id.", requestID)
	}
	if status == NormalizeHostedBootstrapConversationStatus("failed") {
		return validateHostedFailedConversation(out, requestID)
	}
	if isHostedBootstrapTerminalDeclarationStatus(status) {
		return ValidateHostedBootstrapCompletionEvidence(bootstrapConversationCompleteResultFromHost(fallbackRegistrationID, out, requestID), out.ConversationID)
	}
	return nil
}

func validateHostedFailedConversation(out hostMintConversationResponse, hostRequestID string) error {
	if strings.TrimSpace(out.Failure.Code) == "" {
		return hostBootstrapInvalidResponse("Host failed conversation response did not include a failure code.", hostRequestID)
	}
	if strings.TrimSpace(out.Failure.Message) == "" {
		return hostBootstrapInvalidResponse("Host failed conversation response did not include a failure message.", hostRequestID)
	}
	if !validHostedFailureRecoveryAction(out.Failure.Recovery.Action) {
		return hostBootstrapInvalidResponse("Host failed conversation response did not include a locked recovery action.", hostRequestID)
	}
	return nil
}

func validHostedFailureRecoveryAction(action string) bool {
	switch strings.TrimSpace(action) {
	case "refresh_state", "retry_same_step", "restart_soul_bootstrap", "operator_action":
		return true
	default:
		return false
	}
}

func hostConversationRequestID(headerRequestID string, bodyRequestID string) string {
	if body := strings.TrimSpace(bodyRequestID); body != "" {
		return body
	}
	return strings.TrimSpace(headerRequestID)
}

func unsupportedHostConversationVersionError(responseName string, headerRequestID string, bodyRequestID string) *HostBootstrapError {
	return &HostBootstrapError{
		Code:          "HOST_RESPONSE_INVALID",
		Message:       responseName + " used an unsupported version.",
		Source:        "host",
		StatusCode:    http.StatusOK,
		HostRequestID: hostConversationRequestID(headerRequestID, bodyRequestID),
		Err:           ErrHostUnavailable,
	}
}

// NormalizeHostedBootstrapConversationStatus returns the canonical Host M1.1
// status spelling Lesser projects. Host's Lesser-visible instance-key path
// collapses created to in_progress; completed remains accepted as a legacy
// spelling for declaration_ready only when declaration evidence validates.
func NormalizeHostedBootstrapConversationStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "created", "in_progress":
		return "in_progress"
	case "assistant_turn_ready":
		return "assistant_turn_ready"
	case "declaration_extraction_pending":
		return "declaration_extraction_pending"
	case hostConversationStatusDeclarationReady, "completed":
		return hostConversationStatusDeclarationReady
	case "failed":
		return "failed"
	case "published":
		return "published"
	case "bound":
		return "bound"
	default:
		return ""
	}
}

func isHostedBootstrapTerminalDeclarationStatus(status string) bool {
	return NormalizeHostedBootstrapConversationStatus(status) == hostConversationStatusDeclarationReady
}

// IsHostedBootstrapTerminalDeclarationStatus reports whether a Host status is
// terminal declaration evidence status after Lesser's compatibility
// normalization. It is exported for GraphQL projection/publish-gate helpers.
func IsHostedBootstrapTerminalDeclarationStatus(status string) bool {
	return isHostedBootstrapTerminalDeclarationStatus(status)
}

func hostRawJSONValue(raw json.RawMessage) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return ""
	}
	if len(trimmed) > 0 && trimmed[0] == '"' {
		var value string
		if err := json.Unmarshal(trimmed, &value); err == nil {
			return strings.TrimSpace(value)
		}
	}
	return compactHostJSON(trimmed)
}

func bootstrapConversationMessagesFromHost(raw json.RawMessage) []BootstrapConversationMessage {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil
	}
	if trimmed[0] == '"' {
		var encoded string
		if err := json.Unmarshal(trimmed, &encoded); err != nil {
			return nil
		}
		trimmed = bytes.TrimSpace([]byte(encoded))
		if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
			return nil
		}
	}

	var messages []struct {
		ID        string `json:"id"`
		Role      string `json:"role"`
		Content   string `json:"content"`
		Order     int    `json:"order"`
		CreatedAt string `json:"created_at"`
		Truncated bool   `json:"truncated"`
	}
	if err := json.Unmarshal(trimmed, &messages); err != nil || len(messages) == 0 {
		return nil
	}

	out := make([]BootstrapConversationMessage, 0, len(messages))
	for idx, message := range messages {
		role := normalizeHostConversationMessageRole(message.Role)
		content := strings.TrimSpace(message.Content)
		if role == "" || content == "" {
			continue
		}
		if hostConversationMessageContentUnsafe(content) {
			return nil
		}
		order := message.Order
		if order <= 0 {
			order = idx + 1
		}
		id := strings.TrimSpace(message.ID)
		if id == "" {
			id = fmt.Sprintf("msg_%06d", order)
		}
		out = append(out, BootstrapConversationMessage{
			ID:        id,
			Role:      role,
			Content:   content,
			Order:     order,
			CreatedAt: parseHostTimePtr(message.CreatedAt),
			Truncated: message.Truncated,
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeHostConversationMessageRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case hostConversationMessageRoleUser, hostConversationMessageRoleAssistant:
		return strings.ToLower(strings.TrimSpace(role))
	default:
		return ""
	}
}

func hostConversationMessageContentUnsafe(content string) bool {
	lower := strings.ToLower(content)
	for _, marker := range []string{
		"aws_secret_access_key",
		"aws_access_key_id",
		"aws_session_token",
		"x-amz-security-token",
		"secretaccesskey",
		"organizationaccountaccessrole",
		"arn:aws:iam",
		"arn:aws:sts",
		"microvm endpoint token",
		"microvm_endpoint_token",
		"instance api key",
		"raw instance key",
		"bearer ",
		"ssm parameter",
		"parameter store",
		"/lesser-host/",
		"mint-signer",
		"governance-signer",
		"seed phrase",
		"private key",
		"signing material",
		"provider secret",
		"host bearer",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

// ValidateHostedBootstrapCompletionEvidence fails closed unless Host returned
// terminal completion evidence for the requested hosted mint conversation.
func ValidateHostedBootstrapCompletionEvidence(result *BootstrapConversationCompleteResult, expectedConversationID string) error {
	if result == nil {
		return hostBootstrapInvalidResponse("Host complete response is missing.", "")
	}
	if !isHostedBootstrapTerminalDeclarationStatus(result.Status) {
		return hostBootstrapInvalidResponse("Host complete response was not terminal.", result.HostRequestID)
	}
	expectedConversationID = strings.TrimSpace(expectedConversationID)
	if expectedConversationID == "" {
		return &HostBootstrapError{Code: "HOST_CONVERSATION_ID_REQUIRED", Message: "conversation id is required", Source: "lesser", Err: ErrHostSigningPayloadUnsupported}
	}
	if actual := strings.TrimSpace(result.ConversationID); actual == "" || actual != expectedConversationID {
		return hostBootstrapInvalidResponse("Host complete response conversation id does not match the requested conversation.", result.HostRequestID)
	}
	_, err := compactHostedBootstrapProducedDeclarationsWithContext(result.ProducedDeclarations, result.HostRequestID, hostedProducedDeclarationsContext{
		ExpectedConversationID: expectedConversationID,
		RegistrationID:         result.RegistrationID,
		AgentID:                result.HostSoulAgentID,
		MessageCount:           result.MessageCount,
		RequestID:              result.HostRequestID,
	})
	return err
}

func compactHostedBootstrapProducedDeclarations(raw string, hostRequestID string) (string, error) {
	return compactHostedBootstrapProducedDeclarationsWithContext(raw, hostRequestID, hostedProducedDeclarationsContext{})
}

type hostedProducedDeclarationsContext struct {
	ExpectedConversationID string
	RegistrationID         string
	AgentID                string
	MessageCount           int
	RequestID              string
}

type hostProducedDeclarationsEnvelope struct {
	DeclarationID   string          `json:"declaration_id"`
	DeclarationHash string          `json:"declaration_hash"`
	ProducedAt      string          `json:"produced_at"`
	DeclarationsRaw json.RawMessage `json:"declarations"`
	Evidence        struct {
		Source         string `json:"source"`
		RegistrationID string `json:"registration_id"`
		ConversationID string `json:"conversation_id"`
		AgentID        string `json:"agent_id"`
		MessageCount   int    `json:"message_count"`
		Model          string `json:"model"`
		RequestID      string `json:"request_id"`
	} `json:"evidence"`
}

func compactHostedBootstrapProducedDeclarationsWithContext(raw string, hostRequestID string, context hostedProducedDeclarationsContext) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", hostBootstrapInvalidResponse("Host complete response did not include produced declarations.", hostRequestID)
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return "", hostBootstrapInvalidResponse("Host complete response produced declarations were not valid JSON.", hostRequestID)
	}
	if len(payload) == 0 {
		return "", hostBootstrapInvalidResponse("Host complete response produced declarations were empty.", hostRequestID)
	}
	if declarationsRaw, ok := payload["declarations"]; ok {
		var envelope hostProducedDeclarationsEnvelope
		if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
			return "", hostBootstrapInvalidResponse("Host complete response declaration evidence envelope was invalid.", hostRequestID)
		}
		if err := validateHostedProducedDeclarationsEnvelope(envelope, context, hostRequestID); err != nil {
			return "", err
		}
		return compactHostedBootstrapDeclarationsObject(declarationsRaw, hostRequestID)
	}
	return compactHostedBootstrapDeclarationsObject(json.RawMessage(raw), hostRequestID)
}

func validateHostedProducedDeclarationsEnvelope(envelope hostProducedDeclarationsEnvelope, context hostedProducedDeclarationsContext, hostRequestID string) error {
	if strings.TrimSpace(envelope.DeclarationID) == "" {
		return hostBootstrapInvalidResponse("Host complete response declaration evidence did not include a declaration id.", hostRequestID)
	}
	if !validHostedDeclarationHash(envelope.DeclarationHash) {
		return hostBootstrapInvalidResponse("Host complete response declaration evidence did not include a valid declaration hash.", hostRequestID)
	}
	if _, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(envelope.ProducedAt)); err != nil {
		return hostBootstrapInvalidResponse("Host complete response declaration evidence did not include a valid produced_at timestamp.", hostRequestID)
	}
	if len(bytes.TrimSpace(envelope.DeclarationsRaw)) == 0 {
		return hostBootstrapInvalidResponse("Host complete response declaration evidence did not include declarations.", hostRequestID)
	}
	if strings.TrimSpace(envelope.Evidence.Source) != "host_conversation" {
		return hostBootstrapInvalidResponse("Host complete response declaration evidence source was invalid.", hostRequestID)
	}
	if expected := strings.TrimSpace(context.ExpectedConversationID); expected != "" && strings.TrimSpace(envelope.Evidence.ConversationID) != expected {
		return hostBootstrapInvalidResponse("Host complete response declaration evidence conversation id does not match the requested conversation.", hostRequestID)
	}
	if expected := strings.TrimSpace(context.RegistrationID); expected != "" && strings.TrimSpace(envelope.Evidence.RegistrationID) != expected {
		return hostBootstrapInvalidResponse("Host complete response declaration evidence registration id does not match the requested registration.", hostRequestID)
	}
	if expected := strings.ToLower(strings.TrimSpace(context.AgentID)); expected != "" && strings.ToLower(strings.TrimSpace(envelope.Evidence.AgentID)) != expected {
		return hostBootstrapInvalidResponse("Host complete response declaration evidence agent id does not match the requested agent.", hostRequestID)
	}
	if context.MessageCount > 0 && envelope.Evidence.MessageCount != context.MessageCount {
		return hostBootstrapInvalidResponse("Host complete response declaration evidence message count does not match the conversation snapshot.", hostRequestID)
	}
	if expected := strings.TrimSpace(context.RequestID); expected != "" && strings.TrimSpace(envelope.Evidence.RequestID) != expected {
		return hostBootstrapInvalidResponse("Host complete response declaration evidence request id does not match the conversation snapshot.", hostRequestID)
	}
	if strings.TrimSpace(envelope.Evidence.RegistrationID) == "" ||
		strings.TrimSpace(envelope.Evidence.ConversationID) == "" ||
		strings.TrimSpace(envelope.Evidence.AgentID) == "" ||
		strings.TrimSpace(envelope.Evidence.RequestID) == "" ||
		envelope.Evidence.MessageCount < 1 {
		return hostBootstrapInvalidResponse("Host complete response declaration evidence was incomplete.", hostRequestID)
	}
	return nil
}

func validHostedDeclarationHash(value string) bool {
	value = strings.TrimSpace(value)
	const prefix = "sha256:"
	if len(value) != len(prefix)+64 || !strings.HasPrefix(value, prefix) {
		return false
	}
	for _, r := range value[len(prefix):] {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') {
			continue
		}
		return false
	}
	return true
}

func compactHostedBootstrapDeclarationsObject(raw json.RawMessage, hostRequestID string) (string, error) {
	var declaration map[string]json.RawMessage
	if err := json.Unmarshal(raw, &declaration); err != nil {
		return "", hostBootstrapInvalidResponse("Host complete response produced declarations were not valid JSON.", hostRequestID)
	}
	if len(declaration) == 0 {
		return "", hostBootstrapInvalidResponse("Host complete response produced declarations were empty.", hostRequestID)
	}
	if err := requireHostedDeclarationObject(declaration, "selfDescription", true, hostRequestID); err != nil {
		return "", err
	}
	if err := requireHostedDeclarationArray(declaration, "capabilities", hostRequestID); err != nil {
		return "", err
	}
	if err := requireHostedDeclarationArray(declaration, "boundaries", hostRequestID); err != nil {
		return "", err
	}
	if err := requireHostedDeclarationObject(declaration, "transparency", false, hostRequestID); err != nil {
		return "", err
	}
	return compactHostJSON(raw), nil
}

func requireHostedDeclarationObject(declaration map[string]json.RawMessage, field string, requireNonEmpty bool, hostRequestID string) error {
	raw, ok := declaration[field]
	if !ok {
		return hostBootstrapInvalidResponse("Host complete response produced declarations were missing "+field+".", hostRequestID)
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return hostBootstrapInvalidResponse("Host complete response produced declarations had invalid "+field+".", hostRequestID)
	}
	if requireNonEmpty {
		var object map[string]json.RawMessage
		if err := json.Unmarshal(trimmed, &object); err != nil || len(object) == 0 {
			return hostBootstrapInvalidResponse("Host complete response produced declarations had empty "+field+".", hostRequestID)
		}
	}
	return nil
}

func requireHostedDeclarationArray(declaration map[string]json.RawMessage, field string, hostRequestID string) error {
	raw, ok := declaration[field]
	if !ok {
		return hostBootstrapInvalidResponse("Host complete response produced declarations were missing "+field+".", hostRequestID)
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '[' {
		return hostBootstrapInvalidResponse("Host complete response produced declarations had invalid "+field+".", hostRequestID)
	}
	return nil
}

func hostBootstrapInvalidResponse(message string, hostRequestID string) *HostBootstrapError {
	return &HostBootstrapError{
		Code:          "HOST_RESPONSE_INVALID",
		Message:       message,
		Source:        "host",
		HostRequestID: strings.TrimSpace(hostRequestID),
		Err:           ErrHostUnavailable,
	}
}

func compactHostJSON(raw json.RawMessage) string {
	if len(bytes.TrimSpace(raw)) == 0 {
		return ""
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return strings.TrimSpace(string(raw))
	}
	return buf.String()
}

func validateHostFinalizePreflightPayload(out hostFinalizePreflightResponse) error {
	if strings.TrimSpace(out.Version) != hostBootstrapVersion1 {
		return &HostBootstrapError{Code: "HOST_SIGNING_PAYLOAD_UNSUPPORTED", Message: "Host returned an unsupported finalize payload version.", Source: "lesser", Err: ErrHostSigningPayloadUnsupported}
	}
	if strings.TrimSpace(out.SelfAttestationSigning.SigningMethod) != hostBootstrapSigningMethodEIP191 {
		return &HostBootstrapError{Code: "HOST_SIGNING_PAYLOAD_UNSUPPORTED", Message: "Host returned an unsupported finalize signing method.", Source: "lesser", Err: ErrHostSigningPayloadUnsupported}
	}
	if strings.TrimSpace(out.SelfAttestationSigning.MessageEncoding) != hostBootstrapEncodingHexBytes {
		return &HostBootstrapError{Code: "HOST_SIGNING_PAYLOAD_UNSUPPORTED", Message: "Host returned an unsupported finalize message encoding.", Source: "lesser", Err: ErrHostSigningPayloadUnsupported}
	}
	if strings.TrimSpace(out.SelfAttestationSigning.MessageHex) == "" || strings.TrimSpace(out.SelfAttestationSigning.DigestHex) == "" {
		return &HostBootstrapError{Code: "HOST_SIGNING_PAYLOAD_UNSUPPORTED", Message: "Host returned incomplete finalize signing material.", Source: "lesser", Err: ErrHostSigningPayloadUnsupported}
	}
	if out.ExpectedVersion < 0 || out.NextVersion < 1 {
		return &HostBootstrapError{Code: "HOST_SIGNING_PAYLOAD_UNSUPPORTED", Message: "Host returned invalid finalize version metadata.", Source: "lesser", Err: ErrHostSigningPayloadUnsupported}
	}
	var boundaries []hostFinalizeBoundaryRequirement
	if len(bytes.TrimSpace(out.BoundaryRequirementsRaw)) > 0 {
		if err := json.Unmarshal(out.BoundaryRequirementsRaw, &boundaries); err != nil {
			return &HostBootstrapError{Code: "HOST_SIGNING_PAYLOAD_UNSUPPORTED", Message: "Host returned invalid boundary signing material.", Source: "lesser", Err: ErrHostSigningPayloadUnsupported}
		}
	}
	for _, boundary := range boundaries {
		if strings.TrimSpace(boundary.SigningMethod) != hostBootstrapSigningMethodEIP191 {
			return &HostBootstrapError{Code: "HOST_SIGNING_PAYLOAD_UNSUPPORTED", Message: "Host returned an unsupported boundary signing method.", Source: "lesser", Err: ErrHostSigningPayloadUnsupported}
		}
		if strings.TrimSpace(boundary.MessageEncoding) != "utf8" {
			return &HostBootstrapError{Code: "HOST_SIGNING_PAYLOAD_UNSUPPORTED", Message: "Host returned an unsupported boundary message encoding.", Source: "lesser", Err: ErrHostSigningPayloadUnsupported}
		}
		if strings.TrimSpace(boundary.Message) == "" || strings.TrimSpace(boundary.DigestHex) == "" {
			return &HostBootstrapError{Code: "HOST_SIGNING_PAYLOAD_UNSUPPORTED", Message: "Host returned incomplete boundary signing material.", Source: "lesser", Err: ErrHostSigningPayloadUnsupported}
		}
	}
	return nil
}

func validateHostFinalizeResponse(out hostFinalizeResponse) error {
	if strings.TrimSpace(out.Version) != hostBootstrapVersion1 {
		return &HostBootstrapError{Code: "HOST_RESPONSE_INVALID", Message: "Host returned an unsupported finalize response version.", Source: "host", Err: ErrHostUnavailable}
	}
	agentID := strings.TrimSpace(firstNonEmpty(out.AgentID, out.Agent.AgentID, out.Publication.AgentID, out.Promotion.AgentID))
	if _, err := validateAgentID(agentID); err != nil {
		return &HostBootstrapError{Code: "HOST_RESPONSE_INVALID", Message: "Host finalize response did not include a valid soul agent id.", Source: "host", Err: ErrHostUnavailable}
	}
	if out.PublishedVersion < 1 && out.Publication.PublishedVersion < 1 {
		return &HostBootstrapError{Code: "HOST_RESPONSE_INVALID", Message: "Host finalize response did not include publication evidence.", Source: "host", Err: ErrHostUnavailable}
	}
	return nil
}

func validateHostHostedFinalizeResponse(out hostFinalizeResponse, instanceDomain string, localID string) error {
	if err := validateHostFinalizeResponse(out); err != nil {
		return err
	}
	if strings.TrimSpace(out.Version) != hostBootstrapVersion1 {
		return &HostBootstrapError{Code: "HOST_RESPONSE_INVALID", Message: "Host returned an unsupported hosted finalize response version.", Source: "host", Err: ErrHostUnavailable}
	}
	if _, err := validateHostFinalizeAgentIDConsistency(out); err != nil {
		return &HostBootstrapError{Code: "HOST_RESPONSE_INVALID", Message: "Host hosted finalize response did not include a valid soul agent id.", Source: "host", Err: ErrHostUnavailable}
	}
	if !domainMatches(out.Agent.Domain, instanceDomain) {
		return &HostBootstrapError{Code: "HOST_RESPONSE_INVALID", Message: "Host hosted finalize response domain does not match this instance.", Source: "host", Err: ErrHostUnavailable}
	}
	if !strings.EqualFold(strings.TrimSpace(out.Agent.LocalID), strings.TrimSpace(localID)) {
		return &HostBootstrapError{Code: "HOST_RESPONSE_INVALID", Message: "Host hosted finalize response local id does not match this body.", Source: "host", Err: ErrHostUnavailable}
	}
	if strings.TrimSpace(out.Agent.AuthorityModel) != SoulAuthorityModelInstanceTrust || strings.TrimSpace(out.Publication.AuthorityModel) != SoulAuthorityModelInstanceTrust {
		return &HostBootstrapError{Code: "HOST_RESPONSE_INVALID", Message: "Host hosted finalize response did not preserve instance trust authority.", Source: "host", Err: ErrHostUnavailable}
	}
	if strings.TrimSpace(out.Promotion.AgentID) != "" && strings.TrimSpace(out.Promotion.AuthorityModel) != SoulAuthorityModelInstanceTrust {
		return &HostBootstrapError{Code: "HOST_RESPONSE_INVALID", Message: "Host hosted promotion did not preserve instance trust authority.", Source: "host", Err: ErrHostUnavailable}
	}
	if strings.TrimSpace(out.Agent.AnchorState) != SoulAnchorStateHostedOffchain || strings.TrimSpace(out.Publication.AnchorState) != SoulAnchorStateHostedOffchain {
		return &HostBootstrapError{Code: "HOST_RESPONSE_INVALID", Message: "Host hosted finalize response did not preserve hosted off-chain anchor state.", Source: "host", Err: ErrHostUnavailable}
	}
	if strings.TrimSpace(out.Promotion.AgentID) != "" && strings.TrimSpace(out.Promotion.AnchorState) != SoulAnchorStateHostedOffchain {
		return &HostBootstrapError{Code: "HOST_RESPONSE_INVALID", Message: "Host hosted promotion did not preserve hosted off-chain anchor state.", Source: "host", Err: ErrHostUnavailable}
	}
	if strings.TrimSpace(out.Agent.OperationalBinding) != SoulOperationalBindingHostedBound {
		return &HostBootstrapError{Code: "HOST_RESPONSE_INVALID", Message: "Host hosted finalize response did not preserve hosted operational binding.", Source: "host", Err: ErrHostUnavailable}
	}
	if strings.TrimSpace(out.Agent.Status) != "" && !strings.EqualFold(out.Agent.Status, "active") {
		return &HostBootstrapError{Code: "HOST_RESPONSE_INVALID", Message: "Host hosted finalize response is not active.", Source: "host", Err: ErrHostUnavailable}
	}
	if out.PublishedVersion < 1 && out.Publication.PublishedVersion < 1 {
		return &HostBootstrapError{Code: "HOST_RESPONSE_INVALID", Message: "Host hosted finalize response did not publish a version.", Source: "host", Err: ErrHostUnavailable}
	}
	return nil
}

func validateHostFinalizeAgentIDConsistency(out hostFinalizeResponse) (string, error) {
	var normalizedAgentID string
	for _, agentID := range []string{out.AgentID, out.Agent.AgentID, out.Publication.AgentID, out.Promotion.AgentID} {
		if strings.TrimSpace(agentID) == "" {
			continue
		}
		normalized, err := validateAgentID(agentID)
		if err != nil {
			return "", err
		}
		if normalizedAgentID == "" {
			normalizedAgentID = normalized
			continue
		}
		if normalized != normalizedAgentID {
			return "", fmt.Errorf("inconsistent agent id %q", agentID)
		}
	}
	if normalizedAgentID == "" {
		return "", errors.New("agent id is required")
	}
	return normalizedAgentID, nil
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

type hostMintConversationResponse struct {
	RegistrationID          string                  `json:"registration_id"`
	AgentID                 string                  `json:"agent_id"`
	ConversationID          string                  `json:"conversation_id"`
	Model                   string                  `json:"model"`
	MessagesRaw             json.RawMessage         `json:"messages"`
	MessagesTruncated       bool                    `json:"messages_truncated"`
	FullResponse            string                  `json:"full_response"`
	LatestTurnID            string                  `json:"latest_turn_id"`
	MessageCount            int                     `json:"message_count"`
	ProducedDeclarationsRaw json.RawMessage         `json:"produced_declarations"`
	Status                  string                  `json:"status"`
	UpdatedAt               string                  `json:"updated_at"`
	CompletedAt             string                  `json:"completed_at"`
	RequestID               string                  `json:"request_id"`
	Failure                 hostConversationFailure `json:"failure"`
}

type hostMintConversationReadResponse struct {
	Version         string          `json:"version"`
	RequestID       string          `json:"request_id"`
	ConversationRaw json.RawMessage `json:"conversation"`
}

type hostMintConversationsListResponse struct {
	Conversations []hostMintConversationSummary `json:"conversations"`
}

type hostMintConversationSummary struct {
	ConversationID string `json:"conversation_id"`
	RegistrationID string `json:"registration_id"`
	Status         string `json:"status"`
	MessageCount   int    `json:"message_count"`
	LatestTurnID   string `json:"latest_turn_id"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

type hostConversationFailure struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
	Recovery  struct {
		Action string `json:"action"`
	} `json:"recovery"`
}

type hostFinalizeSigningInput struct {
	SignerWallet    string `json:"signer_wallet"`
	SigningMethod   string `json:"signing_method"`
	MessageEncoding string `json:"message_encoding"`
	MessageHex      string `json:"message_hex"`
	DigestHex       string `json:"digest_hex"`
	CanonicalJSON   string `json:"canonical_json"`
}

type hostFinalizeBoundaryRequirement struct {
	BoundaryID      string `json:"boundary_id"`
	SigningMethod   string `json:"signing_method"`
	MessageEncoding string `json:"message_encoding"`
	Message         string `json:"message"`
	DigestHex       string `json:"digest_hex"`
}

type hostFinalizePreflightResponse struct {
	Version                    string                   `json:"version"`
	DigestHex                  string                   `json:"digest_hex"`
	IssuedAt                   string                   `json:"issued_at"`
	ExpectedVersion            int                      `json:"expected_version"`
	NextVersion                int                      `json:"next_version"`
	SelfAttestationSigning     hostFinalizeSigningInput `json:"self_attestation_signing"`
	BoundaryRequirementsRaw    json.RawMessage          `json:"boundary_requirements"`
	FinalizeRequestTemplateRaw json.RawMessage          `json:"finalize_request_template"`
	RegistrationPreviewRaw     json.RawMessage          `json:"registration_preview,omitempty"`
}

type hostFinalizeResponse struct {
	Version          string                  `json:"version"`
	AgentID          string                  `json:"agent_id"`
	Agent            hostFinalizeAgent       `json:"agent"`
	PublishedVersion int                     `json:"published_version"`
	Publication      hostFinalizePublication `json:"publication"`
	Promotion        hostFinalizePromotion   `json:"promotion,omitempty"`
}

type hostFinalizeAgent struct {
	AgentID            string `json:"agent_id"`
	Domain             string `json:"domain"`
	LocalID            string `json:"local_id"`
	PrincipalAddress   string `json:"principal_address"`
	Wallet             string `json:"wallet"`
	AuthorityModel     string `json:"authority_model"`
	AnchorState        string `json:"anchor_state"`
	OperationalBinding string `json:"operational_binding"`
	Status             string `json:"status"`
	LifecycleStatus    string `json:"lifecycle_status"`
}

type hostFinalizePublication struct {
	AgentID                    string `json:"agent_id"`
	PublishedVersion           int    `json:"published_version"`
	AuthorityModel             string `json:"authority_model"`
	RegistrationURI            string `json:"registration_uri"`
	RegistrationS3Key          string `json:"registration_s3_key"`
	VersionedRegistrationURI   string `json:"versioned_registration_uri"`
	VersionedRegistrationS3Key string `json:"versioned_registration_s3_key"`
	AnchorState                string `json:"anchor_state"`
	PublishedAt                string `json:"published_at"`
}

type hostFinalizePromotion struct {
	AgentID                  string `json:"agent_id"`
	RegistrationID           string `json:"registration_id"`
	Stage                    string `json:"stage"`
	RequestStatus            string `json:"request_status"`
	ReviewStatus             string `json:"review_status"`
	ReadinessStatus          string `json:"readiness_status"`
	AuthorityModel           string `json:"authority_model"`
	AnchorState              string `json:"anchor_state"`
	LatestConversationID     string `json:"latest_conversation_id"`
	LatestConversationStatus string `json:"latest_conversation_status"`
	PublishedVersion         int    `json:"published_version"`
	GraduatedAt              string `json:"graduated_at"`
}

type hostRegistration struct {
	ID               string `json:"id"`
	AgentID          string `json:"agent_id"`
	DomainRaw        string `json:"domain_raw"`
	DomainNormalized string `json:"domain_normalized"`
	LocalIDRaw       string `json:"local_id_raw"`
	LocalID          string `json:"local_id"`
	WalletAddress    string `json:"wallet_address"`
	AuthorityModel   string `json:"authority_model"`
	Status           string `json:"status"`
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
	Domain           string `json:"domain"`
	LocalID          string `json:"local_id"`
	Stage            string `json:"stage"`
	AuthorityModel   string `json:"authority_model"`
	AnchorState      string `json:"anchor_state"`
	PrincipalAddress string `json:"principal_address"`
}

type hostBootstrapErrorEnvelope struct {
	Error struct {
		Code       string          `json:"code"`
		Message    string          `json:"message"`
		StatusCode int             `json:"status_code"`
		DetailsRaw json.RawMessage `json:"details"`
		RequestID  string          `json:"request_id"`
	} `json:"error"`
}
