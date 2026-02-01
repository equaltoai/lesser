package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormerrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

func TestNotificationCostRepository_CRUDAndQueries_Coverage(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	mockDB := new(mocks.MockDB)
	mockCreateQuery := new(mocks.MockQuery)
	repo := NewNotificationCostRepository(mockDB, "test-table", logger, nil)

	tracking := &models.NotificationCostTracking{
		ID:                      "track-1",
		NotificationID:          "notif-1",
		Username:                "user-1",
		UserID:                  "uid-1",
		DeliveryMethod:          "push",
		NotificationType:        "mention",
		Channel:                 "default",
		Success:                 true,
		RetryCount:              1,
		PushCostMicroCents:      100,
		WebSocketCostMicroCents: 10,
		LambdaCostMicroCents:    20,
		DynamoDBCostMicroCents:  5,
		TotalCostMicroCents:     135,
		ProcessingTimeMs:        10,
		DeliveryTimeMs:          20,
		TotalTimeMs:             30,
		Timestamp:               time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
	}

	mockDB.On("WithContext", ctx).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockCreateQuery).Once()
	mockCreateQuery.On("Create").Return(nil).Once()

	require.NoError(t, repo.CreateCostTracking(ctx, tracking))

	// GetCostTrackingByNotification (All)
	mockNotifQuery := new(mocks.MockQuery)
	mockDB.On("Model", mock.Anything).Return(mockNotifQuery).Once()
	mockNotifQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockNotifQuery).Maybe()
	mockNotifQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockNotifQuery).Once()
	mockNotifQuery.On("Limit", mock.Anything).Return(mockNotifQuery).Once()
	mockNotifQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.NotificationCostTracking)
		*dest = []*models.NotificationCostTracking{tracking}
	}).Return(nil).Once()

	records, err := repo.GetCostTrackingByNotification(ctx, "notif-1", 10)
	require.NoError(t, err)
	require.Len(t, records, 1)

	// GetCostTrackingByUser (CostTrackingQueryHelper)
	mockUserQuery := new(mocks.MockQuery)
	mockDB.On("Model", mock.Anything).Return(mockUserQuery).Once()
	mockUserQuery.On("Index", mock.Anything).Return(mockUserQuery).Once()
	mockUserQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockUserQuery).Maybe()
	mockUserQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockUserQuery).Once()
	mockUserQuery.On("Limit", mock.Anything).Return(mockUserQuery).Once()
	mockUserQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.NotificationCostTracking)
		*dest = []*models.NotificationCostTracking{tracking}
	}).Return(nil).Once()

	byUser, err := repo.GetCostTrackingByUser(ctx, "user-1", tracking.Timestamp.Add(-time.Hour), tracking.Timestamp.Add(time.Hour), 10)
	require.NoError(t, err)
	require.Len(t, byUser, 1)

	// GetDailyCostTracking (GSI3)
	mockDailyQuery := new(mocks.MockQuery)
	mockDB.On("Model", mock.Anything).Return(mockDailyQuery).Once()
	mockDailyQuery.On("Index", mock.Anything).Return(mockDailyQuery).Once()
	mockDailyQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockDailyQuery).Maybe()
	mockDailyQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockDailyQuery).Once()
	mockDailyQuery.On("Limit", mock.Anything).Return(mockDailyQuery).Once()
	mockDailyQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.NotificationCostTracking)
		*dest = []*models.NotificationCostTracking{tracking}
	}).Return(nil).Once()

	daily, err := repo.GetDailyCostTracking(ctx, tracking.Timestamp, 10)
	require.NoError(t, err)
	require.Len(t, daily, 1)

	// Budgets
	mockBudgetCreateQuery := new(mocks.MockQuery)
	mockDB.On("Model", mock.Anything).Return(mockBudgetCreateQuery).Once()
	mockBudgetCreateQuery.On("Create").Return(nil).Once()

	budget := &models.NotificationBudget{
		Username:        "user-1",
		Period:          "daily",
		LimitMicroCents: 10000,
		Enabled:         true,
	}
	require.NoError(t, repo.CreateBudget(ctx, budget))

	mockBudgetUpdateQuery := new(mocks.MockQuery)
	mockDB.On("Model", mock.Anything).Return(mockBudgetUpdateQuery).Once()
	mockBudgetUpdateQuery.On("Update", mock.Anything).Return(nil).Once()
	require.NoError(t, repo.UpdateBudget(ctx, budget))

	mockBudgetGetQuery := new(mocks.MockQuery)
	mockDB.On("Model", mock.Anything).Return(mockBudgetGetQuery).Once()
	mockBudgetGetQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockBudgetGetQuery).Maybe()
	mockBudgetGetQuery.On("First", mock.AnythingOfType("*models.NotificationBudget")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*models.NotificationBudget)
		dest.Username = "user-1"
		dest.Period = "daily"
		dest.LimitMicroCents = 10000
		dest.Enabled = true
	}).Return(nil).Once()
	gotBudget, err := repo.GetBudget(ctx, "user-1", "daily")
	require.NoError(t, err)
	require.Equal(t, int64(10000), gotBudget.LimitMicroCents)

	mockUserBudgetsQuery := new(mocks.MockQuery)
	mockDB.On("Model", mock.Anything).Return(mockUserBudgetsQuery).Once()
	mockUserBudgetsQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockUserBudgetsQuery).Maybe()
	mockUserBudgetsQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockUserBudgetsQuery).Once()
	mockUserBudgetsQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.NotificationBudget)
		*dest = []*models.NotificationBudget{budget}
	}).Return(nil).Once()
	userBudgets, err := repo.GetUserBudgets(ctx, "user-1")
	require.NoError(t, err)
	require.Len(t, userBudgets, 1)

	// ListAggregationsByPeriod
	mockListAggQuery := new(mocks.MockQuery)
	mockDB.On("Model", mock.Anything).Return(mockListAggQuery).Once()
	mockListAggQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockListAggQuery).Maybe()
	mockListAggQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockListAggQuery).Once()
	mockListAggQuery.On("Limit", mock.Anything).Return(mockListAggQuery).Once()
	mockListAggQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.NotificationCostAggregation)
		*dest = []*models.NotificationCostAggregation{{Period: "daily", DeliveryMethod: "push"}}
	}).Return(nil).Once()
	aggs, err := repo.ListAggregationsByPeriod(ctx, "daily", "push", tracking.Timestamp, tracking.Timestamp, 10)
	require.NoError(t, err)
	require.Len(t, aggs, 1)
}

func TestNotificationCostRepository_AggregationAndSummary_Coverage(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewNotificationCostRepository(mockDB, "test-table", logger, nil)

	windowStart := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	windowEnd := windowStart.Add(3 * time.Hour)

	c1 := &models.NotificationCostTracking{
		ID:                     "c1",
		NotificationID:         "n1",
		Username:               "user-1",
		DeliveryMethod:         "push",
		NotificationType:       "mention",
		Channel:                "default",
		Success:                true,
		RetryCount:             1,
		ProcessingTimeMs:       10,
		DeliveryTimeMs:         20,
		TotalTimeMs:            30,
		TotalCostMicroCents:    1000,
		PushCostMicroCents:     500,
		LambdaCostMicroCents:   300,
		DynamoDBCostMicroCents: 200,
		Timestamp:              windowStart.Add(1 * time.Hour),
	}
	c2 := &models.NotificationCostTracking{
		ID:                      "c2",
		NotificationID:          "n2",
		Username:                "user-2",
		DeliveryMethod:          "websocket",
		NotificationType:        "follow",
		Channel:                 "ws",
		Success:                 false,
		RetryCount:              0,
		ProcessingTimeMs:        5,
		DeliveryTimeMs:          10,
		TotalTimeMs:             15,
		TotalCostMicroCents:     2000,
		WebSocketCostMicroCents: 2000,
		Timestamp:               windowStart.Add(2 * time.Hour),
	}

	mockDB.On("WithContext", ctx).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("Index", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Update", mock.Anything).Return(nil).Maybe()

	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		switch dest := args.Get(0).(type) {
		case *models.NotificationCostAggregation:
			dest.Period = "daily"
			dest.DeliveryMethod = "all"
			dest.WindowStart = windowStart
			dest.CreatedAt = windowStart
		default:
			// Leave other dests untouched for this sweep.
		}
	}).Return(nil).Maybe()

	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		switch dest := args.Get(0).(type) {
		case *[]*models.NotificationCostTracking:
			*dest = []*models.NotificationCostTracking{c1, c2}
		case *[]*models.NotificationCostAggregation:
			*dest = []*models.NotificationCostAggregation{{Period: "daily", DeliveryMethod: "all", WindowStart: windowStart}}
		default:
			// Leave other dests untouched for this sweep.
		}
	}).Return(nil).Maybe()

	require.NoError(t, repo.AggregateNotificationCosts(ctx, "daily", "all", windowStart, windowEnd))

	summary, err := repo.GetNotificationCostSummary(ctx, windowStart, windowStart.Add(3*time.Hour))
	require.NoError(t, err)
	require.Equal(t, 2, summary.Count)
	require.Greater(t, summary.TotalCostDollars, 0.0)
	require.NotEmpty(t, summary.DeliveryMethodBreakdown)

	high, err := repo.GetHighCostNotifications(ctx, 1500, windowStart, windowStart.Add(3*time.Hour), 10)
	require.NoError(t, err)
	require.Len(t, high, 1)
	require.Equal(t, "c2", high[0].ID)
}

func TestNotificationCostRepository_UserSpending_And_DailySpending_Coverage(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	mockDB := new(mocks.MockDB)
	repo := NewNotificationCostRepository(mockDB, "test-table", logger, nil)

	mockDB.On("WithContext", ctx).Return(mockDB).Maybe()

	// User spending via helper GetCostTrackingByTimeRange
	mockUserQuery := new(mocks.MockQuery)
	mockDB.On("Model", mock.Anything).Return(mockUserQuery).Once()
	mockUserQuery.On("Index", mock.Anything).Return(mockUserQuery).Once()
	mockUserQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockUserQuery).Maybe()
	mockUserQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockUserQuery).Once()
	mockUserQuery.On("Limit", mock.Anything).Return(mockUserQuery).Once()
	mockUserQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.NotificationCostTracking)
		*dest = []*models.NotificationCostTracking{
			{ID: "a", Username: "user-1", DeliveryMethod: "push", Success: true, TotalCostMicroCents: 100, Timestamp: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)},
			{ID: "b", Username: "user-1", DeliveryMethod: "push", Success: false, TotalCostMicroCents: 300, Timestamp: time.Date(2025, 1, 1, 1, 0, 0, 0, time.UTC)},
			{ID: "c", Username: "user-1", DeliveryMethod: "websocket", Success: true, TotalCostMicroCents: 200, Timestamp: time.Date(2025, 1, 1, 2, 0, 0, 0, time.UTC)},
		}
	}).Return(nil).Once()

	summary, err := repo.GetUserSpending(ctx, "user-1", time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2025, 1, 1, 3, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.Equal(t, int64(3), summary.TotalNotifications)
	require.NotEmpty(t, summary.DeliveryMethodBreakdown)

	// Daily spending: not found returns 0, nil
	mockDailySpendQuery := new(mocks.MockQuery)
	mockDB.On("Model", mock.Anything).Return(mockDailySpendQuery).Once()
	mockDailySpendQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockDailySpendQuery).Maybe()
	mockDailySpendQuery.On("All", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()

	cost, err := repo.GetDailySpending(ctx, "user-1")
	require.NoError(t, err)
	require.Equal(t, int64(0), cost)

	// Daily spending: sums costs
	mockDailySpendQuery2 := new(mocks.MockQuery)
	mockDB.On("Model", mock.Anything).Return(mockDailySpendQuery2).Once()
	mockDailySpendQuery2.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockDailySpendQuery2).Maybe()
	mockDailySpendQuery2.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.NotificationCostTracking)
		*dest = []models.NotificationCostTracking{
			{TotalCostMicroCents: 100},
			{TotalCostMicroCents: 300},
		}
	}).Return(nil).Once()

	cost, err = repo.GetDailySpending(ctx, "user-1")
	require.NoError(t, err)
	require.Equal(t, int64(400), cost)
}

func TestNotificationCostRepository_ErrorBranches_Coverage(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	mockDB := new(mocks.MockDB)
	repo := NewNotificationCostRepository(mockDB, "test-table", logger, nil)

	mockDB.On("WithContext", ctx).Return(mockDB).Maybe()

	// GetCostTrackingByNotification query error
	mockNotifQuery := new(mocks.MockQuery)
	mockDB.On("Model", mock.Anything).Return(mockNotifQuery).Once()
	mockNotifQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockNotifQuery).Maybe()
	mockNotifQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockNotifQuery).Maybe()
	mockNotifQuery.On("Limit", mock.Anything).Return(mockNotifQuery).Maybe()
	mockNotifQuery.On("All", mock.Anything).Return(errors.New("query failed")).Once()

	_, err := repo.GetCostTrackingByNotification(ctx, "notif", 10)
	require.Error(t, err)

	// GetBudget not found error path
	mockBudgetGetQuery := new(mocks.MockQuery)
	mockDB.On("Model", mock.Anything).Return(mockBudgetGetQuery).Once()
	mockBudgetGetQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockBudgetGetQuery).Maybe()
	mockBudgetGetQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()

	_, err = repo.GetBudget(ctx, "user-1", "daily")
	require.Error(t, err)

	// CreateAggregation create error path
	mockAggCreateQuery := new(mocks.MockQuery)
	mockDB.On("Model", mock.Anything).Return(mockAggCreateQuery).Once()
	mockAggCreateQuery.On("Create").Return(errors.New("create failed")).Once()
	err = repo.CreateAggregation(ctx, &models.NotificationCostAggregation{Period: "daily", DeliveryMethod: "push", WindowStart: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)})
	require.Error(t, err)
}
