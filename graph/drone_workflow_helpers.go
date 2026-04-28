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
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
)

func requireDroneReadScope(claims *auth.Claims) error {
	if claims == nil {
		return ErrAuthenticationRequired
	}
	if claims.HasScope(auth.ScopeRead) || claims.HasScope(auth.ScopeWrite) {
		return nil
	}
	return apperrors.NewAuthError(
		apperrors.CodeInsufficientScope,
		"insufficient scope: requires one of read, write",
	)
}

func requireDroneWriteScope(claims *auth.Claims) error {
	if claims == nil {
		return ErrAuthenticationRequired
	}
	if claims.HasScope(auth.ScopeWrite) {
		return nil
	}
	return apperrors.InsufficientScope(auth.ScopeWrite)
}

func (r *Resolver) loadDroneAgent(ctx context.Context, username string) (*storage.User, *storage.AgentGovernanceState, error) {
	if r == nil || r.Storage == nil || r.Storage.Account() == nil {
		return nil, nil, ErrStorageUnavailable
	}

	username = strings.TrimSpace(username)
	if username == "" {
		return nil, nil, apperrors.ValidationFailed("username", "is required")
	}

	user, err := r.Storage.Account().GetUser(ctx, username)
	if err != nil || user == nil || !user.IsAgent || user.Suspended {
		return nil, nil, apperrors.NotFound("agent")
	}

	governance, err := r.loadAgentGovernanceState(ctx, username)
	if err != nil {
		return nil, nil, graphAgentGovernanceLoadError(err)
	}

	return user, governance, nil
}

func (r *Resolver) canViewDroneWorkflow(ctx context.Context, claims *auth.Claims, agentUser *storage.User) bool {
	if claims == nil || agentUser == nil {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(claims.Username), strings.TrimSpace(agentUser.Username)) {
		return true
	}
	if isAgentOwnerOrAdmin(claims, agentUser, r.agentOwnerActorURL(claims.Username)) {
		return true
	}
	role, err := r.normalizedRole(ctx, claims.Username)
	return err == nil && authz.IsModeratorOrAdmin(role)
}

func (r *Resolver) canAccessDroneReviewWorkflow(ctx context.Context, claims *auth.Claims, agentUser *storage.User, workflowState *workflow.DroneWorkflowState) bool {
	if r.canViewDroneWorkflow(ctx, claims, agentUser) {
		return true
	}
	return droneWorkflowReviewerMatches(claims, workflowState)
}

func (r *Resolver) canReviewDroneWorkflow(ctx context.Context, claims *auth.Claims, agentUser *storage.User, workflowState *workflow.DroneWorkflowState) bool {
	if claims == nil || agentUser == nil {
		return false
	}
	role, err := r.normalizedRole(ctx, claims.Username)
	if err == nil && authz.IsModeratorOrAdmin(role) {
		return true
	}
	if droneWorkflowReviewerMatches(claims, workflowState) {
		return true
	}
	if workflowHasExplicitReviewer(workflowState) {
		return false
	}
	return r.canViewDroneWorkflow(ctx, claims, agentUser)
}

func workflowHasExplicitReviewer(workflowState *workflow.DroneWorkflowState) bool {
	if workflowState == nil || workflowState.Review == nil {
		return false
	}
	return normalizedDroneWorkflowActorID(workflowState.Review.Reviewer.ID) != "" ||
		normalizedDroneWorkflowActorID(workflowState.Review.Reviewer.Handle) != ""
}

func droneWorkflowReviewerMatches(claims *auth.Claims, workflowState *workflow.DroneWorkflowState) bool {
	if claims == nil || workflowState == nil || workflowState.Review == nil {
		return false
	}
	viewerUsername := normalizedDroneWorkflowActorID(claims.Username)
	if viewerUsername == "" {
		return false
	}
	return viewerUsername == normalizedDroneWorkflowActorID(workflowState.Review.Reviewer.ID) ||
		viewerUsername == normalizedDroneWorkflowActorID(workflowState.Review.Reviewer.Handle)
}

func normalizedDroneWorkflowActorID(value string) string {
	return strings.ToLower(strings.TrimPrefix(strings.TrimSpace(value), "@"))
}

func (r *Resolver) lookupDroneSoulBinding(ctx context.Context, username string) (*storagemodels.InstanceSoulBodyBinding, error) {
	if r == nil || r.Storage == nil || r.Storage.Instance() == nil {
		return nil, nil
	}
	return r.Storage.Instance().GetSoulBodyBindingByUsername(ctx, username)
}

func (r *Resolver) loadDroneWorkflowState(agentUser *storage.User) (*workflow.DroneWorkflowState, error) {
	if agentUser == nil {
		return nil, nil
	}
	return workflow.ParseDroneWorkflowMetadata(agentUser.Metadata)
}

func (r *Resolver) buildDroneWorkflowSurface(
	ctx context.Context,
	viewerUsername string,
	agentUser *storage.User,
	governance *storage.AgentGovernanceState,
) (*model.AgentWorkflowSurface, *model.AgentIdentitySemantics, error) {
	if agentUser == nil {
		return nil, nil, nil
	}

	workflowState, err := r.loadDroneWorkflowState(agentUser)
	if err != nil {
		return nil, nil, apperrors.InternalWithCause(err, "failed to decode drone workflow metadata")
	}

	binding, err := r.lookupDroneSoulBinding(ctx, agentUser.Username)
	if err != nil {
		return nil, nil, apperrors.InternalWithCause(err, "failed to load soul body binding")
	}

	soulAgentID := ""
	soulBound := binding != nil
	if binding != nil {
		soulAgentID = binding.AgentID
	}

	identity := workflow.DeriveDroneIdentitySemantics(agentUser.Username, workflowState, soulBound, soulAgentID)
	identityModel := graphDroneIdentitySemanticsModel(identity)
	requestModel := graphDroneRequestCardModel(workflowState, agentUser)
	reviewModel := graphDroneReviewCardModel(workflowState)
	declarationModel := graphDroneDeclarationCardModel(workflowState)
	checkpointModel := graphDroneCheckpointCardModel(workflowState)
	graduationModel := graphDroneGraduationCardModel(workflowState)
	continuityModel := graphDroneContinuityPanelModel(workflowState)
	lifecycleModel := graphDroneLifecycleModels(workflowState)
	conversationModel := r.graphDroneConversationModel(ctx, viewerUsername, workflowState, agentUser)

	currentPhase := workflow.DroneWorkflowPhaseRequest
	currentState := workflow.DroneWorkflowStateRequestDraft
	if workflowState != nil {
		if strings.TrimSpace(workflowState.CurrentPhase) != "" {
			currentPhase = workflowState.CurrentPhase
		}
		if strings.TrimSpace(workflowState.CurrentState) != "" {
			currentState = workflowState.CurrentState
		}
	}
	if identity.IdentityState == workflow.DroneIdentityStateSouled {
		currentPhase = workflow.DroneWorkflowPhaseContinuity
		if !strings.HasPrefix(strings.TrimSpace(currentState), workflow.DroneWorkflowPhaseContinuity+".") {
			currentState = workflow.DroneWorkflowStateContinuityStable
		}
	}

	return &model.AgentWorkflowSurface{
		Username:          agentUser.Username,
		CurrentPhase:      currentPhase,
		CurrentState:      currentState,
		Identity:          r.graphDroneIdentityCard(ctx, agentUser, governance, identity, currentPhase, currentState),
		Request:           requestModel,
		Review:            reviewModel,
		Declaration:       declarationModel,
		Checkpoint:        checkpointModel,
		Graduation:        graduationModel,
		Continuity:        continuityModel,
		Lifecycle:         lifecycleModel,
		Conversation:      conversationModel,
		IdentitySemantics: identityModel,
	}, identityModel, nil
}

func (r *Resolver) graphDroneIdentityCard(
	ctx context.Context,
	agentUser *storage.User,
	governance *storage.AgentGovernanceState,
	identity workflow.DroneIdentitySemantics,
	currentPhase string,
	currentState string,
) *model.AgentIdentityCard {
	if agentUser == nil {
		return nil
	}

	name := strings.TrimSpace(agentUser.DisplayName)
	if name == "" {
		name = agentUser.Username
	}

	summary := fmt.Sprintf("%s is operating as a %s identity.", name, strings.ToLower(identity.IdentityLabel))
	switch identity.IdentityState {
	case workflow.DroneIdentityStateSouled:
		summary = fmt.Sprintf("%s is now souled while preserving the original body identity.", name)
	case workflow.DroneIdentityStateGraduating:
		summary = fmt.Sprintf("%s is moving through graduation while preserving body continuity.", name)
	}

	metrics := make([]*model.AgentSurfaceMetric, 0, 3)
	if delegated := len(graphAgentDelegatedScopes(governance)); delegated > 0 {
		metrics = append(metrics, &model.AgentSurfaceMetric{
			Label: "Delegated scopes",
			Value: fmt.Sprintf("%d", delegated),
		})
	}
	if caps := agentUser.AgentCapabilities; caps != nil && caps.MaxPostsPerHour > 0 {
		metrics = append(metrics, &model.AgentSurfaceMetric{
			Label: "Post budget",
			Value: fmt.Sprintf("%d/hr", caps.MaxPostsPerHour),
		})
	}
	if identity.SoulAgentID != "" {
		metrics = append(metrics, &model.AgentSurfaceMetric{
			Label: "Soul agent",
			Value: identity.SoulAgentID,
		})
	}

	return &model.AgentIdentityCard{
		ID:           agentUser.Username + ":identity",
		Name:         name,
		Handle:       optionalString("@" + agentUser.Username),
		Summary:      summary,
		CurrentPhase: currentPhase,
		CurrentState: optionalString(currentState),
		Steward:      r.graphDroneOwnerActor(ctx, agentUser),
		Tags:         []string{identity.IdentityLabel, strings.ToLower(strings.TrimSpace(agentUser.AgentType))},
		Metrics:      metrics,
	}
}

func (r *Resolver) graphDroneOwnerActor(ctx context.Context, agentUser *storage.User) *model.AgentSurfaceActor {
	if agentUser == nil {
		return nil
	}
	owner := strings.TrimSpace(agentUser.AgentOwner)
	if owner == "" {
		return nil
	}
	username := strings.TrimPrefix(owner, "@")
	return r.graphDroneActorForUsername(ctx, username, "steward")
}

func (r *Resolver) graphDroneActorForUsername(ctx context.Context, username string, fallbackRole string) *model.AgentSurfaceActor {
	username = strings.TrimSpace(username)
	if username == "" {
		return nil
	}

	name := username
	if r != nil && r.Storage != nil && r.Storage.Account() != nil {
		if user, err := r.Storage.Account().GetUser(ctx, username); err == nil && user != nil {
			if displayName := strings.TrimSpace(user.DisplayName); displayName != "" {
				name = displayName
			}
			if fallbackRole == "" {
				fallbackRole = strings.TrimSpace(user.Role)
			}
		}
	}

	if fallbackRole == "" {
		fallbackRole = "participant"
	}

	return &model.AgentSurfaceActor{
		ID:     username,
		Name:   name,
		Role:   fallbackRole,
		Handle: optionalString("@" + username),
	}
}

func graphDroneIdentitySemanticsModel(identity workflow.DroneIdentitySemantics) *model.AgentIdentitySemantics {
	return &model.AgentIdentitySemantics{
		IdentityState:             identity.IdentityState,
		IdentityLabel:             identity.IdentityLabel,
		LifecycleState:            identity.LifecycleState,
		SoulBindingState:          graphSoulBindingState(identity.SoulBindingState),
		SoulAgentID:               optionalString(identity.SoulAgentID),
		ContinuityState:           identity.ContinuityState,
		ContinuitySummary:         identity.ContinuitySummary,
		BodyIdentityPreserved:     identity.BodyIdentityPreserved,
		TimelinePresencePreserved: identity.TimelinePresencePreserved,
		MemoryReferencesPreserved: identity.MemoryReferencesPreserved,
		AttributionLabel:          identity.AttributionLabel,
		ModerationLabel:           identity.ModerationLabel,
	}
}

func graphSoulBindingState(value string) model.SoulBindingState {
	if strings.EqualFold(strings.TrimSpace(value), "bound") {
		return model.SoulBindingStateBound
	}
	return model.SoulBindingStateUnbound
}

func graphDroneRequestCardModel(workflowState *workflow.DroneWorkflowState, agentUser *storage.User) *model.SoulRequestCard {
	if agentUser == nil {
		return nil
	}
	if workflowState == nil || workflowState.Request == nil {
		return nil
	}
	card := workflowState.Request
	return &model.SoulRequestCard{
		ID:            card.ID,
		Title:         card.Title,
		Summary:       card.Summary,
		RequestedBy:   graphDroneActorModel(card.RequestedBy),
		SubmittedAt:   graphTimePtr(card.SubmittedAt),
		Constraints:   append([]string(nil), card.Constraints...),
		Artifacts:     graphDroneArtifacts(card.Artifacts),
		RouteDecision: optionalString(card.RouteDecision),
		CurrentState:  optionalString(card.CurrentState),
	}
}

func graphDroneReviewCardModel(workflowState *workflow.DroneWorkflowState) *model.ReviewDecisionCard {
	if workflowState == nil || workflowState.Review == nil {
		return nil
	}
	card := workflowState.Review
	return &model.ReviewDecisionCard{
		ID:              card.ID,
		Title:           card.Title,
		Decision:        card.Decision,
		Reviewer:        graphDroneActorModel(card.Reviewer),
		DecisionSummary: card.DecisionSummary,
		Findings:        graphDroneFindings(card.Findings),
		Evidence:        graphDroneArtifacts(card.Evidence),
	}
}

func graphDroneDeclarationCardModel(workflowState *workflow.DroneWorkflowState) *model.DeclarationPreviewCard {
	if workflowState == nil || workflowState.Declaration == nil {
		return nil
	}
	card := workflowState.Declaration
	return &model.DeclarationPreviewCard{
		ID:                  card.ID,
		Title:               card.Title,
		Statement:           card.Statement,
		Confidence:          card.Confidence,
		Owner:               graphDroneOptionalActorModel(card.Owner),
		DeclaredScope:       append([]string(nil), card.DeclaredScope...),
		Risks:               append([]string(nil), card.Risks...),
		SupportingArtifacts: graphDroneArtifacts(card.SupportingArtifacts),
	}
}

func graphDroneCheckpointCardModel(workflowState *workflow.DroneWorkflowState) *model.SignatureCheckpointCard {
	if workflowState == nil || workflowState.Checkpoint == nil {
		return nil
	}
	card := workflowState.Checkpoint
	signers := make([]*model.SignatureCheckpointSigner, 0, len(card.Signers))
	for _, signer := range card.Signers {
		signers = append(signers, &model.SignatureCheckpointSigner{
			ID:     signer.ID,
			Name:   signer.Name,
			Role:   signer.Role,
			Status: signer.Status,
			Note:   optionalString(signer.Note),
		})
	}
	return &model.SignatureCheckpointCard{
		ID:             card.ID,
		Title:          card.Title,
		ReadinessLabel: card.ReadinessLabel,
		ApprovalMemo:   optionalString(card.ApprovalMemo),
		DueAt:          graphTimePtr(card.DueAt),
		Signers:        signers,
	}
}

func graphDroneGraduationCardModel(workflowState *workflow.DroneWorkflowState) *model.GraduationSummaryCard {
	if workflowState == nil || workflowState.Graduation == nil {
		return nil
	}
	card := workflowState.Graduation
	return &model.GraduationSummaryCard{
		ID:                  card.ID,
		Title:               card.Title,
		Readiness:           card.Readiness,
		Summary:             card.Summary,
		LaunchOwner:         graphDroneOptionalActorModel(card.LaunchOwner),
		CompletedMilestones: append([]string(nil), card.CompletedMilestones...),
		ExitCriteria:        append([]string(nil), card.ExitCriteria...),
		NextStep:            optionalString(card.NextStep),
		Metrics:             graphDroneMetrics(card.Metrics),
	}
}

func graphDroneContinuityPanelModel(workflowState *workflow.DroneWorkflowState) *model.ContinuityPanel {
	if workflowState == nil || workflowState.Continuity == nil {
		return nil
	}
	panel := workflowState.Continuity
	followUps := make([]*model.ContinuityFollowUp, 0, len(panel.FollowUps))
	for _, followUp := range panel.FollowUps {
		followUps = append(followUps, &model.ContinuityFollowUp{
			ID:      followUp.ID,
			Title:   followUp.Title,
			Summary: followUp.Summary,
			Owner:   graphDroneActorModel(followUp.Owner),
			Cadence: optionalString(followUp.Cadence),
		})
	}
	return &model.ContinuityPanel{
		ID:           panel.ID,
		Title:        panel.Title,
		Objective:    panel.Objective,
		Owner:        graphDroneActorModel(panel.Owner),
		FeedbackLoop: panel.FeedbackLoop,
		Metrics:      graphDroneMetrics(panel.Metrics),
		FollowUps:    followUps,
	}
}

func graphDroneLifecycleModels(workflowState *workflow.DroneWorkflowState) []*model.AgentLifecycleStep {
	if workflowState == nil {
		workflowState = workflow.NormalizeDroneWorkflow(nil)
	}
	if len(workflowState.Lifecycle) == 0 {
		workflowState.Lifecycle = workflow.BuildDroneLifecycle(workflowState.CurrentPhase, workflowState.CurrentState)
	}
	steps := make([]*model.AgentLifecycleStep, 0, len(workflowState.Lifecycle))
	for _, step := range workflowState.Lifecycle {
		steps = append(steps, &model.AgentLifecycleStep{
			Phase:   step.Phase,
			Title:   optionalString(step.Title),
			Summary: optionalString(step.Summary),
			State:   optionalString(step.State),
			Status:  step.Status,
		})
	}
	return steps
}

func (r *Resolver) graphDroneConversationModel(
	ctx context.Context,
	viewerUsername string,
	workflowState *workflow.DroneWorkflowState,
	agentUser *storage.User,
) *model.AgentWorkflowConversationState {
	if workflowState == nil || workflowState.Conversation == nil {
		return nil
	}

	state := workflowState.Conversation
	conversationID := strings.TrimSpace(state.ConversationID)
	if conversationID == "" {
		return nil
	}

	folder := strings.TrimSpace(state.Folder)
	requestState := strings.TrimSpace(state.RequestState)
	unread := state.Unread
	previewStatusID := strings.TrimSpace(state.PreviewStatusID)
	requestedAt := graphTimePtr(state.RequestedAt)
	acceptedAt := graphTimePtr(state.AcceptedAt)
	declinedAt := graphTimePtr(state.DeclinedAt)
	updatedAt := graphTimePtr(state.UpdatedAt)

	if viewerUsername != "" && r != nil && r.Storage != nil && r.Storage.Conversation() != nil {
		if repoState, err := r.Storage.Conversation().GetUserConversationState(ctx, viewerUsername, conversationID); err == nil && repoState != nil {
			folder = string(repoState.Folder)
			requestState = strings.ToLower(strings.TrimSpace(string(repoState.RequestState)))
			unread = repoState.Unread
			previewStatusID = repoState.PreviewStatusID
			requestedAt = graphTimePtr(repoState.RequestedAt)
			acceptedAt = graphTimePtr(repoState.AcceptedAt)
			declinedAt = graphTimePtr(repoState.DeclinedAt)
			updatedAt = graphTimePtr(&repoState.UpdatedAt)
		}
	}

	if updatedAt == nil {
		fallback := agentUser.UpdatedAt
		updatedAt = graphTimePtr(&fallback)
	}
	if updatedAt == nil {
		now := model.Time(time.Now().UTC())
		updatedAt = &now
	}

	return &model.AgentWorkflowConversationState{
		ConversationID:  conversationID,
		Folder:          defaultString(folder, "INBOX"),
		RequestState:    optionalString(requestState),
		Unread:          unread,
		PreviewStatusID: optionalString(previewStatusID),
		RequestedAt:     requestedAt,
		AcceptedAt:      acceptedAt,
		DeclinedAt:      declinedAt,
		UpdatedAt:       *updatedAt,
	}
}

func graphDroneActorModel(actor workflow.DroneActor) *model.AgentSurfaceActor {
	return &model.AgentSurfaceActor{
		ID:          actor.ID,
		Name:        actor.Name,
		Role:        actor.Role,
		Handle:      optionalString(actor.Handle),
		AvatarLabel: optionalString(actor.AvatarLabel),
		StatusLabel: optionalString(actor.StatusLabel),
	}
}

func graphDroneOptionalActorModel(actor *workflow.DroneActor) *model.AgentSurfaceActor {
	if actor == nil {
		return nil
	}
	return graphDroneActorModel(*actor)
}

func graphDroneArtifacts(in []workflow.DroneArtifact) []*model.AgentSurfaceArtifact {
	if len(in) == 0 {
		return nil
	}
	out := make([]*model.AgentSurfaceArtifact, 0, len(in))
	for _, item := range in {
		out = append(out, &model.AgentSurfaceArtifact{
			ID:          item.ID,
			Title:       item.Title,
			Description: optionalString(item.Description),
			Href:        optionalString(item.Href),
			Emphasis:    optionalString(item.Emphasis),
		})
	}
	return out
}

func graphDroneMetrics(in []workflow.DroneMetric) []*model.AgentSurfaceMetric {
	if len(in) == 0 {
		return nil
	}
	out := make([]*model.AgentSurfaceMetric, 0, len(in))
	for _, item := range in {
		out = append(out, &model.AgentSurfaceMetric{
			Label:  item.Label,
			Value:  item.Value,
			Detail: optionalString(item.Detail),
		})
	}
	return out
}

func graphDroneFindings(in []workflow.DroneReviewFinding) []*model.ReviewFinding {
	if len(in) == 0 {
		return nil
	}
	out := make([]*model.ReviewFinding, 0, len(in))
	for _, item := range in {
		out = append(out, &model.ReviewFinding{
			ID:       item.ID,
			Title:    item.Title,
			Detail:   item.Detail,
			Severity: optionalString(item.Severity),
		})
	}
	return out
}

func graphTimePtr(value *time.Time) *model.Time {
	if value == nil || value.IsZero() {
		return nil
	}
	v := model.Time(value.UTC())
	return &v
}

func defaultString(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func normalizeDroneArtifactsInput(in []*model.AgentSurfaceArtifactInput, prefix string) []workflow.DroneArtifact {
	if len(in) == 0 {
		return nil
	}
	out := make([]workflow.DroneArtifact, 0, len(in))
	for idx, item := range in {
		if item == nil {
			continue
		}
		title := strings.TrimSpace(item.Title)
		if title == "" {
			continue
		}
		out = append(out, workflow.DroneArtifact{
			ID:          fmt.Sprintf("%s-%d", prefix, idx+1),
			Title:       title,
			Description: strings.TrimSpace(derefString(item.Description)),
			Href:        strings.TrimSpace(derefString(item.Href)),
			Emphasis:    strings.TrimSpace(derefString(item.Emphasis)),
		})
	}
	return out
}

func normalizeDroneFindingsInput(in []*model.ReviewFindingInput, prefix string) []workflow.DroneReviewFinding {
	if len(in) == 0 {
		return nil
	}
	out := make([]workflow.DroneReviewFinding, 0, len(in))
	for idx, item := range in {
		if item == nil {
			continue
		}
		title := strings.TrimSpace(item.Title)
		detail := strings.TrimSpace(item.Detail)
		if title == "" || detail == "" {
			continue
		}
		out = append(out, workflow.DroneReviewFinding{
			ID:       fmt.Sprintf("%s-%d", prefix, idx+1),
			Title:    title,
			Detail:   detail,
			Severity: strings.TrimSpace(derefString(item.Severity)),
		})
	}
	return out
}

func normalizeStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
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

func (r *Resolver) persistDroneWorkflowState(ctx context.Context, agentUser *storage.User, workflowState *workflow.DroneWorkflowState) error {
	if r == nil || r.Storage == nil || r.Storage.Account() == nil {
		return ErrStorageUnavailable
	}
	if agentUser == nil {
		return apperrors.NotFound("agent")
	}
	metadata, err := workflow.SetDroneWorkflowMetadata(agentUser.Metadata, workflowState)
	if err != nil {
		return apperrors.InternalWithCause(err, "failed to encode drone workflow metadata")
	}
	if err := r.Storage.Account().UpdateUser(ctx, agentUser.Username, map[string]interface{}{"metadata": metadata}); err != nil {
		return apperrors.InternalWithCause(err, "failed to persist drone workflow metadata")
	}
	agentUser.Metadata = metadata
	return nil
}
