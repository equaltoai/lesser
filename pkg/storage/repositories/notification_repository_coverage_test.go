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
	ddbErrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func countMockCalls(q *mocks.MockQuery, method, field, op string) int {
	count := 0
	for _, call := range q.Calls {
		if call.Method != method || len(call.Arguments) < 2 {
			continue
		}
		if gotField, ok := call.Arguments.Get(0).(string); !ok || gotField != field {
			continue
		}
		if gotOp, ok := call.Arguments.Get(1).(string); !ok || gotOp != op {
			continue
		}
		count++
	}

	return count
}

type fakeNotificationDispatcher struct {
	calls int
}

func (f *fakeNotificationDispatcher) DispatchPushForNotification(_ context.Context, _ *models.Notification) {
	f.calls++
}

func TestRound07_NotificationRepository_SweepSuccess(t *testing.T) {
	baseTime := time.Unix(1, 0).UTC()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupPermissiveRound07Mocks(mockDB, mockQuery, nil, baseTime)

	repo := NewNotificationRepository(mockDB, "test-table", zap.NewNop(), nil)
	dispatcher := &fakeNotificationDispatcher{}
	repo.SetDispatcher(dispatcher)

	ctx := context.Background()

	notification := &models.Notification{
		ID:      "n1",
		UserID:  "user-1",
		Type:    "mention",
		ActorID: "https://example.com/users/a1",
		Title:   "test notification",
	}
	require.NoError(t, repo.CreateNotification(ctx, notification))
	require.NotZero(t, dispatcher.calls)

	_, _ = repo.GetNotification(ctx, "n1")
	require.NoError(t, repo.UpdateNotification(ctx, notification))
	require.NoError(t, repo.DeleteNotification(ctx, "n1"))

	_, _ = repo.GetUserNotifications(ctx, "user-1", interfaces.PaginationOptions{Limit: 1})
	_, _ = repo.GetUnreadNotifications(ctx, "user-1", interfaces.PaginationOptions{Limit: 1})
	_, _ = repo.GetNotificationsByType(ctx, "user-1", "mention", interfaces.PaginationOptions{Limit: 1})

	_ = repo.MarkNotificationRead(ctx, "n1")
	_ = repo.MarkNotificationUnread(ctx, "n1")
	_ = repo.MarkAllNotificationsRead(ctx, "user-1")
	_ = repo.MarkNotificationsReadByType(ctx, "user-1", "mention")
	_ = repo.MarkNotificationPushSent(ctx, "n1")
	_ = repo.MarkNotificationPushFailed(ctx, "n1", "error")

	_, _ = repo.GetPendingPushNotifications(ctx, interfaces.PaginationOptions{Limit: 1})
	_, _ = repo.GetNotificationGroups(ctx, "user-1", interfaces.PaginationOptions{Limit: 1})
	_ = repo.ConsolidateNotifications(ctx, "group-1")

	_, _ = repo.GetUnreadNotificationCount(ctx, "user-1")
	_, _ = repo.GetNotificationCountsByType(ctx, "user-1")

	require.NoError(t, repo.CreateNotifications(ctx, []*models.Notification{{
		ID:      "n2",
		UserID:  "user-1",
		Type:    "mention",
		ActorID: "https://example.com/users/a1",
		Title:   "test notification 2",
	}}))
	require.NoError(t, repo.DeleteNotificationsByType(ctx, "user-1", "mention"))
	require.NoError(t, repo.DeleteNotificationsByObject(ctx, "object-1"))
	_, _ = repo.DeleteExpiredNotifications(ctx, time.Unix(0, 0))

	_, _, _ = repo.GetNotificationsFiltered(ctx, "user-1", map[string]interface{}{"include_read": false, "types": []string{"mention"}, "limit": 1})
	_, _ = repo.ClearOldNotifications(ctx, "user-1", time.Now().Add(24*time.Hour))
	_, _ = repo.GetNotificationsAdvanced(ctx, "user-1", map[string]interface{}{"Type": "mention"}, interfaces.PaginationOptions{Limit: 1})

	prefs, err := repo.GetNotificationPreferences(ctx, "user-1")
	require.NoError(t, err)
	require.NotNil(t, prefs)
	require.NoError(t, repo.UpdateNotificationPreferences(ctx, prefs))
	require.NoError(t, repo.SetNotificationPreference(ctx, "user-1", "push_mention", false))
}

func TestRound07_NotificationRepository_ByTypeFallbackAndPreferencesDefault(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Index", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)

	mockQuery.On("All", mock.Anything).Return(errors.New("index not found")).Once()
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		ptr := args.Get(0).(*[]models.Notification)
		*ptr = []models.Notification{
			{Type: "mention", SK: "notif#2"},
			{Type: "follow", SK: "notif#1"},
		}
	}).Return(nil).Once()

	mockQuery.On("First", mock.Anything).Return(ddbErrors.ErrItemNotFound).Once()

	repo := NewNotificationRepository(mockDB, "test-table", zap.NewNop(), nil)
	result, err := repo.GetNotificationsByType(context.Background(), "user-1", "mention", interfaces.PaginationOptions{Limit: 10})
	require.NoError(t, err)
	require.Len(t, result.Items, 1)

	prefs, err := repo.GetNotificationPreferences(context.Background(), "user-1")
	require.NoError(t, err)
	require.True(t, prefs.EmailEnabled)
}

func TestRound07_NotificationRepository_SetNotificationPreference_UnknownType(t *testing.T) {
	baseTime := time.Unix(1, 0).UTC()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupPermissiveRound07Mocks(mockDB, mockQuery, nil, baseTime)

	repo := NewNotificationRepository(mockDB, "test-table", zap.NewNop(), nil)
	err := repo.SetNotificationPreference(context.Background(), "user-1", "unknown", true)
	require.Error(t, err)
}

func TestRound07_NotificationRepository_SetNotificationPreference_AllKnownTypes(t *testing.T) {
	baseTime := time.Unix(1, 0).UTC()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupPermissiveRound07Mocks(mockDB, mockQuery, nil, baseTime)

	repo := NewNotificationRepository(mockDB, "test-table", zap.NewNop(), nil)
	ctx := context.Background()

	preferenceTypes := []string{
		"email_follow",
		"email_reblog",
		"email_mention",
		"push_follow",
		"push_reblog",
		"push_mention",
	}

	for _, preferenceType := range preferenceTypes {
		require.NoError(t, repo.SetNotificationPreference(ctx, "user-1", preferenceType, true))
	}
}

func TestRound07_NotificationRepository_GetNotificationCountsByType_PaginatesAndHandlesNotFound(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)

	// First call: full batch (forces cursor branch).
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		ptr := args.Get(0).(*[]models.Notification)
		batch := make([]models.Notification, 500)
		for i := range batch {
			batch[i] = models.Notification{Type: "mention", SK: "notif#cursor"}
		}
		*ptr = batch
	}).Return(nil).Once()

	// Second call: empty slice breaks.
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		ptr := args.Get(0).(*[]models.Notification)
		*ptr = nil
	}).Return(nil).Once()

	repo := NewNotificationRepository(mockDB, "test-table", zap.NewNop(), nil)
	counts, err := repo.GetNotificationCountsByType(context.Background(), "user-1")
	require.NoError(t, err)
	require.Equal(t, int64(500), counts["mention"])

	// NotFound breaks without error.
	mockQuery.On("All", mock.Anything).Return(ddbErrors.ErrItemNotFound).Once()
	counts, err = repo.GetNotificationCountsByType(context.Background(), "user-1")
	require.NoError(t, err)
	require.NotNil(t, counts)
}

func TestRound07_NotificationRepository_MarkAllRead_LogsOnUpdateError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		ptr := args.Get(0).(*[]models.Notification)
		*ptr = []models.Notification{
			{ID: "n1", UserID: "user-1"},
			{ID: "n2", UserID: "user-1"},
		}
	}).Return(nil).Once()

	mockQuery.On("Update", mock.Anything).Return(errors.New("update-failed")).Once()
	mockQuery.On("Update", mock.Anything).Return(nil).Once()

	repo := NewNotificationRepository(mockDB, "test-table", zap.NewNop(), nil)
	require.NoError(t, repo.MarkAllNotificationsRead(context.Background(), "user-1"))
}

func TestRound07_NotificationRepository_ConsolidateNotifications_WarnsOnLimitAndUpdateDeleteErrors(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Index", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)

	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		ptr := args.Get(0).(*[]models.Notification)
		batch := make([]models.Notification, 100)
		now := time.Unix(10, 0).UTC()
		for i := range batch {
			batch[i] = models.Notification{
				ID:        "n",
				UserID:    "user-1",
				Type:      "mention",
				ActorID:   "https://example.com/users/a1",
				GroupKey:  "group-1",
				CreatedAt: now.Add(time.Duration(i) * time.Second),
			}
		}
		*ptr = batch
	}).Return(nil).Once()

	mockQuery.On("Update", mock.Anything).Return(errors.New("update-failed")).Once()
	mockQuery.On("Delete").Return(errors.New("delete-failed")).Maybe()

	repo := NewNotificationRepository(mockDB, "test-table", zap.NewNop(), nil)
	require.NoError(t, repo.ConsolidateNotifications(context.Background(), "group-1"))
}

func TestRound07_NotificationRepository_PendingPushNotifications_PaginationBranches(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)

	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		ptr := args.Get(0).(*[]models.Notification)
		now := time.Unix(10, 0).UTC()
		notifications := make([]models.Notification, 101)
		for i := range notifications {
			notifications[i] = models.Notification{CreatedAt: now.Add(time.Duration(i) * time.Second)}
		}
		*ptr = notifications
	}).Return(nil).Once()

	repo := NewNotificationRepository(mockDB, "test-table", zap.NewNop(), nil)
	result, err := repo.GetPendingPushNotifications(context.Background(), interfaces.PaginationOptions{Limit: 0, Cursor: "1"})
	require.NoError(t, err)
	require.True(t, result.HasMore)
	require.NotEmpty(t, result.NextCursor)
	require.Len(t, result.Items, 100)

	// Clamp to 1000.
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		ptr := args.Get(0).(*[]models.Notification)
		*ptr = []models.Notification{}
	}).Return(nil).Once()
	_, err = repo.GetPendingPushNotifications(context.Background(), interfaces.PaginationOptions{Limit: 1001})
	require.NoError(t, err)
}

func TestRound07_NotificationRepository_CreateNotificationAndUpdateNotification_ErrorPaths(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Create").Return(errors.New("create-failed")).Once()

	repo := NewNotificationRepository(mockDB, "test-table", zap.NewNop(), nil)

	invalid := &models.Notification{}
	require.Error(t, repo.CreateNotification(context.Background(), invalid))

	valid := &models.Notification{
		ID:      "n1",
		UserID:  "user-1",
		Type:    "mention",
		ActorID: "https://example.com/users/a1",
	}
	require.Error(t, repo.CreateNotification(context.Background(), valid))

	invalidUpdate := &models.Notification{
		ID:      "n1",
		UserID:  "user-1",
		Type:    "not-a-type",
		ActorID: "https://example.com/users/a1",
	}
	require.Error(t, repo.UpdateNotification(context.Background(), invalidUpdate))
}

func TestRound07_NotificationRepository_CreateNotifications_DeleteByType_Object_Expired_ErrorBranches(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()

	repo := NewNotificationRepository(mockDB, "test-table", zap.NewNop(), nil)

	require.NoError(t, repo.CreateNotifications(context.Background(), nil))
	require.Error(t, repo.CreateNotifications(context.Background(), []*models.Notification{{UserID: "user-1"}}))

	mockQuery.On("BatchCreate", mock.Anything).Return(errors.New("batch-failed")).Once()
	require.Error(t, repo.CreateNotifications(context.Background(), []*models.Notification{{
		ID:      "n1",
		UserID:  "user-1",
		Type:    "mention",
		ActorID: "https://example.com/users/a1",
	}}))

	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		ptr := args.Get(0).(*[]models.Notification)
		*ptr = nil
	}).Return(nil).Once()
	require.NoError(t, repo.DeleteNotificationsByType(context.Background(), "user-1", "mention"))

	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		ptr := args.Get(0).(*[]models.Notification)
		*ptr = []models.Notification{{PK: "USER#user-1", SK: "notif#1"}}
	}).Return(nil).Once()
	mockQuery.On("BatchDelete", mock.Anything).Return(errors.New("batch-delete-failed")).Once()
	require.Error(t, repo.DeleteNotificationsByType(context.Background(), "user-1", "mention"))

	mockQuery.On("All", mock.Anything).Return(ddbErrors.ErrItemNotFound).Once()
	require.NoError(t, repo.DeleteNotificationsByObject(context.Background(), "object-1"))

	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		ptr := args.Get(0).(*[]models.Notification)
		*ptr = []models.Notification{{PK: "USER#user-1", SK: "notif#1"}}
	}).Return(nil).Once()
	mockQuery.On("BatchDelete", mock.Anything).Return(errors.New("batch-delete-failed")).Once()
	require.Error(t, repo.DeleteNotificationsByObject(context.Background(), "object-1"))

	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		ptr := args.Get(0).(*[]models.Notification)
		*ptr = nil
	}).Return(nil).Once()
	deleted, err := repo.DeleteExpiredNotifications(context.Background(), time.Unix(1, 0).UTC())
	require.NoError(t, err)
	require.Equal(t, int64(0), deleted)
}

func TestRound07_NotificationRepository_UnreadCountAndMarkRead_ErrorBranches(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("Count").Return(int64(0), errors.New("count-failed")).Once()
	repo := NewNotificationRepository(mockDB, "test-table", zap.NewNop(), nil)
	_, err := repo.GetUnreadNotificationCount(context.Background(), "user-1")
	require.Error(t, err)

	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("First", mock.Anything).Return(errors.New("get-failed")).Once()
	require.Error(t, repo.MarkNotificationRead(context.Background(), "n1"))
}

func TestRound07_NotificationRepository_DeleteNotification_GetAndDeleteErrors(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()

	// GetNotification error path.
	mockQuery.On("First", mock.Anything).Return(errors.New("get-failed")).Once()
	repo := NewNotificationRepository(mockDB, "test-table", zap.NewNop(), nil)
	require.Error(t, repo.DeleteNotification(context.Background(), "n1"))

	// Delete error path.
	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		notif := args.Get(0).(*models.Notification)
		notif.ID = "n1"
		notif.UserID = "user-1"
		notif.Type = "mention"
		notif.ActorID = "https://example.com/users/a1"
		notif.PK = "NOTIFICATION#n1"
		notif.SK = models.SKMetadata
	}).Return(nil).Once()
	mockQuery.On("Delete").Return(errors.New("delete-failed")).Once()
	require.Error(t, repo.DeleteNotification(context.Background(), "n1"))
}

func TestRound07_NotificationRepository_UserNotifications_PaginationBranches(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		ptr := args.Get(0).(*[]models.Notification)
		*ptr = []models.Notification{
			{SK: "notif#2", UserID: "user-1"},
			{SK: "notif#1", UserID: "user-1"},
		}
	}).Return(nil).Once()

	repo := NewNotificationRepository(mockDB, "test-table", zap.NewNop(), nil)
	result, err := repo.GetUserNotifications(context.Background(), "user-1", interfaces.PaginationOptions{Limit: 1, Cursor: "notif#99"})
	require.NoError(t, err)
	require.True(t, result.HasMore)
	require.Equal(t, "notif#2", result.NextCursor)
	require.Equal(t, 1, countMockCalls(mockQuery, "Where", "SK", "<"))
	require.Zero(t, countMockCalls(mockQuery, "Filter", "SK", "begins_with"))

	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		ptr := args.Get(0).(*[]models.Notification)
		*ptr = []models.Notification{
			{SK: "notif#2", UserID: "user-1"},
			{SK: "notif#1", UserID: "user-1"},
		}
	}).Return(nil).Once()
	result, err = repo.GetUnreadNotifications(context.Background(), "user-1", interfaces.PaginationOptions{Limit: 1, Cursor: "notif#99"})
	require.NoError(t, err)
	require.True(t, result.HasMore)
	require.Equal(t, "notif#2", result.NextCursor)
	require.Equal(t, 2, countMockCalls(mockQuery, "Where", "SK", "<"))
	require.Zero(t, countMockCalls(mockQuery, "Filter", "SK", "begins_with"))
	require.Equal(t, 1, countMockCalls(mockQuery, "Filter", "IsRead", "="))
}

func TestRound07_NotificationRepository_GetUserNotifications_LogsQueryError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()

	queryErr := errors.New("builder failed")
	mockQuery.On("All", mock.Anything).Return(queryErr).Once()

	core, observed := observer.New(zapcore.ErrorLevel)
	repo := NewNotificationRepository(mockDB, "test-table", zap.New(core), nil)

	result, err := repo.GetUserNotifications(context.Background(), "user-1", interfaces.PaginationOptions{Limit: 1})
	require.Nil(t, result)
	require.Error(t, err)

	entries := observed.All()
	require.Len(t, entries, 1)
	require.Equal(t, "GetUserNotifications query error", entries[0].Message)

	fields := entries[0].ContextMap()
	require.Equal(t, "user-1", fields["user_id"])
	require.Equal(t, queryErr.Error(), fields["error"])
}

func TestRound07_NotificationRepository_NotificationQueries_DropSortKeyPrefixFilterWhenCursorPresent(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		ptr := args.Get(0).(*[]models.Notification)
		*ptr = []models.Notification{{SK: "notif#2", UserID: "user-1"}}
	}).Return(nil).Times(2)

	repo := NewNotificationRepository(mockDB, "test-table", zap.NewNop(), nil)

	_, err := repo.GetNotificationGroups(context.Background(), "user-1", interfaces.PaginationOptions{Limit: 1, Cursor: "notif#99"})
	require.NoError(t, err)

	_, err = repo.GetNotificationsAdvanced(context.Background(), "user-1", map[string]interface{}{"Type": "mention"}, interfaces.PaginationOptions{Limit: 1, Cursor: "notif#99"})
	require.NoError(t, err)

	require.Equal(t, 2, countMockCalls(mockQuery, "Where", "SK", "<"))
	require.Zero(t, countMockCalls(mockQuery, "Filter", "SK", "begins_with"))
	require.Equal(t, 1, countMockCalls(mockQuery, "Filter", "Type", "="))
}

func TestRound07_NotificationRepository_MarkUnreadAndPush_GetErrorBranches(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("First", mock.Anything).Return(errors.New("get-failed")).Maybe()

	repo := NewNotificationRepository(mockDB, "test-table", zap.NewNop(), nil)
	require.Error(t, repo.MarkNotificationUnread(context.Background(), "n1"))
	require.Error(t, repo.MarkNotificationPushSent(context.Background(), "n1"))
	require.Error(t, repo.MarkNotificationPushFailed(context.Background(), "n1", "err"))
}

func TestRound07_NotificationRepository_GetPreferencesAndUpdatePreferences_ErrorBranches(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("First", mock.Anything).Return(errors.New("prefs-failed")).Once()
	repo := NewNotificationRepository(mockDB, "test-table", zap.NewNop(), nil)
	_, err := repo.GetNotificationPreferences(context.Background(), "user-1")
	require.Error(t, err)

	mockQuery.On("Update", mock.Anything).Return(errors.New("update-failed")).Once()
	require.Error(t, repo.UpdateNotificationPreferences(context.Background(), &models.NotificationPreferences{Username: "user-1"}))
}

func TestRound07_NotificationRepository_ClearOldNotifications_ContinuesOnDeleteError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()

	old := time.Unix(1, 0).UTC()
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		ptr := args.Get(0).(*[]models.Notification)
		*ptr = []models.Notification{
			{ID: "n1", CreatedAt: old},
			{ID: "n2", CreatedAt: old},
		}
	}).Return(nil).Once()

	// DeleteNotification does a Get (First) then Delete; both IDs will fail delete.
	mockQuery.On("First", mock.Anything).Return(nil).Maybe()
	mockQuery.On("Delete").Return(errors.New("delete-failed")).Maybe()

	repo := NewNotificationRepository(mockDB, "test-table", zap.NewNop(), nil)
	deleted, err := repo.ClearOldNotifications(context.Background(), "user-1", time.Unix(2, 0).UTC())
	require.NoError(t, err)
	require.Equal(t, 0, deleted)
}
