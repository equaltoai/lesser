package graph

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	workflow "github.com/equaltoai/lesser/pkg/agents"
	"github.com/equaltoai/lesser/pkg/auth"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/security/authz"
	"github.com/equaltoai/lesser/pkg/storage"
)

type agentResolver struct{ *Resolver }

func (r *agentResolver) IdentitySemantics(ctx context.Context, obj *model.Agent) (*model.AgentIdentitySemantics, error) {
	if obj == nil {
		return nil, nil
	}
	agentUser, governance, err := r.loadDroneAgent(ctx, obj.Username)
	if err != nil {
		return nil, err
	}
	_, identity, err := r.buildDroneWorkflowSurface(ctx, "", agentUser, governance)
	return identity, err
}

func (r *agentResolver) Workflow(ctx context.Context, obj *model.Agent) (*model.AgentWorkflowSurface, error) {
	if obj == nil {
		return nil, nil
	}
	claims, err := r.requireAuthClaims(ctx)
	if err != nil {
		return nil, nil
	}
	if err := requireDroneReadScope(claims); err != nil {
		return nil, err
	}

	agentUser, governance, err := r.loadDroneAgent(ctx, obj.Username)
	if err != nil {
		return nil, err
	}
	workflowState, err := r.loadDroneWorkflowState(agentUser)
	if err != nil {
		return nil, apperrors.InternalWithCause(err, "failed to decode drone workflow metadata")
	}
	if !r.canAccessDroneReviewWorkflow(ctx, claims, agentUser, workflowState) {
		return nil, nil
	}
	surface, _, err := r.buildDroneWorkflowSurface(ctx, claims.Username, agentUser, governance)
	return surface, err
}

func (r *queryResolver) MyDroneRequests(ctx context.Context) ([]*model.SoulRequestCard, error) {
	claims, err := r.requireAuthClaims(ctx)
	if err != nil {
		return nil, err
	}
	if err := requireDroneReadScope(claims); err != nil {
		return nil, err
	}

	agentUsers, err := r.listOwnedDroneAgents(ctx, claims.Username)
	if err != nil {
		return nil, err
	}
	out := make([]*model.SoulRequestCard, 0, len(agentUsers))
	for _, agentUser := range agentUsers {
		if agentUser == nil {
			continue
		}
		workflowState, err := r.loadDroneWorkflowState(agentUser)
		if err != nil || workflowState == nil || workflowState.Request == nil {
			continue
		}
		card := graphDroneRequestCardModel(workflowState, agentUser)
		if card != nil {
			out = append(out, card)
		}
	}
	return out, nil
}

func (r *queryResolver) MyDroneReviews(ctx context.Context) ([]*model.ReviewDecisionCard, error) {
	claims, err := r.requireAuthClaims(ctx)
	if err != nil {
		return nil, err
	}
	if err := requireDroneReadScope(claims); err != nil {
		return nil, err
	}

	role, _ := r.normalizedRole(ctx, claims.Username)
	agentUsers, err := r.listCandidateDroneReviewAgents(ctx, claims.Username, authz.IsModeratorOrAdmin(role))
	if err != nil {
		return nil, err
	}

	out := make([]*model.ReviewDecisionCard, 0, len(agentUsers))
	for _, agentUser := range agentUsers {
		if agentUser == nil {
			continue
		}
		workflowState, err := r.loadDroneWorkflowState(agentUser)
		if err != nil || workflowState == nil || workflowState.Review == nil {
			continue
		}
		if !authz.IsModeratorOrAdmin(role) && !droneWorkflowReviewerMatches(claims, workflowState) {
			continue
		}
		card := graphDroneReviewCardModel(workflowState)
		if card != nil {
			out = append(out, card)
		}
	}
	return out, nil
}

func (r *queryResolver) DroneWorkflow(ctx context.Context, username string) (*model.AgentWorkflowSurface, error) {
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
		return nil, apperrors.Forbidden("not authorized to view drone workflow")
	}
	surface, _, err := r.buildDroneWorkflowSurface(ctx, claims.Username, agentUser, governance)
	return surface, err
}

func (r *mutationResolver) RequestSoulPromotion(ctx context.Context, input model.RequestSoulPromotionInput) (*model.DroneWorkflowMutationPayload, error) {
	claims, err := r.requireAuthClaims(ctx)
	if err != nil {
		return nil, err
	}
	if err := requireDroneWriteScope(claims); err != nil {
		return nil, err
	}

	agentUser, governance, err := r.loadDroneAgent(ctx, input.Username)
	if err != nil {
		return nil, err
	}
	if !r.canViewDroneWorkflow(ctx, claims, agentUser) {
		return nil, apperrors.Forbidden("not authorized to request soul promotion")
	}

	workflowState, err := r.loadDroneWorkflowState(agentUser)
	if err != nil {
		return nil, apperrors.InternalWithCause(err, "failed to decode drone workflow metadata")
	}
	workflowState = workflow.NormalizeDroneWorkflow(workflowState)

	now := time.Now().UTC()
	title := strings.TrimSpace(input.Title)
	if title == "" {
		title = fmt.Sprintf("Promote %s to graduation review", strings.TrimSpace(agentUser.DisplayName))
	}
	summary := strings.TrimSpace(input.Summary)
	if summary == "" {
		return nil, apperrors.ValidationFailed("input.summary", "is required")
	}

	workflowState.CurrentPhase = workflow.DroneWorkflowPhaseRequest
	workflowState.CurrentState = workflow.DroneWorkflowStateRequestSubmitted
	workflowState.Request = &workflow.DroneRequestCard{
		ID:            agentUser.Username + ":request",
		Title:         title,
		Summary:       summary,
		RequestedBy:   r.droneWorkflowActor(ctx, claims.Username, "requester"),
		SubmittedAt:   &now,
		Constraints:   normalizeStringSlice(input.Constraints),
		Artifacts:     normalizeDroneArtifactsInput(input.Artifacts, agentUser.Username+":request:artifact"),
		RouteDecision: defaultString(derefString(input.RouteDecision), "review_for_graduation"),
		CurrentState:  workflow.DroneWorkflowStateRequestSubmitted,
	}
	if strings.TrimSpace(derefString(input.ConversationID)) != "" {
		workflowState.Conversation = &workflow.DroneConversationState{
			ConversationID: strings.TrimSpace(derefString(input.ConversationID)),
			UpdatedAt:      &now,
		}
	}
	workflowState.UpdatedAt = &now
	workflowState.Lifecycle = workflow.BuildDroneLifecycle(workflowState.CurrentPhase, workflowState.CurrentState)

	if err := r.persistDroneWorkflowState(ctx, agentUser, workflowState); err != nil {
		return nil, err
	}
	return r.droneWorkflowMutationPayload(ctx, claims.Username, agentUser, governance)
}

func (r *mutationResolver) ReviewSoulPromotion(ctx context.Context, input model.ReviewSoulPromotionInput) (*model.DroneWorkflowMutationPayload, error) {
	claims, err := r.requireAuthClaims(ctx)
	if err != nil {
		return nil, err
	}
	if err := requireDroneWriteScope(claims); err != nil {
		return nil, err
	}

	agentUser, governance, err := r.loadDroneAgent(ctx, input.Username)
	if err != nil {
		return nil, err
	}
	workflowState, err := r.loadDroneWorkflowState(agentUser)
	if err != nil {
		return nil, apperrors.InternalWithCause(err, "failed to decode drone workflow metadata")
	}
	if !r.canReviewDroneWorkflow(ctx, claims, agentUser, workflowState) {
		return nil, apperrors.Forbidden("not authorized to review soul promotion")
	}
	workflowState = workflow.NormalizeDroneWorkflow(workflowState)

	decision := strings.TrimSpace(strings.ToLower(input.Decision))
	switch decision {
	case workflow.DroneReviewDecisionApproved,
		workflow.DroneReviewDecisionChangesRequested,
		workflow.DroneReviewDecisionBlocked,
		workflow.DroneReviewDecisionQueued:
	default:
		return nil, apperrors.ValidationFailed("input.decision", "must be one of approved, changes_requested, blocked, queued")
	}

	now := time.Now().UTC()
	title := strings.TrimSpace(derefString(input.Title))
	if title == "" {
		title = fmt.Sprintf("Review %s soul promotion", strings.TrimSpace(agentUser.DisplayName))
	}

	currentState := workflow.DroneWorkflowStateReviewQueued
	switch decision {
	case workflow.DroneReviewDecisionApproved:
		currentState = workflow.DroneWorkflowStateReviewApproved
	case workflow.DroneReviewDecisionChangesRequested:
		currentState = workflow.DroneWorkflowStateReviewChangesRequested
	case workflow.DroneReviewDecisionBlocked:
		currentState = workflow.DroneWorkflowStateReviewBlocked
	}

	workflowState.CurrentPhase = workflow.DroneWorkflowPhaseReview
	workflowState.CurrentState = currentState
	workflowState.Review = &workflow.DroneReviewCard{
		ID:              agentUser.Username + ":review",
		Title:           title,
		Decision:        decision,
		Reviewer:        r.droneWorkflowActor(ctx, claims.Username, "reviewer"),
		DecisionSummary: strings.TrimSpace(input.DecisionSummary),
		Findings:        normalizeDroneFindingsInput(input.Findings, agentUser.Username+":review:finding"),
		Evidence:        normalizeDroneArtifactsInput(input.Evidence, agentUser.Username+":review:evidence"),
	}
	if decision == workflow.DroneReviewDecisionApproved {
		workflowState.Checkpoint = &workflow.DroneSignatureCheckpoint{
			ID:             agentUser.Username + ":checkpoint",
			Title:          "Promotion checkpoint",
			ReadinessLabel: "Reviewer approval captured",
			ApprovalMemo:   strings.TrimSpace(input.DecisionSummary),
			DueAt:          &now,
			Signers: []workflow.DroneSignatureSigner{
				{
					ID:     claims.Username,
					Name:   r.droneWorkflowActor(ctx, claims.Username, "reviewer").Name,
					Role:   "reviewer",
					Status: workflow.DroneSignatureSignerStatusApproved,
					Note:   strings.TrimSpace(input.DecisionSummary),
				},
			},
		}
	}
	if strings.TrimSpace(derefString(input.ConversationID)) != "" {
		workflowState.Conversation = &workflow.DroneConversationState{
			ConversationID: strings.TrimSpace(derefString(input.ConversationID)),
			UpdatedAt:      &now,
		}
	}
	workflowState.UpdatedAt = &now
	workflowState.Lifecycle = workflow.BuildDroneLifecycle(workflowState.CurrentPhase, workflowState.CurrentState)

	if err := r.persistDroneWorkflowState(ctx, agentUser, workflowState); err != nil {
		return nil, err
	}
	return r.droneWorkflowMutationPayload(ctx, claims.Username, agentUser, governance)
}

func (r *mutationResolver) FinalizeSoulPromotion(ctx context.Context, input model.FinalizeSoulPromotionInput) (*model.DroneWorkflowMutationPayload, error) {
	claims, err := r.requireAuthClaims(ctx)
	if err != nil {
		return nil, err
	}
	if err := requireDroneWriteScope(claims); err != nil {
		return nil, err
	}

	agentUser, governance, err := r.loadDroneAgent(ctx, input.Username)
	if err != nil {
		return nil, err
	}
	if !r.canViewDroneWorkflow(ctx, claims, agentUser) {
		return nil, apperrors.Forbidden("not authorized to finalize soul promotion")
	}

	workflowState, err := r.loadDroneWorkflowState(agentUser)
	if err != nil {
		return nil, apperrors.InternalWithCause(err, "failed to decode drone workflow metadata")
	}
	workflowState = workflow.NormalizeDroneWorkflow(workflowState)

	now := time.Now().UTC()
	readiness := strings.TrimSpace(strings.ToLower(input.Readiness))
	switch readiness {
	case workflow.DroneGraduationReadinessReady, workflow.DroneGraduationReadinessWatch, workflow.DroneGraduationReadinessHold:
	default:
		return nil, apperrors.ValidationFailed("input.readiness", "must be one of ready, watch, hold")
	}

	if soulAgentID := strings.TrimSpace(derefString(input.SoulAgentID)); soulAgentID != "" {
		service, err := r.getSoulService()
		if err != nil {
			return nil, err
		}
		if _, err := service.Incorporate(ctx, claims.Username, agentUser.Username, soulAgentID); err != nil {
			return nil, mapSoulServiceError(err)
		}
		workflowState.SoulAgentID = soulAgentID
	}

	ownerActor := r.droneWorkflowActor(ctx, strings.TrimPrefix(strings.TrimSpace(agentUser.AgentOwner), "@"), "steward")
	finalizer := r.droneWorkflowActor(ctx, claims.Username, "launch_owner")
	continuityOwner := ownerActor
	if strings.TrimSpace(continuityOwner.ID) == "" {
		continuityOwner = finalizer
	}

	workflowState.Declaration = &workflow.DroneDeclarationCard{
		ID:                  agentUser.Username + ":declaration",
		Title:               defaultString(derefString(input.DeclarationTitle), "Soul promotion declaration"),
		Statement:           strings.TrimSpace(input.DeclarationStatement),
		Confidence:          strings.TrimSpace(input.DeclarationConfidence),
		Owner:               &ownerActor,
		DeclaredScope:       normalizeStringSlice(input.DeclaredScope),
		Risks:               normalizeStringSlice(input.DeclarationRisks),
		SupportingArtifacts: normalizeDroneArtifactsInput(input.SupportingArtifacts, agentUser.Username+":declaration:artifact"),
	}
	workflowState.Graduation = &workflow.DroneGraduationSummaryCard{
		ID:                  agentUser.Username + ":graduation",
		Title:               "Graduation summary",
		Readiness:           readiness,
		Summary:             strings.TrimSpace(input.Summary),
		LaunchOwner:         &finalizer,
		CompletedMilestones: normalizeStringSlice(input.CompletedMilestones),
		ExitCriteria:        normalizeStringSlice(input.ExitCriteria),
		NextStep:            strings.TrimSpace(derefString(input.NextStep)),
		Metrics: []workflow.DroneMetric{
			{Label: "Identity", Value: agentUser.Username},
		},
	}
	workflowState.Continuity = &workflow.DroneContinuityPanel{
		ID:           agentUser.Username + ":continuity",
		Title:        "Continuity plan",
		Objective:    defaultString(derefString(input.ContinuityObjective), "Preserve the existing body identity, timeline presence, and memory continuity through graduation."),
		Owner:        continuityOwner,
		FeedbackLoop: defaultString(derefString(input.ContinuityFeedbackLoop), "Monitor launch readiness, memory continuity, and attribution after promotion."),
		Metrics: []workflow.DroneMetric{
			{Label: "Body", Value: "preserved"},
			{Label: "Timeline", Value: "preserved"},
			{Label: "Memory", Value: "preserved"},
		},
	}
	if workflowState.Checkpoint == nil {
		workflowState.Checkpoint = &workflow.DroneSignatureCheckpoint{
			ID:             agentUser.Username + ":checkpoint",
			Title:          "Promotion checkpoint",
			ReadinessLabel: "Finalization recorded",
		}
	}
	workflowState.Checkpoint.Signers = append(workflowState.Checkpoint.Signers, workflow.DroneSignatureSigner{
		ID:     claims.Username,
		Name:   finalizer.Name,
		Role:   "launch_owner",
		Status: workflow.DroneSignatureSignerStatusApproved,
		Note:   "Finalized through Lesser GraphQL",
	})
	if strings.TrimSpace(derefString(input.ConversationID)) != "" {
		workflowState.Conversation = &workflow.DroneConversationState{
			ConversationID: strings.TrimSpace(derefString(input.ConversationID)),
			UpdatedAt:      &now,
		}
	}
	if workflowState.SoulAgentID != "" {
		workflowState.CurrentPhase = workflow.DroneWorkflowPhaseContinuity
		workflowState.CurrentState = workflow.DroneWorkflowStateContinuityStable
	} else {
		workflowState.CurrentPhase = workflow.DroneWorkflowPhaseGraduation
		switch readiness {
		case workflow.DroneGraduationReadinessHold:
			workflowState.CurrentState = workflow.DroneWorkflowStateGraduationHold
		case workflow.DroneGraduationReadinessWatch:
			workflowState.CurrentState = workflow.DroneWorkflowStateGraduationWatch
		default:
			workflowState.CurrentState = workflow.DroneWorkflowStateGraduationReady
		}
	}
	workflowState.UpdatedAt = &now
	workflowState.Lifecycle = workflow.BuildDroneLifecycle(workflowState.CurrentPhase, workflowState.CurrentState)

	if err := r.persistDroneWorkflowState(ctx, agentUser, workflowState); err != nil {
		return nil, err
	}
	return r.droneWorkflowMutationPayload(ctx, claims.Username, agentUser, governance)
}

func (r *Resolver) droneWorkflowActor(ctx context.Context, username string, fallbackRole string) workflow.DroneActor {
	actor := r.graphDroneActorForUsername(ctx, username, fallbackRole)
	if actor == nil {
		return workflow.DroneActor{ID: username, Name: username, Role: fallbackRole, Handle: "@" + username}
	}
	return workflow.DroneActor{
		ID:          actor.ID,
		Name:        actor.Name,
		Role:        actor.Role,
		Handle:      derefString(actor.Handle),
		AvatarLabel: derefString(actor.AvatarLabel),
		StatusLabel: derefString(actor.StatusLabel),
	}
}

func (r *Resolver) droneWorkflowMutationPayload(
	ctx context.Context,
	viewerUsername string,
	agentUser *storage.User,
	governance *storage.AgentGovernanceState,
) (*model.DroneWorkflowMutationPayload, error) {
	surface, _, err := r.buildDroneWorkflowSurface(ctx, viewerUsername, agentUser, governance)
	if err != nil {
		return nil, err
	}
	return &model.DroneWorkflowMutationPayload{
		Agent:    r.convertStorageUserToAgent(agentUser, governance),
		Workflow: surface,
	}, nil
}

func (r *Resolver) listOwnedDroneAgents(ctx context.Context, ownerUsername string) ([]*storage.User, error) {
	allAgents, err := r.listAllDroneAgents(ctx)
	if err != nil {
		return nil, err
	}
	ownerHandle := "@" + strings.TrimSpace(ownerUsername)
	out := make([]*storage.User, 0, len(allAgents))
	for _, agentUser := range allAgents {
		if agentUser == nil {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(agentUser.AgentOwner), ownerHandle) {
			out = append(out, agentUser)
		}
	}
	return out, nil
}

func (r *Resolver) listCandidateDroneReviewAgents(ctx context.Context, username string, includeAll bool) ([]*storage.User, error) {
	if includeAll {
		return r.listAllDroneAgents(ctx)
	}
	allAgents, err := r.listAllDroneAgents(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*storage.User, 0, len(allAgents))
	claims := &auth.Claims{Username: username}
	for _, agentUser := range allAgents {
		if agentUser == nil {
			continue
		}
		workflowState, err := r.loadDroneWorkflowState(agentUser)
		if err != nil || workflowState == nil || workflowState.Review == nil {
			continue
		}
		if droneWorkflowReviewerMatches(claims, workflowState) {
			out = append(out, agentUser)
		}
	}
	return out, nil
}

func (r *Resolver) listAllDroneAgents(ctx context.Context) ([]*storage.User, error) {
	if r == nil || r.Storage == nil || r.Storage.User() == nil {
		return nil, ErrStorageUnavailable
	}
	cursor := ""
	out := make([]*storage.User, 0, 16)
	for page := 0; page < 25; page++ {
		users, nextCursor, err := r.Storage.User().ListAgents(ctx, 100, cursor)
		if err != nil {
			return nil, apperrors.InternalWithCause(err, "failed to list agents")
		}
		for _, user := range users {
			if user == nil || !user.IsAgent || user.Suspended {
				continue
			}
			out = append(out, user)
		}
		if nextCursor == "" {
			break
		}
		cursor = nextCursor
	}
	return out, nil
}
