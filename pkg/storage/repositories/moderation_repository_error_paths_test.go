package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

func TestModerationRepository_GetModerationQueue_FilterBranches(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	now := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	start := now.Add(-2 * time.Hour)
	end := now.Add(-30 * time.Minute)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Index", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]models.ModerationEvent")).Run(func(args mock.Arguments) {
		target := args.Get(0).(*[]models.ModerationEvent)
		*target = []models.ModerationEvent{
			{ID: "min", ConfidenceScore: 0.1, ObjectType: "status", EventType: storage.EventTypeFlagged, Created: now.Add(-time.Hour)},
			{ID: "max", ConfidenceScore: 0.99, ObjectType: "status", EventType: storage.EventTypeFlagged, Created: now.Add(-time.Hour)},
			{ID: "type", ConfidenceScore: 0.5, ObjectType: "account", EventType: storage.EventTypeFlagged, Created: now.Add(-time.Hour)},
			{ID: "action", ConfidenceScore: 0.5, ObjectType: "status", EventType: "other", Created: now.Add(-time.Hour)},
			{ID: "start", ConfidenceScore: 0.5, ObjectType: "status", EventType: storage.EventTypeFlagged, Created: now.Add(-24 * time.Hour)},
			{ID: "end", ConfidenceScore: 0.5, ObjectType: "status", EventType: storage.EventTypeFlagged, Created: now.Add(-5 * time.Minute)},
			{ID: "ok", ConfidenceScore: 0.5, ObjectType: "status", EventType: storage.EventTypeFlagged, Created: now.Add(-time.Hour)},
		}
	}).Return(nil)

	// Force countReviews to take its error branch; GetModerationQueue ignores the error.
	mockQuery.On("Count").Return(int64(0), ErrTestMockError).Maybe()

	repo := NewModerationRepository(mockDB, "test-table", zap.NewNop())

	items, err := repo.GetModerationQueue(ctx, &storage.ModerationFilter{
		Limit:       100,
		MinScore:    0.2,
		MaxScore:    0.8,
		ContentType: "status",
		Action:      storage.EventTypeFlagged,
		StartTime:   &start,
		EndTime:     &end,
	})
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "ok", items[0].Event.ID)
}

func TestModerationRepository_GetModerationEvent_NotFoundAndError(t *testing.T) {
	t.Run("not found maps to not found error", func(t *testing.T) {
		ctx := context.Background()
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("First", mock.Anything).Return(errors.ErrItemNotFound).Once()
		setupPermissiveDynamormMocks(mockDB, mockQuery)

		repo := NewModerationRepository(mockDB, "test-table", zap.NewNop())
		_, err := repo.GetModerationEvent(ctx, "evt-1")
		require.Error(t, err)
	})

	t.Run("real error maps to get error", func(t *testing.T) {
		ctx := context.Background()
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("First", mock.Anything).Return(ErrTestMockError).Once()
		setupPermissiveDynamormMocks(mockDB, mockQuery)

		repo := NewModerationRepository(mockDB, "test-table", zap.NewNop())
		_, err := repo.GetModerationEvent(ctx, "evt-1")
		require.Error(t, err)
	})
}

func TestModerationRepository_GetReportsByStatus_NotFoundReturnsEmpty(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockQuery.On("All", mock.Anything).Return(errors.ErrItemNotFound).Once()
	setupPermissiveDynamormMocks(mockDB, mockQuery)

	repo := NewModerationRepository(mockDB, "test-table", zap.NewNop())
	reports, nextCursor, err := repo.GetReportsByStatus(ctx, storage.ReportStatusOpen, 50, "")
	require.NoError(t, err)
	require.Empty(t, nextCursor)
	require.Empty(t, reports)
}

func TestModerationRepository_DeleteModerationPattern_NotFoundAndError(t *testing.T) {
	t.Run("not found returns storage.ErrNotFound", func(t *testing.T) {
		ctx := context.Background()
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("Delete").Return(errors.ErrItemNotFound).Once()
		setupPermissiveDynamormMocks(mockDB, mockQuery)

		repo := NewModerationRepository(mockDB, "test-table", zap.NewNop())
		err := repo.DeleteModerationPattern(ctx, "pattern-1")
		require.ErrorIs(t, err, storage.ErrNotFound)
	})

	t.Run("real error returns delete error", func(t *testing.T) {
		ctx := context.Background()
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("Delete").Return(ErrTestMockError).Once()
		setupPermissiveDynamormMocks(mockDB, mockQuery)

		repo := NewModerationRepository(mockDB, "test-table", zap.NewNop())
		err := repo.DeleteModerationPattern(ctx, "pattern-1")
		require.Error(t, err)
	})
}

func TestModerationRepository_GetReportStats_NotFoundAndError(t *testing.T) {
	t.Run("not found returns empty stats", func(t *testing.T) {
		ctx := context.Background()
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("First", mock.Anything).Return(errors.ErrItemNotFound).Once()
		setupPermissiveDynamormMocks(mockDB, mockQuery)

		repo := NewModerationRepository(mockDB, "test-table", zap.NewNop())
		stats, err := repo.GetReportStats(ctx, "user-1")
		require.NoError(t, err)
		require.NotNil(t, stats)
		require.Equal(t, 0, stats.TotalReports)
		require.Nil(t, stats.LastReportAt)
	})

	t.Run("real error returns get error", func(t *testing.T) {
		ctx := context.Background()
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("First", mock.Anything).Return(ErrTestMockError).Once()
		setupPermissiveDynamormMocks(mockDB, mockQuery)

		repo := NewModerationRepository(mockDB, "test-table", zap.NewNop())
		_, err := repo.GetReportStats(ctx, "user-1")
		require.Error(t, err)
	})
}

func TestModerationRepository_GetOpenReportsCount_And_CountPendingFlags_ErrorBranches(t *testing.T) {
	ctx := context.Background()

	t.Run("GetOpenReportsCount returns 0 on count error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("Count").Return(int64(0), ErrTestMockError).Once()
		setupPermissiveDynamormMocks(mockDB, mockQuery)

		repo := NewModerationRepository(mockDB, "test-table", zap.NewNop())
		count, err := repo.GetOpenReportsCount(ctx)
		require.NoError(t, err)
		require.Equal(t, 0, count)
	})

	t.Run("CountPendingFlags returns 0 on count error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("Count").Return(int64(0), ErrTestMockError).Once()
		setupPermissiveDynamormMocks(mockDB, mockQuery)

		repo := NewModerationRepository(mockDB, "test-table", zap.NewNop())
		count, err := repo.CountPendingFlags(ctx)
		require.NoError(t, err)
		require.Equal(t, 0, count)
	})
}

func TestModerationRepository_GetDecisionHistory_NotFoundAndError(t *testing.T) {
	t.Run("not found returns empty slice", func(t *testing.T) {
		ctx := context.Background()
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("All", mock.Anything).Return(errors.ErrItemNotFound).Once()
		setupPermissiveDynamormMocks(mockDB, mockQuery)

		repo := NewModerationRepository(mockDB, "test-table", zap.NewNop())
		decisions, err := repo.GetDecisionHistory(ctx, "content-1")
		require.NoError(t, err)
		require.Empty(t, decisions)
	})

	t.Run("real error returns query error", func(t *testing.T) {
		ctx := context.Background()
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("All", mock.Anything).Return(ErrTestMockError).Once()
		setupPermissiveDynamormMocks(mockDB, mockQuery)

		repo := NewModerationRepository(mockDB, "test-table", zap.NewNop())
		_, err := repo.GetDecisionHistory(ctx, "content-1")
		require.Error(t, err)
	})
}

func TestModerationRepository_DeleteFilterEntity_KeyedDeletes(t *testing.T) {
	ctx := context.Background()

	t.Run("DeleteFilterKeyword issues keyed delete", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		setupPermissiveDynamormMocks(mockDB, mockQuery)

		repo := NewModerationRepository(mockDB, "test-table", zap.NewNop())
		require.NoError(t, repo.DeleteFilterKeyword(ctx, "filter-1", "keyword-1"))
	})

	t.Run("DeleteFilterStatus issues keyed delete", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		setupPermissiveDynamormMocks(mockDB, mockQuery)

		repo := NewModerationRepository(mockDB, "test-table", zap.NewNop())
		require.NoError(t, repo.DeleteFilterStatus(ctx, "filter-1", "status-id-1"))
	})
}

func TestModerationRepository_IncrementFalseReports_UpdateNotFoundCreates(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	// GetReportStats -> not found returns empty stats
	mockQuery.On("First", mock.Anything).Return(errors.ErrItemNotFound).Once()
	// Update -> not found triggers Create fallback
	mockQuery.On("Update", mock.Anything).Return(errors.ErrItemNotFound).Once()
	mockQuery.On("Create").Return(nil).Once()

	setupPermissiveDynamormMocks(mockDB, mockQuery)

	repo := NewModerationRepository(mockDB, "test-table", zap.NewNop())
	require.NoError(t, repo.IncrementFalseReports(ctx, "user-1"))
}

func TestModerationRepository_GetModerationDecision_And_GetModerationPattern_ErrorBranches(t *testing.T) {
	ctx := context.Background()

	t.Run("GetModerationDecision not found and error", func(t *testing.T) {
		t.Run("not found", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)
			mockQuery.On("First", mock.Anything).Return(errors.ErrItemNotFound).Once()
			setupPermissiveDynamormMocks(mockDB, mockQuery)

			repo := NewModerationRepository(mockDB, "test-table", zap.NewNop())
			_, err := repo.GetModerationDecision(ctx, "obj-1")
			require.Error(t, err)
		})

		t.Run("real error", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)
			mockQuery.On("First", mock.Anything).Return(ErrTestMockError).Once()
			setupPermissiveDynamormMocks(mockDB, mockQuery)

			repo := NewModerationRepository(mockDB, "test-table", zap.NewNop())
			_, err := repo.GetModerationDecision(ctx, "obj-1")
			require.Error(t, err)
		})
	})

	t.Run("GetModerationPattern not found and error", func(t *testing.T) {
		t.Run("not found", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)
			mockQuery.On("First", mock.Anything).Return(errors.ErrItemNotFound).Once()
			setupPermissiveDynamormMocks(mockDB, mockQuery)

			repo := NewModerationRepository(mockDB, "test-table", zap.NewNop())
			_, err := repo.GetModerationPattern(ctx, "pattern-1")
			require.ErrorIs(t, err, storage.ErrNotFound)
		})

		t.Run("real error", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)
			mockQuery.On("First", mock.Anything).Return(ErrTestMockError).Once()
			setupPermissiveDynamormMocks(mockDB, mockQuery)

			repo := NewModerationRepository(mockDB, "test-table", zap.NewNop())
			_, err := repo.GetModerationPattern(ctx, "pattern-1")
			require.Error(t, err)
		})
	})
}

func TestModerationRepository_addToReviewQueue_Branches(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	var captured []*models.ModerationReviewQueue
	mockDB.On("Model", mock.Anything).Run(func(args mock.Arguments) {
		if item, ok := args.Get(0).(*models.ModerationReviewQueue); ok {
			captured = append(captured, item)
		}
	}).Return(mockQuery).Maybe()

	setupPermissiveDynamormMocks(mockDB, mockQuery)

	repo := NewModerationRepository(mockDB, "test-table", zap.NewNop())

	require.NoError(t, repo.addToReviewQueue(ctx, &models.ModerationDecisionResult{
		ContentID:      "content-1",
		AuthorID:       "author-1",
		Action:         "remove",
		ReviewPriority: 8,
		Reasons:        []interface{}{"reason"},
		Recommendations: []string{
			"review",
		},
		Confidence: 0.9,
	}))

	require.NoError(t, repo.addToReviewQueue(ctx, &models.ModerationDecisionResult{
		ContentID:       "content-2",
		AuthorID:        "author-2",
		Action:          "remove",
		ReviewPriority:  5,
		Recommendations: []string{"review"},
		Confidence:      0.9,
	}))

	require.NoError(t, repo.addToReviewQueue(ctx, &models.ModerationDecisionResult{
		ContentID:      "content-3",
		AuthorID:       "author-3",
		Action:         "allow",
		ReviewPriority: 1,
		Confidence:     0.9,
	}))

	require.GreaterOrEqual(t, len(captured), 3)
	require.NotNil(t, captured[0].Evidence)
	require.NotNil(t, captured[1].Evidence)
	require.Nil(t, captured[2].Evidence)
}

func TestModerationRepository_GetModerationEvents_RoutingBranches(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupPermissiveDynamormMocks(mockDB, mockQuery)

	repo := NewModerationRepository(mockDB, "test-table", zap.NewNop())

	_, _, err := repo.GetModerationEvents(ctx, &storage.ModerationEventFilter{ObjectID: "obj-1"}, 1, "cursor-0")
	require.NoError(t, err)

	_, _, err = repo.GetModerationEvents(ctx, &storage.ModerationEventFilter{ActorID: "actor-1"}, 1, "cursor-0")
	require.NoError(t, err)

	_, _, err = repo.GetModerationEvents(ctx, &storage.ModerationEventFilter{}, 1, "cursor-0")
	require.NoError(t, err)
}

func TestModerationRepository_GetReportedStatuses_NotFoundAndError(t *testing.T) {
	ctx := context.Background()

	t.Run("not found", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.Anything).Return(errors.ErrItemNotFound).Once()
		setupPermissiveDynamormMocks(mockDB, mockQuery)

		repo := NewModerationRepository(mockDB, "test-table", zap.NewNop())
		_, err := repo.GetReportedStatuses(ctx, "report-1")
		require.Error(t, err)
	})

	t.Run("real error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.Anything).Return(ErrTestMockError).Once()
		setupPermissiveDynamormMocks(mockDB, mockQuery)

		repo := NewModerationRepository(mockDB, "test-table", zap.NewNop())
		_, err := repo.GetReportedStatuses(ctx, "report-1")
		require.Error(t, err)
	})
}

func TestModerationRepository_CreateModerationDecision_And_AddModerationReview_CreateErrors(t *testing.T) {
	ctx := context.Background()

	t.Run("AddModerationReview create error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("Create").Return(ErrTestMockError).Once()
		setupPermissiveDynamormMocks(mockDB, mockQuery)

		repo := NewModerationRepository(mockDB, "test-table", zap.NewNop())
		err := repo.AddModerationReview(ctx, &storage.ModerationReview{
			EventID:     "event-1",
			ReviewerID:  "reviewer-1",
			Action:      "approve",
			Severity:    string(storage.SeverityMedium),
			Confidence:  0.8,
			ReviewerRep: 0.6,
			Created:     time.Now(),
		})
		require.Error(t, err)
	})

	t.Run("CreateModerationDecision create error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("Create").Return(ErrTestMockError).Once()
		setupPermissiveDynamormMocks(mockDB, mockQuery)

		repo := NewModerationRepository(mockDB, "test-table", zap.NewNop())
		err := repo.CreateModerationDecision(ctx, &storage.ModerationDecision{
			EventID:          "event-1",
			ObjectID:         "obj-1",
			Action:           "allow",
			ConsensusScore:   0.9,
			ReviewerCount:    1,
			TrustWeightTotal: 0.9,
			Reviews:          []interface{}{"review-1"},
		})
		require.Error(t, err)
	})
}

func TestModerationRepository_RecordPatternMatch_CreateError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Create").Return(ErrTestMockError).Once()

	repo := NewModerationRepository(mockDB, "test-table", zap.NewNop())
	err := repo.RecordPatternMatch(context.Background(), "pattern-1", true, time.Now())
	require.Error(t, err)
}

func TestModerationRepository_GetReviewQueue_ValidationError(t *testing.T) {
	repo := NewModerationRepository(nil, "test-table", zap.NewNop())
	_, err := repo.GetReviewQueue(context.Background(), map[string]interface{}{"": "x"})
	require.Error(t, err)
}

func TestModerationRepository_GetReviewQueue_NotFoundReturnsEmpty(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockQuery.On("All", mock.Anything).Return(errors.ErrItemNotFound).Once()
	setupPermissiveDynamormMocks(mockDB, mockQuery)

	repo := NewModerationRepository(mockDB, "test-table", zap.NewNop())
	items, err := repo.GetReviewQueue(ctx, map[string]interface{}{"status": StatusPending, "limit": 1})
	require.NoError(t, err)
	require.Empty(t, items)
}

func TestModerationRepository_GetModerationQueueCount_CountErrorReturnsZero(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockQuery.On("Count").Return(int64(0), ErrTestMockError).Once()
	setupPermissiveDynamormMocks(mockDB, mockQuery)

	repo := NewModerationRepository(mockDB, "test-table", zap.NewNop())
	count, err := repo.GetModerationQueueCount(ctx)
	require.NoError(t, err)
	require.Equal(t, 0, count)
}

func TestModerationRepository_CreateAdminReview_ErrorBranches(t *testing.T) {
	ctx := context.Background()

	t.Run("AddModerationReview fails", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("Create").Return(ErrTestMockError).Once()
		setupPermissiveDynamormMocks(mockDB, mockQuery)

		repo := NewModerationRepository(mockDB, "test-table", zap.NewNop())
		err := repo.CreateAdminReview(ctx, "event-1", "admin-1", storage.ActionType("remove"), "reason")
		require.Error(t, err)
	})

	t.Run("GetModerationEvent fails", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("Create").Return(nil).Once()
		mockQuery.On("First", mock.Anything).Return(ErrTestMockError).Once()
		setupPermissiveDynamormMocks(mockDB, mockQuery)

		repo := NewModerationRepository(mockDB, "test-table", zap.NewNop())
		err := repo.CreateAdminReview(ctx, "event-1", "admin-1", storage.ActionType("remove"), "reason")
		require.Error(t, err)
	})

	t.Run("CreateModerationDecision fails", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("Create").Return(nil).Once()
		mockQuery.On("Create").Return(ErrTestMockError).Once()
		setupPermissiveDynamormMocks(mockDB, mockQuery)

		repo := NewModerationRepository(mockDB, "test-table", zap.NewNop())
		err := repo.CreateAdminReview(ctx, "event-1", "admin-1", storage.ActionType("remove"), "reason")
		require.Error(t, err)
	})
}

func TestModerationRepository_GetFilter_And_GetFlag_NotFoundBranches(t *testing.T) {
	ctx := context.Background()

	t.Run("GetFilter returns not found when query is empty", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("All", mock.AnythingOfType("*[]models.Filter")).Run(func(args mock.Arguments) {
			target := args.Get(0).(*[]models.Filter)
			*target = nil
		}).Return(nil).Once()
		setupPermissiveDynamormMocks(mockDB, mockQuery)

		repo := NewModerationRepository(mockDB, "test-table", zap.NewNop())
		_, err := repo.GetFilter(ctx, "filter-1")
		require.Error(t, err)
	})

	t.Run("GetFlag returns not found when no matching ID in scan", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("All", mock.AnythingOfType("*[]models.Flag")).Run(func(args mock.Arguments) {
			target := args.Get(0).(*[]models.Flag)
			*target = []models.Flag{{ID: "other"}}
		}).Return(nil).Once()
		setupPermissiveDynamormMocks(mockDB, mockQuery)

		repo := NewModerationRepository(mockDB, "test-table", zap.NewNop())
		_, err := repo.GetFlag(ctx, "flag-1")
		require.Error(t, err)
	})
}

func TestModerationRepository_CreateFlag_And_CreateAuditLog_CreateErrors(t *testing.T) {
	ctx := context.Background()

	t.Run("CreateFlag create error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("Create").Return(ErrTestMockError).Once()
		setupPermissiveDynamormMocks(mockDB, mockQuery)

		repo := NewModerationRepository(mockDB, "test-table", zap.NewNop())
		err := repo.CreateFlag(ctx, &storage.Flag{Actor: "actor-1", Object: []string{"obj-1"}, Content: "reason"})
		require.Error(t, err)
	})

	t.Run("CreateAuditLog create error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("Create").Return(ErrTestMockError).Once()
		setupPermissiveDynamormMocks(mockDB, mockQuery)

		repo := NewModerationRepository(mockDB, "test-table", zap.NewNop())
		err := repo.CreateAuditLog(ctx, &storage.AuditLog{
			AdminID:    "admin-1",
			AdminRole:  "admin",
			Action:     "resolve_report",
			TargetType: "report",
			TargetID:   "report-1",
		})
		require.Error(t, err)
	})
}

func TestModerationRepository_CreateFilter_And_FilterEntityCreateErrors(t *testing.T) {
	ctx := context.Background()

	t.Run("CreateFilter create error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("Create").Return(ErrTestMockError).Once()
		setupPermissiveDynamormMocks(mockDB, mockQuery)

		repo := NewModerationRepository(mockDB, "test-table", zap.NewNop())

		filter := &storage.Filter{
			Username:     "user-1",
			Title:        "t",
			Context:      []string{"home"},
			FilterAction: "hide",
		}

		err := repo.CreateFilter(ctx, filter)
		require.Error(t, err)
		require.NotEmpty(t, filter.ID)
	})

	t.Run("AddFilterKeyword create error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("Create").Return(ErrTestMockError).Once()
		setupPermissiveDynamormMocks(mockDB, mockQuery)

		repo := NewModerationRepository(mockDB, "test-table", zap.NewNop())

		kw := &storage.FilterKeyword{Keyword: "spam", WholeWord: true}
		err := repo.AddFilterKeyword(ctx, "filter-1", kw)
		require.Error(t, err)
		require.NotEmpty(t, kw.ID)
	})

	t.Run("AddFilterStatus create error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("Create").Return(ErrTestMockError).Once()
		setupPermissiveDynamormMocks(mockDB, mockQuery)

		repo := NewModerationRepository(mockDB, "test-table", zap.NewNop())

		status := &storage.FilterStatus{StatusID: "status-1"}
		err := repo.AddFilterStatus(ctx, "filter-1", status)
		require.Error(t, err)
		require.NotEmpty(t, status.ID)
	})
}

func TestModerationRepository_DeleteFlag_DeleteError(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockQuery.On("All", mock.AnythingOfType("*[]models.Flag")).Run(func(args mock.Arguments) {
		target := args.Get(0).(*[]models.Flag)
		*target = []models.Flag{
			{
				ID:        "flag-1",
				Actor:     "actor-1",
				Object:    []string{"obj-1"},
				Content:   "reason",
				Published: time.Now(),
				Status:    string(storage.FlagStatusPending),
				CreatedAt: time.Now(),
			},
		}
	}).Return(nil).Once()

	mockQuery.On("Delete").Return(ErrTestMockError).Once()
	setupPermissiveDynamormMocks(mockDB, mockQuery)

	repo := NewModerationRepository(mockDB, "test-table", zap.NewNop())
	err := repo.DeleteFlag(ctx, "flag-1")
	require.Error(t, err)
}
