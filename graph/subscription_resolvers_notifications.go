package graph

import (
	"context"
	"fmt"
	"time"

	"errors"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/streaming"
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

	ch := make(chan *model.Notification, 100)
	r.Logger.Info("Notification stream subscription started",
		zap.String("user", username),
		zap.Int("typeCount", len(types)))

	internalEventBus, err := r.getEventBusForNotifications()
	if err != nil {
		close(ch)
		return ch, err
	}

	filter := r.createNotificationFilter(username)
	subscriber, err := r.subscribeToNotificationEvents(internalEventBus, username, filter)
	if err != nil {
		close(ch)
		return ch, err
	}

	r.startNotificationEventForwarding(ctx, ch, subscriber, types)
	return ch, nil
}

func (r *subscriptionResolver) getEventBusForNotifications() (*streaming.EventBus, error) {
	registryEventBus := r.Registry.EventBus()
	if registryEventBus == nil {
		r.Logger.Error("EventBus not available for NotificationStream subscription")
		return nil, ErrEventBusUnavailable
	}

	internalEventBus := streaming.GetGlobalEventBus(r.Logger)
	if internalEventBus == nil || !internalEventBus.IsRunning() {
		r.Logger.Error("Internal EventBus not available or not running")
		return nil, ErrInternalEventBusUnavailable
	}

	return internalEventBus, nil
}

func (r *subscriptionResolver) createNotificationFilter(username string) *streaming.EventFilter {
	return &streaming.EventFilter{
		Types:   []streaming.EventType{streaming.EventTypeNotification},
		Streams: []string{fmt.Sprintf("user:notification:%s", username)},
		UserID:  username,
	}
}

func (r *subscriptionResolver) subscribeToNotificationEvents(eventBus *streaming.EventBus, username string, filter *streaming.EventFilter) (*streaming.Subscriber, error) {
	subscriber, err := eventBus.Subscribe(fmt.Sprintf("notifications_%s_%d", username, time.Now().UnixNano()), filter, 100)
	if err != nil {
		r.Logger.Error("Failed to subscribe to event bus for NotificationStream", zap.Error(err))
		return nil, errors.Join(errors.New("failed to subscribe to event bus"), err)
	}
	return subscriber, nil
}

func (r *subscriptionResolver) startNotificationEventForwarding(ctx context.Context, ch chan *model.Notification, subscriber *streaming.Subscriber, types []string) {
	go func() {
		defer func() {
			close(ch)
			if subscriber != nil {
				subscriber.Close()
			}
		}()

		for {
			select {
			case event := <-subscriber.Channel:
				if event == nil {
					return
				}

				notification := r.convertEventToNotification(ctx, event)
				if notification != nil && r.shouldSendNotification(notification, types) {
					r.sendNotification(ctx, ch, notification, event.ID)
				}

			case <-subscriber.Quit:
				return
			case <-ctx.Done():
				return
			}
		}
	}()
}

// convertEventToNotification is now defined in schema.resolvers.go to avoid duplication

func (r *subscriptionResolver) shouldSendNotification(notification *model.Notification, types []string) bool {
	return len(types) == 0 || r.notificationMatchesTypes(notification, types)
}

func (r *subscriptionResolver) sendNotification(ctx context.Context, ch chan *model.Notification, notification *model.Notification, eventID string) {
	select {
	case ch <- notification:
	case <-ctx.Done():
		return
	default:
		r.Logger.Warn("Dropping notification event - channel full", zap.String("event_id", eventID))
	}
}

// notificationMatchesTypes is now defined in schema.resolvers.go to avoid duplication
