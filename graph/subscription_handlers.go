package graph

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/moderation"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/equaltoai/lesser/pkg/trust"
	"go.uber.org/zap"
)

// Event bus subscription handlers

// createEventBusSubscription creates a timeline subscription using the event bus
func (sm *GraphQLSubscriptionManager) createEventBusSubscription(ctx context.Context, subscriptionID, subType, username string, filter *streaming.EventFilter, ch chan *model.Object) (<-chan *model.Object, error) {
	// Subscribe to the event bus
	subscriber, err := sm.eventBus.Subscribe(subscriptionID, filter, 100)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to event bus: %w", err)
	}

	// Create subscription context with cancellation
	subCtx, cancel := context.WithCancel(ctx)

	// Create and store subscription
	subscription := &GraphQLSubscription{
		ID:            subscriptionID,
		Type:          subType,
		UserID:        username,
		Filter:        filter,
		Subscriber:    subscriber,
		OutputChannel: ch,
		Context:       subCtx,
		Cancel:        cancel,
		Created:       time.Now(),
		LastActivity:  time.Now(),
	}

	sm.subscriptionsMux.Lock()
	sm.subscriptions[subscriptionID] = subscription
	sm.subscriptionsMux.Unlock()

	// Start the event processing goroutine
	go sm.processTimelineEvents(subscription, ch)

	sm.logger.Info("created event bus timeline subscription",
		zap.String("subscription_id", subscriptionID),
		zap.String("username", username),
		zap.String("type", subType))

	return ch, nil
}

// createNotificationEventBusSubscription creates a notification subscription using the event bus
func (sm *GraphQLSubscriptionManager) createNotificationEventBusSubscription(ctx context.Context, subscriptionID, username string, filter *streaming.EventFilter, ch chan *model.Notification) (<-chan *model.Notification, error) {
	subscriber, err := sm.eventBus.Subscribe(subscriptionID, filter, 50)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to event bus: %w", err)
	}

	subCtx, cancel := context.WithCancel(ctx)

	subscription := &GraphQLSubscription{
		ID:            subscriptionID,
		Type:          "notification",
		UserID:        username,
		Filter:        filter,
		Subscriber:    subscriber,
		OutputChannel: ch,
		Context:       subCtx,
		Cancel:        cancel,
		Created:       time.Now(),
		LastActivity:  time.Now(),
	}

	sm.subscriptionsMux.Lock()
	sm.subscriptions[subscriptionID] = subscription
	sm.subscriptionsMux.Unlock()

	go sm.processNotificationEvents(subscription, ch)

	sm.logger.Info("created event bus notification subscription",
		zap.String("subscription_id", subscriptionID),
		zap.String("username", username))

	return ch, nil
}

// createCostEventBusSubscription creates a cost update subscription using the event bus
func (sm *GraphQLSubscriptionManager) createCostEventBusSubscription(ctx context.Context, subscriptionID, username string, filter *streaming.EventFilter, ch chan *model.CostUpdate, threshold *int) (<-chan *model.CostUpdate, error) {
	subscriber, err := sm.eventBus.Subscribe(subscriptionID, filter, 20)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to event bus: %w", err)
	}

	subCtx, cancel := context.WithCancel(ctx)

	params := make(map[string]interface{})
	if threshold != nil {
		params["threshold"] = *threshold
	}

	subscription := &GraphQLSubscription{
		ID:            subscriptionID,
		Type:          "cost",
		UserID:        username,
		Params:        params,
		Filter:        filter,
		Subscriber:    subscriber,
		OutputChannel: ch,
		Context:       subCtx,
		Cancel:        cancel,
		Created:       time.Now(),
		LastActivity:  time.Now(),
	}

	sm.subscriptionsMux.Lock()
	sm.subscriptions[subscriptionID] = subscription
	sm.subscriptionsMux.Unlock()

	go sm.processCostEvents(subscription, ch, threshold)

	sm.logger.Info("created event bus cost subscription",
		zap.String("subscription_id", subscriptionID),
		zap.String("username", username))

	return ch, nil
}

// createModerationEventBusSubscription creates a moderation subscription using the event bus
func (sm *GraphQLSubscriptionManager) createModerationEventBusSubscription(ctx context.Context, subscriptionID string, actorID *string, filter *streaming.EventFilter, ch chan *moderation.ModerationDecision) (<-chan *moderation.ModerationDecision, error) {
	subscriber, err := sm.eventBus.Subscribe(subscriptionID, filter, 50)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to event bus: %w", err)
	}

	subCtx, cancel := context.WithCancel(ctx)

	subscription := &GraphQLSubscription{
		ID:            subscriptionID,
		Type:          "moderation",
		UserID:        getStringValue(actorID),
		Filter:        filter,
		Subscriber:    subscriber,
		OutputChannel: ch,
		Context:       subCtx,
		Cancel:        cancel,
		Created:       time.Now(),
		LastActivity:  time.Now(),
	}

	sm.subscriptionsMux.Lock()
	sm.subscriptions[subscriptionID] = subscription
	sm.subscriptionsMux.Unlock()

	go sm.processModerationEvents(subscription, ch)

	sm.logger.Info("created event bus moderation subscription",
		zap.String("subscription_id", subscriptionID),
		zap.String("actor_id", getStringValue(actorID)))

	return ch, nil
}

// createTrustEventBusSubscription creates a trust update subscription using the event bus
func (sm *GraphQLSubscriptionManager) createTrustEventBusSubscription(ctx context.Context, subscriptionID, actorID string, filter *streaming.EventFilter, ch chan *trust.TrustEdge) (<-chan *trust.TrustEdge, error) {
	subscriber, err := sm.eventBus.Subscribe(subscriptionID, filter, 20)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to event bus: %w", err)
	}

	subCtx, cancel := context.WithCancel(ctx)

	subscription := &GraphQLSubscription{
		ID:            subscriptionID,
		Type:          "trust",
		UserID:        actorID,
		Filter:        filter,
		Subscriber:    subscriber,
		OutputChannel: ch,
		Context:       subCtx,
		Cancel:        cancel,
		Created:       time.Now(),
		LastActivity:  time.Now(),
	}

	sm.subscriptionsMux.Lock()
	sm.subscriptions[subscriptionID] = subscription
	sm.subscriptionsMux.Unlock()

	go sm.processTrustEvents(subscription, ch)

	sm.logger.Info("created event bus trust subscription",
		zap.String("subscription_id", subscriptionID),
		zap.String("actor_id", actorID))

	return ch, nil
}

// createAIEventBusSubscription creates an AI analysis subscription using the event bus
func (sm *GraphQLSubscriptionManager) createAIEventBusSubscription(ctx context.Context, subscriptionID string, objectID *string, filter *streaming.EventFilter, ch chan *model.AIAnalysis) (<-chan *model.AIAnalysis, error) {
	subscriber, err := sm.eventBus.Subscribe(subscriptionID, filter, 20)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to event bus: %w", err)
	}

	subCtx, cancel := context.WithCancel(ctx)

	subscription := &GraphQLSubscription{
		ID:            subscriptionID,
		Type:          "ai",
		UserID:        getStringValue(objectID),
		Filter:        filter,
		Subscriber:    subscriber,
		OutputChannel: ch,
		Context:       subCtx,
		Cancel:        cancel,
		Created:       time.Now(),
		LastActivity:  time.Now(),
	}

	sm.subscriptionsMux.Lock()
	sm.subscriptions[subscriptionID] = subscription
	sm.subscriptionsMux.Unlock()

	go sm.processAIEvents(subscription, ch)

	sm.logger.Info("created event bus AI subscription",
		zap.String("subscription_id", subscriptionID),
		zap.String("object_id", getStringValue(objectID)))

	return ch, nil
}

// createHashtagEventBusSubscription creates a hashtag activity subscription using the event bus
func (sm *GraphQLSubscriptionManager) createHashtagEventBusSubscription(ctx context.Context, subscriptionID, username string, hashtags []string, filter *streaming.EventFilter, ch chan *model.HashtagActivityUpdate) (<-chan *model.HashtagActivityUpdate, error) {
	subscriber, err := sm.eventBus.Subscribe(subscriptionID, filter, 100)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to event bus: %w", err)
	}

	subCtx, cancel := context.WithCancel(ctx)

	params := map[string]interface{}{
		"hashtags": hashtags,
	}

	subscription := &GraphQLSubscription{
		ID:            subscriptionID,
		Type:          "hashtag",
		UserID:        username,
		Params:        params,
		Filter:        filter,
		Subscriber:    subscriber,
		OutputChannel: ch,
		Context:       subCtx,
		Cancel:        cancel,
		Created:       time.Now(),
		LastActivity:  time.Now(),
	}

	sm.subscriptionsMux.Lock()
	sm.subscriptions[subscriptionID] = subscription
	sm.subscriptionsMux.Unlock()

	go sm.processHashtagEvents(subscription, ch, hashtags)

	sm.logger.Info("created event bus hashtag subscription",
		zap.String("subscription_id", subscriptionID),
		zap.String("username", username),
		zap.Strings("hashtags", hashtags))

	return ch, nil
}

// createQuoteEventBusSubscription creates a quote activity subscription using the event bus
func (sm *GraphQLSubscriptionManager) createQuoteEventBusSubscription(ctx context.Context, subscriptionID, username, noteID string, noteObj any, filter *streaming.EventFilter, ch chan *model.QuoteActivityUpdate) (<-chan *model.QuoteActivityUpdate, error) {
	subscriber, err := sm.eventBus.Subscribe(subscriptionID, filter, 50)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to event bus: %w", err)
	}

	subCtx, cancel := context.WithCancel(ctx)

	params := map[string]interface{}{
		"note_id":  noteID,
		"note_obj": noteObj,
	}

	subscription := &GraphQLSubscription{
		ID:            subscriptionID,
		Type:          "quote",
		UserID:        username,
		Params:        params,
		Filter:        filter,
		Subscriber:    subscriber,
		OutputChannel: ch,
		Context:       subCtx,
		Cancel:        cancel,
		Created:       time.Now(),
		LastActivity:  time.Now(),
	}

	sm.subscriptionsMux.Lock()
	sm.subscriptions[subscriptionID] = subscription
	sm.subscriptionsMux.Unlock()

	go sm.processQuoteEvents(subscription, ch, noteID, noteObj)

	sm.logger.Info("created event bus quote subscription",
		zap.String("subscription_id", subscriptionID),
		zap.String("username", username),
		zap.String("note_id", noteID))

	return ch, nil
}

// Event processing goroutines

// processTimelineEvents processes timeline events from the event bus
func (sm *GraphQLSubscriptionManager) processTimelineEvents(subscription *GraphQLSubscription, ch chan *model.Object) {
	defer func() {
		close(ch)
		sm.subscriptionsMux.Lock()
		delete(sm.subscriptions, subscription.ID)
		sm.subscriptionsMux.Unlock()
	}()

	for {
		select {
		case event := <-subscription.Subscriber.Channel:
			if event == nil {
				return
			}

			subscription.LastActivity = time.Now()

			// Convert event to GraphQL Object
			if obj := sm.converter.ConvertToObject(event); obj != nil {
				select {
				case ch <- obj:
					sm.logger.Debug("sent timeline event to GraphQL subscription",
						zap.String("subscription_id", subscription.ID),
						zap.String("event_id", event.ID))
				case <-subscription.Context.Done():
					return
				default:
					sm.logger.Warn("timeline subscription channel full, dropping event",
						zap.String("subscription_id", subscription.ID),
						zap.String("event_id", event.ID))
				}
			}

		case <-subscription.Subscriber.Quit:
			return
		case <-subscription.Context.Done():
			return
		}
	}
}

// processNotificationEvents processes notification events from the event bus
func (sm *GraphQLSubscriptionManager) processNotificationEvents(subscription *GraphQLSubscription, ch chan *model.Notification) {
	defer func() {
		close(ch)
		sm.subscriptionsMux.Lock()
		delete(sm.subscriptions, subscription.ID)
		sm.subscriptionsMux.Unlock()
	}()

	for {
		select {
		case event := <-subscription.Subscriber.Channel:
			if event == nil {
				return
			}

			subscription.LastActivity = time.Now()

			if notification := sm.converter.ConvertToNotification(event); notification != nil {
				select {
				case ch <- notification:
					sm.logger.Debug("sent notification event to GraphQL subscription",
						zap.String("subscription_id", subscription.ID),
						zap.String("event_id", event.ID))
				case <-subscription.Context.Done():
					return
				default:
					sm.logger.Warn("notification subscription channel full, dropping event",
						zap.String("subscription_id", subscription.ID))
				}
			}

		case <-subscription.Subscriber.Quit:
			return
		case <-subscription.Context.Done():
			return
		}
	}
}

// processCostEvents processes cost events from the event bus
func (sm *GraphQLSubscriptionManager) processCostEvents(subscription *GraphQLSubscription, ch chan *model.CostUpdate, threshold *int) {
	defer func() {
		close(ch)
		sm.subscriptionsMux.Lock()
		delete(sm.subscriptions, subscription.ID)
		sm.subscriptionsMux.Unlock()
	}()

	for {
		select {
		case event := <-subscription.Subscriber.Channel:
			if event == nil {
				return
			}

			subscription.LastActivity = time.Now()

			if costUpdate := sm.converter.ConvertToCostUpdate(event); costUpdate != nil {
				// Apply threshold filter if specified
				if threshold != nil && costUpdate.OperationCost < *threshold {
					continue
				}

				select {
				case ch <- costUpdate:
					sm.logger.Debug("sent cost event to GraphQL subscription",
						zap.String("subscription_id", subscription.ID),
						zap.String("event_id", event.ID))
				case <-subscription.Context.Done():
					return
				default:
					sm.logger.Warn("cost subscription channel full, dropping event",
						zap.String("subscription_id", subscription.ID))
				}
			}

		case <-subscription.Subscriber.Quit:
			return
		case <-subscription.Context.Done():
			return
		}
	}
}

// processModerationEvents processes moderation events from the event bus
func (sm *GraphQLSubscriptionManager) processModerationEvents(subscription *GraphQLSubscription, ch chan *moderation.ModerationDecision) {
	defer func() {
		close(ch)
		sm.subscriptionsMux.Lock()
		delete(sm.subscriptions, subscription.ID)
		sm.subscriptionsMux.Unlock()
	}()

	for {
		select {
		case event := <-subscription.Subscriber.Channel:
			if event == nil {
				return
			}

			subscription.LastActivity = time.Now()

			if decision := sm.converter.ConvertToModerationDecision(event); decision != nil {
				select {
				case ch <- decision:
					sm.logger.Debug("sent moderation event to GraphQL subscription",
						zap.String("subscription_id", subscription.ID),
						zap.String("event_id", event.ID))
				case <-subscription.Context.Done():
					return
				default:
					sm.logger.Warn("moderation subscription channel full, dropping event",
						zap.String("subscription_id", subscription.ID))
				}
			}

		case <-subscription.Subscriber.Quit:
			return
		case <-subscription.Context.Done():
			return
		}
	}
}

// processTrustEvents processes trust events from the event bus
func (sm *GraphQLSubscriptionManager) processTrustEvents(subscription *GraphQLSubscription, ch chan *trust.TrustEdge) {
	defer func() {
		close(ch)
		sm.subscriptionsMux.Lock()
		delete(sm.subscriptions, subscription.ID)
		sm.subscriptionsMux.Unlock()
	}()

	for {
		select {
		case event := <-subscription.Subscriber.Channel:
			if event == nil {
				return
			}

			subscription.LastActivity = time.Now()

			if trustEdge := sm.converter.ConvertToTrustEdge(event); trustEdge != nil {
				select {
				case ch <- trustEdge:
					sm.logger.Debug("sent trust event to GraphQL subscription",
						zap.String("subscription_id", subscription.ID),
						zap.String("event_id", event.ID))
				case <-subscription.Context.Done():
					return
				default:
					sm.logger.Warn("trust subscription channel full, dropping event",
						zap.String("subscription_id", subscription.ID))
				}
			}

		case <-subscription.Subscriber.Quit:
			return
		case <-subscription.Context.Done():
			return
		}
	}
}

// processAIEvents processes AI analysis events from the event bus
func (sm *GraphQLSubscriptionManager) processAIEvents(subscription *GraphQLSubscription, ch chan *model.AIAnalysis) {
	defer func() {
		close(ch)
		sm.subscriptionsMux.Lock()
		delete(sm.subscriptions, subscription.ID)
		sm.subscriptionsMux.Unlock()
	}()

	for {
		select {
		case event := <-subscription.Subscriber.Channel:
			if event == nil {
				return
			}

			subscription.LastActivity = time.Now()

			if analysis := sm.converter.ConvertToAIAnalysis(event); analysis != nil {
				select {
				case ch <- analysis:
					sm.logger.Debug("sent AI event to GraphQL subscription",
						zap.String("subscription_id", subscription.ID),
						zap.String("event_id", event.ID))
				case <-subscription.Context.Done():
					return
				default:
					sm.logger.Warn("AI subscription channel full, dropping event",
						zap.String("subscription_id", subscription.ID))
				}
			}

		case <-subscription.Subscriber.Quit:
			return
		case <-subscription.Context.Done():
			return
		}
	}
}

// processHashtagEvents processes hashtag events from the event bus
func (sm *GraphQLSubscriptionManager) processHashtagEvents(subscription *GraphQLSubscription, ch chan *model.HashtagActivityUpdate, hashtags []string) {
	defer func() {
		close(ch)
		sm.subscriptionsMux.Lock()
		delete(sm.subscriptions, subscription.ID)
		sm.subscriptionsMux.Unlock()
	}()

	for {
		select {
		case event := <-subscription.Subscriber.Channel:
			if event == nil {
				return
			}

			subscription.LastActivity = time.Now()

			if activity := sm.converter.ConvertToHashtagActivity(event); activity != nil {
				// Filter by hashtags if needed
				hashtagMatch := false
				for _, hashtag := range hashtags {
					if activity.Hashtag == hashtag {
						hashtagMatch = true
						break
					}
				}

				if !hashtagMatch {
					continue
				}

				select {
				case ch <- activity:
					sm.logger.Debug("sent hashtag event to GraphQL subscription",
						zap.String("subscription_id", subscription.ID),
						zap.String("event_id", event.ID),
						zap.String("hashtag", activity.Hashtag))
				case <-subscription.Context.Done():
					return
				default:
					sm.logger.Warn("hashtag subscription channel full, dropping event",
						zap.String("subscription_id", subscription.ID))
				}
			}

		case <-subscription.Subscriber.Quit:
			return
		case <-subscription.Context.Done():
			return
		}
	}
}

// processQuoteEvents processes quote activity events from the event bus
func (sm *GraphQLSubscriptionManager) processQuoteEvents(subscription *GraphQLSubscription, ch chan *model.QuoteActivityUpdate, noteID string, _ any) {
	defer func() {
		close(ch)
		sm.subscriptionsMux.Lock()
		delete(sm.subscriptions, subscription.ID)
		sm.subscriptionsMux.Unlock()
	}()

	for {
		select {
		case event := <-subscription.Subscriber.Channel:
			if event == nil {
				return
			}

			subscription.LastActivity = time.Now()

			if activity := sm.converter.ConvertToQuoteActivity(event); activity != nil {
				select {
				case ch <- activity:
					sm.logger.Debug("sent quote event to GraphQL subscription",
						zap.String("subscription_id", subscription.ID),
						zap.String("event_id", event.ID),
						zap.String("note_id", noteID))
				case <-subscription.Context.Done():
					return
				default:
					sm.logger.Warn("quote subscription channel full, dropping event",
						zap.String("subscription_id", subscription.ID))
				}
			}

		case <-subscription.Subscriber.Quit:
			return
		case <-subscription.Context.Done():
			return
		}
	}
}

// createMetricsEventBusSubscription creates a metrics subscription using the event bus
func (sm *GraphQLSubscriptionManager) createMetricsEventBusSubscription(ctx context.Context, subscriptionID, username string, categories, services []string, threshold *float64, filter *streaming.EventFilter, ch chan *model.MetricsUpdate) (<-chan *model.MetricsUpdate, error) {
	subscriber, err := sm.eventBus.Subscribe(subscriptionID, filter, 100)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to event bus: %w", err)
	}

	subCtx, cancel := context.WithCancel(ctx)

	params := make(map[string]interface{})
	if len(categories) > 0 {
		params["categories"] = categories
	}
	if len(services) > 0 {
		params["services"] = services
	}
	if threshold != nil {
		params["threshold"] = *threshold
	}

	subscription := &GraphQLSubscription{
		ID:            subscriptionID,
		Type:          "metrics",
		UserID:        username,
		Params:        params,
		Filter:        filter,
		Subscriber:    subscriber,
		OutputChannel: ch,
		Context:       subCtx,
		Cancel:        cancel,
		Created:       time.Now(),
		LastActivity:  time.Now(),
	}

	sm.subscriptionsMux.Lock()
	sm.subscriptions[subscriptionID] = subscription
	sm.subscriptionsMux.Unlock()

	go sm.processMetricsEvents(subscription, ch, categories, services, threshold)

	sm.logger.Info("created event bus metrics subscription",
		zap.String("subscription_id", subscriptionID),
		zap.String("username", username),
		zap.Strings("categories", categories),
		zap.Strings("services", services))

	return ch, nil
}

// processMetricsEvents processes metrics events from the event bus
func (sm *GraphQLSubscriptionManager) processMetricsEvents(subscription *GraphQLSubscription, ch chan *model.MetricsUpdate, categories, services []string, threshold *float64) {
	defer func() {
		close(ch)
		sm.subscriptionsMux.Lock()
		delete(sm.subscriptions, subscription.ID)
		sm.subscriptionsMux.Unlock()
	}()

	for {
		select {
		case event := <-subscription.Subscriber.Channel:
			if event == nil {
				return
			}

			subscription.LastActivity = time.Now()

			if metricsUpdate := sm.converter.ConvertToMetricsUpdate(event); metricsUpdate != nil {
				// Apply category filter if specified
				if len(categories) > 0 {
					categoryMatch := false
					for _, category := range categories {
						if metricsUpdate.SubscriptionCategory == category {
							categoryMatch = true
							break
						}
					}
					if !categoryMatch {
						continue
					}
				}

				// Apply service filter if specified
				if len(services) > 0 {
					serviceMatch := false
					for _, service := range services {
						if metricsUpdate.ServiceName == service {
							serviceMatch = true
							break
						}
					}
					if !serviceMatch {
						continue
					}
				}

				// Apply threshold filter if specified
				if threshold != nil {
					if metricsUpdate.Sum < *threshold && metricsUpdate.Max < *threshold {
						continue
					}
				}

				select {
				case ch <- metricsUpdate:
					sm.logger.Debug("sent metrics event to GraphQL subscription",
						zap.String("subscription_id", subscription.ID),
						zap.String("event_id", event.ID),
						zap.String("metric_type", metricsUpdate.MetricType),
						zap.String("service", metricsUpdate.ServiceName),
						zap.String("category", metricsUpdate.SubscriptionCategory))
				case <-subscription.Context.Done():
					return
				default:
					sm.logger.Warn("metrics subscription channel full, dropping event",
						zap.String("subscription_id", subscription.ID),
						zap.String("metric_id", metricsUpdate.MetricID))
				}
			}

		case <-subscription.Subscriber.Quit:
			return
		case <-subscription.Context.Done():
			return
		}
	}
}
