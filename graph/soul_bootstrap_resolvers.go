package graph

import (
	"context"
	"strings"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	workflow "github.com/equaltoai/lesser/pkg/agents"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
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
	return r.soulBootstrapHostBridgeUnavailablePayload(ctx, input.Username, graphBootstrapCorrelation(
		input.CorrelationKey,
		input.IdempotencyKey,
		soulBootstrapCorrelationOpBegin,
	))
}

func (r *mutationResolver) VerifySoulBootstrapWallet(ctx context.Context, input model.VerifySoulBootstrapWalletInput) (*model.SoulBootstrapMutationPayload, error) {
	return r.soulBootstrapHostBridgeUnavailablePayload(ctx, input.Username, graphBootstrapCorrelation(
		input.CorrelationKey,
		input.IdempotencyKey,
		soulBootstrapCorrelationOpWalletVerification,
	))
}

func (r *mutationResolver) PrepareSoulBootstrapPrincipalDeclaration(ctx context.Context, input model.PrepareSoulBootstrapPrincipalDeclarationInput) (*model.SoulBootstrapMutationPayload, error) {
	return r.soulBootstrapHostBridgeUnavailablePayload(ctx, input.Username, graphBootstrapCorrelation(
		input.CorrelationKey,
		input.IdempotencyKey,
		soulBootstrapCorrelationOpPrincipalDeclaration,
	))
}

func (r *mutationResolver) VerifySoulBootstrapPrincipalDeclaration(ctx context.Context, input model.VerifySoulBootstrapPrincipalDeclarationInput) (*model.SoulBootstrapMutationPayload, error) {
	return r.soulBootstrapHostBridgeUnavailablePayload(ctx, input.Username, graphBootstrapCorrelation(
		input.CorrelationKey,
		input.IdempotencyKey,
		soulBootstrapCorrelationOpPrincipalDeclaration,
	))
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

	return &model.SoulBootstrapSurface{
		Username:            agentUser.Username,
		Body:                graphSoulBootstrapIdentityTarget(ctx, r, agentUser),
		Workflow:            surface,
		State:               state,
		SoulBindingState:    soulBindingState,
		ExistingSoulAgentID: optionalString(existingSoulAgentID),
		HostBridgeAvailable: false,
		Executable:          false,
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
			Name:             item.Name,
			Status:           item.Status,
			PrincipalAddress: optionalString(item.PrincipalAddress),
			SignerAddress:    optionalString(item.SignerAddress),
			SigningMethod:    optionalString(item.SigningMethod),
			MessageEncoding:  optionalString(item.MessageEncoding),
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
