package graph

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	workflow "github.com/equaltoai/lesser/pkg/agents"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	soulservice "github.com/equaltoai/lesser/pkg/services/souls"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/require"
)

func TestRound13DroneWorkflowQueriesAndResolvers(t *testing.T) {
	resolver, storageRepo := newRound12GraphResolver(t)
	now := time.Date(2026, 3, 28, 9, 30, 0, 0, time.UTC)

	round13SeedGraphUser(t, storageRepo, &storage.User{
		Username:    "owner",
		DisplayName: "Owner",
		Approved:    true,
		CreatedAt:   now.Add(-48 * time.Hour),
		UpdatedAt:   now.Add(-24 * time.Hour),
	}, nil)

	metadata, err := workflow.SetDroneWorkflowMetadata(nil, &workflow.DroneWorkflowState{
		CurrentPhase: workflow.DroneWorkflowPhaseReview,
		CurrentState: workflow.DroneWorkflowStateReviewApproved,
		Request: &workflow.DroneRequestCard{
			ID:      "drone-alpha:request",
			Title:   "Graduation request",
			Summary: "Prepare the drone for graduation review.",
			RequestedBy: workflow.DroneActor{
				ID:     "owner",
				Name:   "Owner",
				Role:   "requester",
				Handle: "@owner",
			},
			SubmittedAt: &now,
			Constraints: []string{"preserve timeline"},
			Artifacts: []workflow.DroneArtifact{
				{ID: "artifact-1", Title: "Request brief", Href: "https://example.com/request"},
			},
			RouteDecision: "review_for_graduation",
			CurrentState:  workflow.DroneWorkflowStateRequestSubmitted,
		},
		Review: &workflow.DroneReviewCard{
			ID:              "drone-alpha:review",
			Title:           "Readiness review",
			Decision:        workflow.DroneReviewDecisionApproved,
			Reviewer:        workflow.DroneActor{ID: "owner", Name: "Owner", Role: "reviewer", Handle: "@owner"},
			DecisionSummary: "Ready to continue.",
			Findings: []workflow.DroneReviewFinding{
				{ID: "finding-1", Title: "Timeline continuity", Detail: "Timeline continuity remains intact.", Severity: "low"},
			},
		},
		Declaration: &workflow.DroneDeclarationCard{
			ID:            "drone-alpha:declaration",
			Title:         "Graduation declaration",
			Statement:     "The drone is ready for supervised launch.",
			Confidence:    "high",
			DeclaredScope: []string{"timeline", "memory"},
		},
		Conversation: &workflow.DroneConversationState{
			ConversationID: "conv-1",
			Folder:         "REQUESTS",
			RequestState:   "accepted",
			Unread:         true,
			UpdatedAt:      &now,
		},
		UpdatedAt: &now,
	})
	require.NoError(t, err)

	round13SeedGraphUser(t, storageRepo, &storage.User{
		Username:    "drone-alpha",
		DisplayName: "Drone Alpha",
		Approved:    true,
		IsAgent:     true,
		AgentOwner:  "@owner",
		CreatedAt:   now.Add(-24 * time.Hour),
		UpdatedAt:   now,
		Metadata:    metadata,
	}, &storage.AgentGovernanceState{
		Username:        "drone-alpha",
		DelegatedScopes: []string{auth.ScopeRead, auth.ScopeWrite},
	})

	identity, err := (&agentResolver{resolver}).IdentitySemantics(context.Background(), &model.Agent{Username: "drone-alpha"})
	require.NoError(t, err)
	require.NotNil(t, identity)
	require.Equal(t, workflow.DroneIdentityStateGraduating, identity.IdentityState)
	require.Equal(t, "Graduating", identity.IdentityLabel)

	workflowSurface, err := (&agentResolver{resolver}).Workflow(round13DroneAuthContext("owner", auth.ScopeRead), &model.Agent{Username: "drone-alpha"})
	require.NoError(t, err)
	require.NotNil(t, workflowSurface)
	require.Equal(t, workflow.DroneWorkflowPhaseReview, workflowSurface.CurrentPhase)
	require.NotNil(t, workflowSurface.Request)
	require.NotNil(t, workflowSurface.Review)
	require.NotNil(t, workflowSurface.Declaration)
	require.NotNil(t, workflowSurface.Conversation)
	require.Equal(t, "conv-1", workflowSurface.Conversation.ConversationID)
	require.Equal(t, "accepted", derefString(workflowSurface.Conversation.RequestState))

	requests, err := (&queryResolver{resolver}).MyDroneRequests(round13DroneAuthContext("owner", auth.ScopeRead))
	require.NoError(t, err)
	require.Len(t, requests, 1)
	require.Equal(t, "Graduation request", requests[0].Title)

	reviews, err := (&queryResolver{resolver}).MyDroneReviews(round13DroneAuthContext("owner", auth.ScopeRead))
	require.NoError(t, err)
	require.Len(t, reviews, 1)
	require.Equal(t, workflow.DroneReviewDecisionApproved, reviews[0].Decision)

	direct, err := (&queryResolver{resolver}).DroneWorkflow(round13DroneAuthContext("owner", auth.ScopeRead), "drone-alpha")
	require.NoError(t, err)
	require.NotNil(t, direct)
	require.Equal(t, "drone-alpha", direct.Username)

	_, err = (&queryResolver{resolver}).DroneWorkflow(round13DroneAuthContext("stranger", auth.ScopeRead), "drone-alpha")
	require.Error(t, err)
	require.True(t, apperrors.HasCode(err, apperrors.CodeForbidden))
}

func TestRound13DroneWorkflowMutationsPersistWorkflowState(t *testing.T) {
	resolver, storageRepo := newRound12GraphResolver(t)
	now := time.Date(2026, 3, 28, 11, 0, 0, 0, time.UTC)

	resolver.soulsClient = &stubSoulService{
		incorporateFunc: func(_ context.Context, username string, targetAgentUsername string, agentID string) (*soulservice.Soul, error) {
			require.Equal(t, "owner", username)
			require.Equal(t, "drone-beta", targetAgentUsername)
			require.Equal(t, "0xsoul", agentID)
			return &soulservice.Soul{AgentID: agentID, Bound: true, BoundAgentUsername: targetAgentUsername}, nil
		},
	}

	round13SeedGraphUser(t, storageRepo, &storage.User{
		Username:    "owner",
		DisplayName: "Owner",
		Approved:    true,
		CreatedAt:   now.Add(-72 * time.Hour),
		UpdatedAt:   now.Add(-24 * time.Hour),
	}, nil)

	round13SeedGraphUser(t, storageRepo, &storage.User{
		Username:    "drone-beta",
		DisplayName: "Drone Beta",
		Approved:    true,
		IsAgent:     true,
		AgentOwner:  "@owner",
		CreatedAt:   now.Add(-24 * time.Hour),
		UpdatedAt:   now,
	}, &storage.AgentGovernanceState{
		Username:        "drone-beta",
		DelegatedScopes: []string{auth.ScopeRead, auth.ScopeWrite},
	})

	mut := &mutationResolver{resolver}

	requestPayload, err := mut.RequestSoulPromotion(round13DroneAuthContext("owner", auth.ScopeWrite), model.RequestSoulPromotionInput{
		Username:    "drone-beta",
		Title:       "Promote drone beta",
		Summary:     "Request graduation review for Drone Beta.",
		Constraints: []string{"preserve body identity"},
		Artifacts: []*model.AgentSurfaceArtifactInput{
			{Title: "Request memo", Href: round13StringPtr("https://example.com/request-memo")},
		},
		ConversationID: round13StringPtr("conv-2"),
	})
	require.NoError(t, err)
	require.NotNil(t, requestPayload)
	require.NotNil(t, requestPayload.Workflow.Request)
	require.Equal(t, workflow.DroneWorkflowStateRequestSubmitted, requestPayload.Workflow.CurrentState)
	require.Len(t, requestPayload.Workflow.Request.Artifacts, 1)

	reviewPayload, err := mut.ReviewSoulPromotion(round13DroneAuthContext("owner", auth.ScopeWrite), model.ReviewSoulPromotionInput{
		Username:        "drone-beta",
		Decision:        workflow.DroneReviewDecisionApproved,
		DecisionSummary: "Ready to advance.",
		Findings: []*model.ReviewFindingInput{
			{Title: "Continuity", Detail: "Continuity safeguards are documented.", Severity: round13StringPtr("low")},
		},
		Evidence: []*model.AgentSurfaceArtifactInput{
			{Title: "Checklist", Href: round13StringPtr("https://example.com/checklist")},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, reviewPayload.Workflow.Review)
	require.Equal(t, workflow.DroneWorkflowStateReviewApproved, reviewPayload.Workflow.CurrentState)
	require.NotNil(t, reviewPayload.Workflow.Checkpoint)
	require.Len(t, reviewPayload.Workflow.Checkpoint.Signers, 1)

	finalPayload, err := mut.FinalizeSoulPromotion(round13DroneAuthContext("owner", auth.ScopeWrite), model.FinalizeSoulPromotionInput{
		Username:              "drone-beta",
		SoulAgentID:           round13StringPtr("0xsoul"),
		DeclarationStatement:  "The drone can graduate while preserving continuity.",
		DeclarationConfidence: "high",
		DeclaredScope:         []string{"timeline", "memory", "identity"},
		SupportingArtifacts: []*model.AgentSurfaceArtifactInput{
			{Title: "Declaration packet", Href: round13StringPtr("https://example.com/declaration")},
		},
		Readiness:              workflow.DroneGraduationReadinessReady,
		Summary:                "Graduation is approved and continuity is stable.",
		CompletedMilestones:    []string{"request", "review"},
		ExitCriteria:           []string{"soul linked"},
		ContinuityObjective:    round13StringPtr("Preserve the existing body through soul binding."),
		ContinuityFeedbackLoop: round13StringPtr("Check memory continuity after launch."),
		ConversationID:         round13StringPtr("conv-2"),
	})
	require.NoError(t, err)
	require.NotNil(t, finalPayload)
	require.NotNil(t, finalPayload.Workflow.Declaration)
	require.NotNil(t, finalPayload.Workflow.Graduation)
	require.NotNil(t, finalPayload.Workflow.Continuity)
	require.Equal(t, workflow.DroneWorkflowPhaseContinuity, finalPayload.Workflow.CurrentPhase)
	require.Equal(t, workflow.DroneIdentityStateSouled, finalPayload.Workflow.IdentitySemantics.IdentityState)
	require.Equal(t, "0xsoul", derefString(finalPayload.Workflow.IdentitySemantics.SoulAgentID))

	storedUser, err := storageRepo.Account().GetUser(context.Background(), "drone-beta")
	require.NoError(t, err)
	storedWorkflow, err := workflow.ParseDroneWorkflowMetadata(storedUser.Metadata)
	require.NoError(t, err)
	require.NotNil(t, storedWorkflow)
	require.Equal(t, workflow.DroneWorkflowPhaseContinuity, storedWorkflow.CurrentPhase)
	require.Equal(t, workflow.DroneWorkflowStateContinuityStable, storedWorkflow.CurrentState)
	require.Equal(t, "0xsoul", storedWorkflow.SoulAgentID)

	identity, err := (&agentResolver{resolver}).IdentitySemantics(context.Background(), &model.Agent{Username: "drone-beta"})
	require.NoError(t, err)
	require.NotNil(t, identity)
	require.Equal(t, workflow.DroneIdentityStateSouled, identity.IdentityState)
}

func TestRound13DroneWorkflowAssignedReviewerAccess(t *testing.T) {
	resolver, storageRepo := newRound12GraphResolver(t)
	now := time.Date(2026, 3, 28, 12, 30, 0, 0, time.UTC)

	round13SeedGraphUser(t, storageRepo, &storage.User{
		Username:    "owner",
		DisplayName: "Owner",
		Approved:    true,
		CreatedAt:   now.Add(-72 * time.Hour),
		UpdatedAt:   now.Add(-24 * time.Hour),
	}, nil)
	round13SeedGraphUser(t, storageRepo, &storage.User{
		Username:    "reviewer",
		DisplayName: "Reviewer",
		Approved:    true,
		CreatedAt:   now.Add(-48 * time.Hour),
		UpdatedAt:   now.Add(-24 * time.Hour),
	}, nil)

	metadata, err := workflow.SetDroneWorkflowMetadata(nil, &workflow.DroneWorkflowState{
		CurrentPhase: workflow.DroneWorkflowPhaseReview,
		CurrentState: workflow.DroneWorkflowStateReviewQueued,
		Request: &workflow.DroneRequestCard{
			ID:      "drone-gamma:request",
			Title:   "Graduation request",
			Summary: "Queue a reviewer-led graduation review.",
			RequestedBy: workflow.DroneActor{
				ID:     "owner",
				Name:   "Owner",
				Role:   "requester",
				Handle: "@owner",
			},
			SubmittedAt: &now,
		},
		Review: &workflow.DroneReviewCard{
			ID:              "drone-gamma:review",
			Title:           "Assigned review",
			Decision:        workflow.DroneReviewDecisionQueued,
			Reviewer:        workflow.DroneActor{ID: "reviewer", Name: "Reviewer", Role: "reviewer", Handle: "@reviewer"},
			DecisionSummary: "Awaiting reviewer input.",
		},
		UpdatedAt: &now,
	})
	require.NoError(t, err)

	round13SeedGraphUser(t, storageRepo, &storage.User{
		Username:    "drone-gamma",
		DisplayName: "Drone Gamma",
		Approved:    true,
		IsAgent:     true,
		AgentOwner:  "@owner",
		CreatedAt:   now.Add(-24 * time.Hour),
		UpdatedAt:   now,
		Metadata:    metadata,
	}, &storage.AgentGovernanceState{
		Username:        "drone-gamma",
		DelegatedScopes: []string{auth.ScopeRead, auth.ScopeWrite},
	})

	reviews, err := (&queryResolver{resolver}).MyDroneReviews(round13DroneAuthContext("reviewer", auth.ScopeRead))
	require.NoError(t, err)
	require.Len(t, reviews, 1)
	require.Equal(t, "Assigned review", reviews[0].Title)

	ownerReviews, err := (&queryResolver{resolver}).MyDroneReviews(round13DroneAuthContext("owner", auth.ScopeRead))
	require.NoError(t, err)
	require.Empty(t, ownerReviews)

	reviewerWorkflow, err := (&queryResolver{resolver}).DroneWorkflow(round13DroneAuthContext("reviewer", auth.ScopeRead), "drone-gamma")
	require.NoError(t, err)
	require.NotNil(t, reviewerWorkflow)
	require.Equal(t, workflow.DroneWorkflowStateReviewQueued, reviewerWorkflow.CurrentState)

	_, err = (&mutationResolver{resolver}).ReviewSoulPromotion(round13DroneAuthContext("owner", auth.ScopeWrite), model.ReviewSoulPromotionInput{
		Username:        "drone-gamma",
		Decision:        workflow.DroneReviewDecisionApproved,
		DecisionSummary: "Owner should not hijack an assigned review.",
	})
	require.Error(t, err)
	require.True(t, apperrors.HasCode(err, apperrors.CodeForbidden))

	reviewPayload, err := (&mutationResolver{resolver}).ReviewSoulPromotion(round13DroneAuthContext("reviewer", auth.ScopeWrite), model.ReviewSoulPromotionInput{
		Username:        "drone-gamma",
		Decision:        workflow.DroneReviewDecisionApproved,
		DecisionSummary: "Assigned reviewer approved the workflow.",
	})
	require.NoError(t, err)
	require.NotNil(t, reviewPayload)
	require.NotNil(t, reviewPayload.Workflow)
	require.Equal(t, workflow.DroneWorkflowStateReviewApproved, reviewPayload.Workflow.CurrentState)
	require.NotNil(t, reviewPayload.Workflow.Review)
	require.Equal(t, "reviewer", reviewPayload.Workflow.Review.Reviewer.ID)
}

func TestRound13DroneWorkflowSouledContinuityStatePreserved(t *testing.T) {
	resolver, storageRepo := newRound12GraphResolver(t)
	now := time.Date(2026, 3, 28, 13, 0, 0, 0, time.UTC)

	round13SeedGraphUser(t, storageRepo, &storage.User{
		Username:    "owner",
		DisplayName: "Owner",
		Approved:    true,
		CreatedAt:   now.Add(-48 * time.Hour),
		UpdatedAt:   now.Add(-24 * time.Hour),
	}, nil)

	metadata, err := workflow.SetDroneWorkflowMetadata(nil, &workflow.DroneWorkflowState{
		CurrentPhase: workflow.DroneWorkflowPhaseContinuity,
		CurrentState: workflow.DroneWorkflowStateContinuityMonitoring,
		SoulAgentID:  "0xsoul",
		Continuity: &workflow.DroneContinuityPanel{
			ID:           "drone-delta:continuity",
			Title:        "Continuity monitor",
			Objective:    "Preserve identity with active monitoring.",
			Owner:        workflow.DroneActor{ID: "owner", Name: "Owner", Role: "steward", Handle: "@owner"},
			FeedbackLoop: "Check continuity after launch.",
		},
		UpdatedAt: &now,
	})
	require.NoError(t, err)

	round13SeedGraphUser(t, storageRepo, &storage.User{
		Username:    "drone-delta",
		DisplayName: "Drone Delta",
		Approved:    true,
		IsAgent:     true,
		AgentOwner:  "@owner",
		CreatedAt:   now.Add(-24 * time.Hour),
		UpdatedAt:   now,
		Metadata:    metadata,
	}, &storage.AgentGovernanceState{
		Username:        "drone-delta",
		DelegatedScopes: []string{auth.ScopeRead, auth.ScopeWrite},
	})

	surface, err := (&queryResolver{resolver}).DroneWorkflow(round13DroneAuthContext("owner", auth.ScopeRead), "drone-delta")
	require.NoError(t, err)
	require.NotNil(t, surface)
	require.Equal(t, workflow.DroneWorkflowPhaseContinuity, surface.CurrentPhase)
	require.Equal(t, workflow.DroneWorkflowStateContinuityMonitoring, surface.CurrentState)
	require.NotNil(t, surface.IdentitySemantics)
	require.Equal(t, workflow.DroneIdentityStateSouled, surface.IdentitySemantics.IdentityState)
	require.Equal(t, workflow.DroneContinuityStateMonitoring, surface.IdentitySemantics.ContinuityState)
	require.Equal(t, workflow.DroneWorkflowStateContinuityMonitoring, surface.IdentitySemantics.LifecycleState)
	require.NotEmpty(t, surface.Lifecycle)
	require.Equal(t, workflow.DroneWorkflowStateContinuityMonitoring, derefString(surface.Lifecycle[len(surface.Lifecycle)-1].State))
}

func round13SeedGraphUser(t *testing.T, storageRepo *round12GraphStorage, user *storage.User, governance *storage.AgentGovernanceState) {
	t.Helper()

	require.NoError(t, storageRepo.User().CreateUser(context.Background(), user))
	require.NoError(t, storageRepo.Account().CreateAccount(context.Background(), &storage.Account{User: user}))
	storageRepo.SeedAccountUser(user)
	if governance != nil {
		storageRepo.SeedAgentGovernanceState(governance)
	}
}

func round13DroneAuthContext(username string, scopes ...string) context.Context {
	return context.WithValue(context.Background(), common.ContextKeyClaims, &auth.Claims{
		Username: username,
		Scopes:   scopes,
	})
}

func round13StringPtr(value string) *string {
	return &value
}
