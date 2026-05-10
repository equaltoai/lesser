package notifications

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/equaltoai/lesser/pkg/testing/inmemory"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type userScopedOnlyNotificationRepo struct {
	*inmemory.NotificationRepository

	pageLimit         int
	globalGetCalls    int
	globalDeleteCalls int
}

func (r *userScopedOnlyNotificationRepo) GetNotification(_ context.Context, _ string) (*storagemodels.Notification, error) {
	r.globalGetCalls++
	return nil, storage.ErrNotFound
}

func (r *userScopedOnlyNotificationRepo) DeleteNotification(_ context.Context, _ string) error {
	r.globalDeleteCalls++
	return storage.ErrNotFound
}

func (r *userScopedOnlyNotificationRepo) GetUserNotification(ctx context.Context, userID, notificationID string) (*storagemodels.Notification, error) {
	limit := r.pageLimit
	if limit <= 0 {
		limit = 3
	}

	cursor := ""
	for {
		page, err := r.GetUserNotifications(ctx, userID, interfaces.PaginationOptions{
			Limit:  limit,
			Cursor: cursor,
		})
		if err != nil {
			return nil, err
		}
		if page == nil {
			break
		}
		for _, notification := range page.Items {
			if notification != nil && notification.ID == notificationID && notification.UserID == userID {
				return notification, nil
			}
		}
		if !page.HasMore || page.NextCursor == "" || page.NextCursor == cursor {
			break
		}
		cursor = page.NextCursor
	}

	return nil, storage.ErrNotFound
}

func (r *userScopedOnlyNotificationRepo) DeleteUserNotification(ctx context.Context, userID, notificationID string) error {
	if _, err := r.GetUserNotification(ctx, userID, notificationID); err != nil {
		return err
	}
	return r.NotificationRepository.DeleteNotification(ctx, notificationID)
}

func TestService_UserScopedNotificationIdentityRoundTripsListIDs(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := &userScopedOnlyNotificationRepo{
		NotificationRepository: inmemory.NewNotificationRepository(),
		pageLimit:              3,
	}
	service := NewService(repo, nil, streaming.NewMockPublisher(), zap.NewNop(), "example.com", nil)

	baseTime := time.Date(2026, time.May, 10, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 7; i++ {
		require.NoError(t, repo.CreateNotification(ctx, &storagemodels.Notification{
			ID:        fmt.Sprintf("filler-%02d", i),
			UserID:    "alice",
			Type:      "status",
			ActorID:   "local-actor",
			CreatedAt: baseTime.Add(time.Duration(i+10) * time.Minute),
		}))
	}

	cases := []struct {
		name       string
		id         string
		notifType  string
		actorID    string
		actorType  string
		targetID   string
		targetType string
		data       map[string]interface{}
	}{
		{
			name:       "local notification",
			id:         "local-favourite",
			notifType:  "favourite",
			actorID:    "bob",
			actorType:  "user",
			targetID:   "status-local",
			targetType: "status",
		},
		{
			name:       "remote reply create",
			id:         "remote-reply-create",
			notifType:  "reply",
			actorID:    "https://remote.example/users/replier",
			actorType:  "remote_actor",
			targetID:   "status-reply",
			targetType: "status",
			data:       map[string]interface{}{"activity_type": "Create"},
		},
		{
			name:       "remote mention create",
			id:         "remote-mention-create",
			notifType:  "mention",
			actorID:    "https://remote.example/users/mentioner",
			actorType:  "remote_actor",
			targetID:   "status-mention",
			targetType: "status",
			data:       map[string]interface{}{"activity_type": "Create"},
		},
		{
			name:      "inbound communication notification",
			id:        "comm-inbound",
			notifType: "communication:inbound",
			actorID:   "sender@example.com",
			data: map[string]interface{}{
				"channel":   "email",
				"messageId": "comm-msg-001",
			},
		},
	}

	for i, tc := range cases {
		require.NoError(t, repo.CreateNotification(ctx, &storagemodels.Notification{
			ID:         tc.id,
			UserID:     "alice",
			Type:       tc.notifType,
			ActorID:    tc.actorID,
			ActorType:  tc.actorType,
			TargetID:   tc.targetID,
			TargetType: tc.targetType,
			Data:       tc.data,
			CreatedAt:  baseTime.Add(time.Duration(i) * time.Minute),
		}), tc.name)
	}
	require.NoError(t, repo.CreateNotification(ctx, &storagemodels.Notification{
		ID:        "bob-private",
		UserID:    "bob",
		Type:      "mention",
		ActorID:   "alice",
		CreatedAt: baseTime.Add(30 * time.Minute),
	}))

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			list, err := service.ListNotifications(ctx, &ListNotificationsQuery{
				UserID:      "alice",
				IncludeRead: true,
				Pagination:  interfaces.PaginationOptions{Limit: 20},
			})
			require.NoError(t, err)
			require.Contains(t, notificationIDs(list.Notifications), tc.id)

			got, err := service.GetNotification(ctx, &GetNotificationQuery{
				UserID:         "alice",
				NotificationID: tc.id,
			})
			require.NoError(t, err)
			require.Equal(t, tc.id, got.ID)
			require.Equal(t, "USER#alice", got.PK)
			require.True(t, strings.HasPrefix(got.SK, "notif#"), got.SK)
			require.Contains(t, got.SK, tc.id)

			_, err = service.GetNotification(ctx, &GetNotificationQuery{
				UserID:         "bob",
				NotificationID: tc.id,
			})
			require.ErrorIs(t, err, ErrNotificationNotFound)

			_, err = service.MarkAsRead(ctx, &MarkAsReadCommand{
				UserID:         "bob",
				NotificationID: tc.id,
			})
			require.ErrorIs(t, err, ErrNotificationNotFound)

			stillUnread, err := repo.GetUserNotification(ctx, "alice", tc.id)
			require.NoError(t, err)
			require.False(t, stillUnread.IsRead)

			_, err = service.MarkAsRead(ctx, &MarkAsReadCommand{
				UserID:         "alice",
				NotificationID: tc.id,
			})
			require.NoError(t, err)

			after, err := service.ListNotifications(ctx, &ListNotificationsQuery{
				UserID:      "alice",
				IncludeRead: true,
				Pagination:  interfaces.PaginationOptions{Limit: 20},
			})
			require.NoError(t, err)
			readNotification := notificationByID(after.Notifications, tc.id)
			require.NotNil(t, readNotification)
			require.True(t, readNotification.IsRead)
		})
	}

	clearTarget := &storagemodels.Notification{
		ID:        "clear-specific",
		UserID:    "alice",
		Type:      "mention",
		ActorID:   "carol",
		CreatedAt: baseTime.Add(-time.Hour),
	}
	require.NoError(t, repo.CreateNotification(ctx, clearTarget))

	clearWrongUser, err := service.ClearNotifications(ctx, &ClearCommand{
		UserID:          "bob",
		NotificationIDs: []string{clearTarget.ID},
	})
	require.NoError(t, err)
	require.Equal(t, int64(0), clearWrongUser.ClearedCount)
	_, err = service.GetNotification(ctx, &GetNotificationQuery{UserID: "alice", NotificationID: clearTarget.ID})
	require.NoError(t, err)

	clearSameUser, err := service.ClearNotifications(ctx, &ClearCommand{
		UserID:          "alice",
		NotificationIDs: []string{clearTarget.ID},
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), clearSameUser.ClearedCount)
	_, err = service.GetNotification(ctx, &GetNotificationQuery{UserID: "alice", NotificationID: clearTarget.ID})
	require.ErrorIs(t, err, ErrNotificationNotFound)

	require.Zero(t, repo.globalGetCalls, "service must not use ID-only GetNotification for user-visible concrete IDs")
	require.Zero(t, repo.globalDeleteCalls, "service must not use ID-only DeleteNotification for user-visible concrete IDs")
}

func notificationIDs(notifications []*storagemodels.Notification) []string {
	ids := make([]string, 0, len(notifications))
	for _, notification := range notifications {
		if notification != nil {
			ids = append(ids, notification.ID)
		}
	}
	return ids
}

func notificationByID(notifications []*storagemodels.Notification, id string) *storagemodels.Notification {
	for _, notification := range notifications {
		if notification != nil && notification.ID == id {
			return notification
		}
	}
	return nil
}
