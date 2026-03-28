package agents

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDroneWorkflowMetadata_RoundTripAndNormalize(t *testing.T) {
	now := time.Date(2026, 3, 28, 14, 15, 0, 0, time.UTC)

	metadata, err := SetDroneWorkflowMetadata(map[string]interface{}{"preserve": "yes"}, &DroneWorkflowState{
		CurrentPhase: DroneWorkflowPhaseReview,
		CurrentState: DroneWorkflowStateReviewApproved,
		Request: &DroneRequestCard{
			ID:      "drone-1:request",
			Title:   "Graduation request",
			Summary: "Move the drone into soul review.",
			RequestedBy: DroneActor{
				ID:   "owner",
				Name: "Owner",
				Role: "requester",
			},
			SubmittedAt: &now,
		},
		UpdatedAt: &now,
	})
	require.NoError(t, err)
	require.Equal(t, "yes", metadata["preserve"])

	parsed, err := ParseDroneWorkflowMetadata(metadata)
	require.NoError(t, err)
	require.NotNil(t, parsed)
	require.Equal(t, DroneWorkflowPhaseReview, parsed.CurrentPhase)
	require.Equal(t, DroneWorkflowStateReviewApproved, parsed.CurrentState)
	require.NotNil(t, parsed.Request)
	require.Equal(t, "Graduation request", parsed.Request.Title)
	require.NotNil(t, parsed.UpdatedAt)
	require.Len(t, parsed.Lifecycle, len(droneWorkflowPhases))

	parsed.Request.Title = "changed locally"
	reparsed, err := ParseDroneWorkflowMetadata(metadata)
	require.NoError(t, err)
	require.Equal(t, "Graduation request", reparsed.Request.Title)
}

func TestDeriveDroneIdentitySemantics_DerivesDroneGraduatingAndSouledStates(t *testing.T) {
	t.Run("draft workflow stays drone", func(t *testing.T) {
		identity := DeriveDroneIdentitySemantics("scout", nil, false, "")
		require.Equal(t, DroneIdentityStateDrone, identity.IdentityState)
		require.Equal(t, "Drone", identity.IdentityLabel)
		require.Equal(t, "UNBOUND", identity.SoulBindingState)
		require.Contains(t, identity.ContinuitySummary, "@scout")
		require.True(t, identity.BodyIdentityPreserved)
		require.True(t, identity.TimelinePresencePreserved)
		require.True(t, identity.MemoryReferencesPreserved)
	})

	t.Run("non-draft workflow becomes graduating", func(t *testing.T) {
		identity := DeriveDroneIdentitySemantics("scout", &DroneWorkflowState{
			CurrentPhase: DroneWorkflowPhaseReview,
			CurrentState: DroneWorkflowStateReviewQueued,
		}, false, "")
		require.Equal(t, DroneIdentityStateGraduating, identity.IdentityState)
		require.Equal(t, "Graduating", identity.IdentityLabel)
		require.Equal(t, DroneWorkflowStateReviewQueued, identity.LifecycleState)
		require.Equal(t, "UNBOUND", identity.SoulBindingState)
	})

	t.Run("bound or soul-linked workflow becomes souled", func(t *testing.T) {
		identity := DeriveDroneIdentitySemantics("scout", &DroneWorkflowState{
			CurrentPhase: DroneWorkflowPhaseGraduation,
			CurrentState: DroneWorkflowStateGraduationReady,
			SoulAgentID:  "0xsoul",
		}, true, "0xsoul")
		require.Equal(t, DroneIdentityStateSouled, identity.IdentityState)
		require.Equal(t, "Souled", identity.IdentityLabel)
		require.Equal(t, "BOUND", identity.SoulBindingState)
		require.Equal(t, "0xsoul", identity.SoulAgentID)
		require.Equal(t, DroneWorkflowStateContinuityStable, identity.LifecycleState)
		require.Equal(t, DroneContinuityStateStable, identity.ContinuityState)
		require.Contains(t, identity.ContinuitySummary, "preserved")
	})
}

func TestParseDroneWorkflowMetadata_CoversMetadataShapesAndErrors(t *testing.T) {
	t.Run("empty metadata returns nil workflow", func(t *testing.T) {
		workflow, err := ParseDroneWorkflowMetadata(nil)
		require.NoError(t, err)
		require.Nil(t, workflow)

		workflow, err = ParseDroneWorkflowMetadata(map[string]interface{}{"other": "value"})
		require.NoError(t, err)
		require.Nil(t, workflow)
	})

	t.Run("state values and pointers are normalized", func(t *testing.T) {
		valueWorkflow, err := ParseDroneWorkflowMetadata(map[string]interface{}{
			DroneWorkflowMetadataKey: DroneWorkflowState{
				CurrentState: DroneWorkflowStateReviewBlocked,
			},
		})
		require.NoError(t, err)
		require.Equal(t, DroneWorkflowPhaseReview, valueWorkflow.CurrentPhase)
		require.Equal(t, DroneLifecycleStatusBlocked, valueWorkflow.Lifecycle[1].Status)

		pointerWorkflow, err := ParseDroneWorkflowMetadata(map[string]interface{}{
			DroneWorkflowMetadataKey: &DroneWorkflowState{
				CurrentPhase: DroneWorkflowPhaseGraduation,
				CurrentState: DroneWorkflowStateGraduationWatch,
			},
		})
		require.NoError(t, err)
		require.Equal(t, DroneWorkflowPhaseGraduation, pointerWorkflow.CurrentPhase)
		require.Equal(t, DroneLifecycleStatusActive, pointerWorkflow.Lifecycle[4].Status)
	})

	t.Run("invalid raw metadata returns an error", func(t *testing.T) {
		workflow, err := ParseDroneWorkflowMetadata(map[string]interface{}{
			DroneWorkflowMetadataKey: make(chan int),
		})
		require.Error(t, err)
		require.Nil(t, workflow)
	})
}

func TestSetDroneWorkflowMetadata_RemovesWorkflowAndClonesSafely(t *testing.T) {
	t.Run("nil workflow removes stored state", func(t *testing.T) {
		metadata, err := SetDroneWorkflowMetadata(map[string]interface{}{
			DroneWorkflowMetadataKey: map[string]interface{}{"current_state": DroneWorkflowStateReviewQueued},
			"keep":                   "yes",
		}, nil)
		require.NoError(t, err)
		require.Equal(t, "yes", metadata["keep"])
		_, exists := metadata[DroneWorkflowMetadataKey]
		require.False(t, exists)
	})

	t.Run("unclonable metadata degrades to empty map", func(t *testing.T) {
		metadata, err := SetDroneWorkflowMetadata(map[string]interface{}{
			"bad": make(chan int),
		}, nil)
		require.NoError(t, err)
		require.Empty(t, metadata)
	})
}

func TestBuildDroneLifecycle_DefaultsAndLabels(t *testing.T) {
	lifecycle := BuildDroneLifecycle("", DroneWorkflowStateReviewBlocked)
	require.Len(t, lifecycle, len(droneWorkflowPhases))
	require.Equal(t, DroneWorkflowPhaseRequest, lifecycle[0].Phase)
	require.Equal(t, DroneLifecycleStatusComplete, lifecycle[0].Status)
	require.Equal(t, DroneWorkflowPhaseReview, lifecycle[1].Phase)
	require.Equal(t, DroneLifecycleStatusBlocked, lifecycle[1].Status)
	require.Equal(t, DroneWorkflowStateReviewBlocked, lifecycle[1].State)
	require.Equal(t, "Declaration", lifecycle[2].Title)
	require.Equal(t, DroneLifecycleStatusUpcoming, lifecycle[2].Status)

	fallback := NormalizeDroneWorkflow(&DroneWorkflowState{
		CurrentState: "review",
	})
	require.Equal(t, "review", fallback.CurrentPhase)
	require.Equal(t, DroneWorkflowStateRequestDraft, NormalizeDroneWorkflow(&DroneWorkflowState{}).CurrentState)
}

func TestDroneWorkflowStateClone_DeepCopiesNestedState(t *testing.T) {
	submittedAt := time.Date(2026, 3, 29, 8, 0, 0, 0, time.FixedZone("EST", -5*60*60))
	dueAt := submittedAt.Add(2 * time.Hour)
	acceptedAt := submittedAt.Add(4 * time.Hour)

	workflow := &DroneWorkflowState{
		CurrentPhase: DroneWorkflowPhaseSigning,
		CurrentState: DroneWorkflowStateSigningPending,
		Request: &DroneRequestCard{
			ID:          "req-1",
			Title:       "Request",
			Summary:     "Request summary",
			RequestedBy: DroneActor{ID: "owner", Name: "Owner", Role: "requester"},
			SubmittedAt: &submittedAt,
			Constraints: []string{"memory", "timeline"},
			Artifacts:   []DroneArtifact{{ID: "artifact-1", Title: "Intake"}},
		},
		Review: &DroneReviewCard{
			ID:              "review-1",
			Title:           "Review",
			Decision:        DroneReviewDecisionApproved,
			Reviewer:        DroneActor{ID: "reviewer", Name: "Reviewer", Role: "steward"},
			DecisionSummary: "Looks good",
			Findings:        []DroneReviewFinding{{ID: "finding-1", Title: "Finding", Detail: "Detail"}},
			Evidence:        []DroneArtifact{{ID: "evidence-1", Title: "Evidence"}},
		},
		Declaration: &DroneDeclarationCard{
			ID:                  "decl-1",
			Title:               "Declaration",
			Statement:           "Statement",
			Confidence:          "high",
			DeclaredScope:       []string{"identity"},
			Risks:               []string{"none"},
			SupportingArtifacts: []DroneArtifact{{ID: "support-1", Title: "Support"}},
		},
		Checkpoint: &DroneSignatureCheckpoint{
			ID:             "checkpoint-1",
			Title:          "Checkpoint",
			ReadinessLabel: DroneGraduationReadinessReady,
			DueAt:          &dueAt,
			Signers:        []DroneSignatureSigner{{ID: "signer-1", Name: "Signer", Role: "approver", Status: DroneSignatureSignerStatusPending}},
		},
		Graduation: &DroneGraduationSummaryCard{
			ID:                  "grad-1",
			Title:               "Graduation",
			Readiness:           DroneGraduationReadinessWatch,
			Summary:             "Summary",
			CompletedMilestones: []string{"review"},
			ExitCriteria:        []string{"ship"},
			Metrics:             []DroneMetric{{Label: "Confidence", Value: "High"}},
		},
		Continuity: &DroneContinuityPanel{
			ID:           "continuity-1",
			Title:        "Continuity",
			Objective:    "Preserve identity",
			Owner:        DroneActor{ID: "owner", Name: "Owner", Role: "owner"},
			FeedbackLoop: "Daily",
			Metrics:      []DroneMetric{{Label: "Continuity", Value: "Stable"}},
			FollowUps:    []DroneContinuityFollowUp{{ID: "follow-up-1", Title: "Check in", Summary: "Confirm memory", Owner: DroneActor{ID: "owner", Name: "Owner", Role: "owner"}}},
		},
		Conversation: &DroneConversationState{
			ConversationID: "conv-1",
			RequestState:   "accepted",
			AcceptedAt:     &acceptedAt,
			UpdatedAt:      &acceptedAt,
		},
		Lifecycle: []DroneLifecycleStep{{Phase: DroneWorkflowPhaseSigning, Status: DroneLifecycleStatusActive}},
		UpdatedAt: &time.Time{},
	}

	cloned := workflow.Clone()
	require.NotNil(t, cloned)
	require.Nil(t, cloned.UpdatedAt)
	require.NotSame(t, workflow.Request, cloned.Request)
	require.NotSame(t, workflow.Review, cloned.Review)
	require.NotSame(t, workflow.Declaration, cloned.Declaration)
	require.NotSame(t, workflow.Checkpoint, cloned.Checkpoint)
	require.NotSame(t, workflow.Graduation, cloned.Graduation)
	require.NotSame(t, workflow.Continuity, cloned.Continuity)
	require.NotSame(t, workflow.Conversation, cloned.Conversation)
	require.Equal(t, submittedAt.UTC(), *cloned.Request.SubmittedAt)

	cloned.Request.Constraints[0] = "changed"
	cloned.Review.Findings[0].Title = "Changed"
	cloned.Declaration.DeclaredScope[0] = "changed"
	cloned.Checkpoint.Signers[0].Status = DroneSignatureSignerStatusApproved
	cloned.Graduation.CompletedMilestones[0] = "changed"
	cloned.Continuity.FollowUps[0].Title = "Changed"
	cloned.Conversation.RequestState = "declined"
	cloned.Lifecycle[0].Status = DroneLifecycleStatusComplete

	require.Equal(t, "memory", workflow.Request.Constraints[0])
	require.Equal(t, "Finding", workflow.Review.Findings[0].Title)
	require.Equal(t, "identity", workflow.Declaration.DeclaredScope[0])
	require.Equal(t, DroneSignatureSignerStatusPending, workflow.Checkpoint.Signers[0].Status)
	require.Equal(t, "review", workflow.Graduation.CompletedMilestones[0])
	require.Equal(t, "Check in", workflow.Continuity.FollowUps[0].Title)
	require.Equal(t, "accepted", workflow.Conversation.RequestState)
	require.Equal(t, DroneLifecycleStatusActive, workflow.Lifecycle[0].Status)
	require.Nil(t, (*DroneWorkflowState)(nil).Clone())
}
