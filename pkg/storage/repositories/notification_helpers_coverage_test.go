package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

func TestRound07_NotificationQueryHelper_PaginationAndCursor(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)

	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		ptr := args.Get(0).(*[]models.Notification)
		*ptr = []models.Notification{
			{SK: "notif#3"},
			{SK: "notif#2"},
			{SK: "notif#1"},
		}
	}).Return(nil)

	helper := NewNotificationQueryHelper(mockDB, "test-table", zap.NewNop())
	result, err := helper.GetPaginatedNotifications(
		context.Background(),
		"user-1",
		interfaces.PaginationOptions{Limit: 2, Cursor: "notif#99"},
		map[string]interface{}{"IsRead": false},
	)
	require.NoError(t, err)
	require.Len(t, result.Items, 2)
	require.True(t, result.HasMore)
	require.Equal(t, "notif#2", result.NextCursor)
}

func TestRound07_NotificationQueryHelper_QueryErrorIsWrapped(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Return(errors.New("boom"))

	helper := NewNotificationQueryHelper(mockDB, "test-table", zap.NewNop())
	_, err := helper.GetPaginatedNotifications(context.Background(), "user-1", interfaces.PaginationOptions{Limit: 1}, nil)
	require.Error(t, err)
}

func TestRound07_NotificationQueryHelper_LimitDefaultsAndClamp(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Return(nil)

	helper := NewNotificationQueryHelper(mockDB, "test-table", zap.NewNop())

	_, err := helper.GetPaginatedNotifications(context.Background(), "user-1", interfaces.PaginationOptions{Limit: 0}, nil)
	require.NoError(t, err)
	mockQuery.AssertCalled(t, "Limit", 21)

	_, err = helper.GetPaginatedNotifications(context.Background(), "user-1", interfaces.PaginationOptions{Limit: 101}, nil)
	require.NoError(t, err)
	mockQuery.AssertCalled(t, "Limit", 101)
}

func TestRound07_CostTrackingQueryHelper_TimeRangeQuery(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Index", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Return(nil)

	helper := NewCostTrackingQueryHelper(mockDB, "test-table", zap.NewNop())
	_, err := helper.GetCostTrackingByTimeRange(
		context.Background(),
		"gsi1",
		"some-pk",
		"COST",
		time.Unix(0, 0),
		time.Unix(100, 0),
		10,
	)
	require.NoError(t, err)
}

func TestRound07_CostTrackingQueryHelper_TimeRangeQuery_Error(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Index", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Return(errors.New("query-failed"))

	helper := NewCostTrackingQueryHelper(mockDB, "test-table", zap.NewNop())
	_, err := helper.GetCostTrackingByTimeRange(
		context.Background(),
		"gsi9",
		"some-pk",
		"COST",
		time.Unix(0, 0),
		time.Unix(100, 0),
		10,
	)
	require.Error(t, err)
}

func TestRound07_BatchOperationHelper_BatchCreateItems_Branches(t *testing.T) {
	baseTime := time.Unix(1, 0).UTC()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("BatchCreate", mock.Anything).Return(errors.New("batch-failed")).Maybe()

	helper := NewBatchOperationHelper(mockDB, "test-table", zap.NewNop())

	require.NoError(t, helper.BatchCreateItems(context.Background(), nil, "notifications"))
	require.Error(t, helper.BatchCreateItems(context.Background(), []interface{}{"not-supported"}, "notifications"))

	notif := &models.Notification{
		ID:      "n1",
		UserID:  "user-1",
		Type:    "mention",
		ActorID: "https://example.com/users/a1",
	}
	require.NoError(t, helper.BatchCreateItems(context.Background(), []interface{}{notif}, "notifications"))

	tl := &models.Timeline{
		TimelineType: "HOME",
		TimelineID:   "user-1",
		PostID:       "post-1",
		ActorID:      "https://example.com/users/a1",
		ActorHandle:  "@user-1@example.com",
		Content:      "hello",
		ContentType:  "text/plain",
		CreatedAt:    baseTime,
		TimelineAt:   baseTime,
	}
	require.NoError(t, helper.BatchCreateItems(context.Background(), []interface{}{tl}, "timeline"))
}

func TestRound07_BatchOperationHelper_BatchCreateItems_InvalidNotificationAndBatchError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("BatchCreate", mock.Anything).Return(errors.New("batch-failed")).Once()

	helper := NewBatchOperationHelper(mockDB, "test-table", zap.NewNop())

	invalid := &models.Notification{
		ID: "n1",
		// Missing UserID/Type/ActorID triggers BeforeCreate validation errors.
	}
	require.Error(t, helper.BatchCreateItems(context.Background(), []interface{}{invalid}, "notifications"))
}
