package repositories

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

func TestNotificationRepository_GetUserNotificationPagesUserSortKeyRange(t *testing.T) {
	t.Parallel()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)

	firstPage := make([]models.Notification, 101)
	base := time.Date(2026, time.May, 10, 12, 0, 0, 0, time.UTC)
	for i := range firstPage {
		id := fmt.Sprintf("newer-%03d", i)
		firstPage[i] = notificationRow("alice", id, base.Add(time.Duration(200-i)*time.Second))
	}
	cursorRow := firstPage[99]
	target := notificationRow("alice", "target-notification", base.Add(30*time.Second))

	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		ptr := args.Get(0).(*[]models.Notification)
		*ptr = firstPage
	}).Return(nil).Once()
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		ptr := args.Get(0).(*[]models.Notification)
		*ptr = []models.Notification{
			cursorRow, // inclusive cursor duplicate from BETWEEN query
			target,
		}
	}).Return(nil).Once()

	repo := NewNotificationRepository(mockDB, "test-table", zap.NewNop(), nil)
	got, err := repo.GetUserNotification(context.Background(), "alice", "target-notification")
	require.NoError(t, err)
	require.Equal(t, "target-notification", got.ID)
	require.Equal(t, "USER#alice", got.PK)
	require.Contains(t, got.SK, "target-notification")

	require.Equal(t, 1, countMockCalls(mockQuery, "Where", "SK", "begins_with"))
	require.Equal(t, 1, countMockCalls(mockQuery, "Where", "SK", "between"))
	require.Equal(t, 1, countMockLimitCalls(mockQuery, 101))
	require.Equal(t, 1, countMockLimitCalls(mockQuery, 102))
	for _, call := range mockQuery.Calls {
		if call.Method == "Where" && len(call.Arguments) >= 3 && call.Arguments.Get(0) == "PK" && call.Arguments.Get(1) == "=" {
			require.NotEqual(t, "NOTIFICATION#target-notification", call.Arguments.Get(2))
		}
	}
}

func TestNotificationRepository_DeleteUserNotificationDeletesAuthoritativeRow(t *testing.T) {
	t.Parallel()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)
	mockQuery.On("Delete").Return(nil).Once()

	target := notificationRow("alice", "delete-me", time.Date(2026, time.May, 10, 12, 0, 0, 0, time.UTC))
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		ptr := args.Get(0).(*[]models.Notification)
		*ptr = []models.Notification{target}
	}).Return(nil).Once()

	repo := NewNotificationRepository(mockDB, "test-table", zap.NewNop(), nil)
	require.NoError(t, repo.DeleteUserNotification(context.Background(), "alice", "delete-me"))

	var deletedModel *models.Notification
	for _, call := range mockDB.Calls {
		if call.Method != "Model" || len(call.Arguments) == 0 {
			continue
		}
		if notification, ok := call.Arguments.Get(0).(*models.Notification); ok && notification.ID == "delete-me" {
			deletedModel = notification
		}
	}
	require.NotNil(t, deletedModel)
	require.Equal(t, "USER#alice", deletedModel.PK)
	require.Contains(t, deletedModel.SK, "delete-me")
}

func notificationRow(userID, id string, createdAt time.Time) models.Notification {
	notification := models.Notification{
		ID:        id,
		UserID:    userID,
		Type:      "mention",
		ActorID:   "actor",
		CreatedAt: createdAt,
	}
	_ = notification.BeforeCreate()
	return notification
}
