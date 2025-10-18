package graph

import (
	"context"

	"github.com/equaltoai/lesser/graph/model"
	"go.uber.org/zap"
)

// NOTE: imports intentionally omitted. Run gofmt/goimports and add any
// required imports after generating these files.

// NotificationStream is the resolver for the notificationStream field.
func (r *subscriptionResolver) NotificationStream(ctx context.Context, types []string) (<-chan *model.Notification, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	r.Logger.Info("Notification stream subscription started",
		zap.String("user", username),
		zap.Int("typeCount", len(types)))

	// Use SubscriptionManager for consistent subscription handling
	sm := r.SubscriptionManager
	if sm == nil {
		r.Logger.Error("subscription manager not available for notifications")
		ch := make(chan *model.Notification)
		close(ch)
		return ch, ErrSubscriptionManagerNotRunning
	}

	if !sm.IsRunning() {
		r.Logger.Error("subscription manager not running for notifications")
		ch := make(chan *model.Notification)
		close(ch)
		return ch, ErrSubscriptionManagerNotRunning
	}

	notificationChan, err := sm.SubscribeToNotifications(ctx, username)
	if err != nil {
		r.Logger.Error("failed to create notification subscription",
			zap.String("user", username),
			zap.Error(err))
		return nil, err
	}

	// If type filtering is needed, wrap the channel
	if len(types) > 0 {
		filteredChan := make(chan *model.Notification, 100)
		go r.filterNotificationsByType(ctx, notificationChan, filteredChan, types)
		r.Logger.Info("started filtered notification subscription",
			zap.String("user", username),
			zap.Strings("types", types))
		return filteredChan, nil
	}

	r.Logger.Info("started notification subscription",
		zap.String("user", username))

	return notificationChan, nil
}

// filterNotificationsByType filters notifications by type
func (r *subscriptionResolver) filterNotificationsByType(ctx context.Context, input <-chan *model.Notification, output chan *model.Notification, types []string) {
	defer close(output)

	for {
		select {
		case notification, ok := <-input:
			if !ok {
				return
			}

			if notification != nil && notificationMatchesTypes(notification, types) {
				select {
				case output <- notification:
				case <-ctx.Done():
					return
				default:
					r.Logger.Warn("Dropping notification event - filtered channel full")
				}
			}

		case <-ctx.Done():
			return
		}
	}
}
