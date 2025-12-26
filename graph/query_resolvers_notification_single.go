package graph

import (
	"context"
	"errors"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/services/notifications"
	"go.uber.org/zap"
)

// Notification is the resolver for the notification field.
func (r *queryResolver) Notification(ctx context.Context, id string) (*model.Notification, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	notificationID := id
	if err := common.ValidateRequiredParam("id", notificationID); err != nil {
		return nil, err
	}

	service := r.Registry.Notifications()
	if service == nil {
		return nil, errors.New("notifications service is not available")
	}

	notif, err := service.GetNotification(ctx, &notifications.GetNotificationQuery{
		UserID:         username,
		NotificationID: notificationID,
	})
	if err != nil {
		r.Logger.Error("Failed to get notification",
			zap.String("user", username),
			zap.String("notification_id", notificationID),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to get notification"), err)
	}

	return r.convertNotificationToGraphQL(ctx, notif), nil
}
