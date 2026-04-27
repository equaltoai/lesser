package graph

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/storage"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestRound12ModerationResolvers_QueryAndMutation(t *testing.T) {
	resolver, storageRepo := newRound12GraphResolver(t)
	userCtx := round12AuthContext("alice")
	adminCtx := round12AuthContext("admin")

	// Ensure a note exists so AddCommunityNote can validate it via notes service.
	status := &storageModels.Status{
		StatusID:            "status-1",
		AuthorID:            "https://localhost/users/alice",
		AuthorUsername:      "alice",
		Content:             "hello world",
		CreatedAt:           time.Now().Add(-time.Hour),
		UpdatedAt:           time.Now().Add(-time.Minute),
		Visibility:          storageModels.VisibilityPublic,
		ReplyCount:          0,
		ReblogCount:         0,
		LikeCount:           0,
		QuoteCount:          0,
		Sensitive:           false,
		Deleted:             false,
		ConversationID:      "",
		InReplyToID:         "",
		QuoteTargetStatusID: "",
	}
	require.NoError(t, storageRepo.Status().CreateStatus(context.Background(), status))

	// Avoid pulling in notes-service boosting checks inside convertStatusToObject.
	originalBoostFn := viewerBoostStateResolverFunc
	viewerBoostStateResolverFunc = func(context.Context, *Resolver, string, string) (bool, error) { return false, nil }
	t.Cleanup(func() { viewerBoostStateResolverFunc = originalBoostFn })

	mut := resolver.Mutation()
	qry := resolver.Query()

	// Flag content (creates a flag + moderation event).
	flagPayload, err := mut.FlagObject(userCtx, model.FlagInput{
		ObjectID: "status-1",
		Reason:   "spam",
		Evidence: []string{"e1"},
	})
	require.NoError(t, err)
	require.NotNil(t, flagPayload)
	require.True(t, flagPayload.Queued)

	// Seed a decision so dashboard/analytics helpers have something to compute from.
	modRepo := storageRepo.Moderation()
	require.NoError(t, modRepo.CreateModerationDecision(context.Background(), &storage.ModerationDecision{
		ID:       "dec-1",
		EventID:  flagPayload.ModerationID,
		ObjectID: "status-1",
		Action:   "warn",
		Reason:   "reviewed",
		Appeal:   true,
		Decided:  time.Now().Add(10 * time.Second),
	}))

	// Add a second event for trend and moderator stats coverage.
	require.NoError(t, modRepo.CreateModerationEvent(context.Background(), &storage.ModerationEvent{
		ID:              "evt-2",
		ObjectID:        "status-2",
		ObjectType:      "status",
		ActorID:         "mod1",
		EventType:       "flagged",
		Category:        "spam",
		Severity:        "low",
		ConfidenceScore: 0.4,
		Reason:          "test",
		CreatedAt:       time.Now(),
		Data:            map[string]interface{}{"content": "spam content", "severity": "high"},
	}))

	// Create a pattern through the mutation resolver (stored in moderation repo).
	active := true
	_, err = mut.CreateModerationPattern(userCtx, model.ModerationPatternInput{
		Pattern:  "spam",
		Type:     model.PatternTypeKeyword,
		Severity: model.ModerationSeverityHigh,
		Active:   &active,
	})
	require.Error(t, err)

	pattern, err := mut.CreateModerationPattern(adminCtx, model.ModerationPatternInput{
		Pattern:  "spam",
		Type:     model.PatternTypeKeyword,
		Severity: model.ModerationSeverityHigh,
		Active:   &active,
	})
	require.NoError(t, err)
	require.NotNil(t, pattern)
	require.NotEmpty(t, pattern.ID)

	// Query: moderation queue should include the flagged events.
	queue, err := qry.ModerationQueue(adminCtx, ptrIntValue(10), nil)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(queue), 1)

	// Query: dashboard aggregates multiple helper methods.
	dashboard, err := qry.ModerationDashboard(adminCtx, nil)
	require.NoError(t, err)
	require.NotNil(t, dashboard)

	// Query: effectiveness uses calculatePatternEffectiveness.
	eff, err := qry.ModerationEffectiveness(adminCtx, pattern.ID, model.ModerationPeriodDaily)
	require.NoError(t, err)
	require.NotNil(t, eff)

	// Query: patterns list.
	patterns, err := qry.ModerationPatterns(adminCtx, nil, nil, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, patterns)

	// Query: moderator stats.
	stats, err := qry.ModeratorActivity(adminCtx, "mod1", model.TimePeriodDay)
	require.NoError(t, err)
	require.NotNil(t, stats)

	// Query: pattern effectiveness uses recent events/accuracy heuristics.
	pStats, err := qry.PatternEffectiveness(adminCtx, pattern.ID)
	require.NoError(t, err)
	require.NotNil(t, pStats)

	// Mutation: add community note.
	notePayload, err := mut.AddCommunityNote(userCtx, model.CommunityNoteInput{
		ObjectID: "status-1",
		Content:  "fact check",
	})
	require.NoError(t, err)
	require.NotNil(t, notePayload)
	require.NotNil(t, notePayload.Note)
	require.NotNil(t, notePayload.Object)

	// Mutation: voting paths (author cannot vote).
	note, err := mut.VoteCommunityNote(userCtx, "note-1", true)
	require.NoError(t, err)
	require.NotNil(t, note)

	_, err = mut.VoteCommunityNote(round12AuthContext("bob"), "note-1", false)
	require.Error(t, err)

	// Mutation: update/delete pattern.
	_, err = mut.UpdateModerationPattern(userCtx, pattern.ID, model.ModerationPatternInput{
		Pattern:  "spam2",
		Type:     model.PatternTypePhrase,
		Severity: model.ModerationSeverityMedium,
	})
	require.Error(t, err)

	updated, err := mut.UpdateModerationPattern(adminCtx, pattern.ID, model.ModerationPatternInput{
		Pattern:  "spam2",
		Type:     model.PatternTypePhrase,
		Severity: model.ModerationSeverityMedium,
	})
	require.NoError(t, err)
	require.NotNil(t, updated)

	ok, err := mut.DeleteModerationPattern(adminCtx, pattern.ID)
	require.NoError(t, err)
	require.True(t, ok)

	// Mutation: ML training is admin-gated before feature flag or service access.
	_, err = mut.TrainModerationModel(userCtx, nil, nil)
	require.ErrorIs(t, err, ErrAdminPrivilegesRequired)
}

func TestRound12ModerationResolvers_DashboardStorageNil(t *testing.T) {
	resolver, _ := newRound12GraphResolver(t)
	resolver.Storage = nil

	q := &queryResolver{resolver}
	_, err := q.ModerationDashboard(round12AuthContext("admin"), nil)
	require.Error(t, err)
}
