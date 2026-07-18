package souls

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	storageRepos "github.com/equaltoai/lesser/pkg/storage/repositories"
	"go.uber.org/zap"
)

const soulBindingReceiptTTL = 24 * time.Hour

var (
	// ErrSoulBindingInvalidRequest indicates the binding intent failed local grammar validation.
	ErrSoulBindingInvalidRequest = errors.New("soul binding request invalid")
	// ErrSoulBindingAgentIDInvalid indicates the soul agent ID is not canonical.
	ErrSoulBindingAgentIDInvalid = errors.New("soul binding agent id invalid")
	// ErrSoulBindingIdempotencyKeyRequired indicates a POST omitted the required idempotency key.
	ErrSoulBindingIdempotencyKeyRequired = errors.New("soul binding idempotency key required")
	// ErrSoulBindingIdempotencyMismatch indicates the same caller/key was reused for a different payload.
	ErrSoulBindingIdempotencyMismatch = errors.New("soul binding idempotency mismatch")
	// ErrSoulBindingNotFound indicates no Lesser-local binding row exists for the requested soul.
	ErrSoulBindingNotFound = errors.New("soul binding not found")
	// ErrSoulBindingActorMismatch indicates a GET actor_username filter conflicts with the stored binding.
	ErrSoulBindingActorMismatch = errors.New("soul binding actor mismatch")
	// ErrSoulBindingHostRegistryRejected indicates Host source truth did not confirm the requested binding.
	ErrSoulBindingHostRegistryRejected = errors.New("soul binding host registry rejected")
	// ErrSoulBindingHostRegistryUnavailable indicates Host source truth could not be checked.
	ErrSoulBindingHostRegistryUnavailable = errors.New("soul binding host registry unavailable")
)

// SoulBindingEvidence carries caller-provided correlation evidence. It is never
// authority for the binding; Host source truth is re-fetched before writes.
type SoulBindingEvidence struct {
	Source          string
	HostRequestID   string
	DeclarationHash string
	IssuedAt        string
}

// BindSoulBodyInput is the server-to-server binding intent accepted from body/Ptah.
type BindSoulBodyInput struct {
	CallerID             string
	IdempotencyKey       string
	ActorUsername        string
	SoulAgentID          string
	BodyActorID          string
	HostRegistrationID   string
	HostConversationID   string
	AuthorityModel       string
	AnchorState          string
	OperationalBinding   string
	PrincipalAddressHint string
	Evidence             SoulBindingEvidence
}

// SoulBindingProjection returns the current Lesser-local binding plus the
// Host-refetched identity projection used to validate it.
type SoulBindingProjection struct {
	Soul           Soul
	IdempotencyKey string
	PayloadHash    string
	Replayed       bool
}

// BindSoulBody validates a body/Ptah binding intent and writes the local
// SOUL_BODY_BINDING rows only through InstanceRepository.BindSoulBody.
func (s *Service) BindSoulBody(ctx context.Context, input BindSoulBodyInput) (*SoulBindingProjection, error) {
	if s == nil || s.instanceRepo == nil {
		return nil, fmt.Errorf("soul service misconfigured")
	}
	callerID := normalizeSoulBindingCaller(input.CallerID)
	if callerID == "" {
		return nil, ErrSoulBindingInvalidRequest
	}
	input.CallerID = callerID
	if strings.TrimSpace(input.IdempotencyKey) == "" {
		return nil, ErrSoulBindingIdempotencyKeyRequired
	}

	normalizedAgentID, err := validateAgentID(input.SoulAgentID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSoulBindingAgentIDInvalid, err)
	}
	if err := validateSoulBindingIntentHints(input); err != nil {
		return nil, err
	}

	targetAgent, err := s.resolveHostedTargetAgent(ctx, input.ActorUsername)
	if err != nil {
		return nil, err
	}
	input.ActorUsername = targetAgent.Username
	input.SoulAgentID = normalizedAgentID

	payloadHash, err := hashSoulBindingPayload(input)
	if err != nil {
		return nil, err
	}
	receipt, replayed, err := s.reserveSoulBindingReceipt(ctx, input, payloadHash)
	if err != nil {
		return nil, err
	}

	identity, err := s.fetchAndValidateHostedSoulBindingIdentity(ctx, normalizedAgentID, targetAgent.Username)
	if err != nil {
		s.updateSoulBindingReceiptState(ctx, receipt, soulBindingReceiptStatusForError(err), "")
		return nil, err
	}

	// Host instance-trust identities may be deliberately walletless/principal-less.
	// The Host refetch and invariant checks above are the authority for this
	// server-to-server path; caller-provided principal hints are never authority.
	binding, err := s.instanceRepo.BindSoulBody(ctx, identity.AgentID, targetAgent.Username, canonicalOwnerAddress(identity))
	if err != nil {
		mapped := mapSoulBindingWriteError(err)
		s.updateSoulBindingReceiptState(ctx, receipt, soulBindingReceiptStatusForError(mapped), "")
		return nil, mapped
	}
	s.updateSoulBindingReceiptState(ctx, receipt, "bound", "bound")

	soul := soulFromIdentity(identity, binding)
	return &SoulBindingProjection{
		Soul:           soul,
		IdempotencyKey: strings.TrimSpace(input.IdempotencyKey),
		PayloadHash:    payloadHash,
		Replayed:       replayed,
	}, nil
}

// GetSoulBinding returns the authenticated body/Ptah status projection for a local binding.
func (s *Service) GetSoulBinding(ctx context.Context, agentID string, actorUsername string) (*SoulBindingProjection, error) {
	if s == nil || s.instanceRepo == nil {
		return nil, fmt.Errorf("soul service misconfigured")
	}

	normalizedAgentID, err := validateAgentID(agentID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSoulBindingAgentIDInvalid, err)
	}

	binding, err := s.instanceRepo.GetSoulBodyBinding(ctx, normalizedAgentID)
	if err != nil {
		return nil, err
	}
	if binding == nil {
		return nil, ErrSoulBindingNotFound
	}

	if strings.TrimSpace(actorUsername) != "" && !strings.EqualFold(strings.TrimSpace(actorUsername), binding.Username) {
		return nil, ErrSoulBindingActorMismatch
	}

	identity, err := s.fetchAndValidateHostedSoulBindingIdentity(ctx, normalizedAgentID, binding.Username)
	if err != nil {
		return nil, err
	}
	soul := soulFromIdentity(identity, binding)
	return &SoulBindingProjection{Soul: soul}, nil
}

func validateSoulBindingIntentHints(input BindSoulBodyInput) error {
	if hint := strings.TrimSpace(input.AuthorityModel); hint != "" && hint != SoulAuthorityModelInstanceTrust {
		return fmt.Errorf("%w: authority_model must be %s", ErrSoulBindingInvalidRequest, SoulAuthorityModelInstanceTrust)
	}
	if hint := strings.TrimSpace(input.AnchorState); hint != "" && hint != SoulAnchorStateHostedOffchain {
		return fmt.Errorf("%w: anchor_state must be %s", ErrSoulBindingInvalidRequest, SoulAnchorStateHostedOffchain)
	}
	if hint := strings.TrimSpace(input.OperationalBinding); hint != "" && hint != SoulOperationalBindingHostedBound {
		return fmt.Errorf("%w: operational_binding must be %s", ErrSoulBindingInvalidRequest, SoulOperationalBindingHostedBound)
	}
	return nil
}

func (s *Service) reserveSoulBindingReceipt(ctx context.Context, input BindSoulBodyInput, payloadHash string) (*storageModels.InstanceSoulBindingIdempotencyReceipt, bool, error) {
	existing, err := s.instanceRepo.GetSoulBindingIdempotencyReceipt(ctx, input.CallerID, input.IdempotencyKey)
	if err != nil {
		return nil, false, err
	}
	if existing != nil {
		if strings.TrimSpace(existing.PayloadHash) != payloadHash {
			return existing, true, ErrSoulBindingIdempotencyMismatch
		}
		return existing, true, nil
	}

	receipt := storageModels.NewInstanceSoulBindingIdempotencyReceipt(
		input.CallerID,
		input.IdempotencyKey,
		payloadHash,
		input.SoulAgentID,
		input.ActorUsername,
		time.Now().UTC().Add(soulBindingReceiptTTL),
	)
	receipt.BodyActorID = strings.TrimSpace(input.BodyActorID)
	if err := receipt.UpdateKeys(); err != nil {
		return nil, false, err
	}

	err = s.instanceRepo.CreateSoulBindingIdempotencyReceipt(ctx, receipt)
	if err == nil {
		return receipt, false, nil
	}

	existing, getErr := s.instanceRepo.GetSoulBindingIdempotencyReceipt(ctx, input.CallerID, input.IdempotencyKey)
	if getErr != nil {
		return nil, false, getErr
	}
	if existing != nil {
		if strings.TrimSpace(existing.PayloadHash) != payloadHash {
			return existing, true, ErrSoulBindingIdempotencyMismatch
		}
		return existing, true, nil
	}
	return nil, false, err
}

func (s *Service) updateSoulBindingReceiptState(ctx context.Context, receipt *storageModels.InstanceSoulBindingIdempotencyReceipt, status string, bindingState string) {
	if s == nil || s.instanceRepo == nil || receipt == nil {
		return
	}
	status = strings.TrimSpace(status)
	if status == "" {
		return
	}
	receipt.Status = status
	receipt.BindingState = strings.TrimSpace(bindingState)
	receipt.UpdatedAt = time.Now().UTC()
	if err := s.instanceRepo.UpdateSoulBindingIdempotencyReceipt(ctx, receipt); err != nil && s.logger != nil {
		s.logger.Warn("failed to update soul binding idempotency receipt",
			zap.String("status", status),
			zap.String("agent_id", receipt.AgentID),
			zap.String("actor_username", receipt.ActorUsername),
			zap.Error(err))
	}
}

func (s *Service) fetchAndValidateHostedSoulBindingIdentity(ctx context.Context, agentID string, actorUsername string) (*hostSoulIdentity, error) {
	baseURL, instanceDomain, instanceKey, err := s.hostBootstrapInputs(ctx)
	if err != nil {
		return nil, mapSoulBindingHostError(err)
	}

	var out hostSoulAgentResponse
	_, err = s.doHostBootstrapJSON(
		ctx,
		http.MethodGet,
		baseURL,
		"/api/v1/soul/agents/"+url.PathEscape(agentID),
		instanceKey,
		nil,
		http.StatusOK,
		&out,
	)
	if err != nil {
		return nil, mapSoulBindingHostError(err)
	}

	identity := &out.Agent
	normalizedAgentID, err := validateAgentID(identity.AgentID)
	if err != nil {
		return nil, ErrSoulBindingHostRegistryUnavailable
	}
	identity.AgentID = normalizedAgentID

	if !strings.EqualFold(identity.AgentID, agentID) {
		return nil, ErrSoulBindingHostRegistryRejected
	}
	if !domainMatches(identity.Domain, instanceDomain) {
		return nil, ErrSoulBindingHostRegistryRejected
	}
	if !strings.EqualFold(strings.TrimSpace(identity.LocalID), strings.TrimSpace(actorUsername)) {
		return nil, ErrSoulBindingHostRegistryRejected
	}
	if strings.TrimSpace(identity.AuthorityModel) != SoulAuthorityModelInstanceTrust {
		return nil, ErrSoulBindingHostRegistryRejected
	}
	if strings.TrimSpace(identity.AnchorState) != SoulAnchorStateHostedOffchain {
		return nil, ErrSoulBindingHostRegistryRejected
	}
	if strings.TrimSpace(identity.OperationalBinding) != SoulOperationalBindingHostedBound {
		return nil, ErrSoulBindingHostRegistryRejected
	}
	if !identityIsActive(identity) {
		return nil, ErrSoulBindingHostRegistryRejected
	}
	if !hostSoulIdentityHasPublicationEvidence(identity) {
		return nil, ErrSoulBindingHostRegistryRejected
	}

	return identity, nil
}

func hostSoulIdentityHasPublicationEvidence(identity *hostSoulIdentity) bool {
	if identity == nil {
		return false
	}
	if identity.PublishedVersion > 0 {
		return true
	}
	return identity.SelfDescriptionVersion != nil && *identity.SelfDescriptionVersion > 0
}

func hashSoulBindingPayload(input BindSoulBodyInput) (string, error) {
	payload := normalizedSoulBindingPayload{
		ActorUsername:      strings.TrimSpace(input.ActorUsername),
		SoulAgentID:        strings.ToLower(strings.TrimSpace(input.SoulAgentID)),
		BodyActorID:        strings.TrimSpace(input.BodyActorID),
		HostRegistrationID: strings.TrimSpace(input.HostRegistrationID),
		HostConversationID: strings.TrimSpace(input.HostConversationID),
		AuthorityModel:     strings.TrimSpace(input.AuthorityModel),
		AnchorState:        strings.TrimSpace(input.AnchorState),
		OperationalBinding: strings.TrimSpace(input.OperationalBinding),
		PrincipalAddress:   strings.ToLower(strings.TrimSpace(input.PrincipalAddressHint)),
		Evidence: normalizedSoulBindingEvidence{
			Source:          strings.TrimSpace(input.Evidence.Source),
			HostRequestID:   strings.TrimSpace(input.Evidence.HostRequestID),
			DeclarationHash: strings.TrimSpace(input.Evidence.DeclarationHash),
			IssuedAt:        strings.TrimSpace(input.Evidence.IssuedAt),
		},
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

type normalizedSoulBindingPayload struct {
	ActorUsername      string                        `json:"actor_username"`
	SoulAgentID        string                        `json:"soul_agent_id"`
	BodyActorID        string                        `json:"body_actor_id,omitempty"`
	HostRegistrationID string                        `json:"host_registration_id,omitempty"`
	HostConversationID string                        `json:"host_conversation_id,omitempty"`
	AuthorityModel     string                        `json:"authority_model,omitempty"`
	AnchorState        string                        `json:"anchor_state,omitempty"`
	OperationalBinding string                        `json:"operational_binding,omitempty"`
	PrincipalAddress   string                        `json:"principal_address,omitempty"`
	Evidence           normalizedSoulBindingEvidence `json:"evidence,omitempty"`
}

type normalizedSoulBindingEvidence struct {
	Source          string `json:"source,omitempty"`
	HostRequestID   string `json:"host_request_id,omitempty"`
	DeclarationHash string `json:"declaration_hash,omitempty"`
	IssuedAt        string `json:"issued_at,omitempty"`
}

func mapSoulBindingWriteError(err error) error {
	switch {
	case errors.Is(err, storageRepos.ErrSoulBodyBindingAlreadyExists):
		return ErrSoulAlreadyBound
	case errors.Is(err, storageRepos.ErrSoulBodyAlreadyHasBinding):
		return ErrTargetAgentAlreadyHasSoul
	default:
		return err
	}
}

func mapSoulBindingHostError(err error) error {
	var hostErr *HostBootstrapError
	if errors.As(err, &hostErr) {
		switch {
		case errors.Is(hostErr.Err, ErrHostTrustNotConfigured),
			errors.Is(hostErr.Err, ErrHostInstanceKeyMissing),
			errors.Is(hostErr.Err, ErrHostInstanceKeyUnavailable),
			errors.Is(hostErr.Err, ErrHostUnavailable):
			switch hostErr.StatusCode {
			case http.StatusBadRequest, http.StatusNotFound, http.StatusConflict, http.StatusUnprocessableEntity:
				return ErrSoulBindingHostRegistryRejected
			default:
				return ErrSoulBindingHostRegistryUnavailable
			}
		default:
			return ErrSoulBindingHostRegistryUnavailable
		}
	}
	if errors.Is(err, ErrSoulNotAvailable) {
		return ErrSoulBindingHostRegistryRejected
	}
	return ErrSoulBindingHostRegistryUnavailable
}

func soulBindingReceiptStatusForError(err error) string {
	switch {
	case errors.Is(err, ErrSoulBindingHostRegistryUnavailable):
		return "host_unavailable"
	case errors.Is(err, ErrSoulAlreadyBound),
		errors.Is(err, ErrTargetAgentAlreadyHasSoul),
		errors.Is(err, ErrSoulBindingIdempotencyMismatch),
		errors.Is(err, ErrSoulBindingActorMismatch):
		return "conflict"
	case err != nil:
		return "rejected"
	default:
		return ""
	}
}

func normalizeSoulBindingCaller(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func firstPositiveInt(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}
