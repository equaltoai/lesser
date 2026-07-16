package graph

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	workflow "github.com/equaltoai/lesser/pkg/agents"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	soulservice "github.com/equaltoai/lesser/pkg/services/souls"
	"github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"go.uber.org/zap"
)

const (
	soulBootstrapCorrelationOpBegin                = "begin"
	soulBootstrapCorrelationOpWalletVerification   = "wallet_verification"
	soulBootstrapCorrelationOpPrincipalDeclaration = "principal_declaration"
	soulBootstrapCorrelationOpConversation         = "conversation"
	soulBootstrapCorrelationOpFinalize             = "finalize"
	soulBootstrapCorrelationOpRestart              = "restart"
	soulBootstrapErrorSourceLesser                 = "lesser"
	soulBootstrapCheckpointConversation            = "conversation"
	soulBootstrapCheckpointHostedConversation      = "hosted_conversation"
	soulBootstrapHostedTranscriptRoleUser          = "user"
	soulBootstrapHostedTranscriptRoleAssistant     = "assistant"
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
	if err := r.reconcileHostedSoulBootstrapState(ctx, claims.Username, agentUser, workflowState); err != nil {
		return nil, err
	}
	return r.buildSoulBootstrapSurface(ctx, claims.Username, agentUser, governance, nil)
}

func (r *queryResolver) ListHostedGenesisConversations(ctx context.Context, username string) ([]*model.HostedGenesisConversationSummary, error) {
	claims, err := r.requireAuthClaims(ctx)
	if err != nil {
		return nil, err
	}
	if err := requireDroneReadScope(claims); err != nil {
		return nil, err
	}

	agentUser, _, err := r.loadDroneAgent(ctx, username)
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

	agentID := ""
	if workflowState != nil && workflowState.SoulBootstrap != nil {
		agentID = strings.TrimSpace(workflowState.SoulBootstrap.HostSoulAgentID)
	}
	if agentID == "" {
		return []*model.HostedGenesisConversationSummary{}, nil
	}

	service, err := r.soulBootstrapService()
	if err != nil {
		return nil, apperrors.InternalWithCause(err, "failed to initialize soul bootstrap service")
	}

	summaries, err := service.ListHostedGenesisConversations(ctx, agentID)
	if err != nil {
		return nil, err
	}

	out := make([]*model.HostedGenesisConversationSummary, 0, len(summaries))
	for _, summary := range summaries {
		out = append(out, &model.HostedGenesisConversationSummary{
			ConversationID: summary.ConversationID,
			RegistrationID: optionalString(summary.RegistrationID),
			Status:         summary.Status,
			MessageCount:   summary.MessageCount,
			LatestTurnID:   optionalString(summary.LatestTurnID),
			CreatedAt:      graphTimePtr(summary.CreatedAt),
			UpdatedAt:      graphTimePtr(summary.UpdatedAt),
		})
	}
	return out, nil
}

func (r *mutationResolver) StartHostedSoulBootstrap(ctx context.Context, input model.StartHostedSoulBootstrapInput) (*model.SoulBootstrapMutationPayload, error) {
	correlation := graphHostedBootstrapCorrelation(
		input.CorrelationKey,
		input.IdempotencyKey,
		soulBootstrapCorrelationOpBegin,
		input.RecoveryAttemptID,
	)
	return r.executeSoulBootstrapMutation(ctx, input.Username, func(
		ctx context.Context,
		agentUser *storage.User,
		_ *storage.AgentGovernanceState,
		service soulBootstrapHostService,
		existing *workflow.SoulBootstrapState,
		now time.Time,
	) (*workflow.SoulBootstrapState, error) {
		if recoveryState, handled := soulBootstrapHostedGenesisMessageRetryState(agentUser, existing, correlation, now); handled {
			// G16 (P52 L3.2): the retry repair rewrites an errored
			// retry_same_step state back to conversation.registration_active /
			// send_hosted_soul_genesis_message. This is intentional for a
			// genuine Host-authored retry_same_step (the failed row of the
			// projection table), not for accept-timeouts (which L3.2 G15 routes
			// to REFRESH_STATE). Log the rewrite so it is observable rather
			// than silent.
			soulBootstrapReconcileLog(r.Resolver, "hosted soul bootstrap retry repair rewrote errored state to send",
				errors.New("retry_same_step repair applied"),
				zap.String("username", agentUser.Username),
				zap.String("prior_state", soulBootstrapPriorStateName(existing)),
			)
			return recoveryState, nil
		}
		if replayState, handled := soulBootstrapHostedBeginReplayState(agentUser, existing, correlation, now); handled {
			return replayState, nil
		}
		result, err := service.BeginHostedBootstrapRegistration(ctx, soulservice.BootstrapBeginInput{
			Username:     soulBootstrapHostLocalID(agentUser),
			BodyID:       soulBootstrapBodyID(agentUser),
			Capabilities: input.Capabilities,
		})
		if err != nil {
			return soulBootstrapErrorState(agentUser, nil, correlation, err, now), nil
		}
		return soulBootstrapStateAfterHostedBegin(agentUser, correlation, result, now), nil
	})
}

func (r *mutationResolver) SendHostedSoulGenesisMessage(ctx context.Context, input model.SendHostedSoulGenesisMessageInput) (*model.SoulBootstrapMutationPayload, error) {
	correlation := graphHostedBootstrapCorrelation(
		input.CorrelationKey,
		input.IdempotencyKey,
		soulBootstrapCorrelationOpConversation,
		input.RecoveryAttemptID,
	)
	return r.executeSoulBootstrapConversationMessageMutation(
		ctx,
		input.Username,
		input.RegistrationID,
		input.ConversationID,
		input.Message,
		input.Model,
		correlation,
		soulBootstrapStateAfterHostedConversationMessage,
	)
}

func (r *mutationResolver) CompleteHostedSoulGenesis(ctx context.Context, input model.CompleteHostedSoulGenesisInput) (*model.SoulBootstrapMutationPayload, error) {
	correlation := graphHostedBootstrapCorrelation(
		input.CorrelationKey,
		input.IdempotencyKey,
		soulBootstrapCorrelationOpConversation,
		input.RecoveryAttemptID,
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
		return soulBootstrapStateAfterHostedConversationComplete(agentUser, existing, correlation, result, now), nil
	})
}

func (r *mutationResolver) RecoverHostedSoulGenesisTurn(ctx context.Context, input model.RecoverHostedSoulGenesisTurnInput) (*model.SoulBootstrapMutationPayload, error) {
	correlation := graphHostedBootstrapCorrelation(
		input.CorrelationKey,
		input.IdempotencyKey,
		soulBootstrapCorrelationOpConversation,
		input.RecoveryAttemptID,
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
		result, err := service.RecoverHostedGenesisTurn(ctx, soulservice.BootstrapConversationRecoverInput{
			RegistrationID:  registrationID,
			ConversationID:  conversationID,
			CorrelationID:   derefString(input.CorrelationKey),
			IdempotencyKey:  derefString(input.IdempotencyKey),
			LesserRequestID: "",
		})
		if err != nil {
			return soulBootstrapErrorState(agentUser, existing, correlation, err, now), nil
		}
		return soulBootstrapStateAfterHostedConversationSnapshot(agentUser, existing, correlation, result, now), nil
	})
}

func (r *mutationResolver) PublishHostedSoul(ctx context.Context, input model.PublishHostedSoulInput) (*model.SoulBootstrapMutationPayload, error) {
	correlation := graphHostedBootstrapCorrelation(
		input.CorrelationKey,
		input.IdempotencyKey,
		soulBootstrapCorrelationOpFinalize,
		input.RecoveryAttemptID,
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
		if err := soulBootstrapRequireHostedPublishEvidence(existing, conversationID); err != nil {
			return soulBootstrapErrorState(agentUser, existing, correlation, err, now), nil
		}
		result, err := service.PublishHostedBootstrap(ctx, soulservice.HostedBootstrapPublishInput{
			RegistrationID: registrationID,
			ConversationID: conversationID,
			LocalID:        soulBootstrapHostLocalID(agentUser),
		})
		if err != nil {
			return soulBootstrapErrorState(agentUser, existing, correlation, err, now), nil
		}
		published := soulBootstrapStateAfterHostedPublish(agentUser, existing, correlation, result, now)
		soul, err := service.BindHostedBootstrap(ctx, agentUser.Username, result)
		if err != nil {
			return soulBootstrapErrorState(agentUser, published, correlation, err, now), nil
		}
		return soulBootstrapStateAfterHostedBinding(agentUser, published, correlation, soul, result, now), nil
	})
}

func (r *mutationResolver) RestartSoulBootstrap(ctx context.Context, input model.RestartSoulBootstrapInput) (*model.SoulBootstrapMutationPayload, error) {
	correlation := graphHostedBootstrapCorrelation(
		input.CorrelationKey,
		input.IdempotencyKey,
		soulBootstrapCorrelationOpRestart,
		&input.RecoveryAttemptID,
	)
	return r.executeSoulBootstrapMutation(ctx, input.Username, func(
		ctx context.Context,
		agentUser *storage.User,
		_ *storage.AgentGovernanceState,
		service soulBootstrapHostService,
		existing *workflow.SoulBootstrapState,
		now time.Time,
	) (*workflow.SoulBootstrapState, error) {
		if replayState, handled := soulBootstrapRestartReplayState(agentUser, existing, correlation, now); handled {
			return replayState, nil
		}
		correlation = soulBootstrapRestartCorrelation(existing, correlation)
		result, err := service.BeginHostedBootstrapRegistration(ctx, soulservice.BootstrapBeginInput{
			Username: soulBootstrapHostLocalID(agentUser),
			BodyID:   soulBootstrapBodyID(agentUser),
		})
		if err != nil {
			state := soulBootstrapErrorState(agentUser, nil, correlation, err, now)
			state.RestartedAt = &now
			return workflow.NormalizeSoulBootstrap(state, agentUser.Username), nil
		}
		state := soulBootstrapStateAfterHostedBegin(agentUser, correlation, result, now)
		state.RestartedAt = &now
		return workflow.NormalizeSoulBootstrap(state, agentUser.Username), nil
	})
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
		if replayState, handled := soulBootstrapBeginReplayState(agentUser, existing, correlation, input.WalletAddress, now); handled {
			return replayState, nil
		}
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
	return r.executeSoulBootstrapConversationMessageMutation(
		ctx,
		input.Username,
		input.RegistrationID,
		input.ConversationID,
		input.Message,
		input.Model,
		correlation,
		soulBootstrapStateAfterConversationMessage,
	)
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
	BindHostedBootstrap(context.Context, string, *soulservice.BootstrapFinalizeResult) (*soulservice.Soul, error)
	BeginBootstrapRegistration(context.Context, soulservice.BootstrapBeginInput) (*soulservice.BootstrapBeginResult, error)
	BeginHostedBootstrapRegistration(context.Context, soulservice.BootstrapBeginInput) (*soulservice.BootstrapBeginResult, error)
	PrepareBootstrapPrincipalDeclaration(context.Context, soulservice.BootstrapPrincipalPreflightInput) (*soulservice.BootstrapPrincipalPreflightResult, error)
	VerifyBootstrapPrincipalDeclaration(context.Context, soulservice.BootstrapPrincipalVerifyInput) (*soulservice.BootstrapPrincipalVerifyResult, error)
	SendBootstrapConversationMessage(context.Context, soulservice.BootstrapConversationMessageInput) (*soulservice.BootstrapConversationMessageResult, error)
	CompleteBootstrapConversation(context.Context, soulservice.BootstrapConversationCompleteInput) (*soulservice.BootstrapConversationCompleteResult, error)
	RecoverHostedGenesisTurn(context.Context, soulservice.BootstrapConversationRecoverInput) (*soulservice.BootstrapConversationCompleteResult, error)
	ReadBootstrapConversation(context.Context, soulservice.BootstrapConversationCompleteInput) (*soulservice.BootstrapConversationCompleteResult, error)
	ListHostedGenesisConversations(context.Context, string) ([]soulservice.HostedGenesisConversationSummary, error)
	PrepareBootstrapFinalize(context.Context, soulservice.BootstrapFinalizePreflightInput) (*soulservice.BootstrapFinalizePreflightResult, error)
	FinalizeBootstrap(context.Context, soulservice.BootstrapFinalizeInput) (*soulservice.BootstrapFinalizeResult, error)
	PublishHostedBootstrap(context.Context, soulservice.HostedBootstrapPublishInput) (*soulservice.BootstrapFinalizeResult, error)
}

type soulBootstrapMutationFunc func(
	context.Context,
	*storage.User,
	*storage.AgentGovernanceState,
	soulBootstrapHostService,
	*workflow.SoulBootstrapState,
	time.Time,
) (*workflow.SoulBootstrapState, error)

type soulBootstrapConversationMessageStateFunc func(
	*storage.User,
	*workflow.SoulBootstrapState,
	*workflow.SoulBootstrapCorrelationState,
	*soulservice.BootstrapConversationMessageResult,
	time.Time,
) *workflow.SoulBootstrapState

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

func (r *mutationResolver) executeSoulBootstrapConversationMessageMutation(
	ctx context.Context,
	username string,
	registrationIDInput *string,
	conversationIDInput *string,
	message string,
	modelInput *string,
	correlation *workflow.SoulBootstrapCorrelationState,
	nextState soulBootstrapConversationMessageStateFunc,
) (*model.SoulBootstrapMutationPayload, error) {
	return r.executeSoulBootstrapReviewMutation(ctx, username, func(
		ctx context.Context,
		agentUser *storage.User,
		_ *storage.AgentGovernanceState,
		service soulBootstrapHostService,
		existing *workflow.SoulBootstrapState,
		now time.Time,
	) (*workflow.SoulBootstrapState, error) {
		registrationID, err := soulBootstrapRegistrationID(existing, registrationIDInput)
		if err != nil {
			return soulBootstrapErrorState(agentUser, existing, correlation, err, now), nil
		}
		conversationID, err := soulBootstrapOptionalConversationID(existing, conversationIDInput)
		if err != nil {
			return soulBootstrapErrorState(agentUser, existing, correlation, err, now), nil
		}
		result, err := service.SendBootstrapConversationMessage(ctx, soulservice.BootstrapConversationMessageInput{
			RegistrationID: registrationID,
			ConversationID: conversationID,
			Message:        message,
			Model:          derefString(modelInput),
		})
		if err != nil {
			return soulBootstrapErrorState(agentUser, existing, correlation, err, now), nil
		}
		return nextState(agentUser, existing, correlation, result, now), nil
	})
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

// soulBootstrapReconcileLog emits a best-effort reconcile diagnostic when a
// non-nil resolver logger is configured. It never panics on a nil resolver or
// logger so reconcile/read paths stay best-effort. Used by G16 (P52 L3.2) to
// surface previously-swallowed errors and silent state rewrites.
func soulBootstrapReconcileLog(r *Resolver, msg string, cause error, fields ...zap.Field) {
	if r == nil || r.Logger == nil {
		return
	}
	fields = append(fields, zap.Error(cause))
	r.Logger.Warn(msg, fields...)
}

// soulBootstrapPriorStateName returns a non-empty label for the state being
// rewritten by a retry repair, for diagnostic logging. Falls back to "unknown"
// so a log line is always emitted with context.
func soulBootstrapPriorStateName(state *workflow.SoulBootstrapState) string {
	if state == nil {
		return unknownValue
	}
	if v := strings.TrimSpace(state.State); v != "" {
		return v
	}
	if v := strings.TrimSpace(state.Phase); v != "" {
		return v
	}
	return unknownValue
}

func (r *Resolver) reconcileHostedSoulBootstrapState(
	ctx context.Context,
	viewerUsername string,
	agentUser *storage.User,
	workflowState *workflow.DroneWorkflowState,
) error {
	if r == nil || agentUser == nil || workflowState == nil || workflowState.SoulBootstrap == nil {
		return nil
	}
	state := workflow.NormalizeSoulBootstrap(workflowState.SoulBootstrap, agentUser.Username)
	if !soulBootstrapShouldReadRepairHostedState(state) {
		now := time.Now().UTC()
		if publishedState, handled := r.autoPublishHostedSoulBootstrap(ctx, agentUser, state, state.Correlation, now); handled {
			workflowState.SoulBootstrap = workflow.NormalizeSoulBootstrap(publishedState, agentUser.Username)
			applySoulBootstrapWorkflowProjection(ctx, r, viewerUsername, agentUser, workflowState, workflowState.SoulBootstrap, now)
			workflowState.UpdatedAt = &now
			if err := r.persistDroneWorkflowState(ctx, agentUser, workflowState); err != nil {
				return apperrors.InternalWithCause(err, "failed to persist hosted soul publication state")
			}
			return nil
		}
		if recoveryState, handled := soulBootstrapHostedGenesisMessageRetryState(agentUser, state, state.Correlation, now); handled {
			// G16 (P52 L3.2): log the silent errored→send retry rewrite so it is
			// observable. Only reachable for a genuine Host-authored
			// retry_same_step (accept-timeouts route to REFRESH_STATE via G15).
			soulBootstrapReconcileLog(r, "hosted soul bootstrap reconcile retry repair rewrote errored state to send",
				errors.New("retry_same_step repair applied"),
				zap.String("username", agentUser.Username),
				zap.String("prior_state", soulBootstrapPriorStateName(state)),
			)
			workflowState.SoulBootstrap = workflow.NormalizeSoulBootstrap(recoveryState, agentUser.Username)
			applySoulBootstrapWorkflowProjection(ctx, r, viewerUsername, agentUser, workflowState, workflowState.SoulBootstrap, now)
			workflowState.UpdatedAt = &now
			if err := r.persistDroneWorkflowState(ctx, agentUser, workflowState); err != nil {
				return apperrors.InternalWithCause(err, "failed to persist recovered hosted soul bootstrap state")
			}
		}
		return nil
	}
	service, err := r.soulBootstrapService()
	if err != nil {
		// G16 (P52 L3.2): surface the swallowed read-repair setup error instead
		// of silently dropping it. Reconcile is best-effort, so we still return
		// nil to avoid failing the read, but the error is logged for operators.
		soulBootstrapReconcileLog(r, "hosted soul bootstrap reconcile could not resolve service", err, zap.String("username", agentUser.Username))
		return nil
	}
	result, err := service.ReadBootstrapConversation(ctx, soulservice.BootstrapConversationCompleteInput{
		RegistrationID: state.HostRegistrationID,
		ConversationID: state.HostConversationID,
	})
	if err != nil {
		// G16 (P52 L3.2): surface the swallowed Host read error instead of
		// silently dropping it. Reconcile is best-effort (a transient Host read
		// failure must not fail the GraphQL read), but the error is logged so a
		// persistent Host bridge failure is observable rather than invisible.
		soulBootstrapReconcileLog(r, "hosted soul bootstrap reconcile Host read failed", err,
			zap.String("username", agentUser.Username),
			zap.String("host_registration_id", state.HostRegistrationID),
			zap.String("host_conversation_id", state.HostConversationID),
		)
		return nil
	}

	now := time.Now().UTC()
	next := soulBootstrapStateAfterHostedConversationComplete(agentUser, state, state.Correlation, result, now)
	if publishedState := autoPublishHostedSoulBootstrapWithService(ctx, agentUser, service, next, next.Correlation, now); publishedState != nil {
		next = publishedState
	}
	workflowState.SoulBootstrap = workflow.NormalizeSoulBootstrap(next, agentUser.Username)
	applySoulBootstrapWorkflowProjection(ctx, r, viewerUsername, agentUser, workflowState, workflowState.SoulBootstrap, now)
	workflowState.UpdatedAt = &now
	if err := r.persistDroneWorkflowState(ctx, agentUser, workflowState); err != nil {
		return apperrors.InternalWithCause(err, "failed to persist reconciled hosted soul bootstrap state")
	}
	return nil
}

func (r *Resolver) autoPublishHostedSoulBootstrap(
	ctx context.Context,
	agentUser *storage.User,
	state *workflow.SoulBootstrapState,
	correlation *workflow.SoulBootstrapCorrelationState,
	now time.Time,
) (*workflow.SoulBootstrapState, bool) {
	if !soulBootstrapShouldAutoPublishHostedSoul(state) {
		return nil, false
	}
	service, err := r.soulBootstrapService()
	if err != nil {
		return nil, false
	}
	return autoPublishHostedSoulBootstrapWithService(ctx, agentUser, service, state, correlation, now), true
}

func autoPublishHostedSoulBootstrapWithService(
	ctx context.Context,
	agentUser *storage.User,
	service soulBootstrapHostService,
	state *workflow.SoulBootstrapState,
	correlation *workflow.SoulBootstrapCorrelationState,
	now time.Time,
) *workflow.SoulBootstrapState {
	if !soulBootstrapShouldAutoPublishHostedSoul(state) {
		return nil
	}
	registrationID, err := soulBootstrapRegistrationID(state, nil)
	if err != nil {
		return soulBootstrapErrorState(agentUser, state, correlation, err, now)
	}
	conversationID, err := soulBootstrapRequiredConversationID(state, "")
	if err != nil {
		return soulBootstrapErrorState(agentUser, state, correlation, err, now)
	}
	if err := soulBootstrapRequireHostedPublishEvidence(state, conversationID); err != nil {
		return soulBootstrapErrorState(agentUser, state, correlation, err, now)
	}
	result, err := service.PublishHostedBootstrap(ctx, soulservice.HostedBootstrapPublishInput{
		RegistrationID: registrationID,
		ConversationID: conversationID,
		LocalID:        soulBootstrapHostLocalID(agentUser),
	})
	if err != nil {
		return soulBootstrapErrorState(agentUser, state, correlation, err, now)
	}
	published := soulBootstrapStateAfterHostedPublish(agentUser, state, correlation, result, now)
	soul, err := service.BindHostedBootstrap(ctx, agentUser.Username, result)
	if err != nil {
		return soulBootstrapErrorState(agentUser, published, correlation, err, now)
	}
	return soulBootstrapStateAfterHostedBinding(agentUser, published, correlation, soul, result, now)
}

func soulBootstrapShouldAutoPublishHostedSoul(state *workflow.SoulBootstrapState) bool {
	state = workflow.NormalizeSoulBootstrap(state, "")
	if state == nil || state.BootstrapMode != workflow.SoulBootstrapModeHosted {
		return false
	}
	if state.Error != nil || state.Phase == workflow.SoulBootstrapPhaseComplete || state.State == workflow.SoulBootstrapStateCompleteBound {
		return false
	}
	if strings.TrimSpace(state.HostRegistrationID) == "" || strings.TrimSpace(state.HostConversationID) == "" {
		return false
	}
	if state.Publication != nil && state.Publication.PublishedVersion > 0 {
		return false
	}
	return soulBootstrapStateHasActiveTerminalDeclarationEvidence(state)
}

func soulBootstrapShouldReadRepairHostedState(state *workflow.SoulBootstrapState) bool {
	state = workflow.NormalizeSoulBootstrap(state, "")
	if state == nil || state.BootstrapMode != workflow.SoulBootstrapModeHosted {
		return false
	}
	if strings.TrimSpace(state.HostRegistrationID) == "" || strings.TrimSpace(state.HostConversationID) == "" {
		return false
	}
	if soulBootstrapHasTerminalConversationDeclarationEvidence(state.SigningCheckpoints, state.HostConversationID) {
		return false
	}
	if state.Phase == workflow.SoulBootstrapPhaseError {
		return strings.EqualFold(strings.TrimSpace(state.NextAction), workflow.SoulBootstrapNextActionRefreshState) ||
			strings.EqualFold(strings.TrimSpace(state.RecoveryAction), workflow.SoulBootstrapRecoveryActionRefreshState) ||
			(state.Error != nil && soulBootstrapConversationNotInProgressConflict(state.Error.Code, state.Error.StatusCode, state.Error.Message))
	}
	return state.Phase == workflow.SoulBootstrapPhaseConversation ||
		state.Phase == workflow.SoulBootstrapPhaseFinalize
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
	state.BootstrapMode = workflow.SoulBootstrapModeWalletPrincipal
	state.AuthorityModel = workflow.SoulBootstrapAuthorityModelWalletPrincipal
	state.NextAction = workflow.SoulBootstrapNextActionVerifyWallet
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

func soulBootstrapBeginReplayState(
	agentUser *storage.User,
	existing *workflow.SoulBootstrapState,
	correlation *workflow.SoulBootstrapCorrelationState,
	walletAddress string,
	now time.Time,
) (*workflow.SoulBootstrapState, bool) {
	if existing == nil || correlation == nil || strings.TrimSpace(correlation.BeginIdempotencyKey) == "" {
		return nil, false
	}
	username := ""
	if agentUser != nil {
		username = agentUser.Username
	}
	state := workflow.NormalizeSoulBootstrap(existing, username)
	if strings.TrimSpace(state.HostRegistrationID) == "" || state.Correlation == nil {
		return nil, false
	}
	if strings.TrimSpace(state.Correlation.BeginIdempotencyKey) != strings.TrimSpace(correlation.BeginIdempotencyKey) {
		return nil, false
	}
	if storedWallet := strings.TrimSpace(state.WalletAddress); storedWallet != "" {
		providedWallet := strings.ToLower(strings.TrimSpace(walletAddress))
		if providedWallet != "" && !strings.EqualFold(storedWallet, providedWallet) {
			return soulBootstrapErrorState(
				agentUser,
				state,
				correlation,
				soulBootstrapReplayRejectedError("wallet address does not match local bootstrap state"),
				now,
			), true
		}
	}
	state.Correlation = mergeSoulBootstrapCorrelation(state.Correlation, correlation)
	state.UpdatedAt = &now
	return workflow.NormalizeSoulBootstrap(state, username), true
}

func soulBootstrapStateAfterHostedBegin(
	agentUser *storage.User,
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
	state := workflow.NormalizeSoulBootstrap(&workflow.SoulBootstrapState{}, username)
	state.Username = username
	state.BodyID = bodyID
	state.HostRegistrationID = strings.TrimSpace(result.RegistrationID)
	state.HostSoulAgentID = strings.TrimSpace(result.HostSoulAgentID)
	state.Phase = workflow.SoulBootstrapPhaseConversation
	state.State = workflow.SoulBootstrapStateConversationRegistrationActive
	state.BootstrapMode = workflow.SoulBootstrapModeHosted
	state.AuthorityModel = workflow.SoulBootstrapAuthorityModelInstanceTrust
	state.AnchorState = firstNonEmpty(result.AnchorState, workflow.SoulBootstrapAnchorStateHostedOffchain)
	state.AssuranceState = state.AnchorState
	state.NextAction = workflow.SoulBootstrapNextActionSendHostedGenesisMessage
	state.RecoveryCategory = ""
	state.RecoveryAction = ""
	state.Retryable = false
	state.RestartRequired = false
	state.SigningCheckpoints = nil
	state.Correlation = mergeSoulBootstrapCorrelation(state.Correlation, correlation)
	if state.Correlation == nil {
		state.Correlation = &workflow.SoulBootstrapCorrelationState{}
	}
	state.Correlation.LastHostRequestID = strings.TrimSpace(result.HostRequestID)
	state.Error = nil
	state.UpdatedAt = &now
	return workflow.NormalizeSoulBootstrap(state, username)
}

func soulBootstrapHostedBeginReplayState(
	agentUser *storage.User,
	existing *workflow.SoulBootstrapState,
	correlation *workflow.SoulBootstrapCorrelationState,
	now time.Time,
) (*workflow.SoulBootstrapState, bool) {
	if existing == nil || correlation == nil || strings.TrimSpace(correlation.BeginIdempotencyKey) == "" {
		return nil, false
	}
	username := ""
	if agentUser != nil {
		username = agentUser.Username
	}
	state := workflow.NormalizeSoulBootstrap(existing, username)
	if state.BootstrapMode != workflow.SoulBootstrapModeHosted || strings.TrimSpace(state.HostRegistrationID) == "" || state.Correlation == nil {
		return nil, false
	}
	if strings.TrimSpace(state.Correlation.BeginIdempotencyKey) != strings.TrimSpace(correlation.BeginIdempotencyKey) {
		return nil, false
	}
	state.Correlation = mergeSoulBootstrapCorrelation(state.Correlation, correlation)
	state.UpdatedAt = &now
	return workflow.NormalizeSoulBootstrap(state, username), true
}

func soulBootstrapHostedGenesisMessageRetryState(
	agentUser *storage.User,
	existing *workflow.SoulBootstrapState,
	correlation *workflow.SoulBootstrapCorrelationState,
	now time.Time,
) (*workflow.SoulBootstrapState, bool) {
	if existing == nil {
		return nil, false
	}
	username := ""
	bodyID := ""
	if agentUser != nil {
		username = agentUser.Username
		bodyID = soulBootstrapBodyID(agentUser)
	}
	state := workflow.NormalizeSoulBootstrap(existing, username)
	if !soulBootstrapHostedGenesisMessageRetryRequired(state) {
		return nil, false
	}
	if username == "" {
		username = state.Username
	}
	if bodyID == "" {
		bodyID = state.BodyID
	}
	state.Username = username
	state.BodyID = defaultString(state.BodyID, bodyID)
	state.BootstrapMode = workflow.SoulBootstrapModeHosted
	state.AuthorityModel = workflow.SoulBootstrapAuthorityModelInstanceTrust
	state.AnchorState = firstNonEmpty(state.AnchorState, workflow.SoulBootstrapAnchorStateHostedOffchain)
	state.AssuranceState = firstNonEmpty(state.AssuranceState, state.AnchorState)
	state.Phase = workflow.SoulBootstrapPhaseConversation
	state.State = workflow.SoulBootstrapStateConversationRegistrationActive
	state.NextAction = workflow.SoulBootstrapNextActionSendHostedGenesisMessage
	state.RecoveryCategory = ""
	state.RecoveryAction = ""
	state.Retryable = false
	state.RestartRequired = false
	state.SigningCheckpoints = nil
	state.Error = nil
	state.Correlation = mergeSoulBootstrapCorrelation(state.Correlation, correlation)
	state.UpdatedAt = &now
	return workflow.NormalizeSoulBootstrap(state, username), true
}

func soulBootstrapHostedGenesisMessageRetryRequired(state *workflow.SoulBootstrapState) bool {
	state = workflow.NormalizeSoulBootstrap(state, "")
	if state == nil || state.BootstrapMode != workflow.SoulBootstrapModeHosted {
		return false
	}
	if state.Phase != workflow.SoulBootstrapPhaseError || state.RestartRequired {
		return false
	}
	if strings.TrimSpace(state.HostRegistrationID) == "" || strings.TrimSpace(state.HostConversationID) != "" {
		return false
	}
	if soulBootstrapRetrySameStepAction(state.NextAction) ||
		soulBootstrapRetrySameStepAction(state.RecoveryCategory) ||
		soulBootstrapRetrySameStepAction(state.RecoveryAction) {
		return true
	}
	if state.Error == nil {
		return false
	}
	return soulBootstrapRetrySameStepAction(state.Error.RecoveryCategory) ||
		soulBootstrapRetrySameStepAction(state.Error.RecoveryAction)
}

func soulBootstrapRetrySameStepAction(action string) bool {
	return strings.EqualFold(strings.TrimSpace(action), workflow.SoulBootstrapRecoveryActionRetrySameStep)
}

func soulBootstrapStateAfterHostedConversationMessage(
	agentUser *storage.User,
	existing *workflow.SoulBootstrapState,
	correlation *workflow.SoulBootstrapCorrelationState,
	result *soulservice.BootstrapConversationMessageResult,
	now time.Time,
) *workflow.SoulBootstrapState {
	if result == nil {
		return soulBootstrapStateAfterHostedConversationSnapshot(agentUser, existing, correlation, nil, now)
	}
	return soulBootstrapStateAfterHostedConversationSnapshot(agentUser, existing, correlation, &soulservice.BootstrapConversationCompleteResult{
		RegistrationID:        result.RegistrationID,
		HostSoulAgentID:       result.HostSoulAgentID,
		ConversationID:        result.ConversationID,
		Status:                result.Status,
		LatestTurnID:          result.LatestTurnID,
		MessageCount:          result.MessageCount,
		Messages:              cloneBootstrapConversationMessages(result.Messages),
		MessagesTruncated:     result.MessagesTruncated,
		ProducedDeclarations:  result.ProducedDeclarations,
		UpdatedAt:             result.UpdatedAt,
		CompletedAt:           result.CompletedAt,
		HostRequestID:         result.HostRequestID,
		FailureCode:           result.FailureCode,
		FailureMessage:        result.FailureMessage,
		FailureRetryable:      result.FailureRetryable,
		FailureRecoveryAction: result.FailureRecoveryAction,
	}, now)
}

func soulBootstrapStateAfterHostedConversationSnapshot(
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
	state.BootstrapMode = workflow.SoulBootstrapModeHosted
	state.AuthorityModel = workflow.SoulBootstrapAuthorityModelInstanceTrust
	state.AnchorState = firstNonEmpty(state.AnchorState, workflow.SoulBootstrapAnchorStateHostedOffchain)
	state.AssuranceState = state.AnchorState

	status := ""
	if result != nil {
		status = soulservice.NormalizeHostedBootstrapConversationStatus(result.Status)
	}
	if status == "" {
		// G16 (P52 L3.2): do not silently coerce an empty/unknown Host status
		// to in_progress — that would mask a Host regression as progress.
		// Prefer the existing state's status; only fall back to in_progress
		// when there is no prior status (an honest default for a brand-new
		// conversation). The transport guarantees in_progress for a 202
		// (L3.1) and validateHostConversationSnapshot rejects empty status for
		// a 200/recover/complete, so reaching here with result != nil and no
		// prior status signals an unexpected caller, not silent progress.
		if prior := graphSoulBootstrapHostConversationStatus(state); prior != "" {
			status = prior
		} else {
			status = workflow.SoulBootstrapHostConversationStatusInProgress
		}
	}
	if result != nil && strings.TrimSpace(result.RegistrationID) != "" {
		state.HostRegistrationID = strings.TrimSpace(result.RegistrationID)
	}
	if result != nil && strings.TrimSpace(result.HostSoulAgentID) != "" {
		state.HostSoulAgentID = strings.TrimSpace(result.HostSoulAgentID)
	}
	if result != nil && strings.TrimSpace(result.ConversationID) != "" {
		state.HostConversationID = strings.TrimSpace(result.ConversationID)
	}
	state.HostedConversation = soulBootstrapHostedConversationProjection(state, result, status, now)

	switch status {
	case workflow.SoulBootstrapHostConversationStatusInProgress:
		// G14 (P52 L3.2): in_progress is a pending turn — Host has accepted
		// the message (202) and is processing it. The client must poll via
		// REFRESH_STATE rather than immediately re-sending (which would
		// re-issue a blocking turn Host is already running). The locked
		// projection table permits SEND as an alternative action for in_progress,
		// so availableActions keeps SEND; the authored typedNextAction is
		// REFRESH_STATE. This realigns the resolver with the contract and with
		// the 202-accepted-pending transport (L3.1).
		state.Phase = workflow.SoulBootstrapPhaseConversation
		state.State = workflow.SoulBootstrapStateConversationInProgress
		state.NextAction = workflow.SoulBootstrapNextActionRefreshState
		state.RecoveryCategory = workflow.SoulBootstrapRecoveryCategoryRefreshState
		state.RecoveryAction = workflow.SoulBootstrapRecoveryActionRefreshState
		state.Retryable = false
		state.RestartRequired = false
		state.SigningCheckpoints = nil
		state.Error = nil
	case workflow.SoulBootstrapHostConversationStatusAssistantTurnReady:
		state.Phase = workflow.SoulBootstrapPhaseConversation
		state.State = workflow.SoulBootstrapStateConversationAssistantTurnReady
		state.NextAction = workflow.SoulBootstrapNextActionCompleteHostedGenesis
		state.RecoveryCategory = ""
		state.RecoveryAction = ""
		state.Retryable = false
		state.RestartRequired = false
		state.SigningCheckpoints = nil
		state.Error = nil
	case workflow.SoulBootstrapHostConversationStatusDeclarationExtractionPending:
		// G14 (P52 L3.2): while Host is still extracting the declaration, the
		// turn is pending — the client must poll (REFRESH_STATE), not send
		// another message. Sending again would re-issue a blocking turn Host
		// is already processing. This realigns the resolver with the locked
		// projection table (docs/contracts/hosted-soul-genesis-projection.md),
		// which routes declaration_extraction_pending to REFRESH_STATE.
		state.Phase = workflow.SoulBootstrapPhaseConversation
		state.State = workflow.SoulBootstrapStateConversationDeclarationExtractionPending
		state.NextAction = workflow.SoulBootstrapNextActionRefreshState
		state.RecoveryCategory = workflow.SoulBootstrapRecoveryCategoryRefreshState
		state.RecoveryAction = workflow.SoulBootstrapRecoveryActionRefreshState
		state.Retryable = false
		state.RestartRequired = false
		state.SigningCheckpoints = nil
		state.Error = nil
	case workflow.SoulBootstrapHostConversationStatusDeclarationReady:
		state.Phase = workflow.SoulBootstrapPhaseFinalize
		state.State = workflow.SoulBootstrapStateConversationDeclarationReady
		state.NextAction = workflow.SoulBootstrapNextActionPublishHostedSoul
		state.RecoveryCategory = ""
		state.RecoveryAction = ""
		state.Retryable = false
		state.RestartRequired = false
		completedAt := now
		if result != nil && result.CompletedAt != nil {
			completedAt = *result.CompletedAt
		}
		state.SigningCheckpoints = upsertSoulBootstrapCheckpoint(state.SigningCheckpoints, workflow.SoulBootstrapSigningCheckpoint{
			Name:          soulBootstrapCheckpointHostedConversation,
			Status:        workflow.SoulBootstrapHostConversationStatusDeclarationReady,
			CanonicalJSON: hostedTerminalEvidenceCanonicalJSON(result.ConversationID, workflow.SoulBootstrapHostConversationStatusDeclarationReady, result.ProducedDeclarations),
			HostRequestID: result.HostRequestID,
			CompletedAt:   &completedAt,
		})
		state.Error = nil
	case workflow.SoulBootstrapHostConversationStatusFailed:
		state.Phase = workflow.SoulBootstrapPhaseError
		state.State = workflow.SoulBootstrapStateHostFailed
		state.SigningCheckpoints = nil
		code := workflow.SoulBootstrapErrorHostConversationFailed
		message := "Host failed before producing declaration evidence."
		retryable := true
		recoveryCategory := workflow.SoulBootstrapRecoveryCategoryRetrySameStep
		recoveryAction := workflow.SoulBootstrapRecoveryActionRetrySameStep
		nextAction := workflow.SoulBootstrapNextActionRetrySameStep
		if result != nil {
			if strings.TrimSpace(result.FailureCode) != "" {
				code = strings.TrimSpace(result.FailureCode)
			}
			if strings.TrimSpace(result.FailureMessage) != "" {
				message = strings.TrimSpace(result.FailureMessage)
			}
			retryable = result.FailureRetryable
			recoveryCategory, recoveryAction, nextAction = hostedFailureRecovery(result.FailureRecoveryAction, result.FailureRetryable)
		}
		state.NextAction = nextAction
		state.RecoveryCategory = recoveryCategory
		state.RecoveryAction = recoveryAction
		state.Retryable = retryable
		state.RestartRequired = recoveryCategory == workflow.SoulBootstrapRecoveryCategoryRestartRequired
		state.Error = &workflow.SoulBootstrapErrorState{
			Code:             code,
			Message:          message,
			Source:           "host",
			HostRequestID:    resultHostRequestID(result),
			RecoveryCategory: recoveryCategory,
			RecoveryAction:   recoveryAction,
			Retryable:        retryable,
			RestartRequired:  state.RestartRequired,
			At:               &now,
		}
	default:
		state.Phase = workflow.SoulBootstrapPhaseError
		state.State = workflow.SoulBootstrapStateHostUnavailable
		state.NextAction = workflow.SoulBootstrapNextActionRetrySameStep
		state.RecoveryCategory = workflow.SoulBootstrapRecoveryCategoryRetrySameStep
		state.RecoveryAction = workflow.SoulBootstrapRecoveryActionRetrySameStep
		state.Retryable = true
		state.RestartRequired = false
		state.SigningCheckpoints = nil
		state.Error = &workflow.SoulBootstrapErrorState{
			Code:             "HOST_RESPONSE_INVALID",
			Message:          "Host conversation response used an unsupported status.",
			Source:           "host",
			HostRequestID:    resultHostRequestID(result),
			RecoveryCategory: workflow.SoulBootstrapRecoveryCategoryRetrySameStep,
			RecoveryAction:   workflow.SoulBootstrapRecoveryActionRetrySameStep,
			Retryable:        true,
			At:               &now,
		}
	}
	state.Correlation = mergeSoulBootstrapCorrelation(state.Correlation, correlation)
	if state.Correlation == nil {
		state.Correlation = &workflow.SoulBootstrapCorrelationState{}
	}
	state.Correlation.LastHostRequestID = resultHostRequestID(result)
	state.UpdatedAt = &now
	return workflow.NormalizeSoulBootstrap(state, username)
}

func soulBootstrapStateAfterHostedConversationComplete(
	agentUser *storage.User,
	existing *workflow.SoulBootstrapState,
	correlation *workflow.SoulBootstrapCorrelationState,
	result *soulservice.BootstrapConversationCompleteResult,
	now time.Time,
) *workflow.SoulBootstrapState {
	return soulBootstrapStateAfterHostedConversationSnapshot(agentUser, existing, correlation, result, now)
}

func resultHostRequestID(result *soulservice.BootstrapConversationCompleteResult) string {
	if result == nil {
		return ""
	}
	return strings.TrimSpace(result.HostRequestID)
}

func cloneBootstrapConversationMessages(in []soulservice.BootstrapConversationMessage) []soulservice.BootstrapConversationMessage {
	if len(in) == 0 {
		return nil
	}
	out := make([]soulservice.BootstrapConversationMessage, len(in))
	copy(out, in)
	return out
}

func soulBootstrapHostedConversationProjection(
	state *workflow.SoulBootstrapState,
	result *soulservice.BootstrapConversationCompleteResult,
	status string,
	now time.Time,
) *workflow.SoulBootstrapHostedConversation {
	registrationID := ""
	conversationID := ""
	if state != nil {
		registrationID = strings.TrimSpace(state.HostRegistrationID)
		conversationID = strings.TrimSpace(state.HostConversationID)
	}
	if result != nil {
		registrationID = firstNonEmpty(result.RegistrationID, registrationID)
		conversationID = firstNonEmpty(result.ConversationID, conversationID)
	}
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return nil
	}
	status = soulservice.NormalizeHostedBootstrapConversationStatus(status)
	if status == "" && state != nil {
		status = graphSoulBootstrapHostConversationStatus(state)
	}
	if status == "" {
		status = workflow.SoulBootstrapHostConversationStatusInProgress
	}

	messages, unsafe := soulBootstrapHostedConversationMessages(resultMessages(result))
	updatedAt := (*time.Time)(nil)
	if result != nil {
		updatedAt = result.UpdatedAt
	}
	if updatedAt == nil && state != nil && state.HostedConversation != nil {
		updatedAt = state.HostedConversation.UpdatedAt
	}
	if updatedAt == nil && !now.IsZero() {
		updated := now.UTC()
		updatedAt = &updated
	}

	return &workflow.SoulBootstrapHostedConversation{
		RegistrationID:    strings.TrimSpace(registrationID),
		ConversationID:    conversationID,
		Status:            status,
		LatestTurnID:      strings.TrimSpace(resultLatestTurnID(result)),
		MessageCount:      resultMessageCount(result),
		Messages:          messages,
		MessagesTruncated: resultMessagesTruncated(result) || unsafe,
		RequestID:         resultHostRequestID(result),
		UpdatedAt:         updatedAt,
	}
}

func resultMessages(result *soulservice.BootstrapConversationCompleteResult) []soulservice.BootstrapConversationMessage {
	if result == nil {
		return nil
	}
	return result.Messages
}

func resultLatestTurnID(result *soulservice.BootstrapConversationCompleteResult) string {
	if result == nil {
		return ""
	}
	return result.LatestTurnID
}

func resultMessageCount(result *soulservice.BootstrapConversationCompleteResult) int {
	if result == nil {
		return 0
	}
	return result.MessageCount
}

func resultMessagesTruncated(result *soulservice.BootstrapConversationCompleteResult) bool {
	if result == nil {
		return false
	}
	return result.MessagesTruncated
}

func soulBootstrapHostedConversationMessages(in []soulservice.BootstrapConversationMessage) ([]workflow.SoulBootstrapHostedConversationMessage, bool) {
	if len(in) == 0 {
		return nil, false
	}
	out := make([]workflow.SoulBootstrapHostedConversationMessage, 0, len(in))
	for idx, message := range in {
		role := strings.ToLower(strings.TrimSpace(message.Role))
		if role != soulBootstrapHostedTranscriptRoleUser && role != soulBootstrapHostedTranscriptRoleAssistant {
			continue
		}
		content := strings.TrimSpace(message.Content)
		if content == "" {
			continue
		}
		if hostedGenesisTranscriptContentUnsafe(content) {
			return nil, true
		}
		order := message.Order
		if order <= 0 {
			order = idx + 1
		}
		id := strings.TrimSpace(message.ID)
		if id == "" {
			id = "msg_" + leftPadInt(order, 6)
		}
		out = append(out, workflow.SoulBootstrapHostedConversationMessage{
			ID:        id,
			Role:      role,
			Content:   content,
			Order:     order,
			CreatedAt: message.CreatedAt,
			Truncated: message.Truncated,
		})
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, false
}

func hostedGenesisTranscriptContentUnsafe(content string) bool {
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

func leftPadInt(value int, width int) string {
	raw := strconv.Itoa(value)
	for len(raw) < width {
		raw = "0" + raw
	}
	return raw
}

func hostedFailureRecovery(action string, _ bool) (string, string, string) {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "refresh_state":
		return workflow.SoulBootstrapRecoveryCategoryRefreshState,
			workflow.SoulBootstrapRecoveryActionRefreshState,
			workflow.SoulBootstrapNextActionRefreshState
	case "retry_same_step":
		return workflow.SoulBootstrapRecoveryCategoryRetrySameStep,
			workflow.SoulBootstrapRecoveryActionRetrySameStep,
			workflow.SoulBootstrapNextActionRetrySameStep
	case "restart_soul_bootstrap", "restart_bootstrap":
		return workflow.SoulBootstrapRecoveryCategoryRestartRequired,
			workflow.SoulBootstrapRecoveryActionRestartBootstrap,
			workflow.SoulBootstrapNextActionRestartSoulBootstrap
	case "operator_action", "operator_action_required", "contact_operator":
		return workflow.SoulBootstrapRecoveryCategoryOperatorActionRequired,
			workflow.SoulBootstrapRecoveryActionContactOperator,
			workflow.SoulBootstrapNextActionOperatorActionRequired
	default:
		return workflow.SoulBootstrapRecoveryCategoryOperatorActionRequired,
			workflow.SoulBootstrapRecoveryActionContactOperator,
			workflow.SoulBootstrapNextActionOperatorActionRequired
	}
}

func soulBootstrapStateAfterHostedPublish(
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
	state.BootstrapMode = workflow.SoulBootstrapModeHosted
	state.AuthorityModel = workflow.SoulBootstrapAuthorityModelInstanceTrust
	state.AnchorState = workflow.SoulBootstrapAnchorStateHostedOffchain
	state.AssuranceState = state.AnchorState
	state.NextAction = workflow.SoulBootstrapNextActionComplete
	if strings.TrimSpace(result.HostSoulAgentID) != "" {
		state.HostSoulAgentID = strings.TrimSpace(result.HostSoulAgentID)
	}
	state.Publication = soulBootstrapPublicationEvidence(result.Publication)
	state.Correlation = mergeSoulBootstrapCorrelation(state.Correlation, correlation)
	if state.Correlation == nil {
		state.Correlation = &workflow.SoulBootstrapCorrelationState{}
	}
	state.Correlation.LastHostRequestID = strings.TrimSpace(result.HostRequestID)
	state.Error = nil
	state.UpdatedAt = &now
	return workflow.NormalizeSoulBootstrap(state, username)
}

func soulBootstrapStateAfterHostedBinding(
	agentUser *storage.User,
	existing *workflow.SoulBootstrapState,
	correlation *workflow.SoulBootstrapCorrelationState,
	soul *soulservice.Soul,
	result *soulservice.BootstrapFinalizeResult,
	now time.Time,
) *workflow.SoulBootstrapState {
	state := soulBootstrapStateAfterBinding(agentUser, existing, correlation, soul, result, now)
	state.BootstrapMode = workflow.SoulBootstrapModeHosted
	state.AuthorityModel = workflow.SoulBootstrapAuthorityModelInstanceTrust
	state.AnchorState = workflow.SoulBootstrapAnchorStateHostedOffchain
	state.AssuranceState = state.AnchorState
	state.NextAction = workflow.SoulBootstrapNextActionComplete
	return workflow.NormalizeSoulBootstrap(state, state.Username)
}

func soulBootstrapRestartReplayState(
	agentUser *storage.User,
	existing *workflow.SoulBootstrapState,
	correlation *workflow.SoulBootstrapCorrelationState,
	now time.Time,
) (*workflow.SoulBootstrapState, bool) {
	if existing == nil || correlation == nil || strings.TrimSpace(correlation.RecoveryAttemptID) == "" {
		return nil, false
	}
	username := ""
	if agentUser != nil {
		username = agentUser.Username
	}
	state := workflow.NormalizeSoulBootstrap(existing, username)
	if state.Correlation == nil || strings.TrimSpace(state.Correlation.RecoveryAttemptID) == "" {
		return nil, false
	}
	if strings.TrimSpace(state.Correlation.RecoveryAttemptID) != strings.TrimSpace(correlation.RecoveryAttemptID) {
		return nil, false
	}
	if strings.TrimSpace(correlation.RestartIdempotencyKey) != "" &&
		strings.TrimSpace(state.Correlation.RestartIdempotencyKey) != "" &&
		strings.TrimSpace(state.Correlation.RestartIdempotencyKey) != strings.TrimSpace(correlation.RestartIdempotencyKey) {
		return nil, false
	}
	state.Correlation = mergeSoulBootstrapCorrelation(state.Correlation, correlation)
	state.UpdatedAt = &now
	return workflow.NormalizeSoulBootstrap(state, username), true
}

func soulBootstrapRestartCorrelation(existing *workflow.SoulBootstrapState, correlation *workflow.SoulBootstrapCorrelationState) *workflow.SoulBootstrapCorrelationState {
	out := mergeSoulBootstrapCorrelation(nil, correlation)
	if out == nil {
		out = &workflow.SoulBootstrapCorrelationState{}
	}
	if existing != nil {
		out.SupersededHostRegistrationID = strings.TrimSpace(existing.HostRegistrationID)
		out.SupersededHostConversationID = strings.TrimSpace(existing.HostConversationID)
	}
	return out
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
	state.BootstrapMode = workflow.SoulBootstrapModeWalletPrincipal
	state.AuthorityModel = workflow.SoulBootstrapAuthorityModelWalletPrincipal
	state.NextAction = workflow.SoulBootstrapNextActionVerifyPrincipalDeclaration
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
	state.BootstrapMode = workflow.SoulBootstrapModeWalletPrincipal
	state.AuthorityModel = workflow.SoulBootstrapAuthorityModelWalletPrincipal
	state.NextAction = workflow.SoulBootstrapNextActionContinueConversation
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
	state.BootstrapMode = workflow.SoulBootstrapModeWalletPrincipal
	state.AuthorityModel = workflow.SoulBootstrapAuthorityModelWalletPrincipal
	state.NextAction = workflow.SoulBootstrapNextActionContinueConversation
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
	state.BootstrapMode = workflow.SoulBootstrapModeWalletPrincipal
	state.AuthorityModel = workflow.SoulBootstrapAuthorityModelWalletPrincipal
	state.NextAction = workflow.SoulBootstrapNextActionFinalize
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
		Name:          soulBootstrapCheckpointConversation,
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
	state.BootstrapMode = workflow.SoulBootstrapModeWalletPrincipal
	state.AuthorityModel = workflow.SoulBootstrapAuthorityModelWalletPrincipal
	state.NextAction = workflow.SoulBootstrapNextActionFinalize
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
	state.BootstrapMode = workflow.SoulBootstrapModeWalletPrincipal
	state.AuthorityModel = workflow.SoulBootstrapAuthorityModelWalletPrincipal
	state.NextAction = workflow.SoulBootstrapNextActionComplete
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
	state.NextAction = workflow.SoulBootstrapNextActionComplete
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
		AuthorityModel:             strings.TrimSpace(in.AuthorityModel),
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
	recovery := soulBootstrapRecoveryForError(details.Code, details.StatusCode, details.Message)
	state.Username = username
	state.BodyID = defaultString(state.BodyID, bodyID)
	state.Phase = workflow.SoulBootstrapPhaseError
	state.State = soulBootstrapErrorWorkflowState(details.Code)
	state.NextAction = recovery.NextAction
	state.RecoveryCategory = recovery.Category
	state.RecoveryAction = recovery.Action
	state.Retryable = recovery.Retryable
	state.RestartRequired = recovery.RestartRequired
	state.Correlation = mergeSoulBootstrapCorrelation(state.Correlation, correlation)
	state.Error = &workflow.SoulBootstrapErrorState{
		Code:             details.Code,
		Message:          details.Message,
		Source:           details.Source,
		StatusCode:       details.StatusCode,
		DetailsJSON:      details.DetailsJSON,
		HostRequestID:    details.HostRequestID,
		RecoveryCategory: recovery.Category,
		RecoveryAction:   recovery.Action,
		Retryable:        recovery.Retryable,
		RestartRequired:  recovery.RestartRequired,
		At:               &now,
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
	DetailsJSON   string
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
			DetailsJSON:   strings.TrimSpace(hostErr.DetailsJSON),
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

type soulBootstrapRecoveryPlan struct {
	Category        string
	Action          string
	NextAction      string
	Retryable       bool
	RestartRequired bool
}

func soulBootstrapRecoveryForError(code string, statusCode int, message string) soulBootstrapRecoveryPlan {
	if soulBootstrapConversationNotInProgressConflict(code, statusCode, message) {
		return soulBootstrapRecoveryPlan{
			Category:        workflow.SoulBootstrapRecoveryCategoryRestartRequired,
			Action:          workflow.SoulBootstrapRecoveryActionRestartBootstrap,
			NextAction:      workflow.SoulBootstrapNextActionRestartSoulBootstrap,
			RestartRequired: true,
		}
	}
	switch strings.TrimSpace(code) {
	case workflow.SoulBootstrapErrorHostUnavailable:
		// G15 (P52 L3.2): a Host unavailable / accept-timeout on the send POST
		// must NOT map to RETRY_SAME_STEP. RETRY_SAME_STEP would instruct the
		// client to re-issue the same blocking send — the binding constraint
		// this project removes. Under the MicroVM-only contract, Host may have
		// accepted the turn and be processing it asynchronously; the safe
		// forward motion is to poll state (REFRESH_STATE), which reconciles via
		// the read path and recovers the conversation id. Not Retryable as a
		// blocking re-issue; RestartRequired stays false.
		return soulBootstrapRecoveryPlan{
			Category:   workflow.SoulBootstrapRecoveryCategoryRefreshState,
			Action:     workflow.SoulBootstrapRecoveryActionRefreshState,
			NextAction: workflow.SoulBootstrapNextActionRefreshState,
		}
	case workflow.SoulBootstrapErrorHostTrustNotConfigured,
		workflow.SoulBootstrapErrorHostInstanceKeyMissing,
		workflow.SoulBootstrapErrorHostInstanceKeyUnavailable,
		"soul_instance.unauthorized",
		"soul_instance.boundary_violation":
		return soulBootstrapRecoveryPlan{
			Category:   workflow.SoulBootstrapRecoveryCategoryOperatorActionRequired,
			Action:     workflow.SoulBootstrapRecoveryActionContactOperator,
			NextAction: workflow.SoulBootstrapNextActionOperatorActionRequired,
		}
	case "soul_instance.not_found":
		return soulBootstrapRecoveryPlan{
			Category:        workflow.SoulBootstrapRecoveryCategoryRestartRequired,
			Action:          workflow.SoulBootstrapRecoveryActionRestartBootstrap,
			NextAction:      workflow.SoulBootstrapNextActionRestartSoulBootstrap,
			RestartRequired: true,
		}
	case workflow.SoulBootstrapErrorHostRegistrationIDRequired,
		workflow.SoulBootstrapErrorHostConversationIDRequired,
		workflow.SoulBootstrapErrorHostBootstrapReplayRejected,
		"soul_instance.invalid_request",
		"soul_instance.conflict":
		return soulBootstrapRecoveryPlan{
			Category:   workflow.SoulBootstrapRecoveryCategoryRefreshState,
			Action:     workflow.SoulBootstrapRecoveryActionRefreshState,
			NextAction: workflow.SoulBootstrapNextActionRefreshState,
		}
	case workflow.SoulBootstrapErrorSoulBindingConflict,
		workflow.SoulBootstrapErrorSoulNotAvailable:
		return soulBootstrapRecoveryPlan{
			Category:   workflow.SoulBootstrapRecoveryCategoryOperatorActionRequired,
			Action:     workflow.SoulBootstrapRecoveryActionContactOperator,
			NextAction: workflow.SoulBootstrapNextActionOperatorActionRequired,
		}
	default:
		_ = statusCode
		return soulBootstrapRecoveryPlan{
			Category:   workflow.SoulBootstrapRecoveryCategoryRetrySameStep,
			Action:     workflow.SoulBootstrapRecoveryActionRetrySameStep,
			NextAction: workflow.SoulBootstrapNextActionRetrySameStep,
			Retryable:  true,
		}
	}
}

func soulBootstrapConversationNotInProgressConflict(code string, statusCode int, message string) bool {
	code = strings.TrimSpace(code)
	if code != "soul_instance.conflict" && code != "HOST_BOOTSTRAP_CONFLICT" && statusCode != 409 {
		return false
	}
	return strings.Contains(strings.ToLower(strings.TrimSpace(message)), "conversation is not in progress")
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
	if strings.TrimSpace(next.RestartIdempotencyKey) != "" {
		out.RestartIdempotencyKey = strings.TrimSpace(next.RestartIdempotencyKey)
	}
	if strings.TrimSpace(next.RecoveryAttemptID) != "" {
		out.RecoveryAttemptID = strings.TrimSpace(next.RecoveryAttemptID)
	}
	if strings.TrimSpace(next.SupersededHostRegistrationID) != "" {
		out.SupersededHostRegistrationID = strings.TrimSpace(next.SupersededHostRegistrationID)
	}
	if strings.TrimSpace(next.SupersededHostConversationID) != "" {
		out.SupersededHostConversationID = strings.TrimSpace(next.SupersededHostConversationID)
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
	out := make([]workflow.SoulBootstrapSigningCheckpoint, 0, soulBootstrapCheckpointUpsertCapacity(existing))
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

func soulBootstrapCheckpointUpsertCapacity(existing []workflow.SoulBootstrapSigningCheckpoint) int {
	capacity := len(existing)
	if capacity < math.MaxInt {
		capacity++
	}
	return capacity
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
	typedNextAction := graphSoulBootstrapNextActionEnum(state, soulBindingState)
	return &model.SoulBootstrapSurface{
		Username:            agentUser.Username,
		Body:                graphSoulBootstrapIdentityTarget(ctx, r, agentUser),
		Workflow:            surface,
		State:               state,
		SoulBindingState:    soulBindingState,
		ExistingSoulAgentID: optionalString(existingSoulAgentID),
		HostBridgeAvailable: hostBridgeAvailable,
		Executable:          hostBridgeAvailable && soulBindingState != model.SoulBindingStateBound,
		NextAction:          optionalString(strings.ToLower(string(typedNextAction))),
		TypedNextAction:     typedNextAction,
		AvailableActions:    graphSoulBootstrapAvailableActions(state, soulBindingState),
		RecoveryCategory:    graphSoulBootstrapSurfaceRecoveryCategory(state),
		RecoveryAction:      graphSoulBootstrapSurfaceRecoveryAction(state),
		Retryable:           state != nil && state.Retryable,
		RestartAvailable:    state != nil && state.RestartAvailable,
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
		Username:                    state.Username,
		BodyID:                      state.BodyID,
		HostRegistrationID:          optionalString(state.HostRegistrationID),
		HostConversationID:          optionalString(state.HostConversationID),
		HostSoulAgentID:             optionalString(state.HostSoulAgentID),
		WalletAddress:               optionalString(state.WalletAddress),
		PrincipalAddress:            optionalString(state.PrincipalAddress),
		BootstrapMode:               graphSoulBootstrapMode(state.BootstrapMode),
		AuthorityModel:              graphSoulBootstrapAuthorityModel(state.AuthorityModel),
		AnchorState:                 graphSoulBootstrapAnchorStatePtr(state.AnchorState),
		AssuranceState:              graphSoulBootstrapAnchorStatePtr(state.AssuranceState),
		Phase:                       graphSoulBootstrapPhase(state.Phase),
		State:                       state.State,
		HostConversationStatus:      optionalString(graphSoulBootstrapHostConversationStatus(state)),
		TypedNextAction:             graphSoulBootstrapNextActionFromStored(state),
		RecoveryCategory:            graphSoulBootstrapRecoveryCategoryPtr(state.RecoveryCategory),
		RecoveryAction:              graphSoulBootstrapRecoveryActionPtr(state.RecoveryAction),
		Retryable:                   state.Retryable,
		RestartRequired:             state.RestartRequired,
		RestartAvailable:            soulBootstrapRestartAvailable(state),
		AvailableActions:            graphSoulBootstrapAvailableActionsFromStored(state),
		HostedGenesisConversation:   graphSoulBootstrapHostedGenesisConversation(state.HostedConversation),
		SigningCheckpoints:          graphSoulBootstrapSigningCheckpoints(state.SigningCheckpoints),
		TerminalDeclarationEvidence: graphSoulBootstrapTerminalDeclarationEvidence(state),
		Publication:                 graphSoulBootstrapPublication(state.Publication),
		PublicationEvidence:         graphSoulBootstrapPublication(state.Publication),
		PublishGate:                 graphSoulBootstrapPublishGate(state),
		Error:                       graphSoulBootstrapError(state.Error),
		Correlation:                 graphSoulBootstrapCorrelationModel(state.Correlation),
		RecoveryAttemptID:           optionalBootstrapRecoveryAttemptID(state.Correlation),
		RestartIdempotencyKey:       optionalBootstrapRestartID(state.Correlation),
		LastHostRequestID:           optionalBootstrapLastHostRequestID(state.Correlation),
		RestartedAt:                 graphTimePtr(state.RestartedAt),
		UpdatedAt:                   graphTimePtr(state.UpdatedAt),
	}
}

func graphSoulBootstrapHostConversationStatus(state *workflow.SoulBootstrapState) string {
	state = workflow.NormalizeSoulBootstrap(state, "")
	if state == nil {
		return ""
	}
	switch strings.TrimSpace(state.State) {
	case workflow.SoulBootstrapStateConversationRegistrationActive:
		return "registration_active"
	case workflow.SoulBootstrapStateConversationInProgress:
		return workflow.SoulBootstrapHostConversationStatusInProgress
	case workflow.SoulBootstrapStateConversationAssistantTurnReady:
		return workflow.SoulBootstrapHostConversationStatusAssistantTurnReady
	case workflow.SoulBootstrapStateConversationDeclarationExtractionPending:
		return workflow.SoulBootstrapHostConversationStatusDeclarationExtractionPending
	case workflow.SoulBootstrapStateConversationDeclarationReady,
		workflow.SoulBootstrapStateConversationCompleted:
		return workflow.SoulBootstrapHostConversationStatusDeclarationReady
	case workflow.SoulBootstrapStateHostFailed:
		return workflow.SoulBootstrapHostConversationStatusFailed
	case workflow.SoulBootstrapStateFinalizePublished:
		return workflow.SoulBootstrapHostConversationStatusPublished
	case workflow.SoulBootstrapStateCompleteBound:
		return workflow.SoulBootstrapHostConversationStatusBound
	default:
		if state.Publication != nil {
			return workflow.SoulBootstrapHostConversationStatusPublished
		}
		return ""
	}
}

func graphSoulBootstrapHostedGenesisConversation(in *workflow.SoulBootstrapHostedConversation) *model.SoulBootstrapHostedGenesisConversation {
	if in == nil || strings.TrimSpace(in.ConversationID) == "" {
		return nil
	}
	messages, unsafe := graphSoulBootstrapHostedGenesisMessages(in.Messages)
	return &model.SoulBootstrapHostedGenesisConversation{
		RegistrationID:    optionalString(in.RegistrationID),
		ConversationID:    strings.TrimSpace(in.ConversationID),
		Status:            defaultString(strings.TrimSpace(in.Status), workflow.SoulBootstrapHostConversationStatusInProgress),
		LatestTurnID:      optionalString(in.LatestTurnID),
		MessageCount:      in.MessageCount,
		Messages:          messages,
		MessagesTruncated: in.MessagesTruncated || unsafe,
		RequestID:         optionalString(in.RequestID),
		UpdatedAt:         graphTimePtr(in.UpdatedAt),
	}
}

func graphSoulBootstrapHostedGenesisMessages(in []workflow.SoulBootstrapHostedConversationMessage) ([]*model.SoulBootstrapHostedGenesisMessage, bool) {
	if len(in) == 0 {
		return []*model.SoulBootstrapHostedGenesisMessage{}, false
	}
	out := make([]*model.SoulBootstrapHostedGenesisMessage, 0, len(in))
	for idx, message := range in {
		role := graphSoulBootstrapHostedGenesisMessageRole(message.Role)
		if role == nil {
			continue
		}
		content := strings.TrimSpace(message.Content)
		if content == "" {
			continue
		}
		if hostedGenesisTranscriptContentUnsafe(content) {
			return []*model.SoulBootstrapHostedGenesisMessage{}, true
		}
		order := message.Order
		if order <= 0 {
			order = idx + 1
		}
		id := strings.TrimSpace(message.ID)
		if id == "" {
			id = "msg_" + leftPadInt(order, 6)
		}
		out = append(out, &model.SoulBootstrapHostedGenesisMessage{
			ID:        id,
			Role:      *role,
			Content:   content,
			Order:     order,
			CreatedAt: graphTimePtr(message.CreatedAt),
			Truncated: message.Truncated,
		})
	}
	return out, false
}

func graphSoulBootstrapHostedGenesisMessageRole(role string) *model.SoulBootstrapHostedGenesisMessageRole {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case soulBootstrapHostedTranscriptRoleUser:
		value := model.SoulBootstrapHostedGenesisMessageRole("USER")
		return &value
	case soulBootstrapHostedTranscriptRoleAssistant:
		value := model.SoulBootstrapHostedGenesisMessageRole("ASSISTANT")
		return &value
	default:
		return nil
	}
}

func graphSoulBootstrapTerminalDeclarationEvidence(state *workflow.SoulBootstrapState) *model.SoulBootstrapTerminalDeclarationEvidence {
	state = workflow.NormalizeSoulBootstrap(state, "")
	if state == nil || state.BootstrapMode != workflow.SoulBootstrapModeHosted {
		return nil
	}
	conversationID := strings.TrimSpace(state.HostConversationID)
	if conversationID == "" {
		return nil
	}
	for _, checkpoint := range state.SigningCheckpoints {
		name := strings.TrimSpace(checkpoint.Name)
		if !strings.EqualFold(name, soulBootstrapCheckpointHostedConversation) && !strings.EqualFold(name, soulBootstrapCheckpointConversation) {
			continue
		}
		if !soulservice.IsHostedBootstrapTerminalDeclarationStatus(checkpoint.Status) {
			continue
		}
		evidence, ok := hostedTerminalEvidenceFromCheckpoint(checkpoint, conversationID)
		if !ok {
			continue
		}
		return &model.SoulBootstrapTerminalDeclarationEvidence{
			ConversationID:              conversationID,
			HostStatus:                  evidence.HostStatus,
			HostRequestID:               optionalString(checkpoint.HostRequestID),
			DeclarationsHash:            optionalString(firstNonEmpty(evidence.DeclarationsHash, hostedDeclarationEvidenceHash(evidence.ProducedDeclarations))),
			ProducedDeclarationsPreview: hostedDeclarationPreview(evidence.ProducedDeclarations),
		}
	}
	return nil
}

type hostedTerminalEvidence struct {
	ConversationID       string
	HostStatus           string
	DeclarationsHash     string
	ProducedDeclarations string
}

type hostedTerminalEvidenceEnvelope struct {
	ConversationID       string          `json:"conversation_id"`
	HostStatus           string          `json:"host_status"`
	DeclarationsHash     string          `json:"declarations_hash,omitempty"`
	ProducedDeclarations json.RawMessage `json:"produced_declarations"`
}

func hostedTerminalEvidenceCanonicalJSON(conversationID string, hostStatus string, producedDeclarations string) string {
	producedDeclarations = strings.TrimSpace(producedDeclarations)
	if producedDeclarations == "" {
		return ""
	}
	compactProduced := compactGraphJSON(producedDeclarations)
	envelope := hostedTerminalEvidenceEnvelope{
		ConversationID:       strings.TrimSpace(conversationID),
		HostStatus:           soulservice.NormalizeHostedBootstrapConversationStatus(hostStatus),
		DeclarationsHash:     hostedDeclarationEvidenceHash(compactProduced),
		ProducedDeclarations: json.RawMessage(compactProduced),
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		return ""
	}
	return string(encoded)
}

func hostedTerminalEvidenceFromCheckpoint(checkpoint workflow.SoulBootstrapSigningCheckpoint, expectedConversationID string) (hostedTerminalEvidence, bool) {
	expectedConversationID = strings.TrimSpace(expectedConversationID)
	canonical := strings.TrimSpace(checkpoint.CanonicalJSON)
	if expectedConversationID == "" || canonical == "" {
		return hostedTerminalEvidence{}, false
	}
	var envelope hostedTerminalEvidenceEnvelope
	if err := json.Unmarshal([]byte(canonical), &envelope); err != nil {
		return hostedTerminalEvidence{}, false
	}
	if strings.TrimSpace(envelope.ConversationID) == "" || strings.TrimSpace(envelope.ConversationID) != expectedConversationID {
		return hostedTerminalEvidence{}, false
	}
	hostStatus := soulservice.NormalizeHostedBootstrapConversationStatus(firstNonEmpty(envelope.HostStatus, checkpoint.Status))
	if !soulservice.IsHostedBootstrapTerminalDeclarationStatus(hostStatus) {
		return hostedTerminalEvidence{}, false
	}
	producedDeclarations := graphRawJSONValue(envelope.ProducedDeclarations)
	if err := soulservice.ValidateHostedBootstrapCompletionEvidence(&soulservice.BootstrapConversationCompleteResult{
		ConversationID:       expectedConversationID,
		Status:               hostStatus,
		ProducedDeclarations: producedDeclarations,
		HostRequestID:        checkpoint.HostRequestID,
	}, expectedConversationID); err != nil {
		return hostedTerminalEvidence{}, false
	}
	return hostedTerminalEvidence{
		ConversationID:       expectedConversationID,
		HostStatus:           hostStatus,
		DeclarationsHash:     strings.TrimSpace(envelope.DeclarationsHash),
		ProducedDeclarations: producedDeclarations,
	}, true
}

func compactGraphJSON(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, []byte(raw)); err != nil {
		return raw
	}
	return compact.String()
}

func graphRawJSONValue(raw json.RawMessage) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return ""
	}
	if trimmed[0] == '"' {
		var value string
		if err := json.Unmarshal(trimmed, &value); err == nil {
			return strings.TrimSpace(value)
		}
	}
	return compactGraphJSON(string(trimmed))
}

func hostedDeclarationEvidenceHash(canonical string) string {
	canonical = strings.TrimSpace(canonical)
	if canonical == "" {
		return ""
	}
	compact := compactGraphJSON(canonical)
	sum := sha256.Sum256([]byte(compact))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func hostedDeclarationPreview(canonical string) *model.SoulBootstrapDeclarationPreview {
	canonical = strings.TrimSpace(canonical)
	if canonical == "" {
		return nil
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(canonical), &payload); err != nil {
		return nil
	}
	preview := &model.SoulBootstrapDeclarationPreview{DeclarationCount: len(payload)}
	if title := hostedDeclarationPreviewString(payload["title"]); title != "" {
		preview.Title = optionalString(title)
		return preview
	}
	var selfDescription map[string]json.RawMessage
	if err := json.Unmarshal(payload["selfDescription"], &selfDescription); err == nil {
		if title := hostedDeclarationPreviewString(selfDescription["title"]); title != "" {
			preview.Title = optionalString(title)
		} else if summary := hostedDeclarationPreviewString(selfDescription["summary"]); summary != "" {
			preview.Title = optionalString(summary)
		} else if name := hostedDeclarationPreviewString(selfDescription["name"]); name != "" {
			preview.Title = optionalString(name)
		}
	}
	return preview
}

func hostedDeclarationPreviewString(raw json.RawMessage) string {
	if len(bytes.TrimSpace(raw)) == 0 {
		return ""
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return ""
	}
	return strings.TrimSpace(value)
}

func graphSoulBootstrapPublishGate(state *workflow.SoulBootstrapState) *model.SoulBootstrapPublishGate {
	state = workflow.NormalizeSoulBootstrap(state, "")
	reason := ""
	canPublish := false
	switch {
	case state == nil || strings.TrimSpace(state.HostRegistrationID) == "":
		reason = "blocked:no_host_registration"
	case strings.TrimSpace(state.HostConversationID) == "":
		reason = "blocked:no_host_conversation"
	case strings.TrimSpace(state.State) == workflow.SoulBootstrapStateConversationInProgress:
		reason = "blocked:conversation_in_progress"
	case strings.TrimSpace(state.State) == workflow.SoulBootstrapStateConversationDeclarationExtractionPending:
		reason = "blocked:declaration_extraction_pending"
	case strings.TrimSpace(state.State) == workflow.SoulBootstrapStateHostFailed:
		reason = "blocked:host_failure"
	case strings.TrimSpace(state.State) == workflow.SoulBootstrapStateCompleteBound:
		reason = "complete:already_published_bound"
	case soulBootstrapStateHasActiveTerminalDeclarationEvidence(state):
		canPublish = true
		reason = "allowed:active_conversation_terminal_declaration_evidence"
	default:
		reason = "blocked:terminal_declaration_evidence_absent"
	}
	return &model.SoulBootstrapPublishGate{
		CanPublishHostedSoul: canPublish,
		Reason:               reason,
		RequiresActiveConversationTerminalDeclarationEvidence: true,
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

func graphSoulBootstrapMode(mode string) model.SoulBootstrapMode {
	switch strings.TrimSpace(mode) {
	case workflow.SoulBootstrapModeWalletPrincipal:
		return model.SoulBootstrapMode("WALLET_PRINCIPAL")
	default:
		return model.SoulBootstrapMode("HOSTED")
	}
}

func graphSoulBootstrapAuthorityModel(authorityModel string) model.SoulBootstrapAuthorityModel {
	switch strings.TrimSpace(authorityModel) {
	case workflow.SoulBootstrapAuthorityModelWalletPrincipal:
		return model.SoulBootstrapAuthorityModel("WALLET_PRINCIPAL")
	default:
		return model.SoulBootstrapAuthorityModel("INSTANCE_TRUST")
	}
}

func graphSoulBootstrapAnchorStatePtr(anchorState string) *model.SoulBootstrapAnchorState {
	switch strings.TrimSpace(anchorState) {
	case workflow.SoulBootstrapAnchorStateHostedOffchain:
		value := model.SoulBootstrapAnchorState("HOSTED_OFFCHAIN")
		return &value
	case workflow.SoulBootstrapAnchorStateImmutableOnchain:
		value := model.SoulBootstrapAnchorState("IMMUTABLE_ONCHAIN")
		return &value
	default:
		return nil
	}
}

func graphSoulBootstrapRecoveryCategoryPtr(category string) *model.SoulBootstrapRecoveryCategory {
	switch strings.TrimSpace(category) {
	case workflow.SoulBootstrapRecoveryCategoryRetrySameStep:
		value := model.SoulBootstrapRecoveryCategory("RETRY_SAME_STEP")
		return &value
	case workflow.SoulBootstrapRecoveryCategoryRestartRequired:
		value := model.SoulBootstrapRecoveryCategory("RESTART_REQUIRED")
		return &value
	case workflow.SoulBootstrapRecoveryCategoryOperatorActionRequired:
		value := model.SoulBootstrapRecoveryCategory("OPERATOR_ACTION_REQUIRED")
		return &value
	case workflow.SoulBootstrapRecoveryCategoryRefreshState:
		value := model.SoulBootstrapRecoveryCategory("REFRESH_STATE")
		return &value
	default:
		return nil
	}
}

func graphSoulBootstrapRecoveryActionPtr(action string) *model.SoulBootstrapRecoveryAction {
	switch strings.TrimSpace(action) {
	case workflow.SoulBootstrapRecoveryActionRetrySameStep:
		value := model.SoulBootstrapRecoveryAction("RETRY_SAME_STEP")
		return &value
	case workflow.SoulBootstrapRecoveryActionRestartBootstrap:
		value := model.SoulBootstrapRecoveryAction("RESTART_BOOTSTRAP")
		return &value
	case workflow.SoulBootstrapRecoveryActionContactOperator:
		value := model.SoulBootstrapRecoveryAction("CONTACT_OPERATOR")
		return &value
	case workflow.SoulBootstrapRecoveryActionRefreshState:
		value := model.SoulBootstrapRecoveryAction("REFRESH_STATE")
		return &value
	default:
		return nil
	}
}

func graphSoulBootstrapNextActionFromStored(state *workflow.SoulBootstrapState) model.SoulBootstrapNextAction {
	if action, ok := graphSoulBootstrapHostedConversationNextAction(state); ok {
		return action
	}
	if state != nil {
		if action := graphSoulBootstrapNextActionEnumString(state.NextAction); action != "" {
			if action == "PUBLISH_HOSTED_SOUL" && !soulBootstrapHasTerminalConversationDeclarationEvidence(state.SigningCheckpoints, state.HostConversationID) {
				return graphSoulBootstrapNextActionEnum(graphSoulBootstrapStoredStateModelShallow(state), model.SoulBindingStateUnbound)
			}
			return model.SoulBootstrapNextAction(action)
		}
	}
	return graphSoulBootstrapNextActionEnum(graphSoulBootstrapStoredStateModelShallow(state), model.SoulBindingStateUnbound)
}

func graphSoulBootstrapAvailableActionsFromStored(state *workflow.SoulBootstrapState) []model.SoulBootstrapNextAction {
	state = workflow.NormalizeSoulBootstrap(state, "")
	if state == nil {
		return []model.SoulBootstrapNextAction{model.SoulBootstrapNextActionStartHostedBootstrap}
	}
	if actions := graphSoulBootstrapHostedConversationAvailableActions(state); len(actions) > 0 {
		return actions
	}
	return []model.SoulBootstrapNextAction{graphSoulBootstrapNextActionFromStored(state)}
}

func graphSoulBootstrapHostedConversationNextAction(state *workflow.SoulBootstrapState) (model.SoulBootstrapNextAction, bool) {
	actions := graphSoulBootstrapHostedConversationAvailableActions(state)
	if len(actions) == 0 {
		return "", false
	}
	switch strings.TrimSpace(state.State) {
	case workflow.SoulBootstrapStateConversationAssistantTurnReady:
		return model.SoulBootstrapNextActionCompleteHostedSoulGenesis, true
	default:
		return actions[0], true
	}
}

func graphSoulBootstrapHostedConversationAvailableActions(state *workflow.SoulBootstrapState) []model.SoulBootstrapNextAction {
	state = workflow.NormalizeSoulBootstrap(state, "")
	if state == nil ||
		state.BootstrapMode != workflow.SoulBootstrapModeHosted ||
		state.Error != nil {
		return nil
	}
	switch strings.TrimSpace(state.State) {
	case workflow.SoulBootstrapStateConversationRegistrationActive:
		return []model.SoulBootstrapNextAction{
			model.SoulBootstrapNextActionSendHostedSoulGenesisMessage,
		}
	case workflow.SoulBootstrapStateConversationInProgress:
		// G14 (P52 L3.2): in_progress is a pending turn. Poll (REFRESH_STATE)
		// is primary; the locked projection table permits SEND as an
		// alternative, so both are advertised with REFRESH_STATE first.
		return []model.SoulBootstrapNextAction{
			model.SoulBootstrapNextActionRefreshState,
			model.SoulBootstrapNextActionSendHostedSoulGenesisMessage,
		}
	case workflow.SoulBootstrapStateConversationDeclarationExtractionPending:
		// G14 (P52 L3.2): declaration extraction is pending — poll only.
		// The locked projection table allows REFRESH_STATE only here.
		return []model.SoulBootstrapNextAction{
			model.SoulBootstrapNextActionRefreshState,
		}
	case workflow.SoulBootstrapStateConversationAssistantTurnReady:
		return []model.SoulBootstrapNextAction{
			model.SoulBootstrapNextActionSendHostedSoulGenesisMessage,
			model.SoulBootstrapNextActionCompleteHostedSoulGenesis,
		}
	default:
		return nil
	}
}

func graphSoulBootstrapAvailableActions(state *model.SoulBootstrapState, bindingState model.SoulBindingState) []model.SoulBootstrapNextAction {
	if bindingState == model.SoulBindingStateBound {
		return []model.SoulBootstrapNextAction{model.SoulBootstrapNextActionComplete}
	}
	if state != nil && len(state.AvailableActions) > 0 {
		return dedupeSoulBootstrapActions(state.AvailableActions)
	}
	return []model.SoulBootstrapNextAction{graphSoulBootstrapNextActionEnum(state, bindingState)}
}

func dedupeSoulBootstrapActions(in []model.SoulBootstrapNextAction) []model.SoulBootstrapNextAction {
	if len(in) == 0 {
		return nil
	}
	out := make([]model.SoulBootstrapNextAction, 0, len(in))
	seen := map[model.SoulBootstrapNextAction]struct{}{}
	for _, action := range in {
		if action == "" {
			continue
		}
		if _, ok := seen[action]; ok {
			continue
		}
		seen[action] = struct{}{}
		out = append(out, action)
	}
	return out
}

func graphSoulBootstrapStoredStateModelShallow(state *workflow.SoulBootstrapState) *model.SoulBootstrapState {
	if state == nil {
		return nil
	}
	return &model.SoulBootstrapState{
		HostConversationID: optionalString(state.HostConversationID),
		Phase:              graphSoulBootstrapPhase(state.Phase),
		State:              state.State,
		BootstrapMode:      graphSoulBootstrapMode(state.BootstrapMode),
		AuthorityModel:     graphSoulBootstrapAuthorityModel(state.AuthorityModel),
		SigningCheckpoints: graphSoulBootstrapSigningCheckpoints(state.SigningCheckpoints),
		RestartRequired:    state.RestartRequired,
		RecoveryCategory:   graphSoulBootstrapRecoveryCategoryPtr(state.RecoveryCategory),
	}
}

func graphSoulBootstrapNextActionEnumString(action string) string {
	switch strings.TrimSpace(action) {
	case workflow.SoulBootstrapNextActionStartHostedBootstrap:
		return "START_HOSTED_BOOTSTRAP"
	case workflow.SoulBootstrapNextActionSendHostedGenesisMessage:
		return "SEND_HOSTED_SOUL_GENESIS_MESSAGE"
	case workflow.SoulBootstrapNextActionCompleteHostedGenesis:
		return "COMPLETE_HOSTED_SOUL_GENESIS"
	case workflow.SoulBootstrapNextActionPublishHostedSoul:
		return "PUBLISH_HOSTED_SOUL"
	case workflow.SoulBootstrapNextActionRestartSoulBootstrap:
		return "RESTART_SOUL_BOOTSTRAP"
	case workflow.SoulBootstrapNextActionRetrySameStep:
		return "RETRY_SAME_STEP"
	case workflow.SoulBootstrapNextActionRefreshState:
		return "REFRESH_STATE"
	case workflow.SoulBootstrapNextActionOperatorActionRequired:
		return "OPERATOR_ACTION_REQUIRED"
	case workflow.SoulBootstrapNextActionVerifyWallet:
		return "VERIFY_WALLET"
	case workflow.SoulBootstrapNextActionPreparePrincipalDeclaration:
		return "PREPARE_PRINCIPAL_DECLARATION"
	case workflow.SoulBootstrapNextActionVerifyPrincipalDeclaration:
		return "VERIFY_PRINCIPAL_DECLARATION"
	case workflow.SoulBootstrapNextActionContinueConversation:
		return "CONTINUE_CONVERSATION"
	case workflow.SoulBootstrapNextActionFinalize:
		return "FINALIZE"
	case workflow.SoulBootstrapNextActionComplete:
		return "COMPLETE"
	default:
		return ""
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
		AuthorityModel:             graphSoulBootstrapAuthorityModelPtr(in.AuthorityModel),
		RegistrationURI:            optionalString(in.RegistrationURI),
		RegistrationS3Key:          optionalString(in.RegistrationS3Key),
		VersionedRegistrationURI:   optionalString(in.VersionedRegistrationURI),
		VersionedRegistrationS3Key: optionalString(in.VersionedRegistrationS3Key),
		AnchorState:                optionalString(in.AnchorState),
		PublishedAt:                graphTimePtr(in.PublishedAt),
	}
}

func graphSoulBootstrapAuthorityModelPtr(authorityModel string) *model.SoulBootstrapAuthorityModel {
	if strings.TrimSpace(authorityModel) == "" {
		return nil
	}
	value := graphSoulBootstrapAuthorityModel(authorityModel)
	return &value
}

func graphSoulBootstrapError(in *workflow.SoulBootstrapErrorState) *model.SoulBootstrapErrorState {
	if in == nil {
		return nil
	}
	return &model.SoulBootstrapErrorState{
		Code:             defaultString(in.Code, workflow.SoulBootstrapErrorHostBridgeUnavailable),
		Message:          defaultString(in.Message, "Soul bootstrap is not executable yet."),
		Source:           optionalString(in.Source),
		StatusCode:       optionalInt(in.StatusCode),
		DetailsJSON:      optionalString(in.DetailsJSON),
		HostRequestID:    optionalString(in.HostRequestID),
		RecoveryCategory: graphSoulBootstrapRecoveryCategoryPtr(in.RecoveryCategory),
		RecoveryAction:   graphSoulBootstrapRecoveryActionPtr(in.RecoveryAction),
		Retryable:        in.Retryable,
		RestartRequired:  in.RestartRequired,
		At:               graphTimePtr(in.At),
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
		RestartIdempotencyKey:              optionalString(in.RestartIdempotencyKey),
		RecoveryAttemptID:                  optionalString(in.RecoveryAttemptID),
		SupersededHostRegistrationID:       optionalString(in.SupersededHostRegistrationID),
		SupersededHostConversationID:       optionalString(in.SupersededHostConversationID),
		LastHostRequestID:                  optionalString(in.LastHostRequestID),
	}
}

func graphSoulBootstrapSurfaceError(state *model.SoulBootstrapState) *model.SoulBootstrapErrorState {
	if state == nil {
		return nil
	}
	return state.Error
}

func graphSoulBootstrapSurfaceRecoveryCategory(state *model.SoulBootstrapState) *model.SoulBootstrapRecoveryCategory {
	if state == nil {
		return nil
	}
	return state.RecoveryCategory
}

func graphSoulBootstrapSurfaceRecoveryAction(state *model.SoulBootstrapState) *model.SoulBootstrapRecoveryAction {
	if state == nil {
		return nil
	}
	return state.RecoveryAction
}

func graphSoulBootstrapNextActionEnum(state *model.SoulBootstrapState, bindingState model.SoulBindingState) model.SoulBootstrapNextAction {
	if bindingState == model.SoulBindingStateBound {
		return model.SoulBootstrapNextAction("COMPLETE")
	}
	if state == nil {
		return model.SoulBootstrapNextAction("START_HOSTED_BOOTSTRAP")
	}
	if state.Error != nil {
		if state.RecoveryAction != nil {
			switch *state.RecoveryAction {
			case model.SoulBootstrapRecoveryAction("RESTART_BOOTSTRAP"):
				return model.SoulBootstrapNextAction("RESTART_SOUL_BOOTSTRAP")
			case model.SoulBootstrapRecoveryAction("CONTACT_OPERATOR"):
				return model.SoulBootstrapNextAction("OPERATOR_ACTION_REQUIRED")
			case model.SoulBootstrapRecoveryAction("REFRESH_STATE"):
				return model.SoulBootstrapNextAction("REFRESH_STATE")
			case model.SoulBootstrapRecoveryAction("RETRY_SAME_STEP"):
				return model.SoulBootstrapNextAction("RETRY_SAME_STEP")
			}
		}
		return model.SoulBootstrapNextAction("RETRY_SAME_STEP")
	}
	if state.TypedNextAction != "" {
		if state.TypedNextAction == model.SoulBootstrapNextAction("PUBLISH_HOSTED_SOUL") && !graphSoulBootstrapStateHasTerminalDeclarationEvidence(state) {
			return model.SoulBootstrapNextAction("COMPLETE_HOSTED_SOUL_GENESIS")
		}
		return state.TypedNextAction
	}
	switch state.Phase {
	case model.SoulBootstrapPhase("BEGIN"):
		if state.BootstrapMode == model.SoulBootstrapMode("HOSTED") {
			return model.SoulBootstrapNextAction("SEND_HOSTED_SOUL_GENESIS_MESSAGE")
		}
		return model.SoulBootstrapNextAction("VERIFY_WALLET")
	case model.SoulBootstrapPhase("WALLET_VERIFICATION"):
		return model.SoulBootstrapNextAction("PREPARE_PRINCIPAL_DECLARATION")
	case model.SoulBootstrapPhase("PRINCIPAL_DECLARATION"):
		return model.SoulBootstrapNextAction("VERIFY_PRINCIPAL_DECLARATION")
	case model.SoulBootstrapPhase("CONVERSATION"):
		if state.BootstrapMode == model.SoulBootstrapMode("HOSTED") && state.State == workflow.SoulBootstrapStateConversationCompleted && graphSoulBootstrapStateHasTerminalDeclarationEvidence(state) {
			return model.SoulBootstrapNextAction("PUBLISH_HOSTED_SOUL")
		}
		if state.BootstrapMode == model.SoulBootstrapMode("HOSTED") {
			return model.SoulBootstrapNextAction("COMPLETE_HOSTED_SOUL_GENESIS")
		}
		return model.SoulBootstrapNextAction("CONTINUE_CONVERSATION")
	case model.SoulBootstrapPhase("FINALIZE"):
		return model.SoulBootstrapNextAction("FINALIZE")
	case model.SoulBootstrapPhase("COMPLETE"):
		return model.SoulBootstrapNextAction("COMPLETE")
	default:
		return model.SoulBootstrapNextAction("START_HOSTED_BOOTSTRAP")
	}
}

func optionalBootstrapRecoveryAttemptID(in *workflow.SoulBootstrapCorrelationState) *string {
	if in == nil {
		return nil
	}
	return optionalString(in.RecoveryAttemptID)
}

func optionalBootstrapRestartID(in *workflow.SoulBootstrapCorrelationState) *string {
	if in == nil {
		return nil
	}
	return optionalString(in.RestartIdempotencyKey)
}

func optionalBootstrapLastHostRequestID(in *workflow.SoulBootstrapCorrelationState) *string {
	if in == nil {
		return nil
	}
	return optionalString(in.LastHostRequestID)
}

func soulBootstrapRestartAvailable(state *workflow.SoulBootstrapState) bool {
	if state == nil {
		return false
	}
	if state.Phase == workflow.SoulBootstrapPhaseComplete {
		return false
	}
	if state.RestartRequired {
		return true
	}
	if state.Phase == workflow.SoulBootstrapPhaseError {
		return true
	}
	if state.BootstrapMode != workflow.SoulBootstrapModeHosted {
		return false
	}
	return strings.TrimSpace(state.HostRegistrationID) != "" || strings.TrimSpace(state.HostConversationID) != ""
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

func graphHostedBootstrapCorrelation(correlationKey *string, idempotencyKey *string, operation string, recoveryAttemptID *string) *workflow.SoulBootstrapCorrelationState {
	correlation := graphBootstrapCorrelation(correlationKey, idempotencyKey, operation)
	if correlation == nil {
		correlation = &workflow.SoulBootstrapCorrelationState{}
	}
	if operation == soulBootstrapCorrelationOpRestart {
		correlation.RestartIdempotencyKey = strings.TrimSpace(derefString(idempotencyKey))
	}
	correlation.RecoveryAttemptID = strings.TrimSpace(derefString(recoveryAttemptID))
	if correlation.CorrelationKey == "" &&
		correlation.BeginIdempotencyKey == "" &&
		correlation.WalletVerificationIdempotencyKey == "" &&
		correlation.PrincipalDeclarationIdempotencyKey == "" &&
		correlation.ConversationIdempotencyKey == "" &&
		correlation.FinalizeIdempotencyKey == "" &&
		correlation.RestartIdempotencyKey == "" &&
		correlation.RecoveryAttemptID == "" {
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
		return "", soulBootstrapReplayRejectedError("registration id does not match local bootstrap state")
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
		return "", soulBootstrapReplayRejectedError("conversation id does not match local bootstrap state")
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
		return "", soulBootstrapReplayRejectedError("conversation id does not match local bootstrap state")
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

func soulBootstrapRequireHostedPublishEvidence(existing *workflow.SoulBootstrapState, conversationID string) error {
	state := workflow.NormalizeSoulBootstrap(existing, "")
	if state == nil {
		return soulBootstrapReplayRejectedError("hosted conversation completion evidence is required before publish")
	}
	conversationID = strings.TrimSpace(conversationID)
	if state.BootstrapMode != workflow.SoulBootstrapModeHosted {
		return soulBootstrapReplayRejectedError("hosted bootstrap state is required before publish")
	}
	if strings.TrimSpace(state.HostConversationID) == "" || strings.TrimSpace(state.HostConversationID) != conversationID {
		return soulBootstrapReplayRejectedError("hosted conversation id does not match local terminal evidence")
	}
	if !soulBootstrapStateHasActiveTerminalDeclarationEvidence(state) {
		return soulBootstrapReplayRejectedError("hosted conversation declaration evidence is required before publish")
	}
	return nil
}

func soulBootstrapHasTerminalConversationDeclarationEvidence(checkpoints []workflow.SoulBootstrapSigningCheckpoint, conversationID string) bool {
	conversationID = strings.TrimSpace(conversationID)
	for _, checkpoint := range checkpoints {
		name := strings.TrimSpace(checkpoint.Name)
		if !strings.EqualFold(name, soulBootstrapCheckpointHostedConversation) && !strings.EqualFold(name, soulBootstrapCheckpointConversation) {
			continue
		}
		if !soulservice.IsHostedBootstrapTerminalDeclarationStatus(checkpoint.Status) {
			continue
		}
		if _, ok := hostedTerminalEvidenceFromCheckpoint(checkpoint, conversationID); ok {
			return true
		}
	}
	return false
}

func soulBootstrapStateHasActiveTerminalDeclarationEvidence(state *workflow.SoulBootstrapState) bool {
	state = workflow.NormalizeSoulBootstrap(state, "")
	if state == nil || state.BootstrapMode != workflow.SoulBootstrapModeHosted {
		return false
	}
	conversationID := strings.TrimSpace(state.HostConversationID)
	if conversationID == "" {
		return false
	}
	return soulBootstrapHasTerminalConversationDeclarationEvidence(state.SigningCheckpoints, conversationID)
}

func graphSoulBootstrapStateHasTerminalDeclarationEvidence(state *model.SoulBootstrapState) bool {
	if state == nil || state.BootstrapMode != model.SoulBootstrapModeHosted {
		return false
	}
	conversationID := strings.TrimSpace(derefString(state.HostConversationID))
	for _, checkpoint := range state.SigningCheckpoints {
		if checkpoint == nil {
			continue
		}
		name := strings.TrimSpace(checkpoint.Name)
		if !strings.EqualFold(name, soulBootstrapCheckpointHostedConversation) && !strings.EqualFold(name, soulBootstrapCheckpointConversation) {
			continue
		}
		if !soulservice.IsHostedBootstrapTerminalDeclarationStatus(checkpoint.Status) {
			continue
		}
		if checkpoint.CanonicalJSON == nil {
			continue
		}
		if _, ok := hostedTerminalEvidenceFromCheckpoint(workflow.SoulBootstrapSigningCheckpoint{
			Name:          checkpoint.Name,
			Status:        checkpoint.Status,
			CanonicalJSON: *checkpoint.CanonicalJSON,
			HostRequestID: derefString(checkpoint.HostRequestID),
		}, conversationID); ok {
			return true
		}
	}
	return false
}

func soulBootstrapReplayRejectedError(message string) *soulservice.HostBootstrapError {
	return &soulservice.HostBootstrapError{
		Code:    workflow.SoulBootstrapErrorHostBootstrapReplayRejected,
		Message: message,
		Source:  soulBootstrapErrorSourceLesser,
		Err:     soulservice.ErrHostSigningPayloadUnsupported,
	}
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
	case bootstrap.BootstrapMode == workflow.SoulBootstrapModeHosted &&
		soulBootstrapStateHasActiveTerminalDeclarationEvidence(bootstrap):
		workflowState.CurrentPhase = workflow.DroneWorkflowPhaseGraduation
		workflowState.CurrentState = workflow.DroneWorkflowStateGraduationReady
	case bootstrap.Phase == workflow.SoulBootstrapPhaseConversation:
		if soulBootstrapStateHasActiveTerminalDeclarationEvidence(bootstrap) ||
			bootstrap.State == workflow.SoulBootstrapStateConversationCompleted {
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

	if soulBootstrapStateHasActiveTerminalDeclarationEvidence(bootstrap) ||
		bootstrap.State == workflow.SoulBootstrapStateConversationCompleted {
		workflowState.Declaration = &workflow.DroneDeclarationCard{
			ID:            agentUser.Username + ":bootstrap-declaration",
			Title:         "Soul bootstrap declaration",
			Statement:     "Host mint conversation completed and produced declaration material.",
			Confidence:    "hosted_offchain",
			Owner:         &ownerActor,
			DeclaredScope: []string{"identity_continuity", "hosted_offchain_registration"},
		}
	}
	if bootstrap.Phase == workflow.SoulBootstrapPhaseFinalize && bootstrap.BootstrapMode != workflow.SoulBootstrapModeHosted {
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
	}
	if bootstrap.Phase == workflow.SoulBootstrapPhaseFinalize && bootstrap.State == workflow.SoulBootstrapStateFinalizePublished {
		completedMilestones := []string{"conversation_completed", "finalize_signed", "host_publication_recorded"}
		if bootstrap.BootstrapMode == workflow.SoulBootstrapModeHosted {
			completedMilestones = []string{"conversation_completed", "hosted_publish_recorded"}
		}
		workflowState.Graduation = &workflow.DroneGraduationSummaryCard{
			ID:                  agentUser.Username + ":bootstrap-graduation",
			Title:               "Hosted soul publication",
			Readiness:           workflow.DroneGraduationReadinessReady,
			Summary:             "Host published the hosted/off-chain soul registration and returned publication evidence.",
			LaunchOwner:         &finalizerActor,
			CompletedMilestones: completedMilestones,
			ExitCriteria:        []string{"local_body_binding"},
			NextStep:            "Bind Host soul to local Lesser body",
			Metrics: []workflow.DroneMetric{
				{Label: "Soul agent", Value: bootstrap.HostSoulAgentID},
			},
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
