package graph

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/agents"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	pkgtesting "github.com/equaltoai/lesser/pkg/testing"
	"github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestRound12MutationResolvers_Notes_BuildCommandAndPollValidation(t *testing.T) {
	resolver, _ := newRound12GraphResolver(t)
	m := &mutationResolver{resolver}

	_, _, err := m.buildCreateNoteCommand("alice", model.CreateNoteInput{
		Content:    "",
		Visibility: model.VisibilityPublic,
	})
	require.Error(t, err)

	quoteID := "status-1"
	cmd, quoteTarget, err := m.buildCreateNoteCommand("alice", model.CreateNoteInput{
		Content:    "hello",
		QuoteID:    &quoteID,
		Visibility: model.VisibilityPublic,
		Poll: &model.PollParamsInput{
			Options:   []string{"a", "b"},
			ExpiresIn: 600,
		},
	})
	require.NoError(t, err)
	require.NotNil(t, cmd)
	require.Equal(t, quoteID, quoteTarget)
	require.Equal(t, "alice", cmd.AuthorID)
	require.Equal(t, "public", cmd.Visibility)
	require.True(t, cmd.PollMultiple == false)
	require.True(t, cmd.PollHideTotals == false)

	badCmd := &notes.CreateNoteCommand{AuthorID: "alice"}
	err = m.applyPollInput(badCmd, &model.PollParamsInput{
		Options:   []string{"only-one"},
		ExpiresIn: 600,
	})
	require.Error(t, err)
}

func TestBuildCreateNoteCommandRejectsUnimplementedStructuredTags(t *testing.T) {
	resolver, _ := newRound12GraphResolver(t)
	mutation := &mutationResolver{resolver}

	for _, input := range []model.CreateNoteInput{
		{Content: "hello", Visibility: model.VisibilityPublic, Mentions: []string{"alice"}},
		{Content: "hello", Visibility: model.VisibilityPublic, Tags: []string{"fediverse"}},
	} {
		_, _, err := mutation.buildCreateNoteCommand("owner", input)
		require.Error(t, err)
		require.True(t, apperrors.HasCode(err, apperrors.CodeValidation))
	}
}

func TestRound12MutationResolvers_Notes_CreateDeleteAndSchedule(t *testing.T) {
	resolver, storageRepo := newRound12GraphResolver(t)

	// Avoid pulling in boosted-state checks inside convertStatusToObject.
	originalBoostFn := viewerBoostStateResolverFunc
	viewerBoostStateResolverFunc = func(context.Context, *Resolver, string, string) (bool, error) { return false, nil }
	t.Cleanup(func() { viewerBoostStateResolverFunc = originalBoostFn })

	mut := resolver.Mutation()
	ctx := round12AuthContext("alice")

	payload, err := mut.CreateNote(ctx, model.CreateNoteInput{
		Content:    "hello world",
		Visibility: model.VisibilityPublic,
	})
	require.NoError(t, err)
	require.NotNil(t, payload)
	require.NotNil(t, payload.Object)
	require.NotNil(t, payload.Activity)

	statusObj, ok := payload.Activity.Object.(*models.Status)
	require.True(t, ok)
	require.NotEmpty(t, statusObj.StatusID)

	okBool, err := mut.DeleteObject(ctx, statusObj.StatusID)
	require.NoError(t, err)
	require.True(t, okBool)

	tooSoon := model.Time(time.Now().Add(2 * time.Minute))
	_, err = mut.ScheduleStatus(ctx, model.ScheduleStatusInput{
		Text:        "scheduled",
		ScheduledAt: tooSoon,
	})
	require.Error(t, err)

	tooFar := model.Time(time.Now().AddDate(2, 0, 0))
	_, err = mut.ScheduleStatus(ctx, model.ScheduleStatusInput{
		Text:        "scheduled",
		ScheduledAt: tooFar,
	})
	require.Error(t, err)

	scheduledAt := model.Time(time.Now().Add(10 * time.Minute))
	scheduled, err := mut.ScheduleStatus(ctx, model.ScheduleStatusInput{
		Text:        "scheduled",
		ScheduledAt: scheduledAt,
	})
	require.NoError(t, err)
	require.NotNil(t, scheduled)

	// Ensure status repo remains usable for other tests.
	require.NotNil(t, storageRepo.Status())
}

func TestCreateNoteDelegationContentClassMatchesEffectiveVisibility(t *testing.T) {
	tests := []struct {
		name         string
		visibility   model.Visibility
		contentClass string
		allowed      bool
	}{
		{name: "note credential cannot attest direct message", visibility: model.VisibilityDirect, contentClass: auth.DelegationContentClassNote},
		{name: "direct message credential attests direct message", visibility: model.VisibilityDirect, contentClass: auth.DelegationContentClassDirectMessage, allowed: true},
		{name: "note credential attests public note", visibility: model.VisibilityPublic, contentClass: auth.DelegationContentClassNote, allowed: true},
		{name: "note credential attests unlisted note", visibility: model.VisibilityUnlisted, contentClass: auth.DelegationContentClassNote, allowed: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resolver, storageRepo := newRound12GraphResolver(t)
			require.NoError(t, storageRepo.User().CreateUser(context.Background(), &storage.User{
				Username: "agent", IsAgent: true, AgentOwner: "owner",
			}))

			claims := &auth.Claims{
				Username: "agent", IsAgent: true, DelegatedBy: "@owner",
				DelegationPrincipal: "owner", DelegationAgent: "agent",
				DelegationContentClass: test.contentClass,
			}
			ctx := context.WithValue(context.Background(), common.ContextKeyClaims, claims)
			payload, err := resolver.Mutation().CreateNote(ctx, model.CreateNoteInput{
				Content: "delegation visibility parity", Visibility: test.visibility,
			})

			restContentClass := auth.DelegationContentClassForVisibility(postingVisibilityFromGraphQL(test.visibility))
			_, _, restDecisionErr := auth.ValidateDelegationAttestation(claims, restContentClass)
			require.Equal(t, test.allowed, restDecisionErr == nil, "REST and GraphQL must classify the same operation identically")
			if !test.allowed {
				require.ErrorIs(t, restDecisionErr, auth.ErrInvalidDelegationCredential)
				require.True(t, apperrors.HasCode(err, apperrors.CodeForbidden))
				require.Nil(t, payload)
				count, countErr := storageRepo.Status().CountStatusesByAuthor(ctx, "agent")
				require.NoError(t, countErr)
				require.Zero(t, count, "a rejected credential must not persist approved_by or any status")
				return
			}

			require.NoError(t, err)
			require.NotNil(t, payload)
			status, ok := payload.Activity.Object.(*models.Status)
			require.True(t, ok)
			require.NotNil(t, status.Note)
			require.NotNil(t, status.Note.AgentAttribution)
			require.Equal(t, resolver.Config.ActorURL("owner"), status.Note.AgentAttribution.ApprovedBy)
		})
	}
}

func TestRound12MutationResolvers_Notes_BuildAgentPostAttribution(t *testing.T) {
	metadata, err := agents.SetDroneWorkflowMetadata(nil, &agents.DroneWorkflowState{
		CurrentPhase: agents.DroneWorkflowPhaseReview,
		CurrentState: agents.DroneWorkflowStateReviewQueued,
	})
	require.NoError(t, err)

	mockUserRepo := mocks.NewMockUserRepositoryInterface()
	mockUserRepo.On("GetUser", mock.Anything, "agent").Return(&storage.User{
		Username:     "agent",
		IsAgent:      true,
		AgentOwner:   "owner",
		AgentVersion: "claude-3",
		Metadata:     metadata,
		AgentCapabilities: &agents.Capabilities{
			RequiresApproval: true,
		},
	}, nil).Twice()

	resolver := &mutationResolver{&Resolver{
		Config:  &config.Config{Domain: "example.com"},
		Storage: pkgtesting.NewMockRepositoryStorage(pkgtesting.WithUserRepository(mockUserRepo)),
	}}

	triggerType := "mention"
	triggerDetails := "test"
	got, err := resolver.buildAgentPostAttribution(context.Background(), &auth.Claims{
		Username:    "agent",
		IsAgent:     true,
		DelegatedBy: "@owner",
		Scopes:      []string{"write"},
	}, &model.AgentPostAttributionInput{
		TriggerType:     &triggerType,
		TriggerDetails:  &triggerDetails,
		MemoryCitations: []string{"status-1", "status-1"},
	}, auth.DelegationContentClassNote)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "mention", got.TriggerType)
	require.Equal(t, "test", got.TriggerDetails)
	require.Equal(t, []string{"status-1"}, got.MemoryCitations)
	require.Equal(t, "https://example.com/users/owner", got.DelegatedBy)
	require.Equal(t, "1.0", got.SchemaVersion)
	require.Equal(t, "claude-3", got.ModelID)
	require.Equal(t, []string{"write"}, got.Scopes)
	require.Contains(t, got.Constraints, "requires_approval")
	require.Equal(t, agents.DroneIdentityStateGraduating, got.IdentityState)
	require.Equal(t, "Graduating", got.IdentityLabel)
	require.Equal(t, agents.DroneContinuityStatePlanned, got.ContinuityState)
	require.Contains(t, got.ContinuitySummary, "@agent")
	require.Equal(t, "Graduating", got.ModerationLabel)
	require.Empty(t, got.ApprovedBy)

	verified, err := resolver.buildAgentPostAttribution(context.Background(), &auth.Claims{
		Username:               "agent",
		IsAgent:                true,
		DelegatedBy:            "@owner",
		Scopes:                 []string{"write"},
		DelegationPrincipal:    "owner",
		DelegationAgent:        "agent",
		DelegationContentClass: auth.DelegationContentClassNote,
	}, nil, auth.DelegationContentClassNote)
	require.NoError(t, err)
	require.Equal(t, "https://example.com/users/owner", verified.DelegatedBy)
	require.Equal(t, "https://example.com/users/owner", verified.ApprovedBy)
	mockUserRepo.AssertExpectations(t)
}
