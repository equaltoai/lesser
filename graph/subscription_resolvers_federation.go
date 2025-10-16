package graph

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/streaming"
	"go.uber.org/zap"
)

// NOTE: imports intentionally omitted. Run gofmt/goimports and add any
// required imports after generating these files.

// FederationHealthUpdates implements SubscriptionResolver
func (r *subscriptionResolver) FederationHealthUpdates(ctx context.Context, domain *string) (<-chan *model.FederationHealthUpdate, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	updateChan := make(chan *model.FederationHealthUpdate, 100)

	r.Logger.Info("Started federation health subscription",
		zap.String("user", username),
		zap.Bool("filtered", domain != nil))

	// Get internal EventBus for real-time federation health updates
	internalEventBus := streaming.GetGlobalEventBus(r.Logger)
	if internalEventBus == nil || !internalEventBus.IsRunning() {
		r.Logger.Error("Internal EventBus not available for FederationHealthUpdates")
		close(updateChan)
		return updateChan, ErrInternalEventBusUnavailable
	}

	// Subscribe to federation health events
	var streamNames []string
	if domain != nil && *domain != "" {
		streamNames = []string{fmt.Sprintf("federation:%s", *domain)}
	} else {
		streamNames = []string{"federation:health"}
	}

	filter := &streaming.EventFilter{
		Types: []streaming.EventType{
			streaming.EventTypeFederationHealthUpdate,
			streaming.EventTypeFederationFailure,
			streaming.EventTypeFederationRecovery,
		},
		Streams: streamNames,
		UserID:  username,
	}

	subscriber, err := internalEventBus.Subscribe(fmt.Sprintf("fedhealth_%s_%d", username, time.Now().UnixNano()), filter, 100)
	if err != nil {
		r.Logger.Error("Failed to subscribe to event bus for FederationHealthUpdates", zap.Error(err))
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

				// Convert internal event to FederationHealthUpdate
				healthUpdate := r.convertEventToFederationHealthUpdate(event)
				if healthUpdate != nil {
					select {
					case updateChan <- healthUpdate:
					case <-ctx.Done():
						return
					default:
						// Drop event if channel is full
						r.Logger.Warn("Dropping federation health update event - channel full", zap.String("event_id", event.ID))
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

// InfrastructureEvent implements SubscriptionResolver
func (r *subscriptionResolver) InfrastructureEvent(ctx context.Context) (<-chan *model.InfrastructureEvent, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	eventChan := make(chan *model.InfrastructureEvent, 100)

	// For now, return empty channel
	// This would be implemented with real infrastructure event streaming
	go func() {
		<-ctx.Done()
		close(eventChan)
	}()

	r.Logger.Info("Started infrastructure events subscription",
		zap.String("user", username))

	return eventChan, nil
}
