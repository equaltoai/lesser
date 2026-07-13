package graph

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/storage"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	dynamormmocks "github.com/theory-cloud/tabletheory/v2/pkg/mocks"
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
		Content:  `<script>alert(1)</script><b>fact check</b>`,
	})
	require.NoError(t, err)
	require.NotNil(t, notePayload)
	require.NotNil(t, notePayload.Note)
	require.NotNil(t, notePayload.Object)
	require.NotContains(t, notePayload.Note.Content, "<script")
	require.NotContains(t, notePayload.Note.Content, "<b>fact check</b>")
	require.Contains(t, notePayload.Note.Content, "&lt;b&gt;fact check&lt;/b&gt;")

	require.NoError(t, storageRepo.CommunityNote().CreateCommunityNote(context.Background(), &storage.CommunityNote{
		ID:               "legacy-raw-note",
		ObjectID:         "status-1",
		ObjectType:       "status",
		AuthorID:         "https://localhost/users/bob",
		Content:          `<script>alert(1)</script><b>legacy</b>`,
		VisibilityStatus: "visible",
		CreatedAt:        time.Now().Add(-time.Minute),
		UpdatedAt:        time.Now().Add(-time.Minute),
	}))

	// Mutation: voting paths (author cannot vote).
	note, err := mut.VoteCommunityNote(userCtx, "legacy-raw-note", true)
	require.NoError(t, err)
	require.NotNil(t, note)
	require.NotContains(t, note.Content, "<script")
	require.Contains(t, note.Content, "&lt;b&gt;legacy note&lt;/b&gt;")

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

// TestAddCommunityNote_RateLimitExceeded proves that the GraphQL AddCommunityNote
// mutation enforces the rate limit repaired under CSR-048 and that the limit
// prevents community note creation at the resolver boundary, not just in the
// pkg/notes.Service layer.
//
// Setup:
//   - A status is pre-created so GetNoteWithViewer passes validation.
//   - The Dynamorm mock's auto-populate is enabled to return 5 CommunityNote
//     models (all with CreatedAt within 24 h).  With a baseline reputation of
//     500, CalculateNoteLimit(500) = 5, so the rate-limit check denies.
//
// Assertions:
//  1. AddCommunityNote returns ErrCommunityNoteRateLimited (or an equivalent
//     app error containing "rate limit").
//  2. No community note is created — proved executably by counting
//     Dynamorm Create/CreateOrUpdate calls before and after the mutation.
//     If CreateCommunityNote were reached it would add a Create or
//     CreateOrUpdate call to the mock query; the before-after diff being
//     zero proves the early return at the rate-limit gate.
//
// Reputation baseline note:
//
//	The GraphQL resolver uses a flat baseline reputation of 500.0, which
//	grants up to 5 notes/day.  The REST handler (HandleCreateNoteLift)
//	fetches the caller's actual reputation via the reputation service and uses
//	the real TotalScore.  The flat baseline is an intentional conservative
//	default for GraphQL, documented in mutation_resolvers_moderation.go.
func TestAddCommunityNote_RateLimitExceeded(t *testing.T) {
	resolver, storage, _, mockQuery, state := newRound12GraphResolverWithMocks(t)

	// Seed a status object so GetNoteWithViewer passes validation.
	status := &storageModels.Status{
		StatusID:            "status-ratelimit",
		AuthorID:            "https://localhost/users/alice",
		AuthorUsername:      "alice",
		Content:             "test content for rate limit",
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
	require.NoError(t, storage.Status().CreateStatus(context.Background(), status))

	// Avoid pulling in notes-service boosting checks inside convertStatusToObject.
	originalBoostFn := viewerBoostStateResolverFunc
	viewerBoostStateResolverFunc = func(context.Context, *Resolver, string, string) (bool, error) { return false, nil }
	t.Cleanup(func() { viewerBoostStateResolverFunc = originalBoostFn })

	// Enable auto-populate on the Dynamorm mock so that GetCommunityNotesByAuthor
	// returns 5 CommunityNote models.  All have CreatedAt within 24 h, so with
	// CalculateNoteLimit(500)=5 the rate-limit check denies.
	state.autoPopulateAll = true
	state.autoPopulateCount = 5

	// Count Create/CreateOrUpdate calls on the Dynamorm mock before the
	// mutation.  If the resolver reaches CreateCommunityNote it will add a
	// Create (or CreateOrUpdate) call; the diff must be zero to prove the
	// early-return at the rate-limit gate works executably.
	createCountBefore := countMockCreateCalls(mockQuery)

	mut := resolver.Mutation()
	userCtx := round12AuthContext("alice")

	payload, err := mut.AddCommunityNote(userCtx, model.CommunityNoteInput{
		ObjectID: "status-ratelimit",
		Content:  "This note should be rate-limited",
	})

	createCountAfter := countMockCreateCalls(mockQuery)

	// 1. Resolver must return a rate-limit error when the caller is at the limit.
	require.Error(t, err)
	require.Nil(t, payload)
	require.Contains(t, err.Error(), "Rate limit",
		"expected rate-limit error, got: %v", err)

	// 2. No community note is created — proved executably: the number of
	//    Dynamorm Create/CreateOrUpdate calls did not change during the
	//    rate-limited mutation.  If CreateCommunityNote were reached it would
	//    add a Create (or CreateOrUpdate) call.
	require.Equal(t, createCountBefore, createCountAfter,
		"CreateCommunityNote must not be invoked after rate-limit denial")
}

// countMockCreateCalls returns the number of Create and CreateOrUpdate calls
// recorded on a Dynamorm MockQuery.  It is used to prove executably that the
// rate-limit gate prevents CreateCommunityNote from being reached: if the call
// count does not change across a rate-limited mutation, no write occurred.
func countMockCreateCalls(q *dynamormmocks.MockQuery) int {
	count := 0
	for _, call := range q.Calls {
		if call.Method == "Create" || call.Method == "CreateOrUpdate" {
			count++
		}
	}
	return count
}

func TestRound12ModerationResolvers_DashboardStorageNil(t *testing.T) {
	resolver, _ := newRound12GraphResolver(t)
	resolver.Storage = nil

	q := &queryResolver{resolver}
	_, err := q.ModerationDashboard(round12AuthContext("admin"), nil)
	require.Error(t, err)
}
