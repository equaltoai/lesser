package agents

import (
	"strings"
	"time"
)

const (
	// SoulBootstrapPhaseNotStarted marks a local body with no Host bootstrap state yet.
	SoulBootstrapPhaseNotStarted = "not_started"
	// SoulBootstrapPhaseBegin tracks Host registration begin.
	SoulBootstrapPhaseBegin = "begin"
	// SoulBootstrapPhaseWalletVerification tracks the wallet challenge verification step.
	SoulBootstrapPhaseWalletVerification = "wallet_verification"
	// SoulBootstrapPhasePrincipalDeclaration tracks the principal declaration signing step.
	SoulBootstrapPhasePrincipalDeclaration = "principal_declaration"
	// SoulBootstrapPhaseConversation tracks the Host mint conversation step.
	SoulBootstrapPhaseConversation = "conversation"
	// SoulBootstrapPhaseFinalize tracks Host finalize preflight/finalize.
	SoulBootstrapPhaseFinalize = "finalize"
	// SoulBootstrapPhaseComplete marks a locally bound soul/body identity.
	SoulBootstrapPhaseComplete = "complete"
	// SoulBootstrapPhaseError marks a typed bootstrap error.
	SoulBootstrapPhaseError = "error"

	// SoulBootstrapStateNotStarted is the default zero-state local bootstrap state.
	SoulBootstrapStateNotStarted = "not_started"
	// SoulBootstrapStateHostBridgeUnavailable is returned by M2.1 resolver skeletons.
	SoulBootstrapStateHostBridgeUnavailable = "error.host_bridge_unavailable"
	// SoulBootstrapStateHostTrustNotConfigured marks a missing effective Host trust base URL.
	SoulBootstrapStateHostTrustNotConfigured = "error.host_trust_not_configured"
	// SoulBootstrapStateHostInstanceKeyMissing marks a missing server-side Host instance key.
	SoulBootstrapStateHostInstanceKeyMissing = "error.host_instance_key_missing"
	// SoulBootstrapStateHostInstanceKeyUnavailable marks an unresolvable Host instance key.
	SoulBootstrapStateHostInstanceKeyUnavailable = "error.host_instance_key_unavailable"
	// SoulBootstrapStateHostUnavailable marks a bounded Host/network failure.
	SoulBootstrapStateHostUnavailable = "error.host_unavailable"
	// SoulBootstrapStateHostSigningPayloadUnsupported marks unsupported Host signing metadata.
	SoulBootstrapStateHostSigningPayloadUnsupported = "error.host_signing_payload_unsupported"
	// SoulBootstrapStateCompleteBound marks an existing soul/body binding projection.
	SoulBootstrapStateCompleteBound = "complete.bound"

	// SoulBootstrapErrorHostBridgeUnavailable is the typed M2.1 not-yet-executable error.
	SoulBootstrapErrorHostBridgeUnavailable = "HOST_BRIDGE_UNAVAILABLE"
	// SoulBootstrapErrorHostTrustNotConfigured is exposed when effective Host trust config is missing.
	SoulBootstrapErrorHostTrustNotConfigured = "HOST_TRUST_NOT_CONFIGURED"
	// SoulBootstrapErrorHostInstanceKeyMissing is exposed when the server-side Host instance key is absent.
	SoulBootstrapErrorHostInstanceKeyMissing = "HOST_INSTANCE_KEY_MISSING"
	// SoulBootstrapErrorHostInstanceKeyUnavailable is exposed when the Host instance key secret cannot be resolved.
	SoulBootstrapErrorHostInstanceKeyUnavailable = "HOST_INSTANCE_KEY_UNAVAILABLE"
	// SoulBootstrapErrorHostUnavailable is exposed for Host network/availability failures.
	SoulBootstrapErrorHostUnavailable = "HOST_UNAVAILABLE"
	// SoulBootstrapErrorHostSigningPayloadUnsupported is exposed for unsupported Host signing metadata.
	SoulBootstrapErrorHostSigningPayloadUnsupported = "HOST_SIGNING_PAYLOAD_UNSUPPORTED"
)

// SoulBootstrapState stores local correlation state for zero-state soul creation.
//
// It is nested inside DroneWorkflowState metadata so the bootstrap workflow remains
// visible through the existing body/drone workflow path. Host instance keys are not
// represented here; only Host-issued identifiers, signing checkpoint metadata, and
// client-safe error/correlation values are persisted.
type SoulBootstrapState struct {
	Username           string                           `json:"username,omitempty"`
	BodyID             string                           `json:"body_id,omitempty"`
	HostRegistrationID string                           `json:"host_registration_id,omitempty"`
	HostConversationID string                           `json:"host_conversation_id,omitempty"`
	HostSoulAgentID    string                           `json:"host_soul_agent_id,omitempty"`
	WalletAddress      string                           `json:"wallet_address,omitempty"`
	PrincipalAddress   string                           `json:"principal_address,omitempty"`
	Phase              string                           `json:"phase,omitempty"`
	State              string                           `json:"state,omitempty"`
	SigningCheckpoints []SoulBootstrapSigningCheckpoint `json:"signing_checkpoints,omitempty"`
	Error              *SoulBootstrapErrorState         `json:"error,omitempty"`
	Correlation        *SoulBootstrapCorrelationState   `json:"correlation,omitempty"`
	UpdatedAt          *time.Time                       `json:"updated_at,omitempty"`
}

// SoulBootstrapSigningCheckpoint records non-secret signing material metadata
// returned by Host preflight endpoints.
type SoulBootstrapSigningCheckpoint struct {
	Version          string     `json:"version,omitempty"`
	Name             string     `json:"name,omitempty"`
	Status           string     `json:"status,omitempty"`
	PrincipalAddress string     `json:"principal_address,omitempty"`
	SignerAddress    string     `json:"signer_address,omitempty"`
	SigningMethod    string     `json:"signing_method,omitempty"`
	MessageEncoding  string     `json:"message_encoding,omitempty"`
	Message          string     `json:"message,omitempty"`
	MessageHex       string     `json:"message_hex,omitempty"`
	DigestHex        string     `json:"digest_hex,omitempty"`
	CanonicalJSON    string     `json:"canonical_json,omitempty"`
	HostRequestID    string     `json:"host_request_id,omitempty"`
	IssuedAt         *time.Time `json:"issued_at,omitempty"`
	DeclaredAt       *time.Time `json:"declared_at,omitempty"`
	CompletedAt      *time.Time `json:"completed_at,omitempty"`
}

// SoulBootstrapErrorState stores a typed, client-safe bootstrap error.
type SoulBootstrapErrorState struct {
	Code          string     `json:"code,omitempty"`
	Message       string     `json:"message,omitempty"`
	Source        string     `json:"source,omitempty"`
	StatusCode    int        `json:"status_code,omitempty"`
	HostRequestID string     `json:"host_request_id,omitempty"`
	At            *time.Time `json:"at,omitempty"`
}

// SoulBootstrapCorrelationState stores caller-provided correlation keys and
// idempotency-key hints. Host M1.5 does not expose idempotency headers for the
// instance-key route family, so these values remain Lesser-local until a future
// Host contract supports them.
type SoulBootstrapCorrelationState struct {
	CorrelationKey                     string `json:"correlation_key,omitempty"`
	BeginIdempotencyKey                string `json:"begin_idempotency_key,omitempty"`
	WalletVerificationIdempotencyKey   string `json:"wallet_verification_idempotency_key,omitempty"`
	PrincipalDeclarationIdempotencyKey string `json:"principal_declaration_idempotency_key,omitempty"`
	ConversationIdempotencyKey         string `json:"conversation_idempotency_key,omitempty"`
	FinalizeIdempotencyKey             string `json:"finalize_idempotency_key,omitempty"`
	LastHostRequestID                  string `json:"last_host_request_id,omitempty"`
}

// NormalizeSoulBootstrap fills defaults and canonical string forms for a
// bootstrap state. The returned value is a clone and can be mutated by callers.
func NormalizeSoulBootstrap(state *SoulBootstrapState, username string) *SoulBootstrapState {
	if state == nil {
		state = &SoulBootstrapState{}
	}
	normalized := state.Clone()
	if normalized == nil {
		normalized = &SoulBootstrapState{}
	}

	normalized.Username = defaultBootstrapString(normalized.Username, username)
	normalized.BodyID = defaultBootstrapString(normalized.BodyID, normalized.Username)
	normalized.HostRegistrationID = strings.TrimSpace(normalized.HostRegistrationID)
	normalized.HostConversationID = strings.TrimSpace(normalized.HostConversationID)
	normalized.HostSoulAgentID = strings.TrimSpace(normalized.HostSoulAgentID)
	normalized.WalletAddress = strings.TrimSpace(normalized.WalletAddress)
	normalized.PrincipalAddress = strings.TrimSpace(normalized.PrincipalAddress)
	normalized.Phase = normalizeBootstrapPhase(normalized.Phase, normalized.State)
	if normalized.State = strings.TrimSpace(normalized.State); normalized.State == "" {
		normalized.State = defaultBootstrapStateForPhase(normalized.Phase)
	}
	if normalized.Correlation != nil {
		normalized.Correlation.trim()
	}
	if normalized.Error != nil {
		normalized.Error.trim()
	}
	for idx := range normalized.SigningCheckpoints {
		normalized.SigningCheckpoints[idx].trim()
	}
	return normalized
}

// NewSoulBootstrapHostBridgeUnavailableState builds the typed M2.1 skeleton state.
func NewSoulBootstrapHostBridgeUnavailableState(username string, correlation *SoulBootstrapCorrelationState, now time.Time) *SoulBootstrapState {
	now = now.UTC()
	return NormalizeSoulBootstrap(&SoulBootstrapState{
		Username:    username,
		BodyID:      username,
		Phase:       SoulBootstrapPhaseError,
		State:       SoulBootstrapStateHostBridgeUnavailable,
		Correlation: correlation,
		Error: &SoulBootstrapErrorState{
			Code:    SoulBootstrapErrorHostBridgeUnavailable,
			Message: "Lesser Host bridge calls are not executable until Project 44 M2.2.",
			Source:  "lesser",
			At:      &now,
		},
		UpdatedAt: &now,
	}, username)
}

// NewSoulBootstrapErrorState builds a typed, client-safe error state.
func NewSoulBootstrapErrorState(username string, correlation *SoulBootstrapCorrelationState, code string, message string, source string, statusCode int, hostRequestID string, now time.Time) *SoulBootstrapState {
	now = now.UTC()
	code = strings.TrimSpace(code)
	if code == "" {
		code = SoulBootstrapErrorHostUnavailable
	}
	message = strings.TrimSpace(message)
	if message == "" {
		message = "Soul bootstrap could not complete."
	}
	source = strings.TrimSpace(source)
	if source == "" {
		source = "lesser"
	}
	return NormalizeSoulBootstrap(&SoulBootstrapState{
		Username:    username,
		BodyID:      username,
		Phase:       SoulBootstrapPhaseError,
		State:       errorStateForBootstrapCode(code),
		Correlation: correlation,
		Error: &SoulBootstrapErrorState{
			Code:          code,
			Message:       message,
			Source:        source,
			StatusCode:    statusCode,
			HostRequestID: strings.TrimSpace(hostRequestID),
			At:            &now,
		},
		UpdatedAt: &now,
	}, username)
}

// Clone returns a deep copy of the bootstrap state.
func (s *SoulBootstrapState) Clone() *SoulBootstrapState {
	if s == nil {
		return nil
	}
	cloned := *s
	cloned.SigningCheckpoints = make([]SoulBootstrapSigningCheckpoint, len(s.SigningCheckpoints))
	for idx := range s.SigningCheckpoints {
		cloned.SigningCheckpoints[idx] = s.SigningCheckpoints[idx].Clone()
	}
	cloned.Error = cloneSoulBootstrapError(s.Error)
	cloned.Correlation = cloneSoulBootstrapCorrelation(s.Correlation)
	cloned.UpdatedAt = cloneDroneTime(s.UpdatedAt)
	return &cloned
}

// Clone returns a deep copy of the signing checkpoint.
func (c SoulBootstrapSigningCheckpoint) Clone() SoulBootstrapSigningCheckpoint {
	cloned := c
	cloned.IssuedAt = cloneDroneTime(c.IssuedAt)
	cloned.DeclaredAt = cloneDroneTime(c.DeclaredAt)
	cloned.CompletedAt = cloneDroneTime(c.CompletedAt)
	return cloned
}

func cloneSoulBootstrapError(in *SoulBootstrapErrorState) *SoulBootstrapErrorState {
	if in == nil {
		return nil
	}
	cloned := *in
	cloned.At = cloneDroneTime(in.At)
	return &cloned
}

func cloneSoulBootstrapCorrelation(in *SoulBootstrapCorrelationState) *SoulBootstrapCorrelationState {
	if in == nil {
		return nil
	}
	cloned := *in
	return &cloned
}

func defaultBootstrapString(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value != "" {
		return value
	}
	return strings.TrimSpace(fallback)
}

func normalizeBootstrapPhase(phase string, state string) string {
	phase = strings.TrimSpace(phase)
	if phase != "" {
		return phase
	}
	state = strings.TrimSpace(state)
	if state == "" {
		return SoulBootstrapPhaseNotStarted
	}
	if idx := strings.Index(state, "."); idx > 0 {
		return state[:idx]
	}
	return SoulBootstrapPhaseNotStarted
}

func defaultBootstrapStateForPhase(phase string) string {
	switch strings.TrimSpace(phase) {
	case SoulBootstrapPhaseBegin:
		return "begin.ready"
	case SoulBootstrapPhaseWalletVerification:
		return "wallet_verification.pending"
	case SoulBootstrapPhasePrincipalDeclaration:
		return "principal_declaration.pending"
	case SoulBootstrapPhaseConversation:
		return "conversation.pending"
	case SoulBootstrapPhaseFinalize:
		return "finalize.pending"
	case SoulBootstrapPhaseError:
		return SoulBootstrapStateHostBridgeUnavailable
	case SoulBootstrapPhaseComplete:
		return SoulBootstrapStateCompleteBound
	default:
		return SoulBootstrapStateNotStarted
	}
}

func errorStateForBootstrapCode(code string) string {
	switch strings.TrimSpace(code) {
	case SoulBootstrapErrorHostBridgeUnavailable:
		return SoulBootstrapStateHostBridgeUnavailable
	case SoulBootstrapErrorHostTrustNotConfigured:
		return SoulBootstrapStateHostTrustNotConfigured
	case SoulBootstrapErrorHostInstanceKeyMissing:
		return SoulBootstrapStateHostInstanceKeyMissing
	case SoulBootstrapErrorHostInstanceKeyUnavailable:
		return SoulBootstrapStateHostInstanceKeyUnavailable
	case SoulBootstrapErrorHostSigningPayloadUnsupported:
		return SoulBootstrapStateHostSigningPayloadUnsupported
	default:
		return SoulBootstrapStateHostUnavailable
	}
}

func (c *SoulBootstrapCorrelationState) trim() {
	if c == nil {
		return
	}
	c.CorrelationKey = strings.TrimSpace(c.CorrelationKey)
	c.BeginIdempotencyKey = strings.TrimSpace(c.BeginIdempotencyKey)
	c.WalletVerificationIdempotencyKey = strings.TrimSpace(c.WalletVerificationIdempotencyKey)
	c.PrincipalDeclarationIdempotencyKey = strings.TrimSpace(c.PrincipalDeclarationIdempotencyKey)
	c.ConversationIdempotencyKey = strings.TrimSpace(c.ConversationIdempotencyKey)
	c.FinalizeIdempotencyKey = strings.TrimSpace(c.FinalizeIdempotencyKey)
	c.LastHostRequestID = strings.TrimSpace(c.LastHostRequestID)
}

func (e *SoulBootstrapErrorState) trim() {
	if e == nil {
		return
	}
	e.Code = strings.TrimSpace(e.Code)
	e.Message = strings.TrimSpace(e.Message)
	e.Source = strings.TrimSpace(e.Source)
	e.HostRequestID = strings.TrimSpace(e.HostRequestID)
}

func (c *SoulBootstrapSigningCheckpoint) trim() {
	if c == nil {
		return
	}
	c.Version = strings.TrimSpace(c.Version)
	c.Name = strings.TrimSpace(c.Name)
	c.Status = strings.TrimSpace(c.Status)
	c.PrincipalAddress = strings.TrimSpace(c.PrincipalAddress)
	c.SignerAddress = strings.TrimSpace(c.SignerAddress)
	c.SigningMethod = strings.TrimSpace(c.SigningMethod)
	c.MessageEncoding = strings.TrimSpace(c.MessageEncoding)
	c.Message = strings.TrimSpace(c.Message)
	c.MessageHex = strings.TrimSpace(c.MessageHex)
	c.DigestHex = strings.TrimSpace(c.DigestHex)
	c.CanonicalJSON = strings.TrimSpace(c.CanonicalJSON)
	c.HostRequestID = strings.TrimSpace(c.HostRequestID)
}
