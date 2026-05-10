package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

func TestNotificationRepository_GetUserNotificationUsesScopedGSI(t *testing.T) {
	t.Parallel()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Index", "gsi4").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi4PK", "=", "NOTIF_ID#target-notification").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi4SK", "=", "USER#alice").Return(mockQuery).Once()
	mockQuery.On("Limit", 1).Return(mockQuery).Once()

	target := notificationRow("alice", "target-notification", time.Date(2026, time.May, 10, 12, 0, 0, 0, time.UTC))
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		ptr := args.Get(0).(*[]models.Notification)
		*ptr = []models.Notification{target}
	}).Return(nil).Once()

	repo := NewNotificationRepository(mockDB, "test-table", zap.NewNop(), nil)
	got, err := repo.GetUserNotification(context.Background(), "alice", "target-notification")
	require.NoError(t, err)
	require.Equal(t, "target-notification", got.ID)
	require.Equal(t, "USER#alice", got.PK)
	require.Contains(t, got.SK, "target-notification")
	require.Equal(t, "NOTIF_ID#target-notification", got.GSI4PK)
	require.Equal(t, "USER#alice", got.GSI4SK)

	require.Equal(t, 0, countMockCalls(mockQuery, "Where", "PK", "="))
	require.Equal(t, 0, countMockCalls(mockQuery, "Where", "SK", "begins_with"))
	require.Equal(t, 0, countMockCalls(mockQuery, "Where", "SK", "between"))
	require.Equal(t, 1, countMockLimitCalls(mockQuery, 1))
}

func TestNotificationRepository_DeleteUserNotificationDeletesAuthoritativeRow(t *testing.T) {
	t.Parallel()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Index", "gsi4").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi4PK", "=", "NOTIF_ID#delete-me").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi4SK", "=", "USER#alice").Return(mockQuery).Once()
	mockQuery.On("Limit", 1).Return(mockQuery).Once()
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
	require.Equal(t, "NOTIF_ID#delete-me", deletedModel.GSI4PK)
	require.Equal(t, "USER#alice", deletedModel.GSI4SK)
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
