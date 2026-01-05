package graph

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/storage/models"
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
