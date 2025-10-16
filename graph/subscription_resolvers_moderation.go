package graph

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/moderation"
	"github.com/equaltoai/lesser/pkg/streaming"
	"go.uber.org/zap"
)

// NOTE: imports intentionally omitted. Run gofmt/goimports and add any
// required imports after generating these files.

// ModerationEvents implements SubscriptionResolver
func (r *subscriptionResolver) ModerationEvents(ctx context.Context, actorID *string) (<-chan *moderation.ModerationDecision, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	eventChan := make(chan *moderation.ModerationDecision, 100)

	// Get internal EventBus for real-time moderation events
	internalEventBus := streaming.GetGlobalEventBus(r.Logger)
	if internalEventBus == nil || !internalEventBus.IsRunning() {
		r.Logger.Error("Internal EventBus not available for ModerationEvents")
		close(eventChan)
		return eventChan, ErrInternalEventBusUnavailable
	}

	// Build filter for moderation events
	streams := []string{"moderation:global"}
	if actorID != nil && *actorID != "" {
		streams = append(streams, fmt.Sprintf("actor:%s", *actorID))
	}

	filter := &streaming.EventFilter{
		Types: []streaming.EventType{
			streaming.EventTypeModeration,
			streaming.EventTypeModerationFlag,
			streaming.EventTypeModerationReview,
			streaming.EventTypeAIModeration,
		},
		Streams: streams,
		UserID:  username,
	}

	if actorID != nil {
		filter.ActorID = *actorID
	}

	subscriber, err := internalEventBus.Subscribe(
		fmt.Sprintf("moderation_%s_%d", username, time.Now().UnixNano()),
		filter, 100)
	if err != nil {
		r.Logger.Error("Failed to subscribe to moderation events", zap.Error(err))
		close(eventChan)
		return eventChan, errors.Join(errors.New("failed to subscribe"), err)
	}

	// Start forwarding events
	go func() {
		defer func() {
			close(eventChan)
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

				// Convert event payload to ModerationDecision
				decision := r.convertEventToModerationDecision(event)
				if decision != nil {
					select {
					case eventChan <- decision:
					case <-ctx.Done():
						return
					}
				}

			case <-ctx.Done():
				return
			}
		}
	}()

	r.Logger.Info("Started moderation events subscription",
		zap.String("user", username))

	return eventChan, nil
}

// ModerationAlerts implements SubscriptionResolver
func (r *subscriptionResolver) ModerationAlerts(ctx context.Context, severity *model.ModerationSeverity) (<-chan *model.ModerationAlert, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	alertChan := make(chan *model.ModerationAlert, 100)

	// Get internal EventBus for real-time moderation alerts
	internalEventBus := streaming.GetGlobalEventBus(r.Logger)
	if internalEventBus == nil || !internalEventBus.IsRunning() {
		r.Logger.Error("Internal EventBus not available for ModerationAlerts")
		close(alertChan)
		return alertChan, ErrInternalEventBusUnavailable
	}

	filter := &streaming.EventFilter{
		Types: []streaming.EventType{
			streaming.EventTypeModerationFlag,
			streaming.EventTypeModerationReview,
			streaming.EventTypeAIModeration,
		},
		Streams: []string{"moderation:alerts", fmt.Sprintf("user:%s:alerts", username)},
		UserID:  username,
	}

	subscriber, err := internalEventBus.Subscribe(
		fmt.Sprintf("mod_alerts_%s_%d", username, time.Now().UnixNano()),
		filter, 100)
	if err != nil {
		r.Logger.Error("Failed to subscribe to moderation alerts", zap.Error(err))
		close(alertChan)
		return alertChan, errors.Join(errors.New("failed to subscribe"), err)
	}

	// Start forwarding events
	go func() {
		defer func() {
			close(alertChan)
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

				// Convert event to ModerationAlert
				alert := r.convertEventToModerationAlert(event, severity)
				if alert != nil {
					select {
					case alertChan <- alert:
					case <-ctx.Done():
						return
					}
				}

			case <-ctx.Done():
				return
			}
		}
	}()

	r.Logger.Info("Started moderation alerts subscription",
		zap.String("user", username))

	return alertChan, nil
}

// ModerationQueueUpdate implements SubscriptionResolver
func (r *subscriptionResolver) ModerationQueueUpdate(ctx context.Context, priority *model.Priority) (<-chan *model.ModerationItem, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	updateChan := make(chan *model.ModerationItem, 100)

	// Get internal EventBus for real-time moderation queue updates
	internalEventBus := streaming.GetGlobalEventBus(r.Logger)
	if internalEventBus == nil || !internalEventBus.IsRunning() {
		r.Logger.Error("Internal EventBus not available for ModerationQueueUpdate")
		close(updateChan)
		return updateChan, ErrInternalEventBusUnavailable
	}

	streams := []string{"moderation:queue"}
	if priority != nil {
		streams = append(streams, fmt.Sprintf("moderation:priority:%s", *priority))
	}

	filter := &streaming.EventFilter{
		Types: []streaming.EventType{
			streaming.EventTypeModerationFlag,
			streaming.EventTypeModerationReview,
			streaming.EventTypeModeration,
		},
		Streams: streams,
		UserID:  username,
	}

	subscriber, err := internalEventBus.Subscribe(
		fmt.Sprintf("mod_queue_%s_%d", username, time.Now().UnixNano()),
		filter, 100)
	if err != nil {
		r.Logger.Error("Failed to subscribe to moderation queue", zap.Error(err))
		close(updateChan)
		return updateChan, errors.Join(errors.New("failed to subscribe"), err)
	}

	// Start forwarding events
	go func() {
		defer func() {
			close(updateChan)
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

				// Convert event to ModerationItem
				item := r.convertEventToModerationItem(event, priority)
				if item != nil {
					select {
					case updateChan <- item:
					case <-ctx.Done():
						return
					}
				}

			case <-ctx.Done():
				return
			}
		}
	}()

	r.Logger.Info("Started moderation queue subscription",
		zap.String("user", username))

	return updateChan, nil
}
