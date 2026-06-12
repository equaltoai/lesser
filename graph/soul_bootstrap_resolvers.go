package graph

import (
	"context"
	"encoding/json"
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
	soulBootstrapErrorSourceLesser                 = "lesser"
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
		bodyID := soulBootstrapBodyID(agentUser)
		result, err := service.BeginBootstrapRegistration(ctx, soulservice.BootstrapBeginInput{
			Username:      soulBootstrapHostLocalID(agentUser),
			BodyID:        bodyID,
			WalletAddress: input.WalletAddress,
			Capabilities:  input.Capabilities,
		})
		if err != nil {
			return soulBootstrapErrorState(agentUser, existing, correlation, err, now), nil
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
			return soulBootstrapErrorState(agentUser, existing, correlation, err, now), nil
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
			return soulBootstrapErrorState(agentUser, existing, correlation, err, now), nil
		}
		return soulBootstrapStateAfterPrincipalVerify(agentUser, existing, correlation, registrationID, result, now), nil
	})
}

func (r *mutationResolver) SendSoulBootstrapConversationMessage(ctx context.Context, input model.SendSoulBootstrapConversationMessageInput) (*model.SoulBootstrapMutationPayload, error) {
	correlation := graphBootstrapCorrelation(
		input.CorrelationKey,
		input.IdempotencyKey,
		soulBootstrapCorrelationOpConversation,
	)
	return r.executeSoulBootstrapReviewMutation(ctx, input.Username, func(
		ctx context.Context,
		agentUser *storage.User,
		_ *storage.AgentGovernanceState,
		service soulBootstrapHostService,
		existing *workflow.SoulBootstrapState,
		now time.Time,
	) (*workflow.SoulBootstrapState, error) {
		registrationID, err := soulBootstrapRegistrationID(existing, input.RegistrationID)
		if err != nil {
			return soulBootstrapErrorState(agentUser, existing, correlation, err, now), nil
		}
		conversationID, err := soulBootstrapOptionalConversationID(existing, input.ConversationID)
		if err != nil {
			return soulBootstrapErrorState(agentUser, existing, correlation, err, now), nil
		}
		result, err := service.SendBootstrapConversationMessage(ctx, soulservice.BootstrapConversationMessageInput{
			RegistrationID: registrationID,
			ConversationID: conversationID,
			Message:        input.Message,
			Model:          derefString(input.Model),
		})
		if err != nil {
			return soulBootstrapErrorState(agentUser, existing, correlation, err, now), nil
		}
		return soulBootstrapStateAfterConversationMessage(agentUser, existing, correlation, result, now), nil
	})
}

func (r *mutationResolver) CompleteSoulBootstrapConversation(ctx context.Context, input model.CompleteSoulBootstrapConversationInput) (*model.SoulBootstrapMutationPayload, error) {
	correlation := graphBootstrapCorrelation(
		input.CorrelationKey,
		input.IdempotencyKey,
		soulBootstrapCorrelationOpConversation,
	)
	return r.executeSoulBootstrapReviewMutation(ctx, input.Username, func(
		ctx context.Context,
		agentUser *storage.User,
		_ *storage.AgentGovernanceState,
		service soulBootstrapHostService,
		existing *workflow.SoulBootstrapState,
		now time.Time,
	) (*workflow.SoulBootstrapState, error) {
		registrationID, err := soulBootstrapRegistrationID(existing, input.RegistrationID)
		if err != nil {
			return soulBootstrapErrorState(agentUser, existing, correlation, err, now), nil
		}
		conversationID, err := soulBootstrapRequiredConversationID(existing, input.ConversationID)
		if err != nil {
			return soulBootstrapErrorState(agentUser, existing, correlation, err, now), nil
		}
		result, err := service.CompleteBootstrapConversation(ctx, soulservice.BootstrapConversationCompleteInput{
			RegistrationID: registrationID,
			ConversationID: conversationID,
		})
		if err != nil {
			return soulBootstrapErrorState(agentUser, existing, correlation, err, now), nil
		}
		return soulBootstrapStateAfterConversationComplete(agentUser, existing, correlation, result, now), nil
	})
}

func (r *mutationResolver) PrepareSoulBootstrapFinalize(ctx context.Context, input model.PrepareSoulBootstrapFinalizeInput) (*model.SoulBootstrapMutationPayload, error) {
	correlation := graphBootstrapCorrelation(
		input.CorrelationKey,
		input.IdempotencyKey,
		soulBootstrapCorrelationOpFinalize,
	)
	return r.executeSoulBootstrapReviewMutation(ctx, input.Username, func(
		ctx context.Context,
		agentUser *storage.User,
		_ *storage.AgentGovernanceState,
		service soulBootstrapHostService,
		existing *workflow.SoulBootstrapState,
		now time.Time,
	) (*workflow.SoulBootstrapState, error) {
		registrationID, err := soulBootstrapRegistrationID(existing, input.RegistrationID)
		if err != nil {
			return soulBootstrapErrorState(agentUser, existing, correlation, err, now), nil
		}
		conversationID, err := soulBootstrapRequiredConversationID(existing, input.ConversationID)
		if err != nil {
			return soulBootstrapErrorState(agentUser, existing, correlation, err, now), nil
		}
		boundarySignatures, err := graphBootstrapSignatureMap(input.BoundarySignaturesJSON)
		if err != nil {
			return soulBootstrapErrorState(agentUser, existing, correlation, err, now), nil
		}
		result, err := service.PrepareBootstrapFinalize(ctx, soulservice.BootstrapFinalizePreflightInput{
			RegistrationID:     registrationID,
			ConversationID:     conversationID,
			BoundarySignatures: boundarySignatures,
		})
		if err != nil {
			return soulBootstrapErrorState(agentUser, existing, correlation, err, now), nil
		}
		return soulBootstrapStateAfterFinalizePreflight(agentUser, existing, correlation, result, now), nil
	})
}

func (r *mutationResolver) FinalizeSoulBootstrap(ctx context.Context, input model.FinalizeSoulBootstrapInput) (*model.SoulBootstrapMutationPayload, error) {
	correlation := graphBootstrapCorrelation(
		input.CorrelationKey,
		input.IdempotencyKey,
		soulBootstrapCorrelationOpFinalize,
	)
	return r.executeSoulBootstrapReviewMutation(ctx, input.Username, func(
		ctx context.Context,
		agentUser *storage.User,
		_ *storage.AgentGovernanceState,
		service soulBootstrapHostService,
		existing *workflow.SoulBootstrapState,
		now time.Time,
	) (*workflow.SoulBootstrapState, error) {
		registrationID, err := soulBootstrapRegistrationID(existing, input.RegistrationID)
		if err != nil {
			return soulBootstrapErrorState(agentUser, existing, correlation, err, now), nil
		}
		conversationID, err := soulBootstrapRequiredConversationID(existing, input.ConversationID)
		if err != nil {
			return soulBootstrapErrorState(agentUser, existing, correlation, err, now), nil
		}
		boundarySignatures, err := graphBootstrapSignatureMap(input.BoundarySignaturesJSON)
		if err != nil {
			return soulBootstrapErrorState(agentUser, existing, correlation, err, now), nil
		}
		issuedAt, expectedVersion := soulBootstrapFinalizeDefaults(existing, input.IssuedAt, input.ExpectedVersion)
		result, err := service.FinalizeBootstrap(ctx, soulservice.BootstrapFinalizeInput{
			RegistrationID:     registrationID,
			ConversationID:     conversationID,
			BoundarySignatures: boundarySignatures,
			IssuedAt:           issuedAt,
			ExpectedVersion:    expectedVersion,
			SelfAttestation:    firstNonEmpty(derefString(input.SelfAttestation), derefString(input.Signature)),
		})
		if err != nil {
			return soulBootstrapErrorState(agentUser, existing, correlation, err, now), nil
		}
		published := soulBootstrapStateAfterFinalize(agentUser, existing, correlation, result, now)
		bindingPrincipal := soulBootstrapBindingPrincipalUsername(agentUser, "")
		if bindingPrincipal == "" {
			bindingPrincipal = soulBootstrapHostLocalID(agentUser)
		}
		soul, err := service.Incorporate(ctx, bindingPrincipal, agentUser.Username, result.HostSoulAgentID)
		if err != nil {
			return soulBootstrapErrorState(agentUser, published, correlation, err, now), nil
		}
		return soulBootstrapStateAfterBinding(agentUser, published, correlation, soul, result, now), nil
	})
}

type soulBootstrapHostService interface {
	Incorporate(context.Context, string, string, string) (*soulservice.Soul, error)
	BeginBootstrapRegistration(context.Context, soulservice.BootstrapBeginInput) (*soulservice.BootstrapBeginResult, error)
	PrepareBootstrapPrincipalDeclaration(context.Context, soulservice.BootstrapPrincipalPreflightInput) (*soulservice.BootstrapPrincipalPreflightResult, error)
	VerifyBootstrapPrincipalDeclaration(context.Context, soulservice.BootstrapPrincipalVerifyInput) (*soulservice.BootstrapPrincipalVerifyResult, error)
	SendBootstrapConversationMessage(context.Context, soulservice.BootstrapConversationMessageInput) (*soulservice.BootstrapConversationMessageResult, error)
	CompleteBootstrapConversation(context.Context, soulservice.BootstrapConversationCompleteInput) (*soulservice.BootstrapConversationCompleteResult, error)
	PrepareBootstrapFinalize(context.Context, soulservice.BootstrapFinalizePreflightInput) (*soulservice.BootstrapFinalizePreflightResult, error)
	FinalizeBootstrap(context.Context, soulservice.BootstrapFinalizeInput) (*soulservice.BootstrapFinalizeResult, error)
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
	return r.executeSoulBootstrapMutationWithAuth(ctx, username, false, mutate)
}

func (r *mutationResolver) executeSoulBootstrapReviewMutation(
	ctx context.Context,
	username string,
	mutate soulBootstrapMutationFunc,
) (*model.SoulBootstrapMutationPayload, error) {
	return r.executeSoulBootstrapMutationWithAuth(ctx, username, true, mutate)
}

func (r *mutationResolver) executeSoulBootstrapMutationWithAuth(
	ctx context.Context,
	username string,
	allowReviewer bool,
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
	workflowState, err := r.loadDroneWorkflowState(agentUser)
	if err != nil {
		return nil, apperrors.InternalWithCause(err, "failed to decode drone workflow metadata")
	}
	authorized := r.canViewDroneWorkflow(ctx, claims, agentUser)
	if allowReviewer {
		authorized = r.canReviewDroneWorkflow(ctx, claims, agentUser, workflowState)
	}
	if !authorized {
		return nil, apperrors.Forbidden("not authorized to mutate soul bootstrap")
	}

	service, err := r.soulBootstrapService()
	if err != nil {
		return nil, apperrors.InternalWithCause(err, "failed to initialize soul bootstrap service")
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
	applySoulBootstrapWorkflowProjection(ctx, r.Resolver, claims.Username, agentUser, workflowState, workflowState.SoulBootstrap, now)
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
		bodyID = soulBootstrapBodyID(agentUser)
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

func soulBootstrapStateAfterConversationMessage(
	agentUser *storage.User,
	existing *workflow.SoulBootstrapState,
	correlation *workflow.SoulBootstrapCorrelationState,
	result *soulservice.BootstrapConversationMessageResult,
	now time.Time,
) *workflow.SoulBootstrapState {
	username := ""
	if agentUser != nil {
		username = agentUser.Username
	}
	state := workflow.NormalizeSoulBootstrap(existing, username)
	state.Phase = workflow.SoulBootstrapPhaseConversation
	state.State = workflow.SoulBootstrapStateConversationInProgress
	if strings.TrimSpace(result.RegistrationID) != "" {
		state.HostRegistrationID = strings.TrimSpace(result.RegistrationID)
	}
	if strings.TrimSpace(result.ConversationID) != "" {
		state.HostConversationID = strings.TrimSpace(result.ConversationID)
	}
	state.Correlation = mergeSoulBootstrapCorrelation(state.Correlation, correlation)
	if state.Correlation == nil {
		state.Correlation = &workflow.SoulBootstrapCorrelationState{}
	}
	state.Correlation.LastHostRequestID = strings.TrimSpace(result.HostRequestID)
	state.SigningCheckpoints = upsertSoulBootstrapCheckpoint(state.SigningCheckpoints, workflow.SoulBootstrapSigningCheckpoint{
		Name:          "conversation",
		Status:        "in_progress",
		HostRequestID: result.HostRequestID,
		CompletedAt:   &now,
	})
	state.Error = nil
	state.UpdatedAt = &now
	return workflow.NormalizeSoulBootstrap(state, username)
}

func soulBootstrapStateAfterConversationComplete(
	agentUser *storage.User,
	existing *workflow.SoulBootstrapState,
	correlation *workflow.SoulBootstrapCorrelationState,
	result *soulservice.BootstrapConversationCompleteResult,
	now time.Time,
) *workflow.SoulBootstrapState {
	username := ""
	if agentUser != nil {
		username = agentUser.Username
	}
	state := workflow.NormalizeSoulBootstrap(existing, username)
	state.Phase = workflow.SoulBootstrapPhaseConversation
	state.State = workflow.SoulBootstrapStateConversationCompleted
	if strings.TrimSpace(result.RegistrationID) != "" {
		state.HostRegistrationID = strings.TrimSpace(result.RegistrationID)
	}
	if strings.TrimSpace(result.HostSoulAgentID) != "" {
		state.HostSoulAgentID = strings.TrimSpace(result.HostSoulAgentID)
	}
	if strings.TrimSpace(result.ConversationID) != "" {
		state.HostConversationID = strings.TrimSpace(result.ConversationID)
	}
	state.Correlation = mergeSoulBootstrapCorrelation(state.Correlation, correlation)
	if state.Correlation == nil {
		state.Correlation = &workflow.SoulBootstrapCorrelationState{}
	}
	state.Correlation.LastHostRequestID = strings.TrimSpace(result.HostRequestID)
	completedAt := result.CompletedAt
	if completedAt == nil {
		completedAt = &now
	}
	state.SigningCheckpoints = upsertSoulBootstrapCheckpoint(state.SigningCheckpoints, workflow.SoulBootstrapSigningCheckpoint{
		Name:          "conversation",
		Status:        strings.TrimSpace(defaultString(result.Status, "completed")),
		CanonicalJSON: strings.TrimSpace(result.ProducedDeclarations),
		HostRequestID: result.HostRequestID,
		CompletedAt:   completedAt,
	})
	state.Error = nil
	state.UpdatedAt = &now
	return workflow.NormalizeSoulBootstrap(state, username)
}

func soulBootstrapStateAfterFinalizePreflight(
	agentUser *storage.User,
	existing *workflow.SoulBootstrapState,
	correlation *workflow.SoulBootstrapCorrelationState,
	result *soulservice.BootstrapFinalizePreflightResult,
	now time.Time,
) *workflow.SoulBootstrapState {
	username := ""
	if agentUser != nil {
		username = agentUser.Username
	}
	state := workflow.NormalizeSoulBootstrap(existing, username)
	state.Phase = workflow.SoulBootstrapPhaseFinalize
	state.State = workflow.SoulBootstrapStateFinalizeReady
	state.Correlation = mergeSoulBootstrapCorrelation(state.Correlation, correlation)
	if state.Correlation == nil {
		state.Correlation = &workflow.SoulBootstrapCorrelationState{}
	}
	state.Correlation.LastHostRequestID = strings.TrimSpace(result.HostRequestID)
	state.SigningCheckpoints = upsertSoulBootstrapCheckpoint(state.SigningCheckpoints, workflow.SoulBootstrapSigningCheckpoint{
		Version:                     result.Version,
		Name:                        "finalize",
		Status:                      "ready",
		SignerAddress:               strings.ToLower(strings.TrimSpace(result.SelfAttestationSigning.SignerWallet)),
		SigningMethod:               result.SelfAttestationSigning.SigningMethod,
		MessageEncoding:             result.SelfAttestationSigning.MessageEncoding,
		MessageHex:                  result.SelfAttestationSigning.MessageHex,
		DigestHex:                   result.SelfAttestationSigning.DigestHex,
		CanonicalJSON:               result.SelfAttestationSigning.CanonicalJSON,
		ExpectedVersion:             result.ExpectedVersion,
		NextVersion:                 result.NextVersion,
		BoundaryRequirementsJSON:    result.BoundaryRequirementsJSON,
		FinalizeRequestTemplateJSON: result.FinalizeRequestTemplateJSON,
		RegistrationPreviewJSON:     result.RegistrationPreviewJSON,
		HostRequestID:               result.HostRequestID,
		IssuedAt:                    result.IssuedAt,
	})
	state.Error = nil
	state.UpdatedAt = &now
	return workflow.NormalizeSoulBootstrap(state, username)
}

func soulBootstrapStateAfterFinalize(
	agentUser *storage.User,
	existing *workflow.SoulBootstrapState,
	correlation *workflow.SoulBootstrapCorrelationState,
	result *soulservice.BootstrapFinalizeResult,
	now time.Time,
) *workflow.SoulBootstrapState {
	username := ""
	if agentUser != nil {
		username = agentUser.Username
	}
	state := workflow.NormalizeSoulBootstrap(existing, username)
	state.Phase = workflow.SoulBootstrapPhaseFinalize
	state.State = workflow.SoulBootstrapStateFinalizePublished
	if strings.TrimSpace(result.HostSoulAgentID) != "" {
		state.HostSoulAgentID = strings.TrimSpace(result.HostSoulAgentID)
	}
	if strings.TrimSpace(result.PrincipalAddress) != "" {
		state.PrincipalAddress = strings.ToLower(strings.TrimSpace(result.PrincipalAddress))
	}
	state.Publication = soulBootstrapPublicationEvidence(result.Publication)
	state.Correlation = mergeSoulBootstrapCorrelation(state.Correlation, correlation)
	if state.Correlation == nil {
		state.Correlation = &workflow.SoulBootstrapCorrelationState{}
	}
	state.Correlation.LastHostRequestID = strings.TrimSpace(result.HostRequestID)
	state.SigningCheckpoints = upsertSoulBootstrapCheckpoint(state.SigningCheckpoints, workflow.SoulBootstrapSigningCheckpoint{
		Version:       result.Version,
		Name:          "finalize",
		Status:        "published",
		HostRequestID: result.HostRequestID,
		CompletedAt:   &now,
	})
	state.Error = nil
	state.UpdatedAt = &now
	return workflow.NormalizeSoulBootstrap(state, username)
}

func soulBootstrapStateAfterBinding(
	agentUser *storage.User,
	existing *workflow.SoulBootstrapState,
	correlation *workflow.SoulBootstrapCorrelationState,
	soul *soulservice.Soul,
	result *soulservice.BootstrapFinalizeResult,
	now time.Time,
) *workflow.SoulBootstrapState {
	username := ""
	if agentUser != nil {
		username = agentUser.Username
	}
	state := workflow.NormalizeSoulBootstrap(existing, username)
	state.Phase = workflow.SoulBootstrapPhaseComplete
	state.State = workflow.SoulBootstrapStateCompleteBound
	if soul != nil && strings.TrimSpace(soul.AgentID) != "" {
		state.HostSoulAgentID = strings.TrimSpace(soul.AgentID)
	} else if result != nil && strings.TrimSpace(result.HostSoulAgentID) != "" {
		state.HostSoulAgentID = strings.TrimSpace(result.HostSoulAgentID)
	}
	if soul != nil && strings.TrimSpace(soul.PrincipalAddress) != "" {
		state.PrincipalAddress = strings.ToLower(strings.TrimSpace(soul.PrincipalAddress))
	} else if result != nil && strings.TrimSpace(result.PrincipalAddress) != "" {
		state.PrincipalAddress = strings.ToLower(strings.TrimSpace(result.PrincipalAddress))
	}
	if result != nil && state.Publication == nil {
		state.Publication = soulBootstrapPublicationEvidence(result.Publication)
	}
	state.Correlation = mergeSoulBootstrapCorrelation(state.Correlation, correlation)
	state.SigningCheckpoints = upsertSoulBootstrapCheckpoint(state.SigningCheckpoints, workflow.SoulBootstrapSigningCheckpoint{
		Name:        "binding",
		Status:      "bound",
		CompletedAt: &now,
	})
	state.Error = nil
	state.UpdatedAt = &now
	return workflow.NormalizeSoulBootstrap(state, username)
}

func soulBootstrapPublicationEvidence(in soulservice.BootstrapPublicationEvidence) *workflow.SoulBootstrapPublicationEvidence {
	if strings.TrimSpace(in.AgentID) == "" && in.PublishedVersion == 0 {
		return nil
	}
	return &workflow.SoulBootstrapPublicationEvidence{
		AgentID:                    strings.TrimSpace(in.AgentID),
		PublishedVersion:           in.PublishedVersion,
		RegistrationURI:            strings.TrimSpace(in.RegistrationURI),
		RegistrationS3Key:          strings.TrimSpace(in.RegistrationS3Key),
		VersionedRegistrationURI:   strings.TrimSpace(in.VersionedRegistrationURI),
		VersionedRegistrationS3Key: strings.TrimSpace(in.VersionedRegistrationS3Key),
		AnchorState:                strings.TrimSpace(in.AnchorState),
		PublishedAt:                in.PublishedAt,
	}
}

func soulBootstrapErrorState(
	agentUser *storage.User,
	existing *workflow.SoulBootstrapState,
	correlation *workflow.SoulBootstrapCorrelationState,
	err error,
	now time.Time,
) *workflow.SoulBootstrapState {
	username := ""
	bodyID := ""
	if agentUser != nil {
		username = agentUser.Username
		bodyID = soulBootstrapBodyID(agentUser)
	}
	state := workflow.NormalizeSoulBootstrap(existing, username)
	if state == nil {
		state = &workflow.SoulBootstrapState{}
	}
	details := soulBootstrapErrorDetails(err)
	state.Username = username
	state.BodyID = defaultString(state.BodyID, bodyID)
	state.Phase = workflow.SoulBootstrapPhaseError
	state.State = soulBootstrapErrorWorkflowState(details.Code)
	state.Correlation = mergeSoulBootstrapCorrelation(state.Correlation, correlation)
	state.Error = &workflow.SoulBootstrapErrorState{
		Code:          details.Code,
		Message:       details.Message,
		Source:        details.Source,
		StatusCode:    details.StatusCode,
		HostRequestID: details.HostRequestID,
		At:            &now,
	}
	state.UpdatedAt = &now
	return workflow.NormalizeSoulBootstrap(state, username)
}

type soulBootstrapErrorInfo struct {
	Code          string
	Message       string
	Source        string
	StatusCode    int
	HostRequestID string
}

func soulBootstrapErrorDetails(err error) soulBootstrapErrorInfo {
	var hostErr *soulservice.HostBootstrapError
	if errors.As(err, &hostErr) {
		return soulBootstrapErrorInfo{
			Code:          defaultString(hostErr.Code, workflow.SoulBootstrapErrorHostUnavailable),
			Message:       defaultString(hostErr.Message, "Soul bootstrap could not complete."),
			Source:        defaultString(hostErr.Source, soulBootstrapErrorSourceLesser),
			StatusCode:    hostErr.StatusCode,
			HostRequestID: strings.TrimSpace(hostErr.HostRequestID),
		}
	}
	code := workflow.SoulBootstrapErrorHostUnavailable
	message := "Soul bootstrap Host bridge is unavailable."
	source := soulBootstrapErrorSourceLesser
	if errors.Is(err, soulservice.ErrHostTrustNotConfigured) {
		code = workflow.SoulBootstrapErrorHostTrustNotConfigured
	}
	if errors.Is(err, soulservice.ErrHostInstanceKeyMissing) {
		code = workflow.SoulBootstrapErrorHostInstanceKeyMissing
	}
	if errors.Is(err, soulservice.ErrHostInstanceKeyUnavailable) {
		code = workflow.SoulBootstrapErrorHostInstanceKeyUnavailable
	}
	if errors.Is(err, soulservice.ErrHostSigningPayloadUnsupported) {
		code = workflow.SoulBootstrapErrorHostSigningPayloadUnsupported
		message = "Host returned unsupported or incomplete bootstrap signing material."
	}
	if errors.Is(err, soulservice.ErrSoulAlreadyBound) || errors.Is(err, soulservice.ErrTargetAgentAlreadyHasSoul) {
		code = workflow.SoulBootstrapErrorSoulBindingConflict
		message = "Soul binding conflicts with an existing local binding."
	}
	if errors.Is(err, soulservice.ErrSoulNotAvailable) {
		code = workflow.SoulBootstrapErrorSoulNotAvailable
		message = "Host-published soul is not available for this local body."
	}
	return soulBootstrapErrorInfo{Code: code, Message: message, Source: source}
}

func soulBootstrapErrorWorkflowState(code string) string {
	switch strings.TrimSpace(code) {
	case workflow.SoulBootstrapErrorHostTrustNotConfigured:
		return workflow.SoulBootstrapStateHostTrustNotConfigured
	case workflow.SoulBootstrapErrorHostInstanceKeyMissing:
		return workflow.SoulBootstrapStateHostInstanceKeyMissing
	case workflow.SoulBootstrapErrorHostInstanceKeyUnavailable:
		return workflow.SoulBootstrapStateHostInstanceKeyUnavailable
	case workflow.SoulBootstrapErrorHostSigningPayloadUnsupported:
		return workflow.SoulBootstrapStateHostSigningPayloadUnsupported
	case workflow.SoulBootstrapErrorHostRegistrationIDRequired,
		workflow.SoulBootstrapErrorHostConversationIDRequired,
		workflow.SoulBootstrapErrorHostBootstrapReplayRejected:
		return workflow.SoulBootstrapStateCorrelationMismatch
	case workflow.SoulBootstrapErrorSoulBindingConflict:
		return workflow.SoulBootstrapStateBindingConflict
	case workflow.SoulBootstrapErrorSoulNotAvailable:
		return workflow.SoulBootstrapStateSoulNotAvailable
	default:
		return workflow.SoulBootstrapStateHostUnavailable
	}
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
	if next.ExpectedVersion != 0 {
		merged.ExpectedVersion = next.ExpectedVersion
	}
	if next.NextVersion != 0 {
		merged.NextVersion = next.NextVersion
	}
	if strings.TrimSpace(next.BoundaryRequirementsJSON) != "" {
		merged.BoundaryRequirementsJSON = next.BoundaryRequirementsJSON
	}
	if strings.TrimSpace(next.FinalizeRequestTemplateJSON) != "" {
		merged.FinalizeRequestTemplateJSON = next.FinalizeRequestTemplateJSON
	}
	if strings.TrimSpace(next.RegistrationPreviewJSON) != "" {
		merged.RegistrationPreviewJSON = next.RegistrationPreviewJSON
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
	return &model.SoulBootstrapIdentityTarget{
		Username:    agentUser.Username,
		BodyID:      soulBootstrapBodyID(agentUser),
		DisplayName: optionalString(agentUser.DisplayName),
		Owner:       r.graphDroneOwnerActor(ctx, agentUser),
	}
}

func soulBootstrapBodyID(agentUser *storage.User) string {
	if agentUser == nil {
		return ""
	}
	bodyID := strings.TrimSpace(agentUser.ID)
	if bodyID == "" {
		bodyID = strings.TrimSpace(agentUser.Username)
	}
	return bodyID
}

func soulBootstrapHostLocalID(agentUser *storage.User) string {
	if agentUser == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(agentUser.Username))
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
		Publication:        graphSoulBootstrapPublication(state.Publication),
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
			Version:                     optionalString(item.Version),
			Name:                        item.Name,
			Status:                      item.Status,
			PrincipalAddress:            optionalString(item.PrincipalAddress),
			SignerAddress:               optionalString(item.SignerAddress),
			SigningMethod:               optionalString(item.SigningMethod),
			MessageEncoding:             optionalString(item.MessageEncoding),
			Message:                     optionalString(item.Message),
			MessageHex:                  optionalString(item.MessageHex),
			DigestHex:                   optionalString(item.DigestHex),
			CanonicalJSON:               optionalString(item.CanonicalJSON),
			ExpectedVersion:             optionalBootstrapExpectedVersion(item),
			NextVersion:                 optionalInt(item.NextVersion),
			BoundaryRequirementsJSON:    optionalString(item.BoundaryRequirementsJSON),
			FinalizeRequestTemplateJSON: optionalString(item.FinalizeRequestTemplateJSON),
			RegistrationPreviewJSON:     optionalString(item.RegistrationPreviewJSON),
			HostRequestID:               optionalString(item.HostRequestID),
			IssuedAt:                    graphTimePtr(item.IssuedAt),
			DeclaredAt:                  graphTimePtr(item.DeclaredAt),
			CompletedAt:                 graphTimePtr(item.CompletedAt),
		})
	}
	return out
}

func graphSoulBootstrapPublication(in *workflow.SoulBootstrapPublicationEvidence) *model.SoulBootstrapPublicationEvidence {
	if in == nil {
		return nil
	}
	return &model.SoulBootstrapPublicationEvidence{
		AgentID:                    optionalString(in.AgentID),
		PublishedVersion:           optionalInt(in.PublishedVersion),
		RegistrationURI:            optionalString(in.RegistrationURI),
		RegistrationS3Key:          optionalString(in.RegistrationS3Key),
		VersionedRegistrationURI:   optionalString(in.VersionedRegistrationURI),
		VersionedRegistrationS3Key: optionalString(in.VersionedRegistrationS3Key),
		AnchorState:                optionalString(in.AnchorState),
		PublishedAt:                graphTimePtr(in.PublishedAt),
	}
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

func soulBootstrapRegistrationID(existing *workflow.SoulBootstrapState, input *string) (string, error) {
	stored := ""
	if existing != nil {
		stored = strings.TrimSpace(existing.HostRegistrationID)
	}
	provided := strings.TrimSpace(derefString(input))
	if stored != "" && provided != "" && stored != provided {
		return "", &soulservice.HostBootstrapError{
			Code:    workflow.SoulBootstrapErrorHostBootstrapReplayRejected,
			Message: "registration id does not match local bootstrap state",
			Source:  soulBootstrapErrorSourceLesser,
			Err:     soulservice.ErrHostSigningPayloadUnsupported,
		}
	}
	registrationID := provided
	if registrationID == "" {
		registrationID = stored
	}
	if registrationID == "" {
		return "", &soulservice.HostBootstrapError{
			Code:    workflow.SoulBootstrapErrorHostRegistrationIDRequired,
			Message: "registration id is required",
			Source:  soulBootstrapErrorSourceLesser,
			Err:     soulservice.ErrHostSigningPayloadUnsupported,
		}
	}
	return registrationID, nil
}

func soulBootstrapOptionalConversationID(existing *workflow.SoulBootstrapState, input *string) (string, error) {
	stored := ""
	if existing != nil {
		stored = strings.TrimSpace(existing.HostConversationID)
	}
	provided := strings.TrimSpace(derefString(input))
	if stored != "" && provided != "" && stored != provided {
		return "", &soulservice.HostBootstrapError{
			Code:    workflow.SoulBootstrapErrorHostBootstrapReplayRejected,
			Message: "conversation id does not match local bootstrap state",
			Source:  soulBootstrapErrorSourceLesser,
			Err:     soulservice.ErrHostSigningPayloadUnsupported,
		}
	}
	if provided != "" {
		return provided, nil
	}
	return stored, nil
}

func soulBootstrapRequiredConversationID(existing *workflow.SoulBootstrapState, input string) (string, error) {
	provided := strings.TrimSpace(input)
	stored := ""
	if existing != nil {
		stored = strings.TrimSpace(existing.HostConversationID)
	}
	if stored != "" && provided != "" && stored != provided {
		return "", &soulservice.HostBootstrapError{
			Code:    workflow.SoulBootstrapErrorHostBootstrapReplayRejected,
			Message: "conversation id does not match local bootstrap state",
			Source:  soulBootstrapErrorSourceLesser,
			Err:     soulservice.ErrHostSigningPayloadUnsupported,
		}
	}
	conversationID := provided
	if conversationID == "" {
		conversationID = stored
	}
	if conversationID == "" {
		return "", &soulservice.HostBootstrapError{
			Code:    workflow.SoulBootstrapErrorHostConversationIDRequired,
			Message: "conversation id is required",
			Source:  soulBootstrapErrorSourceLesser,
			Err:     soulservice.ErrHostSigningPayloadUnsupported,
		}
	}
	return conversationID, nil
}

func graphBootstrapSignatureMap(raw *string) (map[string]string, error) {
	value := strings.TrimSpace(derefString(raw))
	if value == "" {
		return map[string]string{}, nil
	}
	var decoded map[string]string
	if err := json.Unmarshal([]byte(value), &decoded); err != nil {
		return nil, &soulservice.HostBootstrapError{
			Code:    "HOST_INVALID_REQUEST",
			Message: "boundarySignaturesJson must be a JSON object of string signatures",
			Source:  soulBootstrapErrorSourceLesser,
			Err:     soulservice.ErrHostSigningPayloadUnsupported,
		}
	}
	out := make(map[string]string, len(decoded))
	for key, signature := range decoded {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		out[key] = strings.TrimSpace(signature)
	}
	return out, nil
}

func soulBootstrapFinalizeDefaults(existing *workflow.SoulBootstrapState, issuedAt *model.Time, expectedVersion *int) (time.Time, int) {
	resolvedIssuedAt := time.Time{}
	if issuedAt != nil {
		resolvedIssuedAt = time.Time(*issuedAt)
	}
	resolvedExpectedVersion := 0
	if expectedVersion != nil {
		resolvedExpectedVersion = *expectedVersion
	}
	if existing == nil {
		return resolvedIssuedAt, resolvedExpectedVersion
	}
	for _, checkpoint := range existing.SigningCheckpoints {
		if !strings.EqualFold(strings.TrimSpace(checkpoint.Name), "finalize") {
			continue
		}
		if resolvedIssuedAt.IsZero() && checkpoint.IssuedAt != nil {
			resolvedIssuedAt = *checkpoint.IssuedAt
		}
		if expectedVersion == nil && checkpoint.ExpectedVersion >= 0 {
			resolvedExpectedVersion = checkpoint.ExpectedVersion
		}
		break
	}
	return resolvedIssuedAt, resolvedExpectedVersion
}

func soulBootstrapBindingPrincipalUsername(agentUser *storage.User, fallback string) string {
	if agentUser != nil {
		if owner := strings.TrimPrefix(strings.TrimSpace(agentUser.AgentOwner), "@"); owner != "" {
			return owner
		}
	}
	return strings.TrimSpace(fallback)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func applySoulBootstrapWorkflowProjection(
	ctx context.Context,
	r *Resolver,
	viewerUsername string,
	agentUser *storage.User,
	workflowState *workflow.DroneWorkflowState,
	bootstrap *workflow.SoulBootstrapState,
	now time.Time,
) {
	if workflowState == nil || bootstrap == nil {
		return
	}
	bootstrap = workflow.NormalizeSoulBootstrap(bootstrap, "")
	ownerActor := workflow.DroneActor{}
	finalizerActor := workflow.DroneActor{}
	if r != nil {
		ownerActor = r.droneWorkflowActor(ctx, soulBootstrapBindingPrincipalUsername(agentUser, viewerUsername), "steward")
		finalizerActor = r.droneWorkflowActor(ctx, viewerUsername, "launch_owner")
	}
	if strings.TrimSpace(ownerActor.ID) == "" {
		ownerActor = finalizerActor
	}

	if strings.TrimSpace(bootstrap.HostConversationID) != "" {
		workflowState.Conversation = &workflow.DroneConversationState{
			ConversationID: bootstrap.HostConversationID,
			UpdatedAt:      &now,
		}
	}

	switch {
	case bootstrap.Phase == workflow.SoulBootstrapPhasePrincipalDeclaration:
		workflowState.CurrentPhase = workflow.DroneWorkflowPhaseDeclaration
		workflowState.CurrentState = workflow.DroneWorkflowStateDeclarationReady
	case bootstrap.Phase == workflow.SoulBootstrapPhaseConversation:
		if bootstrap.State == workflow.SoulBootstrapStateConversationCompleted {
			workflowState.CurrentPhase = workflow.DroneWorkflowPhaseSigning
			workflowState.CurrentState = workflow.DroneWorkflowStateSigningPending
		} else {
			workflowState.CurrentPhase = workflow.DroneWorkflowPhaseDeclaration
			workflowState.CurrentState = workflow.DroneWorkflowStateDeclarationReady
		}
	case bootstrap.Phase == workflow.SoulBootstrapPhaseFinalize && bootstrap.State == workflow.SoulBootstrapStateFinalizeReady:
		workflowState.CurrentPhase = workflow.DroneWorkflowPhaseSigning
		workflowState.CurrentState = workflow.DroneWorkflowStateSigningPending
	case bootstrap.Phase == workflow.SoulBootstrapPhaseFinalize:
		workflowState.CurrentPhase = workflow.DroneWorkflowPhaseGraduation
		workflowState.CurrentState = workflow.DroneWorkflowStateGraduationReady
	case bootstrap.Phase == workflow.SoulBootstrapPhaseComplete:
		workflowState.SoulAgentID = strings.TrimSpace(bootstrap.HostSoulAgentID)
		workflowState.CurrentPhase = workflow.DroneWorkflowPhaseContinuity
		workflowState.CurrentState = workflow.DroneWorkflowStateContinuityStable
	case bootstrap.Phase == workflow.SoulBootstrapPhaseError:
		// Preserve the last projected non-error phase so UI can show the current
		// actionable card alongside the typed bootstrap error.
	default:
		return
	}

	if bootstrap.Phase == workflow.SoulBootstrapPhaseConversation && bootstrap.State == workflow.SoulBootstrapStateConversationCompleted {
		workflowState.Declaration = &workflow.DroneDeclarationCard{
			ID:            agentUser.Username + ":bootstrap-declaration",
			Title:         "Soul bootstrap declaration",
			Statement:     "Host mint conversation completed and produced declaration material.",
			Confidence:    "hosted_offchain",
			Owner:         &ownerActor,
			DeclaredScope: []string{"identity_continuity", "hosted_offchain_registration"},
		}
	}
	if bootstrap.Phase == workflow.SoulBootstrapPhaseFinalize {
		workflowState.Checkpoint = &workflow.DroneSignatureCheckpoint{
			ID:             agentUser.Username + ":bootstrap-finalize",
			Title:          "Soul bootstrap finalize checkpoint",
			ReadinessLabel: "Host finalize signing material captured",
			DueAt:          &now,
			Signers: []workflow.DroneSignatureSigner{
				{
					ID:     defaultString(bootstrap.PrincipalAddress, finalizerActor.ID),
					Name:   defaultString(bootstrap.PrincipalAddress, finalizerActor.Name),
					Role:   "principal",
					Status: workflow.DroneSignatureSignerStatusApproved,
					Note:   "Host self-attestation checkpoint",
				},
			},
		}
		if bootstrap.State == workflow.SoulBootstrapStateFinalizePublished {
			workflowState.Graduation = &workflow.DroneGraduationSummaryCard{
				ID:                  agentUser.Username + ":bootstrap-graduation",
				Title:               "Hosted soul publication",
				Readiness:           workflow.DroneGraduationReadinessReady,
				Summary:             "Host published the hosted/off-chain soul registration and returned publication evidence.",
				LaunchOwner:         &finalizerActor,
				CompletedMilestones: []string{"conversation_completed", "finalize_signed", "host_publication_recorded"},
				ExitCriteria:        []string{"local_body_binding"},
				NextStep:            "Bind Host soul to local Lesser body",
				Metrics: []workflow.DroneMetric{
					{Label: "Soul agent", Value: bootstrap.HostSoulAgentID},
				},
			}
		}
	}
	if bootstrap.Phase == workflow.SoulBootstrapPhaseComplete {
		anchorState := ""
		if bootstrap.Publication != nil {
			anchorState = bootstrap.Publication.AnchorState
		}
		workflowState.Continuity = &workflow.DroneContinuityPanel{
			ID:           agentUser.Username + ":bootstrap-continuity",
			Title:        "Soul/body continuity",
			Objective:    "Preserve the existing body identity, timeline presence, and memory continuity through hosted soul binding.",
			Owner:        ownerActor,
			FeedbackLoop: "Monitor bootstrap publication evidence, local binding state, and attribution continuity.",
			Metrics: []workflow.DroneMetric{
				{Label: "Body", Value: "preserved"},
				{Label: "Soul binding", Value: "bound"},
				{Label: "Anchor", Value: defaultString(anchorState, "hosted_offchain")},
			},
		}
	}
	workflowState.Lifecycle = workflow.BuildDroneLifecycle(workflowState.CurrentPhase, workflowState.CurrentState)
}

func optionalInt(value int) *int {
	if value == 0 {
		return nil
	}
	return &value
}

func optionalBootstrapExpectedVersion(checkpoint workflow.SoulBootstrapSigningCheckpoint) *int {
	if !strings.EqualFold(strings.TrimSpace(checkpoint.Name), "finalize") {
		return nil
	}
	value := checkpoint.ExpectedVersion
	return &value
}
