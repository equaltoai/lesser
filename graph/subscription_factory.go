package graph

import (
	"context"
	"time"

	"github.com/equaltoai/lesser/pkg/streaming"
	"go.uber.org/zap"
)

// SubscriptionConfig contains common configuration for creating event bus subscriptions
type SubscriptionConfig struct {
	ID            string
	Type          string
	UserID        string
	Filter        *streaming.EventFilter
	OutputChannel interface{}
	BufferSize    int
	Params        map[string]interface{}
}

// EventProcessor is a function that processes events from the event bus
type EventProcessor func(subscription *GraphQLSubscription, outputChannel interface{})

// createGenericEventBusSubscription creates a generic event bus subscription
func (sm *GraphQLSubscriptionManager) createGenericEventBusSubscription(
	ctx context.Context,
	config *SubscriptionConfig,
	processor EventProcessor,
) error {
	// Subscribe to the event bus
	subscriber, err := sm.eventBus.Subscribe(config.ID, config.Filter, config.BufferSize)
	if err != nil {
		return ErrEventBusSubscriptionFailedWithContext(err)
	}

	// Create subscription context with cancellation
	subCtx, cancel := context.WithCancel(ctx)

	// Create and store subscription
	subscription := &GraphQLSubscription{
		ID:            config.ID,
		Type:          config.Type,
		UserID:        config.UserID,
		Filter:        config.Filter,
		Subscriber:    subscriber,
		OutputChannel: config.OutputChannel,
		Context:       subCtx,
		Cancel:        cancel,
		Created:       time.Now(),
		LastActivity:  time.Now(),
		Params:        config.Params,
	}

	sm.subscriptionsMux.Lock()
	sm.subscriptions[config.ID] = subscription
	sm.subscriptionsMux.Unlock()

	// Start the event processing goroutine
	go processor(subscription, config.OutputChannel)

	sm.logger.Info("created event bus subscription",
		zap.String("subscription_id", config.ID),
		zap.String("type", config.Type),
		zap.String("user_id", config.UserID))

	return nil
}

// Note: getStringValue and getIntValue are defined in subscription_manager.go
