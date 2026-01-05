package graph

import (
	"testing"

	"github.com/equaltoai/lesser/pkg/services/notifications"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/testing/inmemory"
	"github.com/stretchr/testify/require"
)

func TestRound12Notifications_ListGetDismissAndClear(t *testing.T) {
	resolver, storageRepo := newRound12GraphResolver(t)
	ctx := round12AuthContext("alice")

	repo, ok := storageRepo.notificationRepo.(*inmemory.NotificationRepository)
	require.True(t, ok)

	require.NoError(t, repo.CreateNotification(ctx, &models.Notification{
		ID:         "notif-1",
		UserID:     "alice",
		Type:       "favourite",
		ActorID:    "bob",
		TargetType: "status",
		TargetID:   "status-1",
	}))
	require.NoError(t, repo.CreateNotification(ctx, &models.Notification{
		ID:         "notif-2",
		UserID:     "alice",
		Type:       "favourite",
		ActorID:    "carol",
		TargetType: "status",
		TargetID:   "status-1",
	}))

	conn, err := resolver.Query().Notifications(ctx, nil, nil, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, conn)
	require.Len(t, conn.Edges, 2)
	require.NotNil(t, conn.PageInfo)
	require.NotNil(t, conn.PageInfo.StartCursor)
	require.NotNil(t, conn.PageInfo.EndCursor)

	notif, err := resolver.Query().Notification(ctx, "notif-1")
	require.NoError(t, err)
	require.NotNil(t, notif)

	okBool, err := resolver.Mutation().DismissNotification(ctx, "notif-1")
	require.NoError(t, err)
	require.True(t, okBool)

	okBool, err = resolver.Mutation().ClearNotifications(ctx)
	require.NoError(t, err)
	require.True(t, okBool)
}

func TestRound12GroupedNotifications_MarkGroupAsRead(t *testing.T) {
	resolver, storageRepo := newRound12GraphResolver(t)
	ctx := round12AuthContext("alice")

	repo, ok := storageRepo.notificationRepo.(*inmemory.NotificationRepository)
	require.True(t, ok)

	require.NoError(t, repo.CreateNotification(ctx, &models.Notification{
		ID:         "notif-group-1",
		UserID:     "alice",
		Type:       "favourite",
		ActorID:    "bob",
		TargetType: "status",
		TargetID:   "status-1",
	}))
	require.NoError(t, repo.CreateNotification(ctx, &models.Notification{
		ID:         "notif-group-2",
		UserID:     "alice",
		Type:       "favourite",
		ActorID:    "carol",
		TargetType: "status",
		TargetID:   "status-1",
	}))

	// Single-notification path (ungrouped).
	okBool, err := resolver.Mutation().MarkNotificationGroupAsRead(ctx, "notif-group-1")
	require.NoError(t, err)
	require.True(t, okBool)

	updated, err := repo.GetNotification(ctx, "notif-group-1")
	require.NoError(t, err)
	require.True(t, updated.IsRead)

	// Grouped path.
	n1, err := repo.GetNotification(ctx, "notif-group-1")
	require.NoError(t, err)
	n2, err := repo.GetNotification(ctx, "notif-group-2")
	require.NoError(t, err)

	grouping := notifications.NewGroupedNotificationsService(resolver.Logger)
	groups, err := grouping.GroupNotifications(ctx, []*models.Notification{n1, n2}, notifications.DefaultGroupingStrategy())
	require.NoError(t, err)
	require.NotEmpty(t, groups)

	okBool, err = resolver.Mutation().MarkNotificationGroupAsRead(ctx, groups[0].ID)
	require.NoError(t, err)
	require.True(t, okBool)

	require.Error(t, func() error {
		_, err := resolver.Mutation().MarkNotificationGroupAsRead(ctx, "group_")
		return err
	}())
	require.Error(t, func() error {
		_, err := resolver.Mutation().MarkNotificationGroupAsRead(ctx, "group_missing")
		return err
	}())
}
