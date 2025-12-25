package graph

import (
	"context"
	"errors"
	"strings"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/services/notifications"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"go.uber.org/zap"
)

func (r *mutationResolver) MarkNotificationGroupAsRead(ctx context.Context, groupID string) (bool, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return false, err
	}
	if err := common.ValidateRequiredParam("groupId", strings.TrimSpace(groupID)); err != nil {
		return false, err
	}

	if r.Registry == nil || r.Registry.Notifications() == nil {
		return false, errors.New("notification service is not available")
	}

	notificationService := r.Registry.Notifications()

	// Single-notification case (ungrouped).
	if !strings.HasPrefix(groupID, "group_") {
		_, err := notificationService.MarkAsRead(ctx, &notifications.MarkAsReadCommand{
			NotificationID: groupID,
			UserID:         username,
		})
		if err != nil {
			r.Logger.Error("failed to mark notification as read",
				zap.String("user", username),
				zap.String("notification_id", groupID),
				zap.Error(err))
			return false, errors.Join(errors.New("failed to mark notification as read"), err)
		}
		r.trackDynamoOperation(ctx, "write", 1)
		return true, nil
	}

	groupKey := strings.TrimPrefix(groupID, "group_")
	if groupKey == "" {
		return false, errors.New("invalid groupId")
	}

	// Recreate group membership with the default grouping strategy.
	listResult, err := notificationService.ListNotifications(ctx, &notifications.ListNotificationsQuery{
		UserID:      username,
		IncludeRead: true,
		Pagination: interfaces.PaginationOptions{
			Limit: 500,
		},
	})
	if err != nil {
		r.Logger.Error("failed to list notifications for mark-as-read",
			zap.String("user", username),
			zap.Error(err))
		return false, errors.Join(errors.New("failed to list notifications"), err)
	}

	groupingService := notifications.NewGroupedNotificationsService(r.Logger)
	groups, err := groupingService.GroupNotifications(ctx, listResult.Notifications, notifications.DefaultGroupingStrategy())
	if err != nil {
		r.Logger.Error("failed to group notifications for mark-as-read",
			zap.String("user", username),
			zap.Error(err))
		return false, errors.Join(errors.New("failed to group notifications"), err)
	}

	var targetGroup *notifications.GroupedNotification
	for _, g := range groups {
		if g == nil {
			continue
		}
		if g.GroupKey == groupKey && g.ID == groupID {
			targetGroup = g
			break
		}
	}
	if targetGroup == nil {
		return false, common.ErrNotFound("notification group")
	}

	err = groupingService.MarkGroupAsRead(ctx, targetGroup, func(ctx context.Context, notificationID string) error {
		_, err := notificationService.MarkAsRead(ctx, &notifications.MarkAsReadCommand{
			NotificationID: notificationID,
			UserID:         username,
		})
		return err
	})
	if err != nil {
		return false, errors.Join(errors.New("failed to mark notification group as read"), err)
	}

	r.trackDynamoOperation(ctx, "write", int64(len(targetGroup.AllNotifications)))
	return true, nil
}
