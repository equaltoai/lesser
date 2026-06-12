package graph

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	workflow "github.com/equaltoai/lesser/pkg/agents"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	soulservice "github.com/equaltoai/lesser/pkg/services/souls"
	"github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
)

const (
	soulBootstrapCorrelationOpBegin                = "begin"
	soulBootstrapCorrelationOpWalletVerification   = "wallet_verification"
	soulBootstrapCorrelationOpPrincipalDeclaration = "principal_declaration"
	soulBootstrapCorrelationOpConversation         = "conversation"
	soulBootstrapCorrelationOpFinalize             = "finalize"
)

func (r *queryResolver) SoulBootstrap(ctx context.Context, username string) (*model.SoulBootstrapSurface, error) {
	claims, err := r.requireAuthClaims(ctx)
	if err != nil {
		return nil, err
	}
	if err := requireDroneReadScope(claims); err != nil {
		return nil, err
	}

	agentUser, governance, err := r.loadDroneAgent(ctx, username)
	if err != nil {
		return nil, err
	}
	workflowState, err := r.loadDroneWorkflowState(agentUser)
	if err != nil {
		return nil, apperrors.InternalWithCause(err, "failed to decode drone workflow metadata")
	}
	if !r.canAccessDroneReviewWorkflow(ctx, claims, agentUser, workflowState) {
		return nil, apperrors.Forbidden("not authorized to view soul bootstrap")
	}
	return r.buildSoulBootstrapSurface(ctx, claims.Username, agentUser, governance, nil)
}

func (r *mutationResolver) BeginSoulBootstrap(ctx context.Context, input model.BeginSoulBootstrapInput) (*model.SoulBootstrapMutationPayload, error) {
	correlation := graphBootstrapCorrelation(
		input.CorrelationKey,
		input.IdempotencyKey,
		soulBootstrapCorrelationOpBegin,
	)
	return r.executeSoulBootstrapMutation(ctx, input.Username, func(
		ctx context.Context,
		agentUser *storage.User,
		_ *storage.AgentGovernanceState,
		service soulBootstrapHostService,
		existing *workflow.SoulBootstrapState,
		now time.Time,
	) (*workflow.SoulBootstrapState, error) {
		bodyID := strings.TrimSpace(agentUser.ID)
		if bodyID == "" {
			bodyID = agentUser.Username
		}
		result, err := service.BeginBootstrapRegistration(ctx, soulservice.BootstrapBeginInput{
			Username:      agentUser.Username,
			BodyID:        bodyID,
			WalletAddress: input.WalletAddress,
			Capabilities:  input.Capabilities,
		})
		if err != nil {
			return soulBootstrapErrorState(agentUser.Username, correlation, err, now), nil
		}
		return soulBootstrapStateAfterBegin(agentUser, existing, correlation, result, now), nil
	})
}

func (r *mutationResolver) VerifySoulBootstrapWallet(ctx context.Context, input model.VerifySoulBootstrapWalletInput) (*model.SoulBootstrapMutationPayload, error) {
	correlation := graphBootstrapCorrelation(
		input.CorrelationKey,
		input.IdempotencyKey,
		soulBootstrapCorrelationOpWalletVerification,
	)
	return r.executeSoulBootstrapMutation(ctx, input.Username, func(
		_ context.Context,
		agentUser *storage.User,
		_ *storage.AgentGovernanceState,
		_ soulBootstrapHostService,
		existing *workflow.SoulBootstrapState,
		now time.Time,
	) (*workflow.SoulBootstrapState, error) {
		state := workflow.NormalizeSoulBootstrap(existing, agentUser.Username)
		state.Phase = workflow.SoulBootstrapPhaseWalletVerification
		state.State = "wallet_verification.signed"
		state.Correlation = mergeSoulBootstrapCorrelation(state.Correlation, correlation)
		state.SigningCheckpoints = upsertSoulBootstrapCheckpoint(state.SigningCheckpoints, workflow.SoulBootstrapSigningCheckpoint{
			Name:        "wallet",
			Status:      "signed",
			CompletedAt: &now,
		})
		state.Error = nil
		state.UpdatedAt = &now
		return workflow.NormalizeSoulBootstrap(state, agentUser.Username), nil
	})
}

func (r *mutationResolver) PrepareSoulBootstrapPrincipalDeclaration(ctx context.Context, input model.PrepareSoulBootstrapPrincipalDeclarationInput) (*model.SoulBootstrapMutationPayload, error) {
	correlation := graphBootstrapCorrelation(
		input.CorrelationKey,
		input.IdempotencyKey,
		soulBootstrapCorrelationOpPrincipalDeclaration,
	)
	return r.executeSoulBootstrapMutation(ctx, input.Username, func(
		ctx context.Context,
		agentUser *storage.User,
		_ *storage.AgentGovernanceState,
		service soulBootstrapHostService,
		existing *workflow.SoulBootstrapState,
		now time.Time,
	) (*workflow.SoulBootstrapState, error) {
		registrationID := strings.TrimSpace(derefString(input.RegistrationID))
		if registrationID == "" && existing != nil {
			registrationID = strings.TrimSpace(existing.HostRegistrationID)
		}
		result, err := service.PrepareBootstrapPrincipalDeclaration(ctx, soulservice.BootstrapPrincipalPreflightInput{
			RegistrationID:       registrationID,
			PrincipalAddress:     input.PrincipalAddress,
			PrincipalDeclaration: input.PrincipalDeclaration,
			DeclaredAt:           time.Time(input.DeclaredAt),
		})
		if err != nil {
			return soulBootstrapErrorState(agentUser.Username, correlation, err, now), nil
		}
		return soulBootstrapStateAfterPrincipalPreflight(agentUser, existing, correlation, registrationID, result, now), nil
	})
}

func (r *mutationResolver) VerifySoulBootstrapPrincipalDeclaration(ctx context.Context, input model.VerifySoulBootstrapPrincipalDeclarationInput) (*model.SoulBootstrapMutationPayload, error) {
	correlation := graphBootstrapCorrelation(
		input.CorrelationKey,
		input.IdempotencyKey,
		soulBootstrapCorrelationOpPrincipalDeclaration,
	)
	return r.executeSoulBootstrapMutation(ctx, input.Username, func(
		ctx context.Context,
		agentUser *storage.User,
		_ *storage.AgentGovernanceState,
		service soulBootstrapHostService,
		existing *workflow.SoulBootstrapState,
		now time.Time,
	) (*workflow.SoulBootstrapState, error) {
		registrationID := strings.TrimSpace(derefString(input.RegistrationID))
		if registrationID == "" && existing != nil {
			registrationID = strings.TrimSpace(existing.HostRegistrationID)
		}
		result, err := service.VerifyBootstrapPrincipalDeclaration(ctx, soulservice.BootstrapPrincipalVerifyInput{
			RegistrationID:       registrationID,
			WalletSignature:      input.Signature,
			PrincipalAddress:     input.PrincipalAddress,
			PrincipalDeclaration: input.PrincipalDeclaration,
			PrincipalSignature:   input.PrincipalSignature,
			DeclaredAt:           time.Time(input.DeclaredAt),
		})
		if err != nil {
			return soulBootstrapErrorState(agentUser.Username, correlation, err, now), nil
		}
		return soulBootstrapStateAfterPrincipalVerify(agentUser, existing, correlation, registrationID, result, now), nil
	})
}

func (r *mutationResolver) SendSoulBootstrapConversationMessage(ctx context.Context, input model.SendSoulBootstrapConversationMessageInput) (*model.SoulBootstrapMutationPayload, error) {
	return r.soulBootstrapHostBridgeUnavailablePayload(ctx, input.Username, graphBootstrapCorrelation(
		input.CorrelationKey,
		input.IdempotencyKey,
		soulBootstrapCorrelationOpConversation,
	))
}

func (r *mutationResolver) CompleteSoulBootstrapConversation(ctx context.Context, input model.CompleteSoulBootstrapConversationInput) (*model.SoulBootstrapMutationPayload, error) {
	return r.soulBootstrapHostBridgeUnavailablePayload(ctx, input.Username, graphBootstrapCorrelation(
		input.CorrelationKey,
		input.IdempotencyKey,
		soulBootstrapCorrelationOpConversation,
	))
}

func (r *mutationResolver) PrepareSoulBootstrapFinalize(ctx context.Context, input model.PrepareSoulBootstrapFinalizeInput) (*model.SoulBootstrapMutationPayload, error) {
	return r.soulBootstrapHostBridgeUnavailablePayload(ctx, input.Username, graphBootstrapCorrelation(
		input.CorrelationKey,
		input.IdempotencyKey,
		soulBootstrapCorrelationOpFinalize,
	))
}

func (r *mutationResolver) FinalizeSoulBootstrap(ctx context.Context, input model.FinalizeSoulBootstrapInput) (*model.SoulBootstrapMutationPayload, error) {
	return r.soulBootstrapHostBridgeUnavailablePayload(ctx, input.Username, graphBootstrapCorrelation(
		input.CorrelationKey,
		input.IdempotencyKey,
		soulBootstrapCorrelationOpFinalize,
	))
}

func (r *mutationResolver) soulBootstrapHostBridgeUnavailablePayload(
	ctx context.Context,
	username string,
	correlation *workflow.SoulBootstrapCorrelationState,
) (*model.SoulBootstrapMutationPayload, error) {
	claims, err := r.requireAuthClaims(ctx)
	if err != nil {
		return nil, err
	}
	if err := requireDroneWriteScope(claims); err != nil {
		return nil, err
	}

	agentUser, governance, err := r.loadDroneAgent(ctx, username)
	if err != nil {
		return nil, err
	}
	if !r.canViewDroneWorkflow(ctx, claims, agentUser) {
		return nil, apperrors.Forbidden("not authorized to mutate soul bootstrap")
	}

	now := time.Now().UTC()
	unavailable := workflow.NewSoulBootstrapHostBridgeUnavailableState(agentUser.Username, correlation, now)
	surface, err := r.buildSoulBootstrapSurface(ctx, claims.Username, agentUser, governance, unavailable)
	if err != nil {
		return nil, err
	}
	return &model.SoulBootstrapMutationPayload{
		Bootstrap:  surface,
		Executable: false,
		Error:      surface.Error,
	}, nil
}

type soulBootstrapHostService interface {
	BeginBootstrapRegistration(context.Context, soulservice.BootstrapBeginInput) (*soulservice.BootstrapBeginResult, error)
	PrepareBootstrapPrincipalDeclaration(context.Context, soulservice.BootstrapPrincipalPreflightInput) (*soulservice.BootstrapPrincipalPreflightResult, error)
	VerifyBootstrapPrincipalDeclaration(context.Context, soulservice.BootstrapPrincipalVerifyInput) (*soulservice.BootstrapPrincipalVerifyResult, error)
}

type soulBootstrapMutationFunc func(
	context.Context,
	*storage.User,
	*storage.AgentGovernanceState,
	soulBootstrapHostService,
	*workflow.SoulBootstrapState,
	time.Time,
) (*workflow.SoulBootstrapState, error)

func (r *mutationResolver) executeSoulBootstrapMutation(
	ctx context.Context,
	username string,
	mutate soulBootstrapMutationFunc,
) (*model.SoulBootstrapMutationPayload, error) {
	claims, err := r.requireAuthClaims(ctx)
	if err != nil {
		return nil, err
	}
	if err := requireDroneWriteScope(claims); err != nil {
		return nil, err
	}

	agentUser, governance, err := r.loadDroneAgent(ctx, username)
	if err != nil {
		return nil, err
	}
	if !r.canViewDroneWorkflow(ctx, claims, agentUser) {
		return nil, apperrors.Forbidden("not authorized to mutate soul bootstrap")
	}

	service, err := r.soulBootstrapService()
	if err != nil {
		return nil, apperrors.InternalWithCause(err, "failed to initialize soul bootstrap service")
	}

	workflowState, err := r.loadDroneWorkflowState(agentUser)
	if err != nil {
		return nil, apperrors.InternalWithCause(err, "failed to decode drone workflow metadata")
	}
	existing := (*workflow.SoulBootstrapState)(nil)
	if workflowState != nil {
		existing = workflowState.SoulBootstrap
	}

	now := time.Now().UTC()
	nextState, err := mutate(ctx, agentUser, governance, service, existing, now)
	if err != nil {
		return nil, err
	}
	if workflowState == nil {
		workflowState = &workflow.DroneWorkflowState{}
	}
	workflowState.SoulBootstrap = workflow.NormalizeSoulBootstrap(nextState, agentUser.Username)
	workflowState.UpdatedAt = &now
	if err := r.persistDroneWorkflowState(ctx, agentUser, workflowState); err != nil {
		return nil, err
	}

	surface, err := r.buildSoulBootstrapSurface(ctx, claims.Username, agentUser, governance, nil)
	if err != nil {
		return nil, err
	}
	return &model.SoulBootstrapMutationPayload{
		Bootstrap:  surface,
		Executable: surface.Executable,
		Error:      surface.Error,
	}, nil
}

func (r *Resolver) soulBootstrapService() (soulBootstrapHostService, error) {
	service, err := r.getSoulService()
	if err != nil {
		return nil, err
	}
	bootstrap, ok := service.(soulBootstrapHostService)
	if !ok {
		return nil, errors.New("soul bootstrap service is not available")
	}
	return bootstrap, nil
}

func soulBootstrapStateAfterBegin(
	agentUser *storage.User,
	existing *workflow.SoulBootstrapState,
	correlation *workflow.SoulBootstrapCorrelationState,
	result *soulservice.BootstrapBeginResult,
	now time.Time,
) *workflow.SoulBootstrapState {
	username := ""
	bodyID := ""
	if agentUser != nil {
		username = agentUser.Username
		bodyID = strings.TrimSpace(agentUser.ID)
	}
	if bodyID == "" {
		bodyID = username
	}

	state := workflow.NormalizeSoulBootstrap(existing, username)
	state.Username = username
	state.BodyID = bodyID
	state.Phase = workflow.SoulBootstrapPhaseBegin
	state.State = "begin.ready"
	state.HostRegistrationID = strings.TrimSpace(result.RegistrationID)
	state.HostSoulAgentID = strings.TrimSpace(result.HostSoulAgentID)
	state.WalletAddress = strings.ToLower(strings.TrimSpace(result.WalletAddress))
	state.Correlation = mergeSoulBootstrapCorrelation(state.Correlation, correlation)
	if state.Correlation == nil {
		state.Correlation = &workflow.SoulBootstrapCorrelationState{}
	}
	state.Correlation.LastHostRequestID = strings.TrimSpace(result.HostRequestID)
	state.SigningCheckpoints = upsertSoulBootstrapCheckpoint(state.SigningCheckpoints, workflow.SoulBootstrapSigningCheckpoint{
		Version:         "1",
		Name:            "wallet",
		Status:          "ready",
		SignerAddress:   strings.ToLower(strings.TrimSpace(result.WalletChallenge.Address)),
		SigningMethod:   "eip191_personal_sign",
		MessageEncoding: "utf8",
		Message:         result.WalletChallenge.Message,
		HostRequestID:   result.HostRequestID,
		IssuedAt:        result.WalletChallenge.IssuedAt,
	})
	state.Error = nil
	state.UpdatedAt = &now
	return workflow.NormalizeSoulBootstrap(state, username)
}

func soulBootstrapStateAfterPrincipalPreflight(
	agentUser *storage.User,
	existing *workflow.SoulBootstrapState,
	correlation *workflow.SoulBootstrapCorrelationState,
	registrationID string,
	result *soulservice.BootstrapPrincipalPreflightResult,
	now time.Time,
) *workflow.SoulBootstrapState {
	username := ""
	if agentUser != nil {
		username = agentUser.Username
	}
	state := workflow.NormalizeSoulBootstrap(existing, username)
	state.Phase = workflow.SoulBootstrapPhasePrincipalDeclaration
	state.State = "principal_declaration.pending"
	if strings.TrimSpace(state.HostRegistrationID) == "" {
		state.HostRegistrationID = strings.TrimSpace(registrationID)
	}
	state.PrincipalAddress = strings.ToLower(strings.TrimSpace(result.PrincipalAddress))
	state.Correlation = mergeSoulBootstrapCorrelation(state.Correlation, correlation)
	if state.Correlation == nil {
		state.Correlation = &workflow.SoulBootstrapCorrelationState{}
	}
	state.Correlation.LastHostRequestID = strings.TrimSpace(result.HostRequestID)
	state.SigningCheckpoints = upsertSoulBootstrapCheckpoint(state.SigningCheckpoints, workflow.SoulBootstrapSigningCheckpoint{
		Version:          result.Version,
		Name:             "principal_declaration",
		Status:           "ready",
		PrincipalAddress: result.PrincipalAddress,
		SignerAddress:    result.SignerAddress,
		SigningMethod:    result.SigningMethod,
		MessageEncoding:  result.MessageEncoding,
		MessageHex:       result.MessageHex,
		DigestHex:        result.DigestHex,
		CanonicalJSON:    result.CanonicalJSON,
		HostRequestID:    result.HostRequestID,
		DeclaredAt:       result.DeclaredAt,
	})
	state.Error = nil
	state.UpdatedAt = &now
	return workflow.NormalizeSoulBootstrap(state, username)
}

func soulBootstrapStateAfterPrincipalVerify(
	agentUser *storage.User,
	existing *workflow.SoulBootstrapState,
	correlation *workflow.SoulBootstrapCorrelationState,
	registrationID string,
	result *soulservice.BootstrapPrincipalVerifyResult,
	now time.Time,
) *workflow.SoulBootstrapState {
	username := ""
	if agentUser != nil {
		username = agentUser.Username
	}
	state := workflow.NormalizeSoulBootstrap(existing, username)
	state.Phase = workflow.SoulBootstrapPhaseConversation
	state.State = "conversation.pending"
	if strings.TrimSpace(result.RegistrationID) != "" {
		state.HostRegistrationID = strings.TrimSpace(result.RegistrationID)
	} else if strings.TrimSpace(state.HostRegistrationID) == "" {
		state.HostRegistrationID = strings.TrimSpace(registrationID)
	}
	if strings.TrimSpace(result.HostSoulAgentID) != "" {
		state.HostSoulAgentID = strings.TrimSpace(result.HostSoulAgentID)
	}
	if strings.TrimSpace(result.WalletAddress) != "" {
		state.WalletAddress = strings.ToLower(strings.TrimSpace(result.WalletAddress))
	}
	if strings.TrimSpace(result.PrincipalAddress) != "" {
		state.PrincipalAddress = strings.ToLower(strings.TrimSpace(result.PrincipalAddress))
	}
	state.Correlation = mergeSoulBootstrapCorrelation(state.Correlation, correlation)
	if state.Correlation == nil {
		state.Correlation = &workflow.SoulBootstrapCorrelationState{}
	}
	state.Correlation.LastHostRequestID = strings.TrimSpace(result.HostRequestID)
	state.SigningCheckpoints = upsertSoulBootstrapCheckpoint(state.SigningCheckpoints, workflow.SoulBootstrapSigningCheckpoint{
		Name:          "principal_declaration",
		Status:        "verified",
		HostRequestID: result.HostRequestID,
		CompletedAt:   &now,
	})
	state.Error = nil
	state.UpdatedAt = &now
	return workflow.NormalizeSoulBootstrap(state, username)
}

func soulBootstrapErrorState(username string, correlation *workflow.SoulBootstrapCorrelationState, err error, now time.Time) *workflow.SoulBootstrapState {
	var hostErr *soulservice.HostBootstrapError
	if errors.As(err, &hostErr) {
		return workflow.NewSoulBootstrapErrorState(
			username,
			correlation,
			hostErr.Code,
			hostErr.Message,
			hostErr.Source,
			hostErr.StatusCode,
			hostErr.HostRequestID,
			now,
		)
	}
	code := workflow.SoulBootstrapErrorHostUnavailable
	if errors.Is(err, soulservice.ErrHostTrustNotConfigured) {
		code = workflow.SoulBootstrapErrorHostTrustNotConfigured
	}
	return workflow.NewSoulBootstrapErrorState(username, correlation, code, "Soul bootstrap Host bridge is unavailable.", "lesser", 0, "", now)
}

func mergeSoulBootstrapCorrelation(existing *workflow.SoulBootstrapCorrelationState, next *workflow.SoulBootstrapCorrelationState) *workflow.SoulBootstrapCorrelationState {
	if existing == nil && next == nil {
		return nil
	}
	out := &workflow.SoulBootstrapCorrelationState{}
	if existing != nil {
		*out = *existing
	}
	if next == nil {
		return out
	}
	if strings.TrimSpace(next.CorrelationKey) != "" {
		out.CorrelationKey = strings.TrimSpace(next.CorrelationKey)
	}
	if strings.TrimSpace(next.BeginIdempotencyKey) != "" {
		out.BeginIdempotencyKey = strings.TrimSpace(next.BeginIdempotencyKey)
	}
	if strings.TrimSpace(next.WalletVerificationIdempotencyKey) != "" {
		out.WalletVerificationIdempotencyKey = strings.TrimSpace(next.WalletVerificationIdempotencyKey)
	}
	if strings.TrimSpace(next.PrincipalDeclarationIdempotencyKey) != "" {
		out.PrincipalDeclarationIdempotencyKey = strings.TrimSpace(next.PrincipalDeclarationIdempotencyKey)
	}
	if strings.TrimSpace(next.ConversationIdempotencyKey) != "" {
		out.ConversationIdempotencyKey = strings.TrimSpace(next.ConversationIdempotencyKey)
	}
	if strings.TrimSpace(next.FinalizeIdempotencyKey) != "" {
		out.FinalizeIdempotencyKey = strings.TrimSpace(next.FinalizeIdempotencyKey)
	}
	if strings.TrimSpace(next.LastHostRequestID) != "" {
		out.LastHostRequestID = strings.TrimSpace(next.LastHostRequestID)
	}
	return out
}

func upsertSoulBootstrapCheckpoint(existing []workflow.SoulBootstrapSigningCheckpoint, next workflow.SoulBootstrapSigningCheckpoint) []workflow.SoulBootstrapSigningCheckpoint {
	name := strings.TrimSpace(next.Name)
	if name == "" {
		return existing
	}
	out := make([]workflow.SoulBootstrapSigningCheckpoint, 0, len(existing)+1)
	replaced := false
	for _, checkpoint := range existing {
		if strings.EqualFold(strings.TrimSpace(checkpoint.Name), name) {
			out = append(out, mergeSoulBootstrapCheckpoint(checkpoint, next))
			replaced = true
			continue
		}
		out = append(out, checkpoint)
	}
	if !replaced {
		out = append(out, next)
	}
	return out
}

func mergeSoulBootstrapCheckpoint(existing workflow.SoulBootstrapSigningCheckpoint, next workflow.SoulBootstrapSigningCheckpoint) workflow.SoulBootstrapSigningCheckpoint {
	merged := existing
	if strings.TrimSpace(next.Version) != "" {
		merged.Version = next.Version
	}
	if strings.TrimSpace(next.Name) != "" {
		merged.Name = next.Name
	}
	if strings.TrimSpace(next.Status) != "" {
		merged.Status = next.Status
	}
	if strings.TrimSpace(next.PrincipalAddress) != "" {
		merged.PrincipalAddress = next.PrincipalAddress
	}
	if strings.TrimSpace(next.SignerAddress) != "" {
		merged.SignerAddress = next.SignerAddress
	}
	if strings.TrimSpace(next.SigningMethod) != "" {
		merged.SigningMethod = next.SigningMethod
	}
	if strings.TrimSpace(next.MessageEncoding) != "" {
		merged.MessageEncoding = next.MessageEncoding
	}
	if strings.TrimSpace(next.Message) != "" {
		merged.Message = next.Message
	}
	if strings.TrimSpace(next.MessageHex) != "" {
		merged.MessageHex = next.MessageHex
	}
	if strings.TrimSpace(next.DigestHex) != "" {
		merged.DigestHex = next.DigestHex
	}
	if strings.TrimSpace(next.CanonicalJSON) != "" {
		merged.CanonicalJSON = next.CanonicalJSON
	}
	if strings.TrimSpace(next.HostRequestID) != "" {
		merged.HostRequestID = next.HostRequestID
	}
	if next.IssuedAt != nil {
		merged.IssuedAt = next.IssuedAt
	}
	if next.DeclaredAt != nil {
		merged.DeclaredAt = next.DeclaredAt
	}
	if next.CompletedAt != nil {
		merged.CompletedAt = next.CompletedAt
	}
	return merged
}

func (r *Resolver) buildSoulBootstrapSurface(
	ctx context.Context,
	viewerUsername string,
	agentUser *storage.User,
	governance *storage.AgentGovernanceState,
	overrideState *workflow.SoulBootstrapState,
) (*model.SoulBootstrapSurface, error) {
	if agentUser == nil {
		return nil, nil
	}
	workflowState, err := r.loadDroneWorkflowState(agentUser)
	if err != nil {
		return nil, apperrors.InternalWithCause(err, "failed to decode drone workflow metadata")
	}
	binding, err := r.lookupDroneSoulBinding(ctx, agentUser.Username)
	if err != nil {
		return nil, apperrors.InternalWithCause(err, "failed to load soul body binding")
	}
	surface, _, err := r.buildDroneWorkflowSurface(ctx, viewerUsername, agentUser, governance)
	if err != nil {
		return nil, err
	}

	state := graphSoulBootstrapStateModel(workflowState, agentUser, binding)
	if overrideState != nil {
		state = graphSoulBootstrapStoredStateModel(overrideState)
	}

	existingSoulAgentID := ""
	soulBindingState := model.SoulBindingStateUnbound
	if binding != nil {
		existingSoulAgentID = binding.AgentID
		soulBindingState = model.SoulBindingStateBound
	}
	if existingSoulAgentID == "" && state != nil && state.HostSoulAgentID != nil {
		existingSoulAgentID = strings.TrimSpace(*state.HostSoulAgentID)
	}

	hostBridgeAvailable := state != nil && state.Error == nil && state.Phase != model.SoulBootstrapPhaseNotStarted
	return &model.SoulBootstrapSurface{
		Username:            agentUser.Username,
		Body:                graphSoulBootstrapIdentityTarget(ctx, r, agentUser),
		Workflow:            surface,
		State:               state,
		SoulBindingState:    soulBindingState,
		ExistingSoulAgentID: optionalString(existingSoulAgentID),
		HostBridgeAvailable: hostBridgeAvailable,
		Executable:          hostBridgeAvailable && soulBindingState != model.SoulBindingStateBound,
		NextAction:          graphSoulBootstrapNextAction(state, soulBindingState),
		Error:               graphSoulBootstrapSurfaceError(state),
	}, nil
}

func graphSoulBootstrapIdentityTarget(ctx context.Context, r *Resolver, agentUser *storage.User) *model.SoulBootstrapIdentityTarget {
	if agentUser == nil {
		return nil
	}
	bodyID := strings.TrimSpace(agentUser.ID)
	if bodyID == "" {
		bodyID = agentUser.Username
	}
	return &model.SoulBootstrapIdentityTarget{
		Username:    agentUser.Username,
		BodyID:      bodyID,
		DisplayName: optionalString(agentUser.DisplayName),
		Owner:       r.graphDroneOwnerActor(ctx, agentUser),
	}
}

func graphSoulBootstrapStateModel(
	workflowState *workflow.DroneWorkflowState,
	agentUser *storage.User,
	binding *storagemodels.InstanceSoulBodyBinding,
) *model.SoulBootstrapState {
	username := ""
	var stored *workflow.SoulBootstrapState
	if agentUser != nil {
		username = agentUser.Username
	}
	if workflowState != nil {
		stored = workflowState.SoulBootstrap
	}

	state := workflow.NormalizeSoulBootstrap(stored, username)
	if binding != nil {
		if state.HostSoulAgentID == "" {
			state.HostSoulAgentID = binding.AgentID
		}
		if state.PrincipalAddress == "" {
			state.PrincipalAddress = binding.PrincipalAddress
		}
		state.Phase = workflow.SoulBootstrapPhaseComplete
		state.State = workflow.SoulBootstrapStateCompleteBound
		state.Error = nil
		if state.UpdatedAt == nil && !binding.UpdatedAt.IsZero() {
			updatedAt := binding.UpdatedAt.UTC()
			state.UpdatedAt = &updatedAt
		}
	}
	return graphSoulBootstrapStoredStateModel(state)
}

func graphSoulBootstrapStoredStateModel(state *workflow.SoulBootstrapState) *model.SoulBootstrapState {
	state = workflow.NormalizeSoulBootstrap(state, "")
	if state == nil {
		return nil
	}
	return &model.SoulBootstrapState{
		Username:           state.Username,
		BodyID:             state.BodyID,
		HostRegistrationID: optionalString(state.HostRegistrationID),
		HostConversationID: optionalString(state.HostConversationID),
		HostSoulAgentID:    optionalString(state.HostSoulAgentID),
		WalletAddress:      optionalString(state.WalletAddress),
		PrincipalAddress:   optionalString(state.PrincipalAddress),
		Phase:              graphSoulBootstrapPhase(state.Phase),
		State:              state.State,
		SigningCheckpoints: graphSoulBootstrapSigningCheckpoints(state.SigningCheckpoints),
		Error:              graphSoulBootstrapError(state.Error),
		Correlation:        graphSoulBootstrapCorrelationModel(state.Correlation),
		UpdatedAt:          graphTimePtr(state.UpdatedAt),
	}
}

func graphSoulBootstrapPhase(phase string) model.SoulBootstrapPhase {
	switch strings.TrimSpace(phase) {
	case workflow.SoulBootstrapPhaseBegin:
		return model.SoulBootstrapPhase("BEGIN")
	case workflow.SoulBootstrapPhaseWalletVerification:
		return model.SoulBootstrapPhase("WALLET_VERIFICATION")
	case workflow.SoulBootstrapPhasePrincipalDeclaration:
		return model.SoulBootstrapPhase("PRINCIPAL_DECLARATION")
	case workflow.SoulBootstrapPhaseConversation:
		return model.SoulBootstrapPhase("CONVERSATION")
	case workflow.SoulBootstrapPhaseFinalize:
		return model.SoulBootstrapPhase("FINALIZE")
	case workflow.SoulBootstrapPhaseComplete:
		return model.SoulBootstrapPhase("COMPLETE")
	case workflow.SoulBootstrapPhaseError:
		return model.SoulBootstrapPhase("ERROR")
	default:
		return model.SoulBootstrapPhase("NOT_STARTED")
	}
}

func graphSoulBootstrapSigningCheckpoints(in []workflow.SoulBootstrapSigningCheckpoint) []*model.SoulBootstrapSigningCheckpoint {
	if len(in) == 0 {
		return []*model.SoulBootstrapSigningCheckpoint{}
	}
	out := make([]*model.SoulBootstrapSigningCheckpoint, 0, len(in))
	for _, item := range in {
		out = append(out, &model.SoulBootstrapSigningCheckpoint{
			Version:          optionalString(item.Version),
			Name:             item.Name,
			Status:           item.Status,
			PrincipalAddress: optionalString(item.PrincipalAddress),
			SignerAddress:    optionalString(item.SignerAddress),
			SigningMethod:    optionalString(item.SigningMethod),
			MessageEncoding:  optionalString(item.MessageEncoding),
			Message:          optionalString(item.Message),
			MessageHex:       optionalString(item.MessageHex),
			DigestHex:        optionalString(item.DigestHex),
			CanonicalJSON:    optionalString(item.CanonicalJSON),
			HostRequestID:    optionalString(item.HostRequestID),
			IssuedAt:         graphTimePtr(item.IssuedAt),
			DeclaredAt:       graphTimePtr(item.DeclaredAt),
			CompletedAt:      graphTimePtr(item.CompletedAt),
		})
	}
	return out
}

func graphSoulBootstrapError(in *workflow.SoulBootstrapErrorState) *model.SoulBootstrapErrorState {
	if in == nil {
		return nil
	}
	return &model.SoulBootstrapErrorState{
		Code:          defaultString(in.Code, workflow.SoulBootstrapErrorHostBridgeUnavailable),
		Message:       defaultString(in.Message, "Soul bootstrap is not executable yet."),
		Source:        optionalString(in.Source),
		StatusCode:    optionalInt(in.StatusCode),
		HostRequestID: optionalString(in.HostRequestID),
		At:            graphTimePtr(in.At),
	}
}

func graphSoulBootstrapCorrelationModel(in *workflow.SoulBootstrapCorrelationState) *model.SoulBootstrapCorrelationState {
	if in == nil {
		return nil
	}
	return &model.SoulBootstrapCorrelationState{
		CorrelationKey:                     optionalString(in.CorrelationKey),
		BeginIdempotencyKey:                optionalString(in.BeginIdempotencyKey),
		WalletVerificationIdempotencyKey:   optionalString(in.WalletVerificationIdempotencyKey),
		PrincipalDeclarationIdempotencyKey: optionalString(in.PrincipalDeclarationIdempotencyKey),
		ConversationIdempotencyKey:         optionalString(in.ConversationIdempotencyKey),
		FinalizeIdempotencyKey:             optionalString(in.FinalizeIdempotencyKey),
		LastHostRequestID:                  optionalString(in.LastHostRequestID),
	}
}

func graphSoulBootstrapSurfaceError(state *model.SoulBootstrapState) *model.SoulBootstrapErrorState {
	if state == nil {
		return nil
	}
	return state.Error
}

func graphSoulBootstrapNextAction(state *model.SoulBootstrapState, bindingState model.SoulBindingState) *string {
	if bindingState == model.SoulBindingStateBound {
		return optionalString("complete")
	}
	if state == nil {
		return optionalString("begin")
	}
	if state.Error != nil {
		return optionalString("wait_for_host_bridge")
	}
	switch state.Phase {
	case model.SoulBootstrapPhase("BEGIN"):
		return optionalString("verify_wallet")
	case model.SoulBootstrapPhase("WALLET_VERIFICATION"):
		return optionalString("prepare_principal_declaration")
	case model.SoulBootstrapPhase("PRINCIPAL_DECLARATION"):
		return optionalString("verify_principal_declaration")
	case model.SoulBootstrapPhase("CONVERSATION"):
		return optionalString("continue_conversation")
	case model.SoulBootstrapPhase("FINALIZE"):
		return optionalString("finalize")
	case model.SoulBootstrapPhase("COMPLETE"):
		return optionalString("complete")
	default:
		return optionalString("begin")
	}
}

func graphBootstrapCorrelation(correlationKey *string, idempotencyKey *string, operation string) *workflow.SoulBootstrapCorrelationState {
	correlation := &workflow.SoulBootstrapCorrelationState{
		CorrelationKey: strings.TrimSpace(derefString(correlationKey)),
	}
	switch operation {
	case soulBootstrapCorrelationOpBegin:
		correlation.BeginIdempotencyKey = strings.TrimSpace(derefString(idempotencyKey))
	case soulBootstrapCorrelationOpWalletVerification:
		correlation.WalletVerificationIdempotencyKey = strings.TrimSpace(derefString(idempotencyKey))
	case soulBootstrapCorrelationOpPrincipalDeclaration:
		correlation.PrincipalDeclarationIdempotencyKey = strings.TrimSpace(derefString(idempotencyKey))
	case soulBootstrapCorrelationOpConversation:
		correlation.ConversationIdempotencyKey = strings.TrimSpace(derefString(idempotencyKey))
	case soulBootstrapCorrelationOpFinalize:
		correlation.FinalizeIdempotencyKey = strings.TrimSpace(derefString(idempotencyKey))
	}
	if correlation.CorrelationKey == "" &&
		correlation.BeginIdempotencyKey == "" &&
		correlation.WalletVerificationIdempotencyKey == "" &&
		correlation.PrincipalDeclarationIdempotencyKey == "" &&
		correlation.ConversationIdempotencyKey == "" &&
		correlation.FinalizeIdempotencyKey == "" {
		return nil
	}
	return correlation
}

func optionalInt(value int) *int {
	if value == 0 {
		return nil
	}
	return &value
}
