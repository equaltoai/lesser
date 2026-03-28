package agents

import (
	"encoding/json"
	"strings"
	"time"
)

const (
	// DroneWorkflowMetadataKey stores typed drone workflow state on the agent user row.
	DroneWorkflowMetadataKey = "drone_workflow"

	// DroneWorkflowPhaseRequest marks the request phase of the workflow.
	DroneWorkflowPhaseRequest = "request"
	// DroneWorkflowPhaseReview marks the review phase of the workflow.
	DroneWorkflowPhaseReview = "review"
	// DroneWorkflowPhaseDeclaration marks the declaration phase of the workflow.
	DroneWorkflowPhaseDeclaration = "declaration"
	// DroneWorkflowPhaseSigning marks the signing phase of the workflow.
	DroneWorkflowPhaseSigning = "signing"
	// DroneWorkflowPhaseGraduation marks the graduation phase of the workflow.
	DroneWorkflowPhaseGraduation = "graduation"
	// DroneWorkflowPhaseContinuity marks the continuity phase of the workflow.
	DroneWorkflowPhaseContinuity = "continuity"

	// DroneWorkflowStateRequestDraft marks a draft request state.
	DroneWorkflowStateRequestDraft = "request.draft"
	// DroneWorkflowStateRequestSubmitted marks a submitted request state.
	DroneWorkflowStateRequestSubmitted = "request.submitted"
	// DroneWorkflowStateReviewQueued marks a queued review state.
	DroneWorkflowStateReviewQueued = "review.queued"
	// DroneWorkflowStateReviewApproved marks an approved review state.
	DroneWorkflowStateReviewApproved = "review.approved"
	// DroneWorkflowStateReviewChangesRequested marks a changes-requested review state.
	DroneWorkflowStateReviewChangesRequested = "review.changes_requested"
	// DroneWorkflowStateReviewBlocked marks a blocked review state.
	DroneWorkflowStateReviewBlocked = "review.blocked"
	// DroneWorkflowStateDeclarationReady marks a ready declaration state.
	DroneWorkflowStateDeclarationReady = "declaration.ready"
	// DroneWorkflowStateSigningPending marks a pending signing state.
	DroneWorkflowStateSigningPending = "signing.pending"
	// DroneWorkflowStateGraduationReady marks a ready graduation state.
	DroneWorkflowStateGraduationReady = "graduation.ready"
	// DroneWorkflowStateGraduationHold marks a held graduation state.
	DroneWorkflowStateGraduationHold = "graduation.hold"
	// DroneWorkflowStateGraduationWatch marks a watch graduation state.
	DroneWorkflowStateGraduationWatch = "graduation.watch"
	// DroneWorkflowStateContinuityStable marks a stable continuity state.
	DroneWorkflowStateContinuityStable = "continuity.stable"
	// DroneWorkflowStateContinuityMonitoring marks a monitoring continuity state.
	DroneWorkflowStateContinuityMonitoring = "continuity.monitoring"
	// DroneWorkflowStateContinuityEscalated marks an escalated continuity state.
	DroneWorkflowStateContinuityEscalated = "continuity.escalated"
	// DroneIdentityStateDrone marks an unsouled drone identity state.
	DroneIdentityStateDrone = "drone"
	// DroneIdentityStateGraduating marks a graduating identity state.
	DroneIdentityStateGraduating = "graduating"
	// DroneIdentityStateSouled marks a souled identity state.
	DroneIdentityStateSouled = "souled"
	// DroneContinuityStatePlanned marks planned continuity semantics.
	DroneContinuityStatePlanned = "planned"
	// DroneContinuityStateStable marks stable continuity semantics.
	DroneContinuityStateStable = "stable"
	// DroneLifecycleStatusUpcoming marks an upcoming lifecycle step.
	DroneLifecycleStatusUpcoming = "upcoming"
	// DroneLifecycleStatusActive marks an active lifecycle step.
	DroneLifecycleStatusActive = "active"
	// DroneLifecycleStatusComplete marks a completed lifecycle step.
	DroneLifecycleStatusComplete = "complete"
	// DroneLifecycleStatusBlocked marks a blocked lifecycle step.
	DroneLifecycleStatusBlocked = "blocked"
	// DroneReviewDecisionQueued marks a queued review decision.
	DroneReviewDecisionQueued = "queued"
	// DroneReviewDecisionApproved marks an approved review decision.
	DroneReviewDecisionApproved = "approved"
	// DroneReviewDecisionChangesRequested marks a changes-requested review decision.
	DroneReviewDecisionChangesRequested = "changes_requested"
	// DroneReviewDecisionBlocked marks a blocked review decision.
	DroneReviewDecisionBlocked = "blocked"
	// DroneGraduationReadinessReady marks a ready graduation decision.
	DroneGraduationReadinessReady = "ready"
	// DroneGraduationReadinessWatch marks a watch graduation decision.
	DroneGraduationReadinessWatch = "watch"
	// DroneGraduationReadinessHold marks a hold graduation decision.
	DroneGraduationReadinessHold = "hold"
	// DroneSignatureSignerStatusPending marks a pending signer status.
	DroneSignatureSignerStatusPending = "pending"
	// DroneSignatureSignerStatusApproved marks an approved signer status.
	DroneSignatureSignerStatusApproved = "approved"
	// DroneSignatureSignerStatusRejected marks a rejected signer status.
	DroneSignatureSignerStatusRejected = "rejected"
)

var droneWorkflowPhases = []string{
	DroneWorkflowPhaseRequest,
	DroneWorkflowPhaseReview,
	DroneWorkflowPhaseDeclaration,
	DroneWorkflowPhaseSigning,
	DroneWorkflowPhaseGraduation,
	DroneWorkflowPhaseContinuity,
}

// DroneWorkflowState stores the typed agent-first workflow metadata published to clients.
type DroneWorkflowState struct {
	CurrentPhase string `json:"current_phase,omitempty"`
	CurrentState string `json:"current_state,omitempty"`
	SoulAgentID  string `json:"soul_agent_id,omitempty"`

	Request      *DroneRequestCard           `json:"request,omitempty"`
	Review       *DroneReviewCard            `json:"review,omitempty"`
	Declaration  *DroneDeclarationCard       `json:"declaration,omitempty"`
	Checkpoint   *DroneSignatureCheckpoint   `json:"checkpoint,omitempty"`
	Graduation   *DroneGraduationSummaryCard `json:"graduation,omitempty"`
	Continuity   *DroneContinuityPanel       `json:"continuity,omitempty"`
	Conversation *DroneConversationState     `json:"conversation,omitempty"`
	Lifecycle    []DroneLifecycleStep        `json:"lifecycle,omitempty"`
	UpdatedAt    *time.Time                  `json:"updated_at,omitempty"`
}

// DroneMetric describes one labeled workflow metric.
type DroneMetric struct {
	Label  string `json:"label"`
	Value  string `json:"value"`
	Detail string `json:"detail,omitempty"`
}

// DroneActor describes one person or system represented in the workflow.
type DroneActor struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Role        string `json:"role"`
	Handle      string `json:"handle,omitempty"`
	AvatarLabel string `json:"avatar_label,omitempty"`
	StatusLabel string `json:"status_label,omitempty"`
}

// DroneArtifact describes one machine-readable or user-visible workflow artifact.
type DroneArtifact struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Href        string `json:"href,omitempty"`
	Emphasis    string `json:"emphasis,omitempty"`
}

// DroneIdentityCard summarizes the current identity panel for a drone workflow.
type DroneIdentityCard struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	Handle       string        `json:"handle,omitempty"`
	Summary      string        `json:"summary"`
	CurrentPhase string        `json:"current_phase"`
	CurrentState string        `json:"current_state,omitempty"`
	Steward      *DroneActor   `json:"steward,omitempty"`
	Tags         []string      `json:"tags,omitempty"`
	Metrics      []DroneMetric `json:"metrics,omitempty"`
}

// DroneRequestCard captures the request stage of a drone workflow.
type DroneRequestCard struct {
	ID            string          `json:"id"`
	Title         string          `json:"title"`
	Summary       string          `json:"summary"`
	RequestedBy   DroneActor      `json:"requested_by"`
	SubmittedAt   *time.Time      `json:"submitted_at,omitempty"`
	Constraints   []string        `json:"constraints,omitempty"`
	Artifacts     []DroneArtifact `json:"artifacts,omitempty"`
	RouteDecision string          `json:"route_decision,omitempty"`
	CurrentState  string          `json:"current_state,omitempty"`
}

// DroneReviewFinding captures one review finding attached to a workflow review.
type DroneReviewFinding struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Detail   string `json:"detail"`
	Severity string `json:"severity,omitempty"`
}

// DroneReviewCard captures the review stage of a drone workflow.
type DroneReviewCard struct {
	ID              string               `json:"id"`
	Title           string               `json:"title"`
	Decision        string               `json:"decision"`
	Reviewer        DroneActor           `json:"reviewer"`
	DecisionSummary string               `json:"decision_summary"`
	Findings        []DroneReviewFinding `json:"findings,omitempty"`
	Evidence        []DroneArtifact      `json:"evidence,omitempty"`
}

// DroneDeclarationCard captures the declaration artifacts used for promotion.
type DroneDeclarationCard struct {
	ID                  string          `json:"id"`
	Title               string          `json:"title"`
	Statement           string          `json:"statement"`
	Confidence          string          `json:"confidence"`
	Owner               *DroneActor     `json:"owner,omitempty"`
	DeclaredScope       []string        `json:"declared_scope,omitempty"`
	Risks               []string        `json:"risks,omitempty"`
	SupportingArtifacts []DroneArtifact `json:"supporting_artifacts,omitempty"`
}

// DroneSignatureSigner describes one signer in a workflow checkpoint.
type DroneSignatureSigner struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Role   string `json:"role"`
	Status string `json:"status"`
	Note   string `json:"note,omitempty"`
}

// DroneSignatureCheckpoint captures the approval checkpoint for promotion.
type DroneSignatureCheckpoint struct {
	ID             string                 `json:"id"`
	Title          string                 `json:"title"`
	ReadinessLabel string                 `json:"readiness_label"`
	ApprovalMemo   string                 `json:"approval_memo,omitempty"`
	DueAt          *time.Time             `json:"due_at,omitempty"`
	Signers        []DroneSignatureSigner `json:"signers,omitempty"`
}

// DroneContinuityFollowUp captures one continuity follow-up action.
type DroneContinuityFollowUp struct {
	ID      string     `json:"id"`
	Title   string     `json:"title"`
	Summary string     `json:"summary"`
	Owner   DroneActor `json:"owner"`
	Cadence string     `json:"cadence,omitempty"`
}

// DroneContinuityPanel describes the continuity plan after graduation.
type DroneContinuityPanel struct {
	ID           string                    `json:"id"`
	Title        string                    `json:"title"`
	Objective    string                    `json:"objective"`
	Owner        DroneActor                `json:"owner"`
	FeedbackLoop string                    `json:"feedback_loop"`
	Metrics      []DroneMetric             `json:"metrics,omitempty"`
	FollowUps    []DroneContinuityFollowUp `json:"follow_ups,omitempty"`
}

// DroneGraduationSummaryCard captures the graduation summary for the workflow.
type DroneGraduationSummaryCard struct {
	ID                  string        `json:"id"`
	Title               string        `json:"title"`
	Readiness           string        `json:"readiness"`
	Summary             string        `json:"summary"`
	LaunchOwner         *DroneActor   `json:"launch_owner,omitempty"`
	CompletedMilestones []string      `json:"completed_milestones,omitempty"`
	ExitCriteria        []string      `json:"exit_criteria,omitempty"`
	NextStep            string        `json:"next_step,omitempty"`
	Metrics             []DroneMetric `json:"metrics,omitempty"`
}

// DroneLifecycleStep describes one lifecycle phase in the workflow timeline.
type DroneLifecycleStep struct {
	Phase   string `json:"phase"`
	Title   string `json:"title,omitempty"`
	Summary string `json:"summary,omitempty"`
	State   string `json:"state,omitempty"`
	Status  string `json:"status"`
}

// DroneConversationState captures related conversation progress for the workflow.
type DroneConversationState struct {
	ConversationID  string     `json:"conversation_id"`
	Folder          string     `json:"folder,omitempty"`
	RequestState    string     `json:"request_state,omitempty"`
	Unread          bool       `json:"unread"`
	PreviewStatusID string     `json:"preview_status_id,omitempty"`
	RequestedAt     *time.Time `json:"requested_at,omitempty"`
	AcceptedAt      *time.Time `json:"accepted_at,omitempty"`
	DeclinedAt      *time.Time `json:"declined_at,omitempty"`
	UpdatedAt       *time.Time `json:"updated_at,omitempty"`
}

// DroneIdentitySemantics describes identity and continuity semantics for a drone.
type DroneIdentitySemantics struct {
	IdentityState             string `json:"identity_state"`
	IdentityLabel             string `json:"identity_label"`
	LifecycleState            string `json:"lifecycle_state"`
	SoulBindingState          string `json:"soul_binding_state"`
	SoulAgentID               string `json:"soul_agent_id,omitempty"`
	ContinuityState           string `json:"continuity_state"`
	ContinuitySummary         string `json:"continuity_summary"`
	BodyIdentityPreserved     bool   `json:"body_identity_preserved"`
	TimelinePresencePreserved bool   `json:"timeline_presence_preserved"`
	MemoryReferencesPreserved bool   `json:"memory_references_preserved"`
	AttributionLabel          string `json:"attribution_label"`
	ModerationLabel           string `json:"moderation_label"`
}

// ParseDroneWorkflowMetadata decodes typed drone workflow state from user metadata.
func ParseDroneWorkflowMetadata(metadata map[string]interface{}) (*DroneWorkflowState, error) {
	if len(metadata) == 0 {
		return nil, nil
	}
	raw, ok := metadata[DroneWorkflowMetadataKey]
	if !ok || raw == nil {
		return nil, nil
	}
	switch value := raw.(type) {
	case *DroneWorkflowState:
		return NormalizeDroneWorkflow(value.Clone()), nil
	case DroneWorkflowState:
		workflow := value
		return NormalizeDroneWorkflow(workflow.Clone()), nil
	}

	bytes, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	var workflow DroneWorkflowState
	if err := json.Unmarshal(bytes, &workflow); err != nil {
		return nil, err
	}
	return NormalizeDroneWorkflow(workflow.Clone()), nil
}

// SetDroneWorkflowMetadata stores typed drone workflow state back into user metadata.
func SetDroneWorkflowMetadata(metadata map[string]interface{}, workflow *DroneWorkflowState) (map[string]interface{}, error) {
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	cloned := cloneMetadataMap(metadata)
	if workflow == nil {
		delete(cloned, DroneWorkflowMetadataKey)
		return cloned, nil
	}

	normalized := NormalizeDroneWorkflow(workflow.Clone())
	bytes, err := json.Marshal(normalized)
	if err != nil {
		return nil, err
	}
	var stored map[string]interface{}
	if err := json.Unmarshal(bytes, &stored); err != nil {
		return nil, err
	}
	cloned[DroneWorkflowMetadataKey] = stored
	return cloned, nil
}

// NormalizeDroneWorkflow fills default fields for an incomplete workflow state.
func NormalizeDroneWorkflow(workflow *DroneWorkflowState) *DroneWorkflowState {
	if workflow == nil {
		workflow = &DroneWorkflowState{}
	}
	if workflow.CurrentState == "" {
		workflow.CurrentState = DroneWorkflowStateRequestDraft
	}
	if workflow.CurrentPhase == "" {
		workflow.CurrentPhase = workflowPhaseFromState(workflow.CurrentState)
	}
	if workflow.CurrentPhase == "" {
		workflow.CurrentPhase = DroneWorkflowPhaseRequest
	}
	if len(workflow.Lifecycle) == 0 {
		workflow.Lifecycle = BuildDroneLifecycle(workflow.CurrentPhase, workflow.CurrentState)
	}
	return workflow
}

// BuildDroneLifecycle derives lifecycle steps for the current workflow phase and state.
func BuildDroneLifecycle(currentPhase string, currentState string) []DroneLifecycleStep {
	currentPhase = strings.TrimSpace(currentPhase)
	currentState = strings.TrimSpace(currentState)
	if currentPhase == "" {
		currentPhase = workflowPhaseFromState(currentState)
	}
	if currentPhase == "" {
		currentPhase = DroneWorkflowPhaseRequest
	}

	phaseIndex := indexOfDronePhase(currentPhase)
	lifecycle := make([]DroneLifecycleStep, 0, len(droneWorkflowPhases))
	for idx, phase := range droneWorkflowPhases {
		status := DroneLifecycleStatusUpcoming
		switch {
		case phase == currentPhase && stateLooksBlocked(currentState):
			status = DroneLifecycleStatusBlocked
		case phase == currentPhase:
			status = DroneLifecycleStatusActive
		case idx < phaseIndex:
			status = DroneLifecycleStatusComplete
		}

		lifecycle = append(lifecycle, DroneLifecycleStep{
			Phase:  phase,
			Title:  formatDroneWorkflowLabel(phase),
			State:  currentStateForPhase(phase, currentPhase, currentState),
			Status: status,
		})
	}
	return lifecycle
}

// DeriveDroneIdentitySemantics derives identity semantics from workflow and soul state.
func DeriveDroneIdentitySemantics(username string, workflow *DroneWorkflowState, soulBound bool, soulAgentID string) DroneIdentitySemantics {
	workflow = NormalizeDroneWorkflow(workflow)

	if soulAgentID == "" && workflow != nil {
		soulAgentID = strings.TrimSpace(workflow.SoulAgentID)
	}

	state := DroneIdentityStateDrone
	label := "Drone"
	soulBindingState := "UNBOUND"
	lifecycleState := workflow.CurrentState
	continuityState := DroneContinuityStatePlanned
	continuitySummary := "Graduation will preserve the existing body identity, timeline presence, and memory references."

	if soulBound || soulAgentID != "" {
		state = DroneIdentityStateSouled
		label = "Souled"
		soulBindingState = "BOUND"
		if lifecycleState == "" || !strings.HasPrefix(lifecycleState, DroneWorkflowPhaseContinuity+".") {
			lifecycleState = DroneWorkflowStateContinuityStable
		}
		continuityState = DroneContinuityStateStable
		continuitySummary = "Graduation preserved the existing body identity, timeline presence, and memory references."
	} else if workflow != nil && workflow.CurrentState != "" && workflow.CurrentState != DroneWorkflowStateRequestDraft {
		state = DroneIdentityStateGraduating
		label = "Graduating"
		if lifecycleState == "" {
			lifecycleState = workflow.CurrentState
		}
	}

	if lifecycleState == "" {
		lifecycleState = DroneWorkflowStateRequestDraft
	}

	return DroneIdentitySemantics{
		IdentityState:             state,
		IdentityLabel:             label,
		LifecycleState:            lifecycleState,
		SoulBindingState:          soulBindingState,
		SoulAgentID:               strings.TrimSpace(soulAgentID),
		ContinuityState:           continuityState,
		ContinuitySummary:         continuitySummaryForUsername(username, continuitySummary),
		BodyIdentityPreserved:     true,
		TimelinePresencePreserved: true,
		MemoryReferencesPreserved: true,
		AttributionLabel:          label,
		ModerationLabel:           label,
	}
}

// Clone returns a deep copy of the workflow state.
func (w *DroneWorkflowState) Clone() *DroneWorkflowState {
	if w == nil {
		return nil
	}

	cloned := *w
	cloned.Request = cloneDroneRequestCard(w.Request)
	cloned.Review = cloneDroneReviewCard(w.Review)
	cloned.Declaration = cloneDroneDeclarationCard(w.Declaration)
	cloned.Checkpoint = cloneDroneCheckpoint(w.Checkpoint)
	cloned.Graduation = cloneDroneGraduationCard(w.Graduation)
	cloned.Continuity = cloneDroneContinuityPanel(w.Continuity)
	cloned.Conversation = cloneDroneConversationState(w.Conversation)
	cloned.Lifecycle = append([]DroneLifecycleStep(nil), w.Lifecycle...)
	cloned.UpdatedAt = cloneDroneTime(w.UpdatedAt)
	return &cloned
}

func cloneDroneRequestCard(card *DroneRequestCard) *DroneRequestCard {
	if card == nil {
		return nil
	}
	cloned := *card
	cloned.SubmittedAt = cloneDroneTime(card.SubmittedAt)
	cloned.Constraints = append([]string(nil), card.Constraints...)
	cloned.Artifacts = append([]DroneArtifact(nil), card.Artifacts...)
	return &cloned
}

func cloneDroneReviewCard(card *DroneReviewCard) *DroneReviewCard {
	if card == nil {
		return nil
	}
	cloned := *card
	cloned.Findings = append([]DroneReviewFinding(nil), card.Findings...)
	cloned.Evidence = append([]DroneArtifact(nil), card.Evidence...)
	return &cloned
}

func cloneDroneDeclarationCard(card *DroneDeclarationCard) *DroneDeclarationCard {
	if card == nil {
		return nil
	}
	cloned := *card
	cloned.DeclaredScope = append([]string(nil), card.DeclaredScope...)
	cloned.Risks = append([]string(nil), card.Risks...)
	cloned.SupportingArtifacts = append([]DroneArtifact(nil), card.SupportingArtifacts...)
	return &cloned
}

func cloneDroneCheckpoint(card *DroneSignatureCheckpoint) *DroneSignatureCheckpoint {
	if card == nil {
		return nil
	}
	cloned := *card
	cloned.DueAt = cloneDroneTime(card.DueAt)
	cloned.Signers = append([]DroneSignatureSigner(nil), card.Signers...)
	return &cloned
}

func cloneDroneGraduationCard(card *DroneGraduationSummaryCard) *DroneGraduationSummaryCard {
	if card == nil {
		return nil
	}
	cloned := *card
	cloned.CompletedMilestones = append([]string(nil), card.CompletedMilestones...)
	cloned.ExitCriteria = append([]string(nil), card.ExitCriteria...)
	cloned.Metrics = append([]DroneMetric(nil), card.Metrics...)
	return &cloned
}

func cloneDroneContinuityPanel(panel *DroneContinuityPanel) *DroneContinuityPanel {
	if panel == nil {
		return nil
	}
	cloned := *panel
	cloned.Metrics = append([]DroneMetric(nil), panel.Metrics...)
	cloned.FollowUps = append([]DroneContinuityFollowUp(nil), panel.FollowUps...)
	return &cloned
}

func cloneDroneConversationState(state *DroneConversationState) *DroneConversationState {
	if state == nil {
		return nil
	}
	cloned := *state
	cloned.RequestedAt = cloneDroneTime(state.RequestedAt)
	cloned.AcceptedAt = cloneDroneTime(state.AcceptedAt)
	cloned.DeclinedAt = cloneDroneTime(state.DeclinedAt)
	cloned.UpdatedAt = cloneDroneTime(state.UpdatedAt)
	return &cloned
}

func cloneDroneTime(value *time.Time) *time.Time {
	if value == nil || value.IsZero() {
		return nil
	}
	cloned := value.UTC()
	return &cloned
}

func cloneMetadataMap(metadata map[string]interface{}) map[string]interface{} {
	if metadata == nil {
		return map[string]interface{}{}
	}
	bytes, err := json.Marshal(metadata)
	if err != nil {
		return map[string]interface{}{}
	}
	var cloned map[string]interface{}
	if err := json.Unmarshal(bytes, &cloned); err != nil {
		return map[string]interface{}{}
	}
	return cloned
}

func workflowPhaseFromState(state string) string {
	state = strings.TrimSpace(state)
	if state == "" {
		return ""
	}
	if idx := strings.Index(state, "."); idx > 0 {
		return state[:idx]
	}
	return state
}

func currentStateForPhase(phase string, currentPhase string, currentState string) string {
	if phase == currentPhase {
		return currentState
	}
	return ""
}

func indexOfDronePhase(phase string) int {
	for idx, candidate := range droneWorkflowPhases {
		if candidate == phase {
			return idx
		}
	}
	return 0
}

func stateLooksBlocked(state string) bool {
	return strings.Contains(state, "blocked") || strings.Contains(state, "declined")
}

func formatDroneWorkflowLabel(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == '.' || r == '_' || r == '-'
	})
	for idx, part := range parts {
		if part == "" {
			continue
		}
		parts[idx] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, " ")
}

func continuitySummaryForUsername(username, summary string) string {
	username = strings.TrimSpace(username)
	if username == "" {
		return summary
	}
	return strings.ReplaceAll(summary, "the existing body identity", "@"+username+" body identity")
}
