package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

// Batch N3 (umbrella #1469, 2026-08-27) — the final keyed-read batch.
//
// Workstream 1 (search / announcements / cost-metrics plane): page-capped
// walks for keyed whole-partition `.All()` reads, floor/clamp fixes for the
// degenerate-input class (`Limit(n)` with n <= 0 compiles to NO limit in
// tabletheory v3.0.6), and sentinel-routing splits where a pre-existing
// swallow would otherwise route errBoundedPageCapExceeded into a silent
// empty/partial result.
//
// Workstream 2 (keyed `.Count()` class): tabletheory Count() strips Limit
// (query_execution.go:95), so every keyed Count is now a bounded page walk on
// the CountObjectReplies pattern.
//
// Every assertion pins a LITERAL (Limit(500), Limit(20)/Limit(100) clamp
// values, "c1" cursor strings, exact page counts) so that removing a bound or
// breaking a cap comparison kills the test.

// ===== Workstream 2 — keyed Count() → walk-based counts =====

func TestBatchN3_CountCollectionItems_PageCappedWalk(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.CollectionItem")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "COLLECTION#alice").Return(mockQuery).Once()
	// Two bounded pages with a cursor handoff; reverting to a keyed Count()
	// leaves Limit/AllPaginated/Cursor unfulfilled and dies.
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.CollectionItem")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.CollectionItem)
		*dest = []models.CollectionItem{{ItemID: "i1"}, {ItemID: "i2"}}
	}).Return(&core.PaginatedResult{HasMore: true, NextCursor: "c1"}, nil).Once()
	mockQuery.On("Cursor", "c1").Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.CollectionItem")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.CollectionItem)
		*dest = []models.CollectionItem{{ItemID: "i3"}}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())
	count, err := repo.CountCollectionItems(ctx, "alice")
	require.NoError(t, err)
	require.Equal(t, 3, count)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN3_CountCollectionItems_PageCapExhaustionFailsClosed(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.CollectionItem")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "COLLECTION#alice").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	full := func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.CollectionItem)
		*dest = make([]models.CollectionItem, 500)
	}
	// Exactly 100 full pages with more available: the walk must fail closed at
	// the 100-page cap. A `>` vs `>=` off-by-one changes the call count.
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.CollectionItem")).Run(full).Return(&core.PaginatedResult{HasMore: true, NextCursor: "c"}, nil).Times(100)
	mockQuery.On("Cursor", "c").Return(mockQuery).Times(99)

	repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())
	_, err := repo.CountCollectionItems(ctx, "alice")
	require.Error(t, err)
	require.ErrorIs(t, err, errBoundedPageCapExceeded)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN3_CountQuotes_PageCappedWalk(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.QuoteRelationship")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1PK", "=", "QUOTED#note-1").Return(mockQuery).Once()
	mockWalkExpectations(t, mockQuery, 500, []core.PaginatedResult{
		{HasMore: true, NextCursor: "c1"},
		{HasMore: false},
	})

	repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())
	count, err := repo.CountQuotes(ctx, "note-1")
	require.NoError(t, err)
	require.Zero(t, count)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN3_CountReplies_PageCappedWalk(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.Object")).Return(mockQuery)
	mockQuery.On("Index", "gsi6").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi6PK", "=", "REPLIES#https://example.com/objects/status-1").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.Object")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.Object)
		*dest = []models.Object{{ID: "r1"}, {ID: "r2"}, {ID: "r3"}}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())
	count, err := repo.CountReplies(ctx, "status-1")
	require.NoError(t, err)
	require.Equal(t, 3, count)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN3_GetUserStatusCount_PageCappedWalk(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.Object")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1PK", "=", "actor#user-1").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.Object")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.Object)
		*dest = []models.Object{{ID: "s1"}}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())
	count, err := repo.GetUserStatusCount(ctx, "user-1")
	require.NoError(t, err)
	require.Equal(t, 1, count)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN3_GetTotalUserCount_PageCappedWalk(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.User")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1PK", "=", "USERS").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.User")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.User)
		*dest = []models.User{{Username: "u1"}, {Username: "u2"}}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())
	count, err := repo.GetTotalUserCount(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(2), count)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN3_GetTotalUserCount_PageCapExhaustionFailsClosed(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.User")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1PK", "=", "USERS").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	full := func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.User)
		*dest = make([]models.User, 500)
	}
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.User")).Run(full).Return(&core.PaginatedResult{HasMore: true, NextCursor: "c"}, nil).Times(100)
	mockQuery.On("Cursor", "c").Return(mockQuery).Times(99)

	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())
	_, err := repo.GetTotalUserCount(ctx)
	require.Error(t, err)
	require.ErrorIs(t, err, errBoundedPageCapExceeded)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN3_GetUnreadNotificationCount_PageCappedWalk(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.Notification")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "USER#alice").Return(mockQuery).Once()
	// The IsRead=false filter stays on the chain and applies per page.
	mockQuery.On("Filter", "IsRead", "=", false).Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.Notification")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.Notification)
		*dest = []models.Notification{{ID: "n1"}, {ID: "n2"}}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	repo := NewNotificationRepository(mockDB, "test-table", zap.NewNop(), nil)
	count, err := repo.GetUnreadNotificationCount(ctx, "alice")
	require.NoError(t, err)
	require.Equal(t, int64(2), count)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN3_CountQuery_PageCappedWalk(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*map[string]interface {}")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "FOLLOW#alice").Return(mockQuery).Once()
	mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]map[string]interface {}")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]map[string]interface{})
		*dest = []map[string]interface{}{{"PK": "x"}, {"PK": "y"}}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	qu := NewQueryUtils(mockDB, zap.NewNop())
	count, err := qu.CountQuery(ctx, "FOLLOW#alice", "gsi1")
	require.NoError(t, err)
	require.Equal(t, 2, count)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// ===== Workstream 1 — announcement_repository.go walks =====

func TestBatchN3_DeleteAnnouncement_WalksCleanupPartitions(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	baseTime := time.Date(2025, 12, 28, 1, 2, 3, 0, time.UTC)

	// Base delete succeeds (permissive mocks cover the r.Delete flow).
	mockQuery.On("Delete").Return(nil).Once()

	// Reactions cleanup: bounded page walk returns one reaction to delete.
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]*models.AnnouncementReaction")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]*models.AnnouncementReaction)
		*out = []*models.AnnouncementReaction{{Username: "u", AnnouncementID: "a1", EmojiName: "thumbsup"}}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	// Dismissals cleanup: bounded page walk returns one dismissal to delete.
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]*models.AnnouncementDismissal")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]*models.AnnouncementDismissal)
		*out = []*models.AnnouncementDismissal{{Username: "u", AnnouncementID: "a1"}}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	// The two deletes for the walked rows.
	mockQuery.On("Delete").Return(nil).Twice()

	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
	repo := NewAnnouncementRepository(mockDB, "test-table", zap.NewNop())
	require.NoError(t, repo.DeleteAnnouncement(ctx, "a1"))
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN3_DeleteAnnouncement_CapExhaustionFailsClosed(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	baseTime := time.Date(2025, 12, 28, 1, 2, 3, 0, time.UTC)

	mockQuery.On("Delete").Return(nil).Once()

	// Reactions walk caps out: the delete must FAIL CLOSED (a bare
	// warn-and-continue swallow would return nil here — the test dies on
	// require.Error + ErrorIs).
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	full := func(args mock.Arguments) {
		out := args.Get(0).(*[]*models.AnnouncementReaction)
		*out = make([]*models.AnnouncementReaction, 500)
	}
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]*models.AnnouncementReaction")).Run(full).Return(&core.PaginatedResult{HasMore: true, NextCursor: "c"}, nil).Times(100)
	mockQuery.On("Cursor", "c").Return(mockQuery).Times(99)

	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
	repo := NewAnnouncementRepository(mockDB, "test-table", zap.NewNop())
	err := repo.DeleteAnnouncement(ctx, "a1")
	require.Error(t, err)
	require.ErrorIs(t, err, errBoundedPageCapExceeded)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN3_GetDismissedAnnouncements_PageCappedWalk(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.AnnouncementDismissal")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "USER#alice").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "begins_with", "ANNOUNCEMENT_DISMISSED#").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]*models.AnnouncementDismissal")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]*models.AnnouncementDismissal)
		*out = []*models.AnnouncementDismissal{{Username: "alice", AnnouncementID: "a1"}}
	}).Return(&core.PaginatedResult{HasMore: true, NextCursor: "c1"}, nil).Once()
	mockQuery.On("Cursor", "c1").Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]*models.AnnouncementDismissal")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]*models.AnnouncementDismissal)
		*out = []*models.AnnouncementDismissal{{Username: "alice", AnnouncementID: "a2"}}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	repo := NewAnnouncementRepository(mockDB, "test-table", zap.NewNop())
	ids, err := repo.GetDismissedAnnouncements(ctx, "alice")
	require.NoError(t, err)
	require.Equal(t, []string{"a1", "a2"}, ids)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN3_GetAnnouncementReactions_PageCappedWalk(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.AnnouncementReaction")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "ANNOUNCEMENT_REACTION#a1").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]*models.AnnouncementReaction")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]*models.AnnouncementReaction)
		*out = []*models.AnnouncementReaction{
			{Username: "u1", AnnouncementID: "a1", EmojiName: "thumbsup"},
			{Username: "u2", AnnouncementID: "a1", EmojiName: "thumbsup"},
			{Username: "u3", AnnouncementID: "a1", EmojiName: "heart"},
		}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	repo := NewAnnouncementRepository(mockDB, "test-table", zap.NewNop())
	reactions, err := repo.GetAnnouncementReactions(ctx, "a1")
	require.NoError(t, err)
	require.Len(t, reactions["thumbsup"], 2)
	require.Len(t, reactions["heart"], 1)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// ===== Workstream 1 — search_repository.go =====

func TestBatchN3_GetSearchAnalytics_PerDayWalk_SentinelRoutesThroughSwallow(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.SearchAnalytics")).Return(mockQuery)
	// Day 1 walk caps out: the pre-existing warn-and-skip swallow must NOT
	// route cap exhaustion into a partial analytics set — the error propagates.
	mockQuery.On("Where", "PK", "=", "SEARCH_LOG#2026-08-26").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	full := func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.SearchAnalytics)
		*dest = make([]models.SearchAnalytics, 500)
	}
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.SearchAnalytics")).Run(full).Return(&core.PaginatedResult{HasMore: true, NextCursor: "c"}, nil).Times(100)
	mockQuery.On("Cursor", "c").Return(mockQuery).Times(99)

	repo := NewSearchRepository(mockDB, "test-table", zap.NewNop(), nil)
	_, err := repo.GetSearchAnalytics(ctx, time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC), time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC))
	require.Error(t, err)
	require.ErrorIs(t, err, errBoundedPageCapExceeded)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN3_GetSearchAnalytics_TransientErrorSkipsDay(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.SearchAnalytics")).Return(mockQuery)
	// Day 1: transient (non-cap) error keeps the skip-this-day behavior.
	mockQuery.On("Where", "PK", "=", "SEARCH_LOG#2026-08-26").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.SearchAnalytics")).Return(nil, errors.New("transient")).Once()
	// Day 2: a page of analytics is collected.
	mockQuery.On("Where", "PK", "=", "SEARCH_LOG#2026-08-27").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.SearchAnalytics")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.SearchAnalytics)
		*dest = []models.SearchAnalytics{{Query: "q1", Timestamp: time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)}}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	repo := NewSearchRepository(mockDB, "test-table", zap.NewNop(), nil)
	analytics, err := repo.GetSearchAnalytics(ctx, time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC), time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.Len(t, analytics, 1)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN3_SearchHashtags_LimitClamped(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name          string
		limit         int
		expectedLimit int // the compiled Limit literal
	}{
		{"limit zero takes default 20", 0, 20},
		{"negative limit takes default 20", -1, 20},
		{"over max clamps to 100", 500, 100},
		{"in-range limit passes through", 10, 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)

			mockDB.On("WithContext", ctx).Return(mockDB)
			mockDB.On("Model", mock.AnythingOfType("*models.Hashtag")).Return(mockQuery)
			mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
			mockQuery.On("Where", "gsi1PK", "=", "HASHTAG").Return(mockQuery).Once()
			mockQuery.On("Where", "gsi1SK", "BEGINS_WITH", "go").Return(mockQuery).Once()
			// The clamp must always issue Limit(<sanitized>) — reverting it
			// issues Limit(0) (or none), which this literal pin rejects.
			mockQuery.On("Limit", tt.expectedLimit).Return(mockQuery).Once()
			mockQuery.On("All", mock.AnythingOfType("*[]models.Hashtag")).Return(nil).Once()

			repo := NewSearchRepository(mockDB, "test-table", zap.NewNop(), nil)
			results, err := repo.SearchHashtags(ctx, "#go", tt.limit)
			require.NoError(t, err)
			require.Empty(t, results)
			mockDB.AssertExpectations(t)
			mockQuery.AssertExpectations(t)
		})
	}
}

func TestBatchN3_SearchStatusesByHashtag_LimitClamped(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name          string
		limit         int
		expectedLimit int
	}{
		{"limit zero takes default 20", 0, 20},
		{"negative limit takes default 20", -2, 20},
		{"over max clamps to 100", 300, 100},
		{"in-range limit passes through", 5, 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)

			mockDB.On("WithContext", ctx).Return(mockDB)
			mockDB.On("Model", mock.AnythingOfType("*models.HashtagStatusIndex")).Return(mockQuery)
			mockQuery.On("Where", "PK", "=", "HASHTAG_TIMELINE#golang").Return(mockQuery).Once()
			mockQuery.On("Where", "SK", "BEGINS_WITH", "STATUS#").Return(mockQuery).Once()
			mockQuery.On("OrderBy", "SK", "DESC").Return(mockQuery).Once()
			mockQuery.On("Limit", tt.expectedLimit).Return(mockQuery).Once()
			mockQuery.On("All", mock.AnythingOfType("*[]models.HashtagStatusIndex")).Return(nil).Once()

			repo := NewSearchRepository(mockDB, "test-table", zap.NewNop(), nil)
			results, err := repo.SearchStatusesByHashtag(ctx, "golang", tt.limit)
			require.NoError(t, err)
			require.Empty(t, results)
			mockDB.AssertExpectations(t)
			mockQuery.AssertExpectations(t)
		})
	}
}

func TestBatchN3_SearchStatusesByAuthor_LimitClamped(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name          string
		limit         int
		expectedLimit int
	}{
		{"limit zero takes default 20", 0, 20},
		{"negative limit takes default 20", -1, 20},
		{"over max clamps to 100", 200, 100},
		{"in-range limit passes through", 7, 7},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)

			mockDB.On("WithContext", ctx).Return(mockDB)
			mockDB.On("Model", mock.AnythingOfType("*models.Status")).Return(mockQuery)
			mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
			mockQuery.On("Where", "gsi1PK", "=", "AUTHOR#alice").Return(mockQuery).Once()
			mockQuery.On("OrderBy", "gsi1SK", "DESC").Return(mockQuery).Once()
			mockQuery.On("Limit", tt.expectedLimit).Return(mockQuery).Once()
			mockQuery.On("All", mock.AnythingOfType("*[]models.Status")).Return(nil).Once()

			repo := NewSearchRepository(mockDB, "test-table", zap.NewNop(), nil)
			results, err := repo.SearchStatusesByAuthor(ctx, "alice", tt.limit)
			require.NoError(t, err)
			require.Empty(t, results)
			mockDB.AssertExpectations(t)
			mockQuery.AssertExpectations(t)
		})
	}
}

func TestBatchN3_GetSearchSuggestions_LimitClamped(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name          string
		limit         int
		expectedLimit int
	}{
		{"limit zero takes default 20", 0, 20},
		{"negative limit takes default 20", -3, 20},
		{"over max clamps to 100", 250, 100},
		{"in-range limit passes through", 4, 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)

			mockDB.On("WithContext", ctx).Return(mockDB)
			mockDB.On("Model", mock.AnythingOfType("*models.SearchSuggestion")).Return(mockQuery)
			// Three suggestion partitions are queried (username/hashtag/display_name).
			mockQuery.On("Where", "PK", "=", mock.AnythingOfType("string")).Return(mockQuery).Times(3)
			mockQuery.On("Filter", "SK", "BEGINS_WITH", "al").Return(mockQuery).Times(3)
			mockQuery.On("Limit", tt.expectedLimit).Return(mockQuery).Times(3)
			mockQuery.On("All", mock.AnythingOfType("*[]models.SearchSuggestion")).Return(nil).Times(3)

			repo := NewSearchRepository(mockDB, "test-table", zap.NewNop(), nil)
			results, err := repo.GetSearchSuggestions(ctx, "al", tt.limit)
			require.NoError(t, err)
			require.Empty(t, results)
			mockDB.AssertExpectations(t)
			mockQuery.AssertExpectations(t)
		})
	}
}

func TestBatchN3_SearchUsernamePrefix_FetchWindowFloored(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.Actor")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1PK", "=", "USERNAME_SEARCH#al").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1SK", "BEGINS_WITH", "alice").Return(mockQuery).Once()
	// limit=0 + offset=0 previously compiled Limit(0) — no limit. The floor
	// must issue Limit(20) (the searchDefaultLimit literal).
	mockQuery.On("Limit", 20).Return(mockQuery).Once()
	mockQuery.On("All", mock.AnythingOfType("*[]models.Actor")).Return(nil).Once()

	repo := NewSearchRepository(mockDB, "test-table", zap.NewNop(), nil)
	results := make([]*activitypub.Actor, 0)
	seen := map[string]bool{}
	stats, err := repo.searchUsernamePrefix(ctx, "alice", 0, 0, &results, seen)
	require.NoError(t, err)
	require.NotNil(t, stats)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN3_SearchDisplayName_LimitFloored(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.Actor")).Return(mockQuery)
	mockQuery.On("Index", "gsi2").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi2PK", "=", "NAME_SEARCH#al").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi2SK", "BEGINS_WITH", "alice").Return(mockQuery).Once()
	// limit <= 0 previously compiled Limit(0) — no limit; the floor issues
	// Limit(20).
	mockQuery.On("Limit", 20).Return(mockQuery).Once()
	mockQuery.On("All", mock.AnythingOfType("*[]models.Actor")).Return(nil).Once()

	repo := NewSearchRepository(mockDB, "test-table", zap.NewNop(), nil)
	results := make([]*activitypub.Actor, 0)
	seen := map[string]bool{}
	stats, err := repo.searchDisplayName(ctx, "alice", -1, &results, seen)
	require.NoError(t, err)
	require.NotNil(t, stats)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// ===== Workstream 1 — media_analytics / metrics walks =====

func TestBatchN3_GetMediaAnalyticsByDate_PageCappedWalk(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.MediaAnalytics")).Return(mockQuery)
	mockQuery.On("Where", "gsi1PK", "=", "DATE#2026-08-27").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]*models.MediaAnalytics")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.MediaAnalytics)
		*dest = []*models.MediaAnalytics{{MediaID: "m1"}, {MediaID: "m2"}}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	repo := NewMediaAnalyticsRepository(mockDB, "test-table", zap.NewNop(), nil)
	list, err := repo.GetMediaAnalyticsByDate(ctx, "2026-08-27")
	require.NoError(t, err)
	require.Len(t, list, 2)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN3_GetMediaAnalyticsByVariant_PageCappedWalk(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.MediaAnalytics")).Return(mockQuery)
	mockQuery.On("Where", "gsi2PK", "=", "VARIANT#hls").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]*models.MediaAnalytics")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.MediaAnalytics)
		*dest = []*models.MediaAnalytics{{MediaID: "m1"}}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	repo := NewMediaAnalyticsRepository(mockDB, "test-table", zap.NewNop(), nil)
	list, err := repo.GetMediaAnalyticsByVariant(ctx, "hls")
	require.NoError(t, err)
	require.Len(t, list, 1)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN3_GetMediaAnalyticsByTimeRange_PerDayWalk(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	day := time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.MediaAnalytics")).Return(mockQuery)
	mockQuery.On("Where", "gsi1PK", "=", "DATE#2026-08-27").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]*models.MediaAnalytics")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.MediaAnalytics)
		*dest = []*models.MediaAnalytics{
			{MediaID: "m1", Timestamp: day.Add(time.Hour)},
			{MediaID: "m2", Timestamp: day.Add(2 * time.Hour)},
		}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	repo := NewMediaAnalyticsRepository(mockDB, "test-table", zap.NewNop(), nil)
	list, err := repo.GetMediaAnalyticsByTimeRange(ctx, "m1", day, day.Add(23*time.Hour), 10)
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, "m1", list[0].MediaID)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN3_GetMetricsByService_PageCappedWalk(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	start := time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.MetricRecord")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1PK", "=", "SERVICE#api").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1SK", ">=", mock.AnythingOfType("string")).Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1SK", "<=", mock.AnythingOfType("string")).Return(mockQuery).Once()
	mockQuery.On("OrderBy", "gsi1SK", "DESC").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]*models.MetricRecord")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.MetricRecord)
		*dest = []*models.MetricRecord{{MetricID: "r1"}, {MetricID: "r2"}}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	repo := NewMetricRecordRepository(mockDB, "test-table", zap.NewNop(), nil)
	records, err := repo.GetMetricsByService(ctx, "api", start, end)
	require.NoError(t, err)
	require.Len(t, records, 2)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN3_GetMetricsByDate_PageCappedWalk(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.MetricRecord")).Return(mockQuery)
	mockQuery.On("Index", "gsi3").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi3PK", "=", "DATE#2026-08-27").Return(mockQuery).Once()
	mockQuery.On("OrderBy", "gsi3SK", "DESC").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]*models.MetricRecord")).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	repo := NewMetricRecordRepository(mockDB, "test-table", zap.NewNop(), nil)
	records, err := repo.GetMetricsByDate(ctx, time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC), "")
	require.NoError(t, err)
	require.Empty(t, records)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN3_CleanupAggregatedMetricsByPeriod_PageCappedWalk(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	cutoff := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.AggregatedMetrics")).Return(mockQuery)
	mockQuery.On("Index", "gsi2").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi2PK", "=", "METRICS_AGG#day").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi2SK", "<", mock.AnythingOfType("string")).Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.AggregatedMetrics")).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	repo := NewMetricsRepository(mockDB, "test-table", zap.NewNop(), nil)
	deleted, err := repo.cleanupAggregatedMetricsByPeriod(ctx, "day", cutoff)
	require.NoError(t, err)
	require.Zero(t, deleted)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// ===== Workstream 1 — moderation_metrics walks + sentinel splits =====

func TestBatchN3_ModerationMetrics_AllBranch_SentinelRoutesThroughSwallow(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	start := time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.ModerationMetricsEntry")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "METRICS#2026-08-26").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "begins_with", "STATS#").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	full := func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.ModerationMetricsEntry)
		*dest = make([]*models.ModerationMetricsEntry, 500)
	}
	// The pre-existing `if err == nil { append }` swallow would drop this
	// error; the sentinel split must route it out FIRST.
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]*models.ModerationMetricsEntry")).Run(full).Return(&core.PaginatedResult{HasMore: true, NextCursor: "c"}, nil).Times(100)
	mockQuery.On("Cursor", "c").Return(mockQuery).Times(99)

	repo := NewModerationMetricsRepository(mockDB, zap.NewNop())
	_, err := repo.GetMetricsEntries(ctx, models.ModerationMetricsTimeRange{Start: start, End: start}, nil)
	require.Error(t, err)
	require.ErrorIs(t, err, errBoundedPageCapExceeded)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN3_ModerationMetrics_TypesBranch_PageCappedWalk(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	start := time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.ModerationMetricsEntry")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1PK", "=", "METRIC_TYPE#spam").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1SK", ">=", "DATE#2026-08-26").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1SK", "<=", "DATE#2026-08-26#Z").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]*models.ModerationMetricsEntry")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.ModerationMetricsEntry)
		*dest = []*models.ModerationMetricsEntry{{MetricType: "spam"}}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	repo := NewModerationMetricsRepository(mockDB, zap.NewNop())
	entries, err := repo.GetMetricsEntries(ctx, models.ModerationMetricsTimeRange{Start: start, End: start}, []string{"spam"})
	require.NoError(t, err)
	require.Len(t, entries, 1)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN3_GetTopPatterns_LimitClamped(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name          string
		limit         int
		expectedLimit int
	}{
		{"limit zero takes default 20", 0, 20},
		{"negative limit takes default 20", -1, 20},
		{"over max clamps to 100", 500, 100},
		{"in-range limit passes through", 5, 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)

			mockDB.On("WithContext", ctx).Return(mockDB)
			mockDB.On("Model", mock.AnythingOfType("*models.ModerationPatternStats")).Return(mockQuery)
			mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
			mockQuery.On("Where", "gsi1PK", "=", "PATTERN_HITS").Return(mockQuery).Once()
			mockQuery.On("Limit", tt.expectedLimit).Return(mockQuery).Once()
			mockQuery.On("All", mock.AnythingOfType("*[]*models.ModerationPatternStats")).Return(nil).Once()

			repo := NewModerationMetricsRepository(mockDB, zap.NewNop())
			patterns, err := repo.GetTopPatterns(ctx, tt.limit)
			require.NoError(t, err)
			require.Empty(t, patterns)
			mockDB.AssertExpectations(t)
			mockQuery.AssertExpectations(t)
		})
	}
}

// ===== Workstream 1 — notification / scheduled-job / search cost =====

func TestBatchN3_GetUserBudgets_PageCappedWalk(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.NotificationBudget")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "NOTIF_BUDGET#alice").Return(mockQuery).Once()
	mockQuery.On("OrderBy", "SK", "ASC").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]*models.NotificationBudget")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.NotificationBudget)
		*dest = []*models.NotificationBudget{{Period: "day"}}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	repo := NewNotificationCostRepository(mockDB, "test-table", zap.NewNop(), nil)
	budgets, err := repo.GetUserBudgets(ctx, "alice")
	require.NoError(t, err)
	require.Len(t, budgets, 1)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN3_GetDailySpending_PageCappedWalk(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.NotificationCostTracking")).Return(mockQuery)
	mockQuery.On("Where", "gsi1PK", "=", "USER#alice").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1SK", ">=", mock.AnythingOfType("string")).Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1SK", "<", mock.AnythingOfType("string")).Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.NotificationCostTracking")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.NotificationCostTracking)
		*dest = []models.NotificationCostTracking{
			{TotalCostMicroCents: 100},
			{TotalCostMicroCents: 200},
		}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	repo := NewNotificationCostRepository(mockDB, "test-table", zap.NewNop(), nil)
	total, err := repo.GetDailySpending(ctx, "alice")
	require.NoError(t, err)
	require.Equal(t, int64(300), total)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN3_ScheduledJobGetByID_SentinelRoutesThroughSwallow(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.ScheduledJobCostRecord")).Return(mockQuery)
	// First status partition caps out: the warn-and-continue swallow must NOT
	// turn cap exhaustion into a not-found result.
	mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1PK", "=", "SCHEDULED_JOB_STATUS#success").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1SK", "contains", "#job-1").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	full := func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.ScheduledJobCostRecord)
		*dest = make([]*models.ScheduledJobCostRecord, 500)
	}
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]*models.ScheduledJobCostRecord")).Run(full).Return(&core.PaginatedResult{HasMore: true, NextCursor: "c"}, nil).Times(100)
	mockQuery.On("Cursor", "c").Return(mockQuery).Times(99)

	repo := NewScheduledJobCostRepository(mockDB, "test-table", zap.NewNop(), nil)
	_, err := repo.GetByID(ctx, "job-1")
	require.Error(t, err)
	require.ErrorIs(t, err, errBoundedPageCapExceeded)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN3_GetSearchCosts_PerDayWalk_SentinelRoutesThroughSwallow(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.SearchCostTracking")).Return(mockQuery)
	// Day 1 caps out: the warn-and-skip swallow must NOT route cap exhaustion
	// into a partial cost set.
	mockQuery.On("Where", "PK", "=", "SEARCH_COST#2026-08-26#user-1").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	full := func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.SearchCostTracking)
		*dest = make([]models.SearchCostTracking, 500)
	}
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.SearchCostTracking")).Run(full).Return(&core.PaginatedResult{HasMore: true, NextCursor: "c"}, nil).Times(100)
	mockQuery.On("Cursor", "c").Return(mockQuery).Times(99)

	repo := NewSearchCostRepository(mockDB, "test-table", zap.NewNop(), nil)
	_, err := repo.GetSearchCosts(ctx, "user-1", time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC), time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC))
	require.Error(t, err)
	require.ErrorIs(t, err, errBoundedPageCapExceeded)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// ===== Workstream 1 — floor clamps (degenerate-input class) =====

func TestBatchN3_ListByConnection_ZeroLimitFloored(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	start := time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.WebSocketCostRecord")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1PK", "=", "WS_CONN#conn-1").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1SK", ">=", mock.AnythingOfType("string")).Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1SK", "<=", mock.AnythingOfType("string")).Return(mockQuery).Once()
	mockQuery.On("OrderBy", "gsi1SK", "DESC").Return(mockQuery).Once()
	// limit=0 previously skipped Limit entirely (the `if limit > 0` gate); the
	// floor must issue Limit(500).
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("All", mock.AnythingOfType("*[]*models.WebSocketCostRecord")).Return(nil).Once()

	repo := NewWebSocketCostRepository(mockDB, "test-table", zap.NewNop(), nil)
	records, err := repo.ListByConnection(ctx, "conn-1", start, start.Add(time.Hour), 0)
	require.NoError(t, err)
	require.Empty(t, records)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN3_GetRouteResults_ZeroLimitFloored(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.RouteDeliveryResult")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "ROUTE#r1").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "begins_with", "RESULT#").Return(mockQuery).Once()
	mockQuery.On("OrderBy", "SK", "DESC").Return(mockQuery).Once()
	// limit=0 previously compiled Limit(0) — no limit; the floor issues
	// Limit(500).
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("All", mock.AnythingOfType("*[]*models.RouteDeliveryResult")).Return(nil).Once()

	repo := NewRouteOptimizerRepository(mockDB, "test-table", zap.NewNop(), nil)
	results, err := repo.GetRouteResults(ctx, "r1", 0)
	require.NoError(t, err)
	require.Empty(t, results)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN3_GetActiveBudgets_ZeroLimitFloored(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.FederationBudget")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1PK", "=", "ACTIVE_BUDGETS").Return(mockQuery).Once()
	mockQuery.On("Filter", "IsActive", "=", true).Return(mockQuery).Once()
	// limit=0 previously compiled Limit(0) — no limit; the floor issues
	// Limit(500).
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("All", mock.AnythingOfType("*[]*models.FederationBudget")).Return(nil).Once()

	baseRepo := NewBaseRepository[*models.FederationCostTracking](mockDB, "test-table", zap.NewNop())
	budgetRepo := NewBaseRepository[*models.FederationBudget](mockDB, "test-table", zap.NewNop())
	repo := NewFederationCostRepositoryFromBase(baseRepo, budgetRepo, nil)
	budgets, err := repo.GetActiveBudgets(ctx, 0)
	require.NoError(t, err)
	require.Empty(t, budgets)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN3_ListByType_ZeroLimitFloored(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	start := time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.Metrics")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "metrics#request").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", ">=", mock.AnythingOfType("string")).Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "<=", mock.AnythingOfType("string")).Return(mockQuery).Once()
	mockQuery.On("OrderBy", "SK", "DESC").Return(mockQuery).Once()
	// limit=0 previously compiled Limit(0) — no limit; the floor issues
	// Limit(20).
	mockQuery.On("Limit", 20).Return(mockQuery).Once()
	mockQuery.On("All", mock.AnythingOfType("*[]*models.Metrics")).Return(nil).Once()

	repo := NewMetricsRepository(mockDB, "test-table", zap.NewNop(), nil)
	list, err := repo.ListByType(ctx, "request", start, start.Add(time.Hour), 0)
	require.NoError(t, err)
	require.Empty(t, list)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN3_GetFederationCosts_ZeroLimitFloored(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	start := time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.FederationCostTracking")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1PK", "=", "FED_COSTS#DOMAIN#example.com#2026-08").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1SK", ">=", mock.AnythingOfType("string")).Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1SK", "<=", mock.AnythingOfType("string")).Return(mockQuery).Once()
	mockQuery.On("OrderBy", "gsi1SK", "ASC").Return(mockQuery).Once()
	// limit=0 previously skipped Limit entirely (the `if limit > 0` gate); the
	// floor must issue Limit(500).
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("All", mock.AnythingOfType("*[]*models.FederationCostTracking")).Return(nil).Once()

	baseRepo := NewBaseRepository[*models.FederationCostTracking](mockDB, "test-table", zap.NewNop())
	budgetRepo := NewBaseRepository[*models.FederationBudget](mockDB, "test-table", zap.NewNop())
	repo := NewFederationCostRepositoryFromBase(baseRepo, budgetRepo, nil)
	costs, err := repo.GetFederationCosts(ctx, "example.com", start, start.Add(time.Hour), 0)
	require.NoError(t, err)
	require.Empty(t, costs)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN3_GetCostTrackingByNotification_ZeroLimitFloored(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.NotificationCostTracking")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "NOTIF_COST#n1").Return(mockQuery).Once()
	mockQuery.On("OrderBy", "SK", "DESC").Return(mockQuery).Once()
	// limit=0 previously compiled Limit(0) — no limit; the floor issues
	// Limit(500).
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("All", mock.AnythingOfType("*[]*models.NotificationCostTracking")).Return(nil).Once()

	repo := NewNotificationCostRepository(mockDB, "test-table", zap.NewNop(), nil)
	records, err := repo.GetCostTrackingByNotification(ctx, "n1", 0)
	require.NoError(t, err)
	require.Empty(t, records)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN3_GetMetricsInRange_ZeroLimitFloored(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	start := time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.RouteDeliveryResult")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "ROUTE#r1").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", ">=", mock.AnythingOfType("string")).Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "<=", mock.AnythingOfType("string")).Return(mockQuery).Once()
	mockQuery.On("OrderBy", "SK", "DESC").Return(mockQuery).Once()
	// limit=0 previously compiled Limit(0) — no limit; the floor issues
	// Limit(500).
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("All", mock.AnythingOfType("*[]*models.RouteDeliveryResult")).Return(nil).Once()

	repo := NewRouteOptimizerRepository(mockDB, "test-table", zap.NewNop(), nil)
	results, err := repo.GetMetricsInRange(ctx, "r1", start, start.Add(time.Hour), 0)
	require.NoError(t, err)
	require.Empty(t, results)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// ===== Additional branch coverage for newly-bounded sites (verify ci gate) =====

func TestBatchN3_CountReplies_PageCapExhaustionFailsClosed(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.Object")).Return(mockQuery)
	mockQuery.On("Index", "gsi6").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi6PK", "=", "REPLIES#https://example.com/objects/status-1").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	full := func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.Object)
		*dest = make([]models.Object, 500)
	}
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.Object")).Run(full).Return(&core.PaginatedResult{HasMore: true, NextCursor: "c"}, nil).Times(100)
	mockQuery.On("Cursor", "c").Return(mockQuery).Times(99)

	repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())
	_, err := repo.CountReplies(ctx, "status-1")
	require.Error(t, err)
	require.ErrorIs(t, err, errBoundedPageCapExceeded)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN3_GetUnreadNotificationCount_PageCapExhaustionFailsClosed(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.Notification")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "USER#alice").Return(mockQuery).Once()
	mockQuery.On("Filter", "IsRead", "=", false).Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	full := func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.Notification)
		*dest = make([]models.Notification, 500)
	}
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.Notification")).Run(full).Return(&core.PaginatedResult{HasMore: true, NextCursor: "c"}, nil).Times(100)
	mockQuery.On("Cursor", "c").Return(mockQuery).Times(99)

	repo := NewNotificationRepository(mockDB, "test-table", zap.NewNop(), nil)
	_, err := repo.GetUnreadNotificationCount(ctx, "alice")
	require.Error(t, err)
	require.ErrorIs(t, err, errBoundedPageCapExceeded)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN3_GetOpenReportsCount_CapFailsClosed(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.Report")).Return(mockQuery)
	mockQuery.On("Index", "gsi3").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi3PK", "=", "STATUS#open").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	full := func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.Report)
		*dest = make([]models.Report, 500)
	}
	// The legacy 0-nil swallow would drop this; the sentinel split must
	// propagate it (wave #1469).
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.Report")).Run(full).Return(&core.PaginatedResult{HasMore: true, NextCursor: "c"}, nil).Times(100)
	mockQuery.On("Cursor", "c").Return(mockQuery).Times(99)

	repo := NewModerationRepository(mockDB, "test-table", zap.NewNop())
	_, err := repo.GetOpenReportsCount(ctx)
	require.Error(t, err)
	require.ErrorIs(t, err, errBoundedPageCapExceeded)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN3_CountPendingFlags_CapFailsClosed(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.Flag")).Return(mockQuery)
	mockQuery.On("Index", "gsi2").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi2PK", "=", "FLAG_STATUS#pending").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	full := func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.Flag)
		*dest = make([]models.Flag, 500)
	}
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.Flag")).Run(full).Return(&core.PaginatedResult{HasMore: true, NextCursor: "c"}, nil).Times(100)
	mockQuery.On("Cursor", "c").Return(mockQuery).Times(99)

	repo := NewModerationRepository(mockDB, "test-table", zap.NewNop())
	_, err := repo.CountPendingFlags(ctx)
	require.Error(t, err)
	require.ErrorIs(t, err, errBoundedPageCapExceeded)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN3_CountStatusesForAdmin_PageCappedWalk(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.Status")).Return(mockQuery)
	mockQuery.On("Index", "gsi8").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi8PK", "=", "ADMIN_TIMELINE").Return(mockQuery).Once()
	mockQuery.On("Filter", "Deleted", "=", false).Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.Status")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.Status)
		*dest = []models.Status{{StatusID: "s1"}, {StatusID: "s2"}}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	repo := NewStatusRepository(mockDB, "test-table", zap.NewNop(), nil)
	count, err := repo.CountStatusesForAdmin(ctx, &interfaces.StatusFilter{})
	require.NoError(t, err)
	require.Equal(t, int64(2), count)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN3_GetSearchCosts_TransientErrorSkipsDay(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.SearchCostTracking")).Return(mockQuery)
	// Day 1: transient (non-cap) error keeps the skip-this-day behavior.
	mockQuery.On("Where", "PK", "=", "SEARCH_COST#2026-08-26#user-1").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.SearchCostTracking")).Return(nil, errors.New("transient")).Once()
	// Day 2: one cost record collected.
	mockQuery.On("Where", "PK", "=", "SEARCH_COST#2026-08-27#user-1").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.SearchCostTracking")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.SearchCostTracking)
		*dest = []models.SearchCostTracking{{UserID: "user-1", TotalCostMicros: 50}}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	repo := NewSearchCostRepository(mockDB, "test-table", zap.NewNop(), nil)
	costs, err := repo.GetSearchCosts(ctx, "user-1", time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC), time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.Len(t, costs, 1)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN3_GetRecentResults_ZeroLimitFloored(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.RouteDeliveryResult")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1PK", "=", "RESULTS").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1SK", ">", mock.AnythingOfType("string")).Return(mockQuery).Once()
	mockQuery.On("OrderBy", "gsi1SK", "DESC").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("All", mock.AnythingOfType("*[]*models.RouteDeliveryResult")).Return(nil).Once()

	repo := NewRouteOptimizerRepository(mockDB, "test-table", zap.NewNop(), nil)
	results, err := repo.GetRecentResults(ctx, time.Now().Add(-time.Hour), 0)
	require.NoError(t, err)
	require.Empty(t, results)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN3_GetFederationCostsByActivityType_ZeroLimitFloored(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	start := time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.FederationCostTracking")).Return(mockQuery)
	mockQuery.On("Index", "gsi2").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi2PK", "=", "FED_TYPE#announce").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi2SK", ">=", mock.AnythingOfType("string")).Return(mockQuery).Once()
	mockQuery.On("Where", "gsi2SK", "<=", mock.AnythingOfType("string")).Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("All", mock.AnythingOfType("*[]*models.FederationCostTracking")).Return(nil).Once()

	baseRepo := NewBaseRepository[*models.FederationCostTracking](mockDB, "test-table", zap.NewNop())
	budgetRepo := NewBaseRepository[*models.FederationBudget](mockDB, "test-table", zap.NewNop())
	repo := NewFederationCostRepositoryFromBase(baseRepo, budgetRepo, nil)
	costs, err := repo.GetFederationCostsByActivityType(ctx, "announce", start, start.Add(time.Hour), 0)
	require.NoError(t, err)
	require.Empty(t, costs)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN3_GetMetricsInRange_NoEndZeroLimitFloored(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	start := time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.RouteDeliveryResult")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "ROUTE#r1").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", ">=", mock.AnythingOfType("string")).Return(mockQuery).Once()
	mockQuery.On("OrderBy", "SK", "DESC").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("All", mock.AnythingOfType("*[]*models.RouteDeliveryResult")).Return(nil).Once()

	repo := NewRouteOptimizerRepository(mockDB, "test-table", zap.NewNop(), nil)
	// end.IsZero() skips the <= filter branch.
	results, err := repo.GetMetricsInRange(ctx, "r1", start, time.Time{}, 0)
	require.NoError(t, err)
	require.Empty(t, results)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN3_GetModerationQueueCount_TransientErrorReturnsZero(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.ModerationEvent")).Return(mockQuery)
	mockQuery.On("Index", "gsi2").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi2PK", "=", "TYPE#flagged#pending").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	// A transient (non-cap) error keeps the legacy 0-on-error contract.
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.ModerationEvent")).Return(nil, errors.New("transient")).Once()

	repo := NewModerationRepository(mockDB, "test-table", zap.NewNop())
	count, err := repo.GetModerationQueueCount(ctx)
	require.NoError(t, err)
	require.Zero(t, count)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN3_GetOpenReportsCount_TransientErrorReturnsZero(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.Report")).Return(mockQuery)
	mockQuery.On("Index", "gsi3").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi3PK", "=", "STATUS#open").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.Report")).Return(nil, errors.New("transient")).Once()

	repo := NewModerationRepository(mockDB, "test-table", zap.NewNop())
	count, err := repo.GetOpenReportsCount(ctx)
	require.NoError(t, err)
	require.Zero(t, count)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN3_WalkKeyedPages_DefaultFallbacksFailClosed(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	// Direct helper test: pageSize=0/maxPages=0 fall back to 500/100 (the
	// shared helper's defensive defaults, wave #1469); a cap exhaustion still
	// fails closed.
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.Notification")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "USER#u1").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	full := func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.Notification)
		*dest = make([]models.Notification, 500)
	}
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.Notification")).Run(full).Return(&core.PaginatedResult{HasMore: true, NextCursor: "c"}, nil).Times(100)
	mockQuery.On("Cursor", "c").Return(mockQuery).Times(99)

	var notifications []models.Notification
	err := walkKeyedPages(
		mockDB.WithContext(ctx).Model(&models.Notification{}).Where("PK", "=", "USER#u1"),
		0, 0,
		func(page []models.Notification) (bool, error) {
			notifications = append(notifications, page...)
			return false, nil
		},
	)
	require.Error(t, err)
	require.ErrorIs(t, err, errBoundedPageCapExceeded)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN3_GetFederationCosts_TwoMonthBucketLoop(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	start := time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.FederationCostTracking")).Return(mockQuery)
	// Two month buckets: July and August (the month loop, wave #1469 floor
	// Limit(500) applied per bucket).
	mockQuery.On("Index", "gsi1").Return(mockQuery).Times(2)
	mockQuery.On("Where", "gsi1PK", "=", "FED_COSTS#DOMAIN#example.com#2026-07").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1SK", ">=", mock.AnythingOfType("string")).Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1SK", "<=", mock.AnythingOfType("string")).Return(mockQuery).Once()
	mockQuery.On("OrderBy", "gsi1SK", "ASC").Return(mockQuery).Times(2)
	// Per-bucket remaining-limit semantics: first bucket Limit(10) (limit), then
	// Limit(9) once one cost was gathered. The floor guarantees > 0.
	mockQuery.On("Limit", 10).Return(mockQuery).Once()
	mockQuery.On("Limit", 9).Return(mockQuery).Once()
	mockQuery.On("All", mock.AnythingOfType("*[]*models.FederationCostTracking")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.FederationCostTracking)
		*dest = []*models.FederationCostTracking{{Domain: "example.com"}}
	}).Return(nil).Once()
	mockQuery.On("Where", "gsi1PK", "=", "FED_COSTS#DOMAIN#example.com#2026-08").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1SK", ">=", mock.AnythingOfType("string")).Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1SK", "<=", mock.AnythingOfType("string")).Return(mockQuery).Once()
	mockQuery.On("All", mock.AnythingOfType("*[]*models.FederationCostTracking")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.FederationCostTracking)
		*dest = []*models.FederationCostTracking{{Domain: "example.com"}}
	}).Return(nil).Once()

	baseRepo := NewBaseRepository[*models.FederationCostTracking](mockDB, "test-table", zap.NewNop())
	budgetRepo := NewBaseRepository[*models.FederationBudget](mockDB, "test-table", zap.NewNop())
	repo := NewFederationCostRepositoryFromBase(baseRepo, budgetRepo, nil)
	costs, err := repo.GetFederationCosts(ctx, "example.com", start, end, 10)
	require.NoError(t, err)
	require.Len(t, costs, 2)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN3_GetMediaAnalyticsByDate_TracksCost(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	costService := cost.NewTrackingService(nil, zap.NewNop(), cost.DefaultTrackingServiceConfig())
	t.Cleanup(func() { _ = costService.Close(ctx) })

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.MediaAnalytics")).Return(mockQuery)
	mockQuery.On("Where", "gsi1PK", "=", "DATE#2026-08-27").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]*models.MediaAnalytics")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.MediaAnalytics)
		*dest = []*models.MediaAnalytics{{MediaID: "m1"}}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	repo := NewMediaAnalyticsRepository(mockDB, "test-table", zap.NewNop(), costService)
	list, err := repo.GetMediaAnalyticsByDate(ctx, "2026-08-27")
	require.NoError(t, err)
	require.Len(t, list, 1)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN3_CountStatusesForAdmin_RemoteOnlyPageLoop(t *testing.T) {
	t.Setenv("DOMAIN", "example.com")
	// config.Get() caches the singleton across tests, so set the domain
	// directly to force the remote-only branch regardless of test order.
	cfg := config.Get()
	cfg.Domain = "example.com"
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.Status")).Return(mockQuery)
	mockQuery.On("Index", "gsi8").Return(mockQuery)
	mockQuery.On("Where", "gsi8PK", "=", "ADMIN_TIMELINE").Return(mockQuery)
	mockQuery.On("Filter", "Deleted", "=", false).Return(mockQuery)
	mockQuery.On("OrderBy", "gsi8SK", "DESC").Return(mockQuery)
	mockQuery.On("Limit", 201).Return(mockQuery)
	// One page with one local and one remote status; the local-domain row is
	// excluded from the remote-only count.
	mockQuery.On("All", mock.AnythingOfType("*[]models.Status")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.Status)
		*dest = []models.Status{
			{StatusID: "local-1", AuthorID: "https://example.com/users/a"},
			{StatusID: "remote-1", AuthorID: "https://other.example/users/b"},
		}
	}).Return(nil).Once()

	repo := NewStatusRepository(mockDB, "test-table", zap.NewNop(), nil)
	count, err := repo.CountStatusesForAdmin(ctx, &interfaces.StatusFilter{Remote: boolPtr(true)})
	require.NoError(t, err)
	require.Equal(t, int64(1), count)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN3_GetUserBudgets_QueryError(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.NotificationBudget")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "NOTIF_BUDGET#alice").Return(mockQuery).Once()
	mockQuery.On("OrderBy", "SK", "ASC").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]*models.NotificationBudget")).Return(nil, errors.New("boom")).Once()

	repo := NewNotificationCostRepository(mockDB, "test-table", zap.NewNop(), nil)
	_, err := repo.GetUserBudgets(ctx, "alice")
	require.Error(t, err)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN3_GetDailySpending_QueryError(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.NotificationCostTracking")).Return(mockQuery)
	mockQuery.On("Where", "gsi1PK", "=", "USER#alice").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1SK", ">=", mock.AnythingOfType("string")).Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1SK", "<", mock.AnythingOfType("string")).Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.NotificationCostTracking")).Return(nil, errors.New("boom")).Once()

	repo := NewNotificationCostRepository(mockDB, "test-table", zap.NewNop(), nil)
	_, err := repo.GetDailySpending(ctx, "alice")
	require.Error(t, err)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN3_GetModerationQueue_CountReviewsCapFailsClosed(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	// GetModerationQueue reads the queue events, then countReviews per event;
	// cap exhaustion in the count must fail the whole queue read closed (the
	// pre-existing `reviewCount, _ :=` discard would have made it silent).
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*[]models.ModerationEvent")).Return(mockQuery).Once()
	mockQuery.On("Index", "gsi2").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi2PK", "=", "TYPE#flagged#pending").Return(mockQuery).Once()
	mockQuery.On("Limit", 100).Return(mockQuery).Once()
	mockQuery.On("All", mock.AnythingOfType("*[]models.ModerationEvent")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]models.ModerationEvent)
		*out = []models.ModerationEvent{{ID: "e1", EventType: storage.EventTypeFlagged, ObjectType: "status", ConfidenceScore: 0.5, Created: time.Now()}}
	}).Return(nil).Once()

	// countReviews walk caps out.
	mockDB.On("Model", mock.AnythingOfType("*models.ModerationReview")).Return(mockQuery).Once()
	mockQuery.On("Where", "PK", "=", "REVIEW#e1").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	full := func(args mock.Arguments) {
		out := args.Get(0).(*[]models.ModerationReview)
		*out = make([]models.ModerationReview, 500)
	}
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.ModerationReview")).Run(full).Return(&core.PaginatedResult{HasMore: true, NextCursor: "c"}, nil).Times(100)
	mockQuery.On("Cursor", "c").Return(mockQuery).Times(99)

	repo := NewModerationRepository(mockDB, "test-table", zap.NewNop())
	_, err := repo.GetModerationQueue(ctx, &storage.ModerationFilter{Limit: 100})
	require.Error(t, err)
	require.ErrorIs(t, err, errBoundedPageCapExceeded)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN3_ModerationMetrics_TypesBranch_TransientErrorSkipsType(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	start := time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.ModerationMetricsEntry")).Return(mockQuery)
	// Type 1 walk errors transiently (skip-type); type 2 returns data.
	mockQuery.On("Index", "gsi1").Return(mockQuery).Times(2)
	mockQuery.On("Where", "gsi1PK", "=", "METRIC_TYPE#spam").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1SK", ">=", "DATE#2026-08-26").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1SK", "<=", "DATE#2026-08-26#Z").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Times(2)
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]*models.ModerationMetricsEntry")).Return(nil, errors.New("transient")).Once()
	mockQuery.On("Where", "gsi1PK", "=", "METRIC_TYPE#abuse").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1SK", ">=", "DATE#2026-08-26").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1SK", "<=", "DATE#2026-08-26#Z").Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]*models.ModerationMetricsEntry")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]*models.ModerationMetricsEntry)
		*out = []*models.ModerationMetricsEntry{{MetricType: "abuse"}}
	}).Return(&core.PaginatedResult{}, nil).Once()

	repo := NewModerationMetricsRepository(mockDB, zap.NewNop())
	entries, err := repo.GetMetricsEntries(ctx, models.ModerationMetricsTimeRange{Start: start, End: start}, []string{"spam", "abuse"})
	require.NoError(t, err)
	require.Len(t, entries, 1)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchN3_CountQuotes_PageCapExhaustionFailsClosed(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.QuoteRelationship")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1PK", "=", "QUOTED#note-1").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	full := func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.QuoteRelationship)
		*dest = make([]models.QuoteRelationship, 500)
	}
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.QuoteRelationship")).Run(full).Return(&core.PaginatedResult{HasMore: true, NextCursor: "c"}, nil).Times(100)
	mockQuery.On("Cursor", "c").Return(mockQuery).Times(99)

	repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())
	_, err := repo.CountQuotes(ctx, "note-1")
	require.Error(t, err)
	require.ErrorIs(t, err, errBoundedPageCapExceeded)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}
