package graph

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/equaltoai/lesser/pkg/trust"
	"go.uber.org/zap"
)

// NOTE: imports intentionally omitted. Run gofmt/goimports and add any
// required imports after generating these files.

// RelationshipUpdates is the resolver for the relationshipUpdates field.
func (r *subscriptionResolver) RelationshipUpdates(ctx context.Context, actorID *string) (<-chan *model.RelationshipUpdate, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	// Create channel for streaming
	ch := make(chan *model.RelationshipUpdate, 100)

	r.Logger.Info("Relationship updates subscription started",
		zap.String("user", username))

	// Get internal EventBus for real-time relationship updates
	internalEventBus := streaming.GetGlobalEventBus(r.Logger)
	if internalEventBus == nil || !internalEventBus.IsRunning() {
		r.Logger.Error("Internal EventBus not available for RelationshipUpdates")
		close(ch)
		return ch, ErrInternalEventBusUnavailable
	}

	// Determine stream target - either specific actor or user's relationships
	var streamTarget string
	if actorID != nil && *actorID != "" {
		streamTarget = *actorID
	} else {
		streamTarget = username
	}

	// Subscribe to relationship events
	filter := &streaming.EventFilter{
		Types: []streaming.EventType{
			"relationship.create",
			"relationship.update",
			"relationship.delete",
		},
		Streams: []string{fmt.Sprintf("user:%s", username)},
		ActorID: streamTarget,
	}

	subscriber, err := internalEventBus.Subscribe(fmt.Sprintf("rel_%s_%s_%d", username, streamTarget, time.Now().UnixNano()), filter, 100)
	if err != nil {
		r.Logger.Error("Failed to subscribe to event bus for RelationshipUpdates", zap.Error(err))
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

				// Convert internal event to RelationshipUpdate
				relationshipUpdate := r.convertEventToRelationshipUpdate(ctx, event)
				if relationshipUpdate != nil {
					select {
					case ch <- relationshipUpdate:
					case <-ctx.Done():
						return
					default:
						// Drop event if channel is full
						r.Logger.Warn("Dropping relationship update event - channel full", zap.String("event_id", event.ID))
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

// TrustUpdates implements SubscriptionResolver
func (r *subscriptionResolver) TrustUpdates(ctx context.Context, actorID string) (<-chan *trust.TrustEdge, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	updateChan := make(chan *trust.TrustEdge, 100)

	r.Logger.Info("Started trust updates subscription",
		zap.String("user", username),
		zap.String("actor", actorID))

	// Get internal EventBus for real-time trust updates
	internalEventBus := streaming.GetGlobalEventBus(r.Logger)
	if internalEventBus == nil || !internalEventBus.IsRunning() {
		r.Logger.Error("Internal EventBus not available for TrustUpdates")
		close(updateChan)
		return updateChan, ErrInternalEventBusUnavailable
	}

	// Subscribe to trust events for the specified actor
	filter := &streaming.EventFilter{
		Types: []streaming.EventType{
			streaming.EventTypeTrustUpdate,
			streaming.EventTypeReputationUpdate,
			streaming.EventTypeVouchUpdate,
		},
		Streams: []string{fmt.Sprintf("actor:%s", actorID), "trust:global"},
		ActorID: actorID,
		UserID:  username,
	}

	subscriber, err := internalEventBus.Subscribe(fmt.Sprintf("trust_%s_%s_%d", actorID, username, time.Now().UnixNano()), filter, 100)
	if err != nil {
		r.Logger.Error("Failed to subscribe to event bus for TrustUpdates", zap.Error(err))
		close(updateChan)
		return updateChan, errors.Join(errors.New("failed to subscribe to event bus"), err)
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

				// Convert internal event to TrustEdge
				trustEdge := r.convertEventToTrustEdge(event)
				if trustEdge != nil {
					select {
					case updateChan <- trustEdge:
					case <-ctx.Done():
						return
					default:
						// Drop event if channel is full
						r.Logger.Warn("Dropping trust update event - channel full", zap.String("event_id", event.ID))
					}
				}

			case <-subscriber.Quit:
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	return updateChan, nil
}
