package streaming

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// ExampleGraphQLSubscription demonstrates how GraphQL subscriptions would use the internal event bus
func ExampleGraphQLSubscription(logger *zap.Logger) {
	// This would typically be called from GraphQL resolver

	// Get the global event bus (from stream-router)
	eventBus := GetGlobalEventBus(logger)
	if eventBus == nil {
		logger.Error("global event bus not available")
		return
	}

	// Create a filter for timeline events for a specific user
	filter := &EventFilter{
		Types:   []EventType{EventTypeStatus, EventTypeStatusUpdate},
		UserID:  "alice", // Subscribe only to alice's timeline
		Streams: []string{"user:alice", "public"},
	}

	// Subscribe to events
	subscriber, err := eventBus.Subscribe("graphql-timeline-alice", filter, 50)
	if err != nil {
		logger.Error("failed to subscribe to events", zap.Error(err))
		return
	}

	// Process events (this would be in a GraphQL subscription resolver)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	logger.Info("GraphQL subscription started for user alice")

	for {
		select {
		case event := <-subscriber.Channel:
			// Convert internal event to GraphQL subscription data
			graphqlData := convertEventToGraphQL(event)
			logger.Info("sending GraphQL subscription update",
				zap.String("event_type", string(event.Type)),
				zap.String("user_id", event.UserID),
				zap.Any("data", graphqlData))

			// In real implementation, this would be sent to the GraphQL client
			// via WebSocket or Server-Sent Events

		case <-subscriber.Quit:
			logger.Info("subscriber disconnected")
			return

		case <-ctx.Done():
			logger.Info("subscription timeout")
			_ = eventBus.Unsubscribe("graphql-timeline-alice")
			return
		}
	}
}

// convertEventToGraphQL converts an internal event to GraphQL subscription format
func convertEventToGraphQL(event *InternalEvent) map[string]interface{} {
	switch event.Type {
	case EventTypeStatus:
		if payload, ok := event.Data.(*StatusEventPayload); ok {
			return map[string]interface{}{
				"__typename": "StatusUpdate",
				"status": map[string]interface{}{
					"id":         payload.StatusID,
					"content":    payload.Content,
					"visibility": payload.Visibility,
					"author": map[string]interface{}{
						"id":       payload.AuthorID,
						"username": payload.AuthorUsername,
					},
					"createdAt": payload.CreatedAt.Format(time.RFC3339),
				},
				"streams": event.Streams,
			}
		}

	case EventTypeNotification:
		if payload, ok := event.Data.(*NotificationEventPayload); ok {
			return map[string]interface{}{
				"__typename": "NotificationUpdate",
				"notification": map[string]interface{}{
					"id":   payload.NotificationID,
					"type": payload.Type,
					"read": payload.Read,
					"actor": map[string]interface{}{
						"id": payload.ActorID,
					},
					"createdAt": payload.CreatedAt.Format(time.RFC3339),
				},
			}
		}

	case EventTypeAccountUpdate:
		if payload, ok := event.Data.(*AccountEventPayload); ok {
			return map[string]interface{}{
				"__typename": "AccountUpdate",
				"account": map[string]interface{}{
					"id":        payload.AccountID,
					"username":  payload.Username,
					"updatedAt": payload.UpdatedAt.Format(time.RFC3339),
				},
			}
		}
	}

	// Fallback for unknown event types
	return map[string]interface{}{
		"__typename": "GenericUpdate",
		"eventType":  string(event.Type),
		"action":     string(event.Action),
		"timestamp":  event.Timestamp.Format(time.RFC3339),
		"data":       event.Data,
	}
}

// ExampleTimelineSubscription shows how to subscribe to timeline updates
func ExampleTimelineSubscription(userID string, eventBus *EventBus, logger *zap.Logger) (string, error) {
	// Filter for timeline events relevant to this user
	filter := &EventFilter{
		Types: []EventType{
			EventTypeStatus,
			EventTypeStatusUpdate,
			EventTypeStatusDelete,
		},
		Streams: []string{
			fmt.Sprintf("user:%s", userID),
			"public",
			"public:local",
		},
	}

	subscriberID := fmt.Sprintf("timeline:%s:%d", userID, time.Now().UnixNano())

	_, err := eventBus.Subscribe(subscriberID, filter, 100)
	if err != nil {
		return "", fmt.Errorf("failed to subscribe to timeline: %w", err)
	}

	logger.Info("timeline subscription created",
		zap.String("subscriber_id", subscriberID),
		zap.String("user_id", userID))

	return subscriberID, nil
}

// ExampleNotificationSubscription shows how to subscribe to notifications
func ExampleNotificationSubscription(userID string, eventBus *EventBus, logger *zap.Logger) (string, error) {
	// Filter for notification events for this user only
	filter := &EventFilter{
		Types: []EventType{
			EventTypeNotification,
			EventTypeNotificationRead,
		},
		UserID: userID, // Only notifications for this user
		Streams: []string{
			fmt.Sprintf("user:notification:%s", userID),
		},
	}

	subscriberID := fmt.Sprintf("notifications:%s:%d", userID, time.Now().UnixNano())

	_, err := eventBus.Subscribe(subscriberID, filter, 50)
	if err != nil {
		return "", fmt.Errorf("failed to subscribe to notifications: %w", err)
	}

	logger.Info("notification subscription created",
		zap.String("subscriber_id", subscriberID),
		zap.String("user_id", userID))

	return subscriberID, nil
}

// ExampleModerationSubscription shows how moderators can subscribe to moderation events
func ExampleModerationSubscription(moderatorID string, eventBus *EventBus, logger *zap.Logger) (string, error) {
	// Filter for moderation events
	filter := &EventFilter{
		Types: []EventType{
			EventTypeModeration,
			EventTypeModerationFlag,
			EventTypeModerationReview,
		},
		MinPriority: PriorityHigh, // Only high priority moderation events
	}

	subscriberID := fmt.Sprintf("moderation:%s:%d", moderatorID, time.Now().UnixNano())

	_, err := eventBus.Subscribe(subscriberID, filter, 200) // Larger buffer for moderators
	if err != nil {
		return "", fmt.Errorf("failed to subscribe to moderation events: %w", err)
	}

	logger.Info("moderation subscription created",
		zap.String("subscriber_id", subscriberID),
		zap.String("moderator_id", moderatorID))

	return subscriberID, nil
}

// ExampleHashtagSubscription shows how to subscribe to hashtag trends
func ExampleHashtagSubscription(hashtag string, eventBus *EventBus, logger *zap.Logger) (string, error) {
	// Filter for hashtag-related events
	filter := &EventFilter{
		Types: []EventType{
			EventTypeHashtagTrend,
			EventTypeHashtagUpdate,
		},
		Metadata: map[string]string{
			"hashtag_0": hashtag, // Match hashtag in metadata
		},
	}

	subscriberID := fmt.Sprintf("hashtag:%s:%d", hashtag, time.Now().UnixNano())

	_, err := eventBus.Subscribe(subscriberID, filter, 30)
	if err != nil {
		return "", fmt.Errorf("failed to subscribe to hashtag events: %w", err)
	}

	logger.Info("hashtag subscription created",
		zap.String("subscriber_id", subscriberID),
		zap.String("hashtag", hashtag))

	return subscriberID, nil
}

// ExampleUnsubscribe shows how to clean up subscriptions
func ExampleUnsubscribe(subscriberID string, eventBus *EventBus, logger *zap.Logger) error {
	err := eventBus.Unsubscribe(subscriberID)
	if err != nil {
		logger.Error("failed to unsubscribe",
			zap.String("subscriber_id", subscriberID),
			zap.Error(err))
		return err
	}

	logger.Info("successfully unsubscribed",
		zap.String("subscriber_id", subscriberID))

	return nil
}

// ExampleEventBusHealthCheck shows how to monitor event bus health
func ExampleEventBusHealthCheck(eventBus *EventBus, logger *zap.Logger) {
	metrics := eventBus.GetMetrics()
	if metrics == nil {
		logger.Warn("event bus metrics not available")
		return
	}

	logger.Info("event bus health check",
		zap.Int64("events_published", metrics.EventsPublished),
		zap.Int64("events_delivered", metrics.EventsDelivered),
		zap.Int64("events_dropped", metrics.EventsDropped),
		zap.Int64("active_subscribers", metrics.SubscribersActive),
		zap.Int64("total_subscribers", metrics.SubscribersTotal),
		zap.Int64("delivery_errors", metrics.DeliveryErrors),
		zap.Duration("avg_delivery_time", metrics.AverageDeliveryTime),
		zap.Time("last_event_time", metrics.LastEventTime))

	// Alert if error rate is too high
	if metrics.EventsPublished > 0 {
		errorRate := float64(metrics.DeliveryErrors) / float64(metrics.EventsPublished)
		if errorRate > 0.05 { // More than 5% error rate
			logger.Warn("high event bus error rate",
				zap.Float64("error_rate", errorRate))
		}
	}

	// Alert if too many events are being dropped
	if metrics.EventsDropped > 100 {
		logger.Warn("high number of dropped events",
			zap.Int64("dropped_events", metrics.EventsDropped))
	}
}
