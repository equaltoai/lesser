package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

// Batch N1 (umbrella #1469, 2026-08-27) — the BOUNDED-QUERY class in
// moderation_repository.go / status_repository.go / query_utils.go: keyed
// whole-partition `.All()` reads with no enforced bound. Every assertion is
// mutation-viable: it pins the LITERAL bound (reverting a clamp/cap fails the
// test), the exact page-cap count at which a walk fails closed, and the cursor
// handoff sequence between pages.

func mockWalkExpectations(t *testing.T, mockQuery *mocks.MockQuery, pageSize int, pages []core.PaginatedResult) {
	t.Helper()
	mockQuery.On("Limit", pageSize).Return(mockQuery).Once()
	prevCursor := ""
	for i, page := range pages {
		if i > 0 {
			mockQuery.On("Cursor", prevCursor).Return(mockQuery).Once()
		}
		mockQuery.On("AllPaginated", mock.Anything).Return(&page, nil).Once()
		prevCursor = page.NextCursor
	}
}

// ===== moderation_repository.go =====

func TestBatchN1_GetModerationQueue_ClampedLimit(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	// filter.Limit of 1000 must clamp to the 100 hard max. Reverting the clamp
	// issues Limit(1000), which this mock does not expect — the test dies.
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Index", "gsi2").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi2PK", "=", "TYPE#flagged#pending").Return(mockQuery).Once()
	mockQuery.On("Limit", 100).Return(mockQuery).Once()
	mockQuery.On("All", mock.Anything).Return(nil).Once()

	repo := NewModerationRepository(mockDB, "test-table", zap.NewNop())
	_, err := repo.GetModerationQueue(ctx, &storage.ModerationFilter{Limit: 1000})
	require.NoError(t, err)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN1_GetModerationReviews_PageCappedWalk(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	// The full-set read must iterate in bounded pages: a clamped Limit(500)
	// per page, an AllPaginated read (not a bare unbounded All), and a cursor
	// handoff between pages.
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "REVIEW#event-1").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.ModerationReview)
		*dest = []models.ModerationReview{{Type: "REVIEW", ID: "r1"}}
	}).Return(&core.PaginatedResult{HasMore: true, NextCursor: "c1"}, nil).Once()
	mockQuery.On("Cursor", "c1").Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.ModerationReview)
		*dest = []models.ModerationReview{{Type: "REVIEW", ID: "r2"}}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	repo := NewModerationRepository(mockDB, "test-table", zap.NewNop())
	reviews, err := repo.GetModerationReviews(ctx, "event-1")
	require.NoError(t, err)
	require.Len(t, reviews, 2)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN1_GetModerationReviews_PageCapExhaustionFailsClosed(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "REVIEW#event-1").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	// 100 full pages of 500 with more pages available: the walk must fail
	// closed at exactly the 100-page cap — a `>` vs `>=` off-by-one (or a
	// different cap constant) either issues a 101st read or short-circuits,
	// both of which fail this test.
	full := func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.ModerationReview)
		*dest = make([]models.ModerationReview, 500)
	}
	mockQuery.On("AllPaginated", mock.Anything).Run(full).Return(&core.PaginatedResult{HasMore: true, NextCursor: "c"}, nil).Times(100)
	mockQuery.On("Cursor", "c").Return(mockQuery).Times(99)

	repo := NewModerationRepository(mockDB, "test-table", zap.NewNop())
	_, err := repo.GetModerationReviews(ctx, "event-1")
	require.Error(t, err)
	require.ErrorIs(t, err, errBoundedPageCapExceeded)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN1_GetModerationHistory_DecisionsWalk(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	// Events leg already bounded (Limit 100 + 1 sentinel) — unchanged.
	mockQuery.On("Where", "PK", "=", "EVENT#obj-1").Return(mockQuery).Once()
	mockQuery.On("OrderBy", "SK", "DESC").Return(mockQuery).Once()
	mockQuery.On("Limit", 101).Return(mockQuery).Once()
	mockQuery.On("All", mock.Anything).Return(nil).Once()
	// Decisions leg must be a bounded page walk: Limit(500) + AllPaginated.
	mockQuery.On("Where", "PK", "=", "DECISION#obj-1").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.ModerationDecision)
		*dest = []models.ModerationDecision{{Type: "DECISION", ID: "d1"}}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	repo := NewModerationRepository(mockDB, "test-table", zap.NewNop())
	history, err := repo.GetModerationHistory(ctx, "obj-1")
	require.NoError(t, err)
	require.Len(t, history.Decisions, 1)
	require.NotEmpty(t, history.Timeline)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN1_GetReviewerStats_PageCappedWalk(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	// User + trust-score point reads (unchanged shape).
	mockQuery.On("Where", "PK", "=", "USER#reviewer-1").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "=", "PROFILE").Return(mockQuery).Once()
	mockQuery.On("First", mock.Anything).Return(nil).Once()
	mockQuery.On("Where", "PK", "=", "TRUST_SCORE#reviewer-1").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "=", "CATEGORY#moderation").Return(mockQuery).Once()
	mockQuery.On("First", mock.Anything).Return(nil).Once()
	// Reviews leg must be a bounded page walk over GSI1.
	mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1PK", "=", "REVIEWER#reviewer-1").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.ModerationReview)
		*dest = []models.ModerationReview{
			{Type: "REVIEW", ReviewerID: "reviewer-1", Severity: "high", ReviewerRep: 0.8, Created: time.Now()},
			{Type: "REVIEW", ReviewerID: "reviewer-1", Severity: "medium", ReviewerRep: 0.2, Created: time.Now()},
		}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	repo := NewModerationRepository(mockDB, "test-table", zap.NewNop())
	stats, err := repo.GetReviewerStats(ctx, "reviewer-1")
	require.NoError(t, err)
	require.Equal(t, 2, stats.TotalReviews)
	require.Equal(t, 1, stats.AccurateReviews)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN1_GetFiltersForUser_PageCappedWalk(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "USER#alice").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", ">=", "FILTER#").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "<", "FILTER~").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.Filter)
		*dest = []models.Filter{{ID: "f1", Username: "alice"}}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	repo := NewModerationRepository(mockDB, "test-table", zap.NewNop())
	filters, err := repo.GetFiltersForUser(ctx, "alice")
	require.NoError(t, err)
	require.Len(t, filters, 1)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN1_GetFilterKeywords_PageCappedWalk(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "FILTER#f1").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", ">=", "KEYWORD#").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "<", "KEYWORD~").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.FilterKeyword)
		*dest = []models.FilterKeyword{{ID: "k1"}}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	repo := NewModerationRepository(mockDB, "test-table", zap.NewNop())
	keywords, err := repo.GetFilterKeywords(ctx, "f1")
	require.NoError(t, err)
	require.Len(t, keywords, 1)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN1_GetFilterStatuses_PageCappedWalk(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "FILTER#f1").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", ">=", "STATUS#").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "<", "STATUS~").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.FilterStatus)
		*dest = []models.FilterStatus{{ID: "s1"}}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	repo := NewModerationRepository(mockDB, "test-table", zap.NewNop())
	statuses, err := repo.GetFilterStatuses(ctx, "f1")
	require.NoError(t, err)
	require.Len(t, statuses, 1)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN1_GetPendingModerationCount_PageCappedWalks(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	// All three branches (open reports, in-progress reports, flags) must walk
	// in bounded pages: Limit(500) per page, AllPaginated reads.
	mockQuery.On("Index", "gsi4").Return(mockQuery).Times(2)
	mockQuery.On("Where", "gsi4PK", "=", "ASSIGNED#mod-1").Return(mockQuery).Times(2)
	mockQuery.On("Where", "gsi4SK", "begins_with", string(storage.ReportStatusOpen)+"#REPORT#").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi4SK", "begins_with", string(storage.ReportStatusInProgress)+"#REPORT#").Return(mockQuery).Once()
	mockQuery.On("Index", "gsi2").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi2PK", "=", "FLAG_STATUS#pending").Return(mockQuery).Once()
	mockQuery.On("Filter", "AssignedTo", "=", "mod-1").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Times(3)
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.Report")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.Report)
		*dest = []models.Report{{ID: "r1"}}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Times(2)
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.Flag")).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	repo := NewModerationRepository(mockDB, "test-table", zap.NewNop())
	count, err := repo.GetPendingModerationCount(ctx, "mod-1")
	require.NoError(t, err)
	require.Equal(t, 2, count)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN1_GetPendingModerationCount_CapFailsClosed(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	// Open-reports branch exhausts the page cap: the count must fail closed
	// instead of silently truncating.
	mockQuery.On("Index", "gsi4").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi4PK", "=", "ASSIGNED#mod-1").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi4SK", "begins_with", string(storage.ReportStatusOpen)+"#REPORT#").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	full := func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.Report)
		*dest = make([]models.Report, 500)
	}
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.Report")).Run(full).Return(&core.PaginatedResult{HasMore: true, NextCursor: "c"}, nil).Times(100)
	mockQuery.On("Cursor", "c").Return(mockQuery).Times(99)

	repo := NewModerationRepository(mockDB, "test-table", zap.NewNop())
	_, err := repo.GetPendingModerationCount(ctx, "mod-1")
	require.Error(t, err)
	require.ErrorIs(t, err, errBoundedPageCapExceeded)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN1_GetDecisionHistory_PageCappedWalk(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "DECISION_RESULT#content-1").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "prefix", "TIME#").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.ModerationDecisionResult)
		*dest = []models.ModerationDecisionResult{{ID: "d1"}}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	repo := NewModerationRepository(mockDB, "test-table", zap.NewNop())
	decisions, err := repo.GetDecisionHistory(ctx, "content-1")
	require.NoError(t, err)
	require.Len(t, decisions, 1)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN1_GetModerationDecisionsByModerator_ClampedLimit(t *testing.T) {
	ctx := context.Background()

	t.Run("limit zero clamps to default 100", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", ctx).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
		mockQuery.On("Where", "gsi1PK", "=", "REVIEWER#mod").Return(mockQuery).Once()
		// The old `if limit > 0` gate skipped Limit entirely at 0 — the clamp
		// must always issue the bounded read.
		mockQuery.On("Limit", 100).Return(mockQuery).Once()
		mockQuery.On("All", mock.Anything).Return(nil).Once()

		repo := NewModerationRepository(mockDB, "test-table", zap.NewNop())
		_, err := repo.GetModerationDecisionsByModerator(ctx, "mod", 0)
		require.NoError(t, err)
		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("oversized limit clamps to hard max 500", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", ctx).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
		mockQuery.On("Where", "gsi1PK", "=", "REVIEWER#mod").Return(mockQuery).Once()
		mockQuery.On("Limit", 500).Return(mockQuery).Once()
		mockQuery.On("All", mock.Anything).Return(nil).Once()

		repo := NewModerationRepository(mockDB, "test-table", zap.NewNop())
		_, err := repo.GetModerationDecisionsByModerator(ctx, "mod", 1000)
		require.NoError(t, err)
		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})
}

func TestBatchN1_GetModerationPatterns_WalkUpToLimit(t *testing.T) {
	ctx := context.Background()

	t.Run("engine-style full set (1000) walks two bounded pages", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", ctx).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
		mockQuery.On("Where", "gsi1PK", "=", "MODERATION_PATTERNS#ACTIVE").Return(mockQuery).Once()
		mockQuery.On("Limit", 500).Return(mockQuery).Once()
		mockQuery.On("AllPaginated", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.ModerationPattern)
			*dest = make([]models.ModerationPattern, 500)
		}).Return(&core.PaginatedResult{HasMore: true, NextCursor: "c1"}, nil).Once()
		mockQuery.On("Cursor", "c1").Return(mockQuery).Once()
		mockQuery.On("AllPaginated", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.ModerationPattern)
			*dest = make([]models.ModerationPattern, 500)
		}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

		repo := NewModerationRepository(mockDB, "test-table", zap.NewNop())
		patterns, err := repo.GetModerationPatterns(ctx, true, "", 1000)
		require.NoError(t, err)
		require.Len(t, patterns, 1000)
		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("small limit keeps a single page of that size", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", ctx).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Index", "gsi3").Return(mockQuery).Once()
		mockQuery.On("Where", "gsi3PK", "=", "MODERATION_PATTERNS#ALL").Return(mockQuery).Once()
		mockQuery.On("OrderBy", "gsi3SK", "ASC").Return(mockQuery).Once()
		mockQuery.On("Limit", 10).Return(mockQuery).Once()
		mockQuery.On("AllPaginated", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.ModerationPattern)
			*dest = make([]models.ModerationPattern, 10)
		}).Return(&core.PaginatedResult{HasMore: true, NextCursor: "c"}, nil).Once()

		repo := NewModerationRepository(mockDB, "test-table", zap.NewNop())
		patterns, err := repo.GetModerationPatterns(ctx, false, "", 10)
		require.NoError(t, err)
		require.Len(t, patterns, 10)
		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})
}

func TestBatchN1_GetReviewQueue_ClampedLimit(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "REVIEW_QUEUE#pending").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "prefix", "PRIORITY#").Return(mockQuery).Once()
	mockQuery.On("Limit", 100).Return(mockQuery).Once()
	mockQuery.On("All", mock.Anything).Return(nil).Once()

	repo := NewModerationRepository(mockDB, "test-table", zap.NewNop())
	_, err := repo.GetReviewQueue(ctx, map[string]interface{}{"status": "pending", "limit": 1000})
	require.NoError(t, err)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// ===== status_repository.go =====

func TestBatchN1_GetStatusByURL_PageCappedWalk(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Index", "gsi7").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi7PK", "=", "URL#https://example.com/status/1").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	// First page has no exact match; the walk must continue with the cursor.
	mockQuery.On("AllPaginated", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.Status)
		*dest = []models.Status{{StatusID: "other", URLs: []string{"https://example.com/other"}}}
	}).Return(&core.PaginatedResult{HasMore: true, NextCursor: "c1"}, nil).Once()
	mockQuery.On("Cursor", "c1").Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.Status)
		*dest = []models.Status{{StatusID: "hit", URLs: []string{"https://example.com/status/1"}}}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	repo := NewStatusRepository(mockDB, "test-table", zap.NewNop(), nil)
	found, err := repo.GetStatusByURL(ctx, "https://example.com/status/1")
	require.NoError(t, err)
	require.NotNil(t, found)
	require.Equal(t, "hit", found.StatusID)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN1_GetStatusByURL_CrossPagePriority(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Index", "gsi7").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi7PK", "=", "URL#https://example.com/status/1").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	// Page 1 holds only a loop-2 (URLs membership) match; page 2 holds the
	// loop-1 (Note.ID == url) canonical match. Selection priority is
	// partition-wide, so the walk must collect both pages and return the
	// page-2 loop-1 match — per-page priority (loop 1 then loop 2 within each
	// page, stopping at the first hit) would return the page-1 quoter.
	mockQuery.On("AllPaginated", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.Status)
		*dest = []models.Status{{StatusID: "quoter", URLs: []string{"https://example.com/status/1"}}}
	}).Return(&core.PaginatedResult{HasMore: true, NextCursor: "c1"}, nil).Once()
	mockQuery.On("Cursor", "c1").Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.Status)
		*dest = []models.Status{{StatusID: "canonical", Note: &activitypub.Note{BaseObject: activitypub.BaseObject{ID: "https://example.com/status/1"}}}}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	repo := NewStatusRepository(mockDB, "test-table", zap.NewNop(), nil)
	found, err := repo.GetStatusByURL(ctx, "https://example.com/status/1")
	require.NoError(t, err)
	require.NotNil(t, found)
	require.Equal(t, "canonical", found.StatusID)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN1_GetStatusByURL_NotFound(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Index", "gsi7").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi7PK", "=", "URL#https://example.com/status/1").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.Anything).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	repo := NewStatusRepository(mockDB, "test-table", zap.NewNop(), nil)
	_, err := repo.GetStatusByURL(ctx, "https://example.com/status/1")
	require.ErrorIs(t, err, storage.ErrNotFound)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN1_RemoveEngagement_PageCappedWalk(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockUpdateBuilder := new(mocks.MockUpdateBuilder)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	// The engagement find must be a bounded page walk over the keyed type
	// prefix (like#) with the user filter preserved.
	mockQuery.On("Where", "PK", "=", "STATUS_ENGAGEMENT#s1").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "BEGINS_WITH", "like#").Return(mockQuery).Once()
	mockQuery.On("Filter", "UserID", "=", "user1").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.StatusEngagement)
		*dest = []models.StatusEngagement{{PK: "STATUS_ENGAGEMENT#s1", SK: "like#1#user1", UserID: "user1"}}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()
	mockQuery.On("Delete").Return(nil).Once()
	// Counter decrement (unchanged shape).
	mockQuery.On("Where", "PK", "=", "status#s1").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "=", "status#s1").Return(mockQuery).Once()
	mockQuery.On("UpdateBuilder").Return(mockUpdateBuilder).Once()
	mockUpdateBuilder.On("Add", "LikeCount", -1).Return(mockUpdateBuilder).Once()
	mockUpdateBuilder.On("Condition", "LikeCount", ">", 0).Return(mockUpdateBuilder).Once()
	mockUpdateBuilder.On("Execute").Return(nil).Once()

	repo := NewStatusRepository(mockDB, "test-table", zap.NewNop(), nil)
	require.NoError(t, repo.UnlikeStatus(ctx, "user1", "s1"))
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
	mockUpdateBuilder.AssertExpectations(t)
}

func TestBatchN1_GetStatusEngagement_PageCappedWalks(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	bookmarkRepo := NewBookmarkRepository(mockDB, "test-table", zap.NewNop())
	bookmarkRepo.getObjectBookmarkFn = func(_ context.Context, _, _ string) (*models.Bookmark, error) {
		return &models.Bookmark{Username: "user1", ObjectID: "s1"}, nil
	}

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	// Like existence: bounded walk finds a row -> liked=true.
	mockQuery.On("Where", "PK", "=", "STATUS_ENGAGEMENT#s1").Return(mockQuery).Times(2)
	mockQuery.On("Where", "SK", "BEGINS_WITH", "like#").Return(mockQuery).Once()
	mockQuery.On("Filter", "UserID", "=", "user1").Return(mockQuery).Times(2)
	mockQuery.On("Limit", 500).Return(mockQuery).Times(2)
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.StatusEngagement")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.StatusEngagement)
		*dest = []models.StatusEngagement{{SK: "like#1#user1"}}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()
	// Boost existence: bounded walk finds nothing -> reblogged=false.
	mockQuery.On("Where", "SK", "BEGINS_WITH", "boost#").Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.StatusEngagement")).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	repo := NewStatusRepository(mockDB, "test-table", zap.NewNop(), nil)
	repo.SetBookmarkRepository(bookmarkRepo)
	liked, reblogged, bookmarked, err := repo.GetStatusEngagement(ctx, "s1", "user1")
	require.NoError(t, err)
	require.True(t, liked)
	require.False(t, reblogged)
	require.True(t, bookmarked)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN1_GetPublicTimeline_ClampedLimit(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Index", "gsi2").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi2PK", "=", "PUBLIC_TIMELINE").Return(mockQuery).Once()
	mockQuery.On("OrderBy", "gsi2SK", "DESC").Return(mockQuery).Once()
	mockQuery.On("Limit", 100).Return(mockQuery).Once()
	mockQuery.On("All", mock.Anything).Return(nil).Once()

	repo := NewStatusRepository(mockDB, "test-table", zap.NewNop(), nil)
	page, err := repo.GetPublicTimeline(ctx, interfaces.PaginationOptions{Limit: 500})
	require.NoError(t, err)
	require.False(t, page.HasMore)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN1_GetReplies_ClampedLimit(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Index", "gsi4").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi4PK", "=", "REPLIES#parent").Return(mockQuery).Once()
	mockQuery.On("OrderBy", "gsi4SK", "ASC").Return(mockQuery).Once()
	// Limit 0 must clamp to the 20 default instead of an unbounded read.
	mockQuery.On("Limit", 20).Return(mockQuery).Once()
	mockQuery.On("All", mock.Anything).Return(nil).Once()

	repo := NewStatusRepository(mockDB, "test-table", zap.NewNop(), nil)
	page, err := repo.GetReplies(ctx, "parent", interfaces.PaginationOptions{Limit: 0})
	require.NoError(t, err)
	require.False(t, page.HasMore)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// ===== query_utils.go =====

func TestBatchN1_QueryUtils_ConditionalLimitBuildersClamped(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name  string
		call  func(q *QueryUtils)
		limit int
	}{
		{"UserRelationshipQuery", func(q *QueryUtils) {
			_, _ = q.UserRelationshipQuery(ctx, "alice", "FOLLOWING", &QueryOptions{Limit: 0})
		}, 51}, // 50 default + 1 sentinel
		{"TimeRangeQuery", func(q *QueryUtils) {
			_, _ = q.TimeRangeQuery(ctx, "pk", 0, 0, &QueryOptions{Limit: 0})
		}, 51},
		{"GSIStatusQuery", func(q *QueryUtils) {
			_, _ = q.GSIStatusQuery(ctx, "gsi1", "user", &QueryOptions{Limit: 0})
		}, 51},
		{"QueryByGSI", func(q *QueryUtils) {
			_, _ = q.QueryByGSI(ctx, "gsi1", "pk", "", &QueryOptions{Limit: 0})
		}, 51},
		{"QueryWithPrefix", func(q *QueryUtils) {
			_, _ = q.QueryWithPrefix(ctx, "pk", "pref#", &QueryOptions{Limit: 0})
		}, 51},
		{"GenericList", func(q *QueryUtils) {
			_, _ = GenericList[map[string]interface{}](ctx, q, "pk", "pref#", &QueryOptions{Limit: 0})
		}, 51},
		{"QueryBuilder", func(q *QueryUtils) {
			_, _ = NewQueryBuilder[map[string]interface{}](ctx, q).WithPK("pk").WithSKPrefix("pref#").WithLimit(0).Execute()
		}, 51},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)
			q := NewQueryUtils(mockDB, zap.NewNop())

			mockDB.On("WithContext", ctx).Return(mockDB)
			mockDB.On("Model", mock.Anything).Return(mockQuery)
			mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
			mockQuery.On("Index", mock.Anything).Return(mockQuery).Maybe()
			// The old `if opts.Limit > 0` gate skipped Limit entirely at 0;
			// the clamp must always issue the bounded read (default 50 + 1).
			mockQuery.On("Limit", tt.limit).Return(mockQuery).Once()
			mockQuery.On("All", mock.Anything).Return(nil).Once()

			tt.call(q)
			mockDB.AssertExpectations(t)
			mockQuery.AssertExpectations(t)
		})
	}
}

func TestBatchN1_QueryUtils_OversizedLimitClamped(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	q := NewQueryUtils(mockDB, zap.NewNop())

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	// Limit 10_000 must clamp to the 100 hard max (+1 sentinel).
	mockQuery.On("Limit", 101).Return(mockQuery).Once()
	mockQuery.On("All", mock.Anything).Return(nil).Once()

	_, err := q.QueryWithPrefix(ctx, "pk", "pref#", &QueryOptions{Limit: 10000})
	require.NoError(t, err)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN1_QueryWithPKAndSKPrefix_PageCappedWalk(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	q := NewQueryUtils(mockDB, zap.NewNop())

	type m struct{ PK, SK string }
	convert := func(in m) string { return in.PK + "#" + in.SK }

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "pk").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "BEGINS_WITH", "pref#").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]m)
		*dest = []m{{PK: "pk", SK: "pref#1"}}
	}).Return(&core.PaginatedResult{HasMore: true, NextCursor: "c1"}, nil).Once()
	mockQuery.On("Cursor", "c1").Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]m)
		*dest = []m{{PK: "pk", SK: "pref#2"}}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	out, err := QueryWithPKAndSKPrefix[m, string](ctx, q, func() *m { return &m{} }, "pk", "pref#", false, convert, "op", "param")
	require.NoError(t, err)
	require.Equal(t, []string{"pk#pref#1", "pk#pref#2"}, out)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN1_QueryWithPKAndSKPrefix_PageCapExhaustionFailsClosed(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	q := NewQueryUtils(mockDB, zap.NewNop())

	type m struct{ PK, SK string }

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "pk").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "BEGINS_WITH", "pref#").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	full := func(args mock.Arguments) {
		dest := args.Get(0).(*[]m)
		*dest = make([]m, 500)
	}
	mockQuery.On("AllPaginated", mock.Anything).Run(full).Return(&core.PaginatedResult{HasMore: true, NextCursor: "c"}, nil).Times(100)
	mockQuery.On("Cursor", "c").Return(mockQuery).Times(99)

	_, err := QueryWithPKAndSKPrefix[m, string](ctx, q, func() *m { return &m{} }, "pk", "pref#", false, func(in m) string { return in.PK }, "op", "param")
	require.Error(t, err)
	require.ErrorIs(t, err, errBoundedPageCapExceeded)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// ===== walkKeyedPages unit-level page-cap pinning =====

func TestWalkKeyedPages_PageCapExactCount(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	full := func(args mock.Arguments) {
		dest := args.Get(0).(*[]int)
		*dest = make([]int, 500)
	}
	// Exactly 100 full pages with more available: iteration must fail closed
	// at the 100-page cap. Any `>` vs `>=` off-by-one changes the call count
	// and fails this test (either a 101st read or an unfulfilled expectation).
	mockQuery.On("AllPaginated", mock.Anything).Run(full).Return(&core.PaginatedResult{HasMore: true, NextCursor: "c"}, nil).Times(100)
	mockQuery.On("Cursor", "c").Return(mockQuery).Times(99)

	var collected int
	err := walkKeyedPages(
		mockDB.WithContext(ctx).Model(&models.ModerationReview{}),
		500, 100,
		func(page []int) (bool, error) {
			collected += len(page)
			return false, nil
		},
	)
	require.ErrorIs(t, err, errBoundedPageCapExceeded)
	require.Equal(t, 50000, collected)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestWalkKeyedPages_StopsOnShortPage(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]int)
		*dest = []int{1}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	var collected []int
	err := walkKeyedPages(
		mockDB.WithContext(ctx).Model(&models.ModerationReview{}),
		500, 100,
		func(page []int) (bool, error) {
			collected = append(collected, page...)
			return false, nil
		},
	)
	require.NoError(t, err)
	require.Equal(t, []int{1}, collected)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}
