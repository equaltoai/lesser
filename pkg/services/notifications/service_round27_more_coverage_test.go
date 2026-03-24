package notifications

import (
	"context"
	"errors"
	"testing"
	"time"

	notifpush "github.com/equaltoai/lesser/pkg/notifications"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

type testStringer struct{ value string }

func (t testStringer) String() string { return t.value }

func TestExtractUsernameFromIdentifier_round27_coverage(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "", extractUsernameFromIdentifier(""))
	assert.Equal(t, "alice", extractUsernameFromIdentifier("  @alice  "))
	assert.Equal(t, "alice", extractUsernameFromIdentifier("https://example.com/users/alice"))
	assert.Equal(t, "example.com", extractUsernameFromIdentifier("https://example.com"))
	assert.Equal(t, "bob", extractUsernameFromIdentifier("bob@remote.example"))
	assert.Equal(t, "carol", extractUsernameFromIdentifier("carol"))
}

func TestResolveActorDisplayName_round27_coverage(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "Someone", resolveActorDisplayName(nil, ""))
	assert.Equal(t, "alice", resolveActorDisplayName(nil, "https://example.com/users/alice"))

	actor := &storage.Account{User: &storage.User{DisplayName: " Alice "}}
	assert.Equal(t, "Alice", resolveActorDisplayName(actor, "ignored"))

	actor = &storage.Account{User: &storage.User{Username: "bob"}}
	assert.Equal(t, "bob", resolveActorDisplayName(actor, "ignored"))
}

func TestExtractNotificationDataHelpers_round27_coverage(t *testing.T) {
	t.Parallel()

	notif := &models.Notification{
		Body: "",
		Data: map[string]any{
			"content":      "  hello  ",
			"icon_url":     testStringer{value: " https://cdn.example/icon.png "},
			"access_token": " tok ",
		},
	}

	assert.Equal(t, "hello", extractNotificationContent(notif))
	assert.Equal(t, "https://cdn.example/icon.png", extractNotificationIcon(notif))
	assert.Equal(t, "tok", extractNotificationAccessToken(notif))
	assert.Equal(t, "", extractNotificationContent(nil))
	assert.Equal(t, "", extractNotificationIcon(&models.Notification{}))
	assert.Equal(t, "", extractNotificationAccessToken(&models.Notification{}))
}

func TestService_queuePushNotification_round27_coverage(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("returns_early_when_push_disabled", func(t *testing.T) {
		service := NewService(nil, nil, nil, zap.NewNop(), "example.com", nil)
		service.queuePushNotification(ctx, &storage.Account{User: &storage.User{Username: "alice"}}, nil, &models.Notification{ID: "n1"})
	})

	t.Run("returns_early_when_username_missing", func(t *testing.T) {
		push := &fakePushService{}
		service := NewService(nil, nil, nil, zap.NewNop(), "example.com", push)
		service.queuePushNotification(ctx, &storage.Account{User: &storage.User{Username: "  "}}, nil, &models.Notification{ID: "n1"})
		assert.Empty(t, push.Messages())
	})

	t.Run("formats_title_and_body_when_missing", func(t *testing.T) {
		push := &fakePushService{err: errors.New("boom")}
		service := NewService(nil, nil, nil, zap.NewNop(), "example.com", push)

		recipient := &storage.Account{User: &storage.User{Username: "alice"}}
		notification := &models.Notification{
			ID:      "n1",
			UserID:  "alice",
			Type:    "mention",
			ActorID: "https://example.com/users/bob",
			Title:   "",
			Body:    "",
			Data: map[string]any{
				"content":      "hello",
				"icon":         "https://cdn.example/icon.png",
				"access_token": "token",
			},
		}

		service.queuePushNotification(ctx, recipient, nil, notification)
		require.Len(t, push.Messages(), 1)

		msg := push.Messages()[0]
		assert.Equal(t, "alice", msg.Username)
		assert.Equal(t, "mention", msg.NotificationType)
		assert.Equal(t, notifpush.FormatNotificationTitle(notification.Type, "bob"), msg.Title)
		assert.Equal(t, notifpush.FormatNotificationBody(notification.Type, "hello"), msg.Body)
		assert.Equal(t, "https://cdn.example/icon.png", msg.Icon)
		assert.Equal(t, "n1", msg.NotificationID)
		assert.Equal(t, "token", msg.AccessToken)
	})
}

func TestService_DispatchPushForNotification_round27_coverage(t *testing.T) {
	t.Parallel()

	service, _, accountRepo, _, pushService := setupTestService()
	ctx := context.Background()

	// recipient: url identifier -> username extracted
	accountRepo.On("GetAccount", ctx, "https://example.com/users/alice").Return(nil, assert.AnError).Once()
	accountRepo.On("GetAccount", ctx, "alice").Return(&storage.Account{User: &storage.User{Username: "alice"}}, nil).Once()

	// actor: username@domain tries username twice (extract + split fallback)
	accountRepo.On("GetAccount", ctx, "bob@remote.example").Return(nil, assert.AnError).Once()
	accountRepo.On("GetAccount", ctx, "bob").Return(nil, assert.AnError).Once()
	accountRepo.On("GetAccount", ctx, "bob").Return(&storage.Account{User: &storage.User{Username: "bob", DisplayName: "Bob"}}, nil).Once()

	service.DispatchPushForNotification(ctx, &models.Notification{
		ID:      "n1",
		UserID:  "https://example.com/users/alice",
		ActorID: "bob@remote.example",
		Type:    "follow",
		Title:   "followed",
		Body:    "hi",
	})

	require.Len(t, pushService.Messages(), 1)
	assert.Equal(t, "alice", pushService.Messages()[0].Username)

	accountRepo.AssertExpectations(t)
}

func TestService_clearHelpers_round27_coverage(t *testing.T) {
	t.Parallel()

	service, notificationRepo, _, _, _ := setupTestService()
	ctx := context.Background()

	t.Run("clear_all_with_older_than_uses_delete_expired", func(t *testing.T) {
		notificationRepo.On("DeleteExpiredNotifications", ctx, mock.MatchedBy(func(ts time.Time) bool {
			return ts.Before(time.Now().Add(-30 * time.Second))
		})).Return(int64(5), nil).Once()

		result, err := service.ClearNotifications(ctx, &ClearCommand{UserID: "alice", ClearAll: true, OlderThanSeconds: 60})
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, int64(5), result.ClearedCount)
	})

	t.Run("clear_specific_skips_missing_wrong_owner_and_delete_errors", func(t *testing.T) {
		mine := &models.Notification{ID: "ok", UserID: "alice"}
		other := &models.Notification{ID: "other", UserID: "bob"}

		notificationRepo.On("GetNotification", ctx, "missing").Return(nil, assert.AnError).Once()
		notificationRepo.On("GetNotification", ctx, "other").Return(other, nil).Once()
		notificationRepo.On("GetNotification", ctx, "faildelete").Return(&models.Notification{ID: "faildelete", UserID: "alice"}, nil).Once()
		notificationRepo.On("DeleteNotification", ctx, "faildelete").Return(assert.AnError).Once()
		notificationRepo.On("GetNotification", ctx, "ok").Return(mine, nil).Once()
		notificationRepo.On("DeleteNotification", ctx, "ok").Return(nil).Once()

		result, err := service.ClearNotifications(ctx, &ClearCommand{UserID: "alice", NotificationIDs: []string{"missing", "other", "faildelete", "ok"}})
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, int64(1), result.ClearedCount)
	})

	t.Run("clear_by_type_with_errors_counts_only_successes", func(t *testing.T) {
		notificationRepo.On("MarkNotificationsReadByType", ctx, "alice", "mention").Return(nil).Once()
		notificationRepo.On("MarkNotificationsReadByType", ctx, "alice", "follow").Return(assert.AnError).Once()

		result, err := service.ClearNotifications(ctx, &ClearCommand{UserID: "alice", Types: []string{"mention", "follow"}, OlderThanSeconds: 60})
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, int64(1), result.ClearedCount)
	})
}

func TestService_getNotificationSummary_round27_error_paths(t *testing.T) {
	t.Parallel()

	service, notificationRepo, _, _, _ := setupTestService()
	ctx := context.Background()

	t.Run("unread_count_error", func(t *testing.T) {
		notificationRepo.On("GetUnreadNotificationCount", ctx, "alice").Return(int64(0), assert.AnError).Once()
		_, err := service.getNotificationSummary(ctx, "alice")
		assert.ErrorIs(t, err, ErrUnreadCountFailed)
	})

	t.Run("counts_by_type_error", func(t *testing.T) {
		notificationRepo.On("GetUnreadNotificationCount", ctx, "bob").Return(int64(0), nil).Once()
		notificationRepo.On("GetNotificationCountsByType", ctx, "bob").Return(nil, assert.AnError).Once()
		_, err := service.getNotificationSummary(ctx, "bob")
		assert.ErrorIs(t, err, ErrCountsByTypeFailed)
	})

	t.Run("success_without_recent_notification_time", func(t *testing.T) {
		notificationRepo.On("GetUnreadNotificationCount", ctx, "carol").Return(int64(1), nil).Once()
		notificationRepo.On("GetNotificationCountsByType", ctx, "carol").Return(map[string]int64{"mention": 2}, nil).Once()
		notificationRepo.On("GetUserNotifications", ctx, "carol", interfaces.PaginationOptions{Limit: 1}).Return(nil, assert.AnError).Once()

		summary, err := service.getNotificationSummary(ctx, "carol")
		require.NoError(t, err)
		require.NotNil(t, summary)
		assert.Equal(t, int64(2), summary.TotalCount)
		assert.Nil(t, summary.LastNotificationAt)
	})
}

func TestService_applyNotificationFilters_round27_coverage(t *testing.T) {
	t.Parallel()

	service := &Service{logger: zap.NewNop()}
	all := []*models.Notification{
		{ID: "n1", Type: "mention", ActorID: "a1", TargetType: "status", IsRead: false, GroupCount: 1},
		{ID: "n2", Type: "follow", ActorID: "a2", TargetType: "user", IsRead: true, GroupCount: 2},
		{ID: "n3", Type: "reblog", ActorID: "a1", TargetType: "status", IsRead: false, GroupCount: 5},
	}

	filtered := service.applyNotificationFilters(all, &ListNotificationsQuery{
		ExcludeTypes: []string{"follow"},
		ActorID:      "a1",
		TargetType:   "status",
		GroupedOnly:  true,
		IncludeRead:  true,
	})
	require.Len(t, filtered, 1)
	assert.Equal(t, "n3", filtered[0].ID)

	filtered = service.applyNotificationFilters(all, &ListNotificationsQuery{
		OnlyUnread:  true,
		IncludeRead: true,
	})
	assert.Equal(t, 2, len(filtered))
}

func TestService_ListNotifications_round27_error_paths(t *testing.T) {
	t.Parallel()

	service, notificationRepo, _, _, _ := setupTestService()
	ctx := context.Background()

	notificationRepo.On("GetUserNotifications", ctx, "alice", mock.AnythingOfType("interfaces.PaginationOptions")).Return(nil, assert.AnError).Once()

	_, err := service.ListNotifications(ctx, &ListNotificationsQuery{UserID: "alice"})
	assert.ErrorIs(t, err, ErrNotificationQueryFailed)
}

func TestService_ListNotifications_logsUnderlyingQueryError_round27(t *testing.T) {
	t.Parallel()

	service, notificationRepo, _, _, _ := setupTestService()
	ctx := context.Background()

	core, observed := observer.New(zapcore.ErrorLevel)
	service.logger = zap.New(core)

	queryErr := errors.New("builder failed")
	notificationRepo.On("GetUserNotifications", ctx, "alice", mock.AnythingOfType("interfaces.PaginationOptions")).Return(nil, queryErr).Once()

	_, err := service.ListNotifications(ctx, &ListNotificationsQuery{
		UserID:       "alice",
		Types:        nil,
		ExcludeTypes: []string{"follow"},
		OnlyUnread:   false,
		IncludeRead:  false,
		ActorID:      "bob",
		TargetType:   "status",
	})
	require.ErrorIs(t, err, ErrNotificationQueryFailed)

	entries := observed.All()
	require.Len(t, entries, 1)
	assert.Equal(t, "notification query failed", entries[0].Message)
	assert.Equal(t, "alice", entries[0].ContextMap()["user_id"])
	assert.Empty(t, entries[0].ContextMap()["types"])
	assert.Equal(t, []interface{}{"follow"}, entries[0].ContextMap()["exclude_types"])
	assert.Equal(t, false, entries[0].ContextMap()["only_unread"])
	assert.Equal(t, false, entries[0].ContextMap()["include_read"])
	assert.Equal(t, "bob", entries[0].ContextMap()["actor_id"])
	assert.Equal(t, "status", entries[0].ContextMap()["target_type"])
	assert.Contains(t, entries[0].ContextMap()["error"], "builder failed")
}

func TestService_CreateNotification_round27_actor_optional_missing(t *testing.T) {
	t.Parallel()

	service, notificationRepo, accountRepo, _, pushService := setupTestService()
	ctx := context.Background()

	recipient := &storage.Account{User: &storage.User{Username: "alice"}}
	accountRepo.On("GetAccount", ctx, "alice").Return(recipient, nil).Once()
	accountRepo.On("GetAccount", ctx, "missing-actor").Return(nil, assert.AnError).Once()
	notificationRepo.On("CreateNotification", ctx, mock.AnythingOfType("*models.Notification")).Return(nil).Once()

	result, err := service.CreateNotification(ctx, &CreateNotificationCommand{
		UserID:    "alice",
		Type:      "mention",
		ActorID:   "missing-actor",
		ActorType: "user",
		Title:     "title-only",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "alice", result.Notification.UserID)
	assert.Len(t, pushService.Messages(), 1)
}

func TestService_CreateNotification_UsesResolvedRecipientUsernameForStorage(t *testing.T) {
	t.Parallel()

	service, notificationRepo, accountRepo, _, pushService := setupTestService()
	ctx := context.Background()

	recipient := &storage.Account{User: &storage.User{Username: "Medic"}}
	accountRepo.On("GetAccount", ctx, "medic").Return(recipient, nil).Once()
	notificationRepo.On("CreateNotification", ctx, mock.MatchedBy(func(notification *models.Notification) bool {
		return notification != nil && notification.UserID == "Medic"
	})).Return(nil).Once()

	result, err := service.CreateNotification(ctx, &CreateNotificationCommand{
		UserID:    "medic",
		Type:      "communication:inbound",
		ActorID:   "system",
		ActorType: "external",
		Title:     "hello",
		Body:      "test",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "Medic", result.Notification.UserID)
	assert.Len(t, pushService.Messages(), 1)
	assert.Equal(t, "Medic", pushService.Messages()[0].Username)
}
