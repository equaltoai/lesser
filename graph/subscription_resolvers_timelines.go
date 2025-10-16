package graph

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/streaming"
	"go.uber.org/zap"
)

// NOTE: imports intentionally omitted. Run gofmt/goimports and add any
// required imports after generating these files.

// ====================================================================
// SUBSCRIPTION RESOLVERS
// ====================================================================

// ActivityStream is the resolver for the activityStream field.
func (r *subscriptionResolver) ActivityStream(ctx context.Context, types []model.ActivityType) (<-chan *activitypub.Activity, error) {
	username := r.optionalAuth(ctx)

	// Create channel for streaming
	ch := make(chan *activitypub.Activity, 100)

	r.Logger.Info("Activity stream subscription started",
		zap.String("user", username),
		zap.Int("typeCount", len(types)))

	// Get internal EventBus from registry for advanced filtering
	// We need direct access to set up custom filters
	registryEventBus := r.Registry.EventBus()
	if registryEventBus == nil {
		r.Logger.Error("EventBus not available for ActivityStream subscription")
		close(ch)
		return ch, ErrEventBusUnavailable
	}

	// Access the internal event bus through the adapter
	// This is a bit of a hack but allows us to set up advanced filtering
	internalEventBus := streaming.GetGlobalEventBus(r.Logger)
	if internalEventBus == nil || !internalEventBus.IsRunning() {
		r.Logger.Error("Internal EventBus not available or not running")
		close(ch)
		return ch, ErrInternalEventBusUnavailable
	}

	// Subscribe to activity events
	var streamNames []string
	if username != "" {
		// Authenticated user gets personalized stream
		streamNames = append(streamNames, fmt.Sprintf("user:%s", username))
	}
	// Always include public stream
	streamNames = append(streamNames, "public")

	filter := &streaming.EventFilter{
		Types: []streaming.EventType{
			streaming.EventTypeStatus,
			streaming.EventTypeStatusUpdate,
			streaming.EventTypeAccountUpdate,
		},
		Streams: streamNames,
	}

	// Subscribe to internal event bus
	subscriber, err := internalEventBus.Subscribe(fmt.Sprintf("activity_%s_%d", username, time.Now().UnixNano()), filter, 100)
	if err != nil {
		r.Logger.Error("Failed to subscribe to event bus for ActivityStream", zap.Error(err))
		close(ch)
		return ch, errors.Join(errors.New("failed to subscribe to event bus"), err)
	}

	// Start forwarding events
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

				// Convert internal event to ActivityPub Activity
				activity := r.convertEventToActivity(event)
				if activity != nil {
					select {
					case ch <- activity:
					case <-ctx.Done():
						return
					default:
						// Drop event if channel is full
						r.Logger.Warn("Dropping activity event - channel full", zap.String("event_id", event.ID))
					}
				}

			case <-subscriber.Quit:
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	return ch, nil
}

// TimelineUpdates is the resolver for the timelineUpdates field.
func (r *subscriptionResolver) TimelineUpdates(ctx context.Context, timelineType model.TimelineType, listID *string) (<-chan *model.Object, error) {
	username := r.optionalAuth(ctx)

	// Validate request
	if timelineType == model.TimelineTypeList && listID == nil {
		return nil, errors.New("listId is required for list timeline type")
	}
	if timelineType != model.TimelineTypePublic && username == "" {
		return nil, errors.New("authentication required for this timeline type")
	}

	ch := make(chan *model.Object, 100)
	r.Logger.Info("Timeline updates subscription started",
		zap.String("user", username),
		zap.String("type", string(timelineType)))

	// Get the internal event bus
	internalEventBus := streaming.GetGlobalEventBus(r.Logger)
	if internalEventBus == nil || !internalEventBus.IsRunning() {
		close(ch)
		return ch, errors.New("event bus not available")
	}

	// Create filter for timeline events
	filter := &streaming.EventFilter{
		Types:   []streaming.EventType{streaming.EventTypeStatus, streaming.EventTypeStatusUpdate},
		Streams: r.getStreamsForTimeline(timelineType, username, listID),
	}

	// Subscribe to events
	subscriberID := fmt.Sprintf("timeline_%s_%s_%d", timelineType, username, time.Now().UnixNano())
	subscriber, err := internalEventBus.Subscribe(subscriberID, filter, 100)
	if err != nil {
		close(ch)
		return ch, errors.Join(errors.New("failed to subscribe to timeline events"), err)
	}

	// Start forwarding events
	go func() {
		defer func() {
			close(ch)
			subscriber.Close()
			_ = internalEventBus.Unsubscribe(subscriberID)
		}()

		for {
			select {
			case event, ok := <-subscriber.Channel:
				if !ok {
					return
				}

				obj := r.convertEventToObject(ctx, event)
				if obj != nil {
					select {
					case ch <- obj:
					case <-ctx.Done():
						return
					default:
						r.Logger.Warn("Dropping timeline event - channel full",
							zap.String("event_id", event.ID),
							zap.String("timeline_type", string(timelineType)))
					}
				}

			case <-subscriber.Quit:
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	return ch, nil
}

// getStreamsForTimeline returns the appropriate stream names for the timeline type
func (r *subscriptionResolver) getStreamsForTimeline(timelineType model.TimelineType, username string, listID *string) []string {
	switch timelineType {
	case model.TimelineTypeHome:
		return []string{fmt.Sprintf("user:%s", username)}
	case model.TimelineTypePublic:
		return []string{"public"}
	case model.TimelineTypeLocal:
		return []string{"local"}
	case model.TimelineTypeList:
		if listID != nil {
			return []string{fmt.Sprintf("list:%s", *listID)}
		}
		return []string{"public"}
	default:
		return []string{"public"}
	}
}
