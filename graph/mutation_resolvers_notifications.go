package graph

import (
	"context"
	"errors"

	"github.com/equaltoai/lesser/pkg/services/notifications"
	"go.uber.org/zap"
)

// NOTE: imports intentionally omitted. Run gofmt/goimports and add any
// required imports after generating these files.

// DismissNotification is the resolver for the dismissNotification field.
func (r *mutationResolver) DismissNotification(ctx context.Context, id string) (bool, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return false, err
	}

	_, err = r.Registry.Notifications().ClearNotifications(ctx, &notifications.ClearCommand{
		UserID:          username,
		NotificationIDs: []string{id},
	})
	if err != nil {
		r.Logger.Error("Failed to dismiss notification",
			zap.String("user", username),
			zap.String("notification", id),
			zap.Error(err))
		return false, errors.Join(errors.New("failed to dismiss notification"), err)
	}

	// Track cost using centralized tracker
	r.trackDynamoOperation(ctx, "write", 1)
	return true, nil
}

// ClearNotifications is the resolver for the clearNotifications field.
func (r *mutationResolver) ClearNotifications(ctx context.Context) (bool, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return false, err
	}

	result, err := r.Registry.Notifications().ClearNotifications(ctx, &notifications.ClearCommand{
		UserID:   username,
		ClearAll: true,
	})
	if err != nil {
		r.Logger.Error("Failed to clear notifications",
			zap.String("user", username),
			zap.Error(err))
		return false, errors.Join(errors.New("failed to clear notifications"), err)
	}

	// Track cost using centralized tracker
	r.trackDynamoOperation(ctx, "write", int64(result.ClearedCount))
	return true, nil
}
