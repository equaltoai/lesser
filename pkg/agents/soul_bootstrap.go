package agents

import (
	"strings"
	"time"
)

const (
	// SoulBootstrapModeHosted is the hosted-first instance-trust bootstrap path.
	SoulBootstrapModeHosted = "hosted"
	// SoulBootstrapModeWalletPrincipal is the compatibility wallet/principal assurance-upgrade path.
	SoulBootstrapModeWalletPrincipal = "wallet_principal"

	// SoulBootstrapAuthorityModelInstanceTrust is Host's managed instance-key authority model.
	SoulBootstrapAuthorityModelInstanceTrust = "instance_trust"
	// SoulBootstrapAuthorityModelWalletPrincipal is Host's wallet/principal authority model.
	SoulBootstrapAuthorityModelWalletPrincipal = "wallet_principal"

	// SoulBootstrapAnchorStateHostedOffchain is Host's hosted/off-chain anchor state.
	SoulBootstrapAnchorStateHostedOffchain = "hosted_offchain"
	// SoulBootstrapAnchorStateImmutableOnchain is Host's on-chain assurance state.
	SoulBootstrapAnchorStateImmutableOnchain = "immutable_onchain"

	// SoulBootstrapAssuranceStateHostedOffchain is a hosted Host record assurance tier.
	SoulBootstrapAssuranceStateHostedOffchain = "hosted_offchain"
	// SoulBootstrapAssuranceStateImmutableOnchain is an on-chain receipt assurance tier.
	SoulBootstrapAssuranceStateImmutableOnchain = "immutable_onchain"

	// SoulBootstrapNextActionStartHostedBootstrap starts the default hosted bootstrap path.
	SoulBootstrapNextActionStartHostedBootstrap = "start_hosted_bootstrap"
	// SoulBootstrapNextActionSendHostedGenesisMessage sends the hosted genesis conversation turn.
	SoulBootstrapNextActionSendHostedGenesisMessage = "send_hosted_soul_genesis_message"
	// SoulBootstrapNextActionCompleteHostedGenesis completes the hosted genesis conversation.
	SoulBootstrapNextActionCompleteHostedGenesis = "complete_hosted_soul_genesis"
	// SoulBootstrapNextActionPublishHostedSoul publishes and binds the hosted soul.
	SoulBootstrapNextActionPublishHostedSoul = "publish_hosted_soul"
	// SoulBootstrapNextActionRestartSoulBootstrap restarts stale hosted bootstrap state.
	SoulBootstrapNextActionRestartSoulBootstrap = "restart_soul_bootstrap"
	// SoulBootstrapNextActionRetrySameStep retries a transient failed step.
	SoulBootstrapNextActionRetrySameStep = "retry_same_step"
	// SoulBootstrapNextActionRefreshState asks clients to refresh current state.
	SoulBootstrapNextActionRefreshState = "refresh_state"
	// SoulBootstrapNextActionOperatorActionRequired requires operator configuration/action.
	SoulBootstrapNextActionOperatorActionRequired = "operator_action_required"
	// SoulBootstrapNextActionVerifyWallet continues the wallet/principal compatibility path.
	SoulBootstrapNextActionVerifyWallet = "verify_wallet"
	// SoulBootstrapNextActionPreparePrincipalDeclaration prepares wallet/principal declaration signing.
	SoulBootstrapNextActionPreparePrincipalDeclaration = "prepare_principal_declaration"
	// SoulBootstrapNextActionVerifyPrincipalDeclaration verifies wallet/principal declaration signing.
	SoulBootstrapNextActionVerifyPrincipalDeclaration = "verify_principal_declaration"
	// SoulBootstrapNextActionContinueConversation continues a wallet/principal conversation.
	SoulBootstrapNextActionContinueConversation = "continue_conversation"
	// SoulBootstrapNextActionFinalize finalizes the wallet/principal compatibility path.
	SoulBootstrapNextActionFinalize = "finalize"
	// SoulBootstrapNextActionComplete marks completed bootstrap.
	SoulBootstrapNextActionComplete = "complete"

	// SoulBootstrapRecoveryCategoryRetrySameStep is safe to retry the same operation.
	SoulBootstrapRecoveryCategoryRetrySameStep = "retry_same_step"
	// SoulBootstrapRecoveryCategoryRestartRequired requires a new restart attempt.
	SoulBootstrapRecoveryCategoryRestartRequired = "restart_required"
	// SoulBootstrapRecoveryCategoryOperatorActionRequired requires operator action.
	SoulBootstrapRecoveryCategoryOperatorActionRequired = "operator_action_required"
	// SoulBootstrapRecoveryCategoryRefreshState asks clients to refresh current state.
	SoulBootstrapRecoveryCategoryRefreshState = "refresh_state"

	// SoulBootstrapRecoveryActionRetrySameStep is the stable retry action.
	SoulBootstrapRecoveryActionRetrySameStep = "retry_same_step"
	// SoulBootstrapRecoveryActionRestartBootstrap is the stable restart action.
	SoulBootstrapRecoveryActionRestartBootstrap = "restart_bootstrap"
	// SoulBootstrapRecoveryActionContactOperator is the stable operator action.
	SoulBootstrapRecoveryActionContactOperator = "contact_operator"
	// SoulBootstrapRecoveryActionRefreshState is the stable refresh action.
	SoulBootstrapRecoveryActionRefreshState = "refresh_state"

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
	// SoulBootstrapStateHostFailed marks a Host-authored conversation failure.
	SoulBootstrapStateHostFailed = "error.host_failed"
	// SoulBootstrapStateHostSigningPayloadUnsupported marks unsupported Host signing metadata.
	SoulBootstrapStateHostSigningPayloadUnsupported = "error.host_signing_payload_unsupported"
	// SoulBootstrapStateConversationRegistrationActive marks a hosted registration before Host creates a conversation.
	SoulBootstrapStateConversationRegistrationActive = "conversation.registration_active"
	// SoulBootstrapStateConversationInProgress marks an active Host mint conversation.
	SoulBootstrapStateConversationInProgress = "conversation.in_progress"
	// SoulBootstrapStateConversationAssistantTurnReady marks a durable Host assistant turn awaiting completion.
	SoulBootstrapStateConversationAssistantTurnReady = "conversation.assistant_turn_ready"
	// SoulBootstrapStateConversationDeclarationExtractionPending marks Host declaration extraction in progress.
	SoulBootstrapStateConversationDeclarationExtractionPending = "conversation.declaration_extraction_pending"
	// SoulBootstrapStateConversationDeclarationReady marks Host terminal declaration evidence readiness.
	SoulBootstrapStateConversationDeclarationReady = "conversation.declaration_ready"
	// SoulBootstrapStateConversationCompleted marks a completed Host mint conversation.
	SoulBootstrapStateConversationCompleted = "conversation.completed"
	// SoulBootstrapStateFinalizeReady marks Host finalize preflight signing material readiness.
	SoulBootstrapStateFinalizeReady = "finalize.ready"
	// SoulBootstrapStateFinalizePublished marks Host publication before local binding completes.
	SoulBootstrapStateFinalizePublished = "finalize.published"
	// SoulBootstrapStateCorrelationMismatch marks a local replay/correlation mismatch.
	SoulBootstrapStateCorrelationMismatch = "error.correlation_mismatch"
	// SoulBootstrapStateBindingConflict marks a safe local soul/body binding conflict.
	SoulBootstrapStateBindingConflict = "error.binding_conflict"
	// SoulBootstrapStateSoulNotAvailable marks Host publication evidence that cannot be bound locally.
	SoulBootstrapStateSoulNotAvailable = "error.soul_not_available"
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
	// SoulBootstrapErrorHostConversationFailed is exposed for Host-authored durable conversation failures.
	SoulBootstrapErrorHostConversationFailed = "HOST_CONVERSATION_FAILED"
	// SoulBootstrapErrorHostSigningPayloadUnsupported is exposed for unsupported Host signing metadata.
	SoulBootstrapErrorHostSigningPayloadUnsupported = "HOST_SIGNING_PAYLOAD_UNSUPPORTED"
	// SoulBootstrapErrorHostRegistrationIDRequired is exposed when no Host registration id is available.
	SoulBootstrapErrorHostRegistrationIDRequired = "HOST_REGISTRATION_ID_REQUIRED"
	// SoulBootstrapErrorHostConversationIDRequired is exposed when no Host conversation id is available.
	SoulBootstrapErrorHostConversationIDRequired = "HOST_CONVERSATION_ID_REQUIRED"
	// SoulBootstrapErrorHostBootstrapReplayRejected is exposed when caller ids do not match local bootstrap state.
	SoulBootstrapErrorHostBootstrapReplayRejected = "HOST_BOOTSTRAP_REPLAY_REJECTED"
	// SoulBootstrapErrorSoulBindingConflict is exposed when local binding uniqueness rejects finalization.
	SoulBootstrapErrorSoulBindingConflict = "SOUL_BINDING_CONFLICT"
	// SoulBootstrapErrorSoulNotAvailable is exposed when Host-published soul identity is not locally bindable.
	SoulBootstrapErrorSoulNotAvailable = "SOUL_NOT_AVAILABLE"

	// SoulBootstrapHostConversationStatusCreated is Host's pre-turn durable-created status.
	SoulBootstrapHostConversationStatusCreated = "created"
	// SoulBootstrapHostConversationStatusInProgress is Host's active durable work status.
	SoulBootstrapHostConversationStatusInProgress = "in_progress"
	// SoulBootstrapHostConversationStatusAssistantTurnReady is Host's assistant-turn-ready status.
	SoulBootstrapHostConversationStatusAssistantTurnReady = "assistant_turn_ready"
	// SoulBootstrapHostConversationStatusDeclarationExtractionPending is Host's declaration extraction status.
	SoulBootstrapHostConversationStatusDeclarationExtractionPending = "declaration_extraction_pending"
	// SoulBootstrapHostConversationStatusDeclarationReady is Host's terminal declaration-ready status.
	SoulBootstrapHostConversationStatusDeclarationReady = "declaration_ready"
	// SoulBootstrapHostConversationStatusFailed is Host's failed conversation status.
	SoulBootstrapHostConversationStatusFailed = "failed"
	// SoulBootstrapHostConversationStatusPublished is a post-finalize Host publication status.
	SoulBootstrapHostConversationStatusPublished = "published"
	// SoulBootstrapHostConversationStatusBound is a post-bind Host status.
	SoulBootstrapHostConversationStatusBound = "bound"
)

// SoulBootstrapState stores local correlation state for zero-state soul creation.
//
// It is nested inside DroneWorkflowState metadata so the bootstrap workflow remains
// visible through the existing body/drone workflow path. Host instance keys are not
// represented here; only Host-issued identifiers, signing checkpoint metadata, and
// client-safe error/correlation values are persisted.
type SoulBootstrapState struct {
	Username           string                            `json:"username,omitempty"`
	BodyID             string                            `json:"body_id,omitempty"`
	HostRegistrationID string                            `json:"host_registration_id,omitempty"`
	HostConversationID string                            `json:"host_conversation_id,omitempty"`
	HostSoulAgentID    string                            `json:"host_soul_agent_id,omitempty"`
	WalletAddress      string                            `json:"wallet_address,omitempty"`
	PrincipalAddress   string                            `json:"principal_address,omitempty"`
	BootstrapMode      string                            `json:"bootstrap_mode,omitempty"`
	AuthorityModel     string                            `json:"authority_model,omitempty"`
	AnchorState        string                            `json:"anchor_state,omitempty"`
	AssuranceState     string                            `json:"assurance_state,omitempty"`
	Phase              string                            `json:"phase,omitempty"`
	State              string                            `json:"state,omitempty"`
	NextAction         string                            `json:"next_action,omitempty"`
	RecoveryCategory   string                            `json:"recovery_category,omitempty"`
	RecoveryAction     string                            `json:"recovery_action,omitempty"`
	Retryable          bool                              `json:"retryable,omitempty"`
	RestartRequired    bool                              `json:"restart_required,omitempty"`
	SigningCheckpoints []SoulBootstrapSigningCheckpoint  `json:"signing_checkpoints,omitempty"`
	Publication        *SoulBootstrapPublicationEvidence `json:"publication,omitempty"`
	Error              *SoulBootstrapErrorState          `json:"error,omitempty"`
	Correlation        *SoulBootstrapCorrelationState    `json:"correlation,omitempty"`
	RestartedAt        *time.Time                        `json:"restarted_at,omitempty"`
	UpdatedAt          *time.Time                        `json:"updated_at,omitempty"`
}

// SoulBootstrapSigningCheckpoint records non-secret signing material metadata
// returned by Host preflight endpoints.
type SoulBootstrapSigningCheckpoint struct {
	Version                     string     `json:"version,omitempty"`
	Name                        string     `json:"name,omitempty"`
	Status                      string     `json:"status,omitempty"`
	PrincipalAddress            string     `json:"principal_address,omitempty"`
	SignerAddress               string     `json:"signer_address,omitempty"`
	SigningMethod               string     `json:"signing_method,omitempty"`
	MessageEncoding             string     `json:"message_encoding,omitempty"`
	Message                     string     `json:"message,omitempty"`
	MessageHex                  string     `json:"message_hex,omitempty"`
	DigestHex                   string     `json:"digest_hex,omitempty"`
	CanonicalJSON               string     `json:"canonical_json,omitempty"`
	ExpectedVersion             int        `json:"expected_version,omitempty"`
	NextVersion                 int        `json:"next_version,omitempty"`
	BoundaryRequirementsJSON    string     `json:"boundary_requirements_json,omitempty"`
	FinalizeRequestTemplateJSON string     `json:"finalize_request_template_json,omitempty"`
	RegistrationPreviewJSON     string     `json:"registration_preview_json,omitempty"`
	HostRequestID               string     `json:"host_request_id,omitempty"`
	IssuedAt                    *time.Time `json:"issued_at,omitempty"`
	DeclaredAt                  *time.Time `json:"declared_at,omitempty"`
	CompletedAt                 *time.Time `json:"completed_at,omitempty"`
}

// SoulBootstrapPublicationEvidence records Host publication evidence without
// storing any Host instance key or browser credential material.
type SoulBootstrapPublicationEvidence struct {
	AgentID                    string     `json:"agent_id,omitempty"`
	PublishedVersion           int        `json:"published_version,omitempty"`
	AuthorityModel             string     `json:"authority_model,omitempty"`
	RegistrationURI            string     `json:"registration_uri,omitempty"`
	RegistrationS3Key          string     `json:"registration_s3_key,omitempty"`
	VersionedRegistrationURI   string     `json:"versioned_registration_uri,omitempty"`
	VersionedRegistrationS3Key string     `json:"versioned_registration_s3_key,omitempty"`
	AnchorState                string     `json:"anchor_state,omitempty"`
	PublishedAt                *time.Time `json:"published_at,omitempty"`
}

// SoulBootstrapErrorState stores a typed, client-safe bootstrap error.
type SoulBootstrapErrorState struct {
	Code             string     `json:"code,omitempty"`
	Message          string     `json:"message,omitempty"`
	Source           string     `json:"source,omitempty"`
	StatusCode       int        `json:"status_code,omitempty"`
	DetailsJSON      string     `json:"details_json,omitempty"`
	HostRequestID    string     `json:"host_request_id,omitempty"`
	RecoveryCategory string     `json:"recovery_category,omitempty"`
	RecoveryAction   string     `json:"recovery_action,omitempty"`
	Retryable        bool       `json:"retryable,omitempty"`
	RestartRequired  bool       `json:"restart_required,omitempty"`
	At               *time.Time `json:"at,omitempty"`
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
	RestartIdempotencyKey              string `json:"restart_idempotency_key,omitempty"`
	RecoveryAttemptID                  string `json:"recovery_attempt_id,omitempty"`
	SupersededHostRegistrationID       string `json:"superseded_host_registration_id,omitempty"`
	SupersededHostConversationID       string `json:"superseded_host_conversation_id,omitempty"`
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
	normalized.BootstrapMode = normalizeBootstrapMode(normalized.BootstrapMode, normalized.AuthorityModel, normalized.WalletAddress)
	normalized.AuthorityModel = normalizeBootstrapAuthorityModel(normalized.AuthorityModel, normalized.BootstrapMode)
	normalized.AnchorState = strings.TrimSpace(normalized.AnchorState)
	normalized.AssuranceState = normalizeBootstrapAssuranceState(normalized.AssuranceState, normalized.AnchorState)
	normalized.Phase = normalizeBootstrapPhase(normalized.Phase, normalized.State)
	if normalized.State = strings.TrimSpace(normalized.State); normalized.State == "" {
		normalized.State = defaultBootstrapStateForPhase(normalized.Phase)
	}
	normalized.NextAction = strings.TrimSpace(normalized.NextAction)
	normalized.RecoveryCategory = strings.TrimSpace(normalized.RecoveryCategory)
	normalized.RecoveryAction = strings.TrimSpace(normalized.RecoveryAction)
	if normalized.Correlation != nil {
		normalized.Correlation.trim()
	}
	if normalized.Error != nil {
		normalized.Error.trim()
	}
	if normalized.Publication != nil {
		normalized.Publication.trim()
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
	cloned.Publication = cloneSoulBootstrapPublication(s.Publication)
	cloned.RestartedAt = cloneDroneTime(s.RestartedAt)
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

func cloneSoulBootstrapPublication(in *SoulBootstrapPublicationEvidence) *SoulBootstrapPublicationEvidence {
	if in == nil {
		return nil
	}
	cloned := *in
	cloned.PublishedAt = cloneDroneTime(in.PublishedAt)
	return &cloned
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

func normalizeBootstrapMode(mode string, authorityModel string, walletAddress string) string {
	mode = strings.TrimSpace(mode)
	switch mode {
	case SoulBootstrapModeHosted, SoulBootstrapModeWalletPrincipal:
		return mode
	}
	switch strings.TrimSpace(authorityModel) {
	case SoulBootstrapAuthorityModelInstanceTrust:
		return SoulBootstrapModeHosted
	case SoulBootstrapAuthorityModelWalletPrincipal:
		return SoulBootstrapModeWalletPrincipal
	}
	if strings.TrimSpace(walletAddress) != "" {
		return SoulBootstrapModeWalletPrincipal
	}
	return SoulBootstrapModeHosted
}

func normalizeBootstrapAuthorityModel(authorityModel string, mode string) string {
	authorityModel = strings.TrimSpace(authorityModel)
	switch authorityModel {
	case SoulBootstrapAuthorityModelInstanceTrust, SoulBootstrapAuthorityModelWalletPrincipal:
		return authorityModel
	}
	if strings.TrimSpace(mode) == SoulBootstrapModeWalletPrincipal {
		return SoulBootstrapAuthorityModelWalletPrincipal
	}
	return SoulBootstrapAuthorityModelInstanceTrust
}

func normalizeBootstrapAssuranceState(assuranceState string, anchorState string) string {
	assuranceState = strings.TrimSpace(assuranceState)
	if assuranceState != "" {
		return assuranceState
	}
	anchorState = strings.TrimSpace(anchorState)
	if anchorState != "" {
		return anchorState
	}
	return ""
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
	case SoulBootstrapErrorHostRegistrationIDRequired,
		SoulBootstrapErrorHostConversationIDRequired,
		SoulBootstrapErrorHostBootstrapReplayRejected:
		return SoulBootstrapStateCorrelationMismatch
	case SoulBootstrapErrorSoulBindingConflict:
		return SoulBootstrapStateBindingConflict
	case SoulBootstrapErrorSoulNotAvailable:
		return SoulBootstrapStateSoulNotAvailable
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
	c.RestartIdempotencyKey = strings.TrimSpace(c.RestartIdempotencyKey)
	c.RecoveryAttemptID = strings.TrimSpace(c.RecoveryAttemptID)
	c.SupersededHostRegistrationID = strings.TrimSpace(c.SupersededHostRegistrationID)
	c.SupersededHostConversationID = strings.TrimSpace(c.SupersededHostConversationID)
	c.LastHostRequestID = strings.TrimSpace(c.LastHostRequestID)
}

func (e *SoulBootstrapErrorState) trim() {
	if e == nil {
		return
	}
	e.Code = strings.TrimSpace(e.Code)
	e.Message = strings.TrimSpace(e.Message)
	e.Source = strings.TrimSpace(e.Source)
	e.DetailsJSON = strings.TrimSpace(e.DetailsJSON)
	e.HostRequestID = strings.TrimSpace(e.HostRequestID)
	e.RecoveryCategory = strings.TrimSpace(e.RecoveryCategory)
	e.RecoveryAction = strings.TrimSpace(e.RecoveryAction)
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
	c.BoundaryRequirementsJSON = strings.TrimSpace(c.BoundaryRequirementsJSON)
	c.FinalizeRequestTemplateJSON = strings.TrimSpace(c.FinalizeRequestTemplateJSON)
	c.RegistrationPreviewJSON = strings.TrimSpace(c.RegistrationPreviewJSON)
	c.HostRequestID = strings.TrimSpace(c.HostRequestID)
}

func (p *SoulBootstrapPublicationEvidence) trim() {
	if p == nil {
		return
	}
	p.AgentID = strings.TrimSpace(p.AgentID)
	p.AuthorityModel = strings.TrimSpace(p.AuthorityModel)
	p.RegistrationURI = strings.TrimSpace(p.RegistrationURI)
	p.RegistrationS3Key = strings.TrimSpace(p.RegistrationS3Key)
	p.VersionedRegistrationURI = strings.TrimSpace(p.VersionedRegistrationURI)
	p.VersionedRegistrationS3Key = strings.TrimSpace(p.VersionedRegistrationS3Key)
	p.AnchorState = strings.TrimSpace(p.AnchorState)
}
