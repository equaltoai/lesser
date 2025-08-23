package graph

import (
	"context"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/moderation"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/equaltoai/lesser/pkg/trust"
	"go.uber.org/zap"
)

// Event bus subscription handlers

// createEventBusSubscription creates a timeline subscription using the event bus
func (sm *GraphQLSubscriptionManager) createEventBusSubscription(ctx context.Context, subscriptionID, subType, username string, filter *streaming.EventFilter, ch chan *model.Object) (<-chan *model.Object, error) {
	config := &SubscriptionConfig{
		ID:            subscriptionID,
		Type:          subType,
		UserID:        username,
		Filter:        filter,
		OutputChannel: ch,
		BufferSize:    100,
	}

	err := sm.createGenericEventBusSubscription(ctx, config, func(sub *GraphQLSubscription, out interface{}) {
		sm.processTimelineEvents(sub, out.(chan *model.Object))
	})

	if err != nil {
		return nil, err
	}
	return ch, nil
}

// createNotificationEventBusSubscription creates a notification subscription using the event bus
func (sm *GraphQLSubscriptionManager) createNotificationEventBusSubscription(ctx context.Context, subscriptionID, username string, filter *streaming.EventFilter, ch chan *model.Notification) (<-chan *model.Notification, error) {
	config := &SubscriptionConfig{
		ID:            subscriptionID,
		Type:          "notification",
		UserID:        username,
		Filter:        filter,
		OutputChannel: ch,
		BufferSize:    50,
	}

	err := sm.createGenericEventBusSubscription(ctx, config, func(sub *GraphQLSubscription, out interface{}) {
		sm.processNotificationEvents(sub, out.(chan *model.Notification))
	})

	if err != nil {
		return nil, err
	}
	return ch, nil
}

// createCostEventBusSubscription creates a cost update subscription using the event bus
func (sm *GraphQLSubscriptionManager) createCostEventBusSubscription(ctx context.Context, subscriptionID, username string, filter *streaming.EventFilter, ch chan *model.CostUpdate, threshold *int) (<-chan *model.CostUpdate, error) {
	params := make(map[string]interface{})
	if threshold != nil {
		params["threshold"] = *threshold
	}

	config := &SubscriptionConfig{
		ID:            subscriptionID,
		Type:          "cost",
		UserID:        username,
		Filter:        filter,
		OutputChannel: ch,
		BufferSize:    20,
		Params:        params,
	}

	err := sm.createGenericEventBusSubscription(ctx, config, func(sub *GraphQLSubscription, out interface{}) {
		sm.processCostEvents(sub, out.(chan *model.CostUpdate), threshold)
	})

	if err != nil {
		return nil, err
	}
	return ch, nil
}

// createModerationEventBusSubscription creates a moderation subscription using the event bus
func (sm *GraphQLSubscriptionManager) createModerationEventBusSubscription(ctx context.Context, subscriptionID string, actorID *string, filter *streaming.EventFilter, ch chan *moderation.ModerationDecision) (<-chan *moderation.ModerationDecision, error) {
	config := &SubscriptionConfig{
		ID:            subscriptionID,
		Type:          "moderation",
		UserID:        getStringValue(actorID),
		Filter:        filter,
		OutputChannel: ch,
		BufferSize:    50,
	}

	err := sm.createGenericEventBusSubscription(ctx, config, func(sub *GraphQLSubscription, out interface{}) {
		sm.processModerationEvents(sub, out.(chan *moderation.ModerationDecision))
	})

	if err != nil {
		return nil, err
	}
	return ch, nil
}

// createTrustEventBusSubscription creates a trust update subscription using the event bus
func (sm *GraphQLSubscriptionManager) createTrustEventBusSubscription(ctx context.Context, subscriptionID, actorID string, filter *streaming.EventFilter, ch chan *trust.TrustEdge) (<-chan *trust.TrustEdge, error) {
	config := &SubscriptionConfig{
		ID:            subscriptionID,
		Type:          "trust",
		UserID:        actorID,
		Filter:        filter,
		OutputChannel: ch,
		BufferSize:    20,
	}

	err := sm.createGenericEventBusSubscription(ctx, config, func(sub *GraphQLSubscription, out interface{}) {
		sm.processTrustEvents(sub, out.(chan *trust.TrustEdge))
	})

	if err != nil {
		return nil, err
	}
	return ch, nil
}

// createAIEventBusSubscription creates an AI analysis subscription using the event bus
func (sm *GraphQLSubscriptionManager) createAIEventBusSubscription(ctx context.Context, subscriptionID string, objectID *string, filter *streaming.EventFilter, ch chan *model.AIAnalysis) (<-chan *model.AIAnalysis, error) {
	config := &SubscriptionConfig{
		ID:            subscriptionID,
		Type:          "ai",
		UserID:        getStringValue(objectID),
		Filter:        filter,
		OutputChannel: ch,
		BufferSize:    20,
	}

	err := sm.createGenericEventBusSubscription(ctx, config, func(sub *GraphQLSubscription, out interface{}) {
		sm.processAIEvents(sub, out.(chan *model.AIAnalysis))
	})

	if err != nil {
		return nil, err
	}
	return ch, nil
}

// createHashtagEventBusSubscription creates a hashtag activity subscription using the event bus
func (sm *GraphQLSubscriptionManager) createHashtagEventBusSubscription(ctx context.Context, subscriptionID, username string, hashtags []string, filter *streaming.EventFilter, ch chan *model.HashtagActivityUpdate) (<-chan *model.HashtagActivityUpdate, error) {
	params := map[string]interface{}{
		"hashtags": hashtags,
	}

	config := &SubscriptionConfig{
		ID:            subscriptionID,
		Type:          "hashtag",
		UserID:        username,
		Filter:        filter,
		OutputChannel: ch,
		BufferSize:    100,
		Params:        params,
	}

	err := sm.createGenericEventBusSubscription(ctx, config, func(sub *GraphQLSubscription, out interface{}) {
		sm.processHashtagEvents(sub, out.(chan *model.HashtagActivityUpdate), hashtags)
	})

	if err != nil {
		return nil, err
	}
	return ch, nil
}

// createQuoteEventBusSubscription creates a quote activity subscription using the event bus
func (sm *GraphQLSubscriptionManager) createQuoteEventBusSubscription(ctx context.Context, subscriptionID, username, noteID string, noteObj any, filter *streaming.EventFilter, ch chan *model.QuoteActivityUpdate) (<-chan *model.QuoteActivityUpdate, error) {
	params := map[string]interface{}{
		"note_id":  noteID,
		"note_obj": noteObj,
	}

	config := &SubscriptionConfig{
		ID:            subscriptionID,
		Type:          "quote",
		UserID:        username,
		Filter:        filter,
		OutputChannel: ch,
		BufferSize:    50,
		Params:        params,
	}

	err := sm.createGenericEventBusSubscription(ctx, config, func(sub *GraphQLSubscription, out interface{}) {
		sm.processQuoteEvents(sub, out.(chan *model.QuoteActivityUpdate), noteID, noteObj)
	})

	if err != nil {
		return nil, err
	}
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
	sm.processGenericEvents(subscription, ch, "notification", func(event *streaming.InternalEvent) interface{} {
		return sm.converter.ConvertToNotification(event)
	})
}

// processCostEvents processes cost events from the event bus
func (sm *GraphQLSubscriptionManager) processCostEvents(subscription *GraphQLSubscription, ch chan *model.CostUpdate, threshold *int) {
	sm.processGenericEvents(subscription, ch, "cost", func(event *streaming.InternalEvent) interface{} {
		costUpdate := sm.converter.ConvertToCostUpdate(event)
		if costUpdate == nil {
			return nil
		}

		// Apply threshold filter if specified
		if threshold != nil && costUpdate.OperationCost < *threshold {
			return nil
		}

		return costUpdate
	})
}

// processModerationEvents processes moderation events from the event bus
func (sm *GraphQLSubscriptionManager) processModerationEvents(subscription *GraphQLSubscription, ch chan *moderation.ModerationDecision) {
	sm.processGenericEvents(subscription, ch, "moderation", func(event *streaming.InternalEvent) interface{} {
		return sm.converter.ConvertToModerationDecision(event)
	})
}

// processTrustEvents processes trust events from the event bus
func (sm *GraphQLSubscriptionManager) processTrustEvents(subscription *GraphQLSubscription, ch chan *trust.TrustEdge) {
	sm.processGenericEvents(subscription, ch, "trust", func(event *streaming.InternalEvent) interface{} {
		return sm.converter.ConvertToTrustEdge(event)
	})
}

// processAIEvents processes AI analysis events from the event bus
func (sm *GraphQLSubscriptionManager) processAIEvents(subscription *GraphQLSubscription, ch chan *model.AIAnalysis) {
	sm.processGenericEvents(subscription, ch, "AI", func(event *streaming.InternalEvent) interface{} {
		return sm.converter.ConvertToAIAnalysis(event)
	})
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

	config := &SubscriptionConfig{
		ID:            subscriptionID,
		Type:          "metrics",
		UserID:        username,
		Filter:        filter,
		OutputChannel: ch,
		BufferSize:    100,
		Params:        params,
	}

	err := sm.createGenericEventBusSubscription(ctx, config, func(sub *GraphQLSubscription, out interface{}) {
		sm.processMetricsEvents(sub, out.(chan *model.MetricsUpdate), categories, services, threshold)
	})

	if err != nil {
		return nil, err
	}
	return ch, nil
}

// processMetricsEvents processes metrics events from the event bus
func (sm *GraphQLSubscriptionManager) processMetricsEvents(subscription *GraphQLSubscription, ch chan *model.MetricsUpdate, categories, services []string, threshold *float64) {
	defer sm.cleanupMetricsSubscription(subscription, ch)

	for {
		select {
		case event := <-subscription.Subscriber.Channel:
			if event == nil {
				return
			}
			sm.handleMetricsEvent(subscription, event, ch, categories, services, threshold)

		case <-subscription.Subscriber.Quit:
			return
		case <-subscription.Context.Done():
			return
		}
	}
}

// processGenericEvents provides a generic event processing pattern for all subscription types
func (sm *GraphQLSubscriptionManager) processGenericEvents(subscription *GraphQLSubscription, ch interface{}, eventType string, converter func(*streaming.InternalEvent) interface{}) {
	defer func() {
		// Close the channel using reflection since we don't know the exact type
		if closer, ok := ch.(interface{ Close() }); ok {
			closer.Close()
		} else {
			// For channels, we need to close them properly
			sm.closeChannel(ch)
		}
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

			if converted := converter(event); converted != nil {
				sm.sendConvertedEvent(subscription, event, ch, converted, eventType)
			}

		case <-subscription.Subscriber.Quit:
			return
		case <-subscription.Context.Done():
			return
		}
	}
}

// closeChannel safely closes a channel using reflection
func (sm *GraphQLSubscriptionManager) closeChannel(ch interface{}) {
	// Use reflection to close the channel since we don't know the exact type
	// This is a temporary solution - ideally we'd use a better interface design
	switch c := ch.(type) {
	case chan *model.Notification:
		close(c)
	case chan *moderation.ModerationDecision:
		close(c)
	case chan *trust.TrustEdge:
		close(c)
	case chan *model.AIAnalysis:
		close(c)
	case chan *model.CostUpdate:
		close(c)
	}
}

// sendConvertedEvent sends a converted event to the appropriate channel
func (sm *GraphQLSubscriptionManager) sendConvertedEvent(subscription *GraphQLSubscription, event *streaming.InternalEvent, ch interface{}, converted interface{}, eventType string) {
	// Handle different channel types
	switch c := ch.(type) {
	case chan *model.Notification:
		if notif, ok := converted.(*model.Notification); ok {
			select {
			case c <- notif:
				sm.logger.Debug("sent "+eventType+" event to GraphQL subscription",
					zap.String("subscription_id", subscription.ID),
					zap.String("event_id", event.ID))
			case <-subscription.Context.Done():
				return
			default:
				sm.logger.Warn(eventType+" subscription channel full, dropping event",
					zap.String("subscription_id", subscription.ID))
			}
		}
	case chan *moderation.ModerationDecision:
		if decision, ok := converted.(*moderation.ModerationDecision); ok {
			select {
			case c <- decision:
				sm.logger.Debug("sent "+eventType+" event to GraphQL subscription",
					zap.String("subscription_id", subscription.ID),
					zap.String("event_id", event.ID))
			case <-subscription.Context.Done():
				return
			default:
				sm.logger.Warn(eventType+" subscription channel full, dropping event",
					zap.String("subscription_id", subscription.ID))
			}
		}
	case chan *trust.TrustEdge:
		if edge, ok := converted.(*trust.TrustEdge); ok {
			select {
			case c <- edge:
				sm.logger.Debug("sent "+eventType+" event to GraphQL subscription",
					zap.String("subscription_id", subscription.ID),
					zap.String("event_id", event.ID))
			case <-subscription.Context.Done():
				return
			default:
				sm.logger.Warn(eventType+" subscription channel full, dropping event",
					zap.String("subscription_id", subscription.ID))
			}
		}
	case chan *model.AIAnalysis:
		if analysis, ok := converted.(*model.AIAnalysis); ok {
			select {
			case c <- analysis:
				sm.logger.Debug("sent "+eventType+" event to GraphQL subscription",
					zap.String("subscription_id", subscription.ID),
					zap.String("event_id", event.ID))
			case <-subscription.Context.Done():
				return
			default:
				sm.logger.Warn(eventType+" subscription channel full, dropping event",
					zap.String("subscription_id", subscription.ID))
			}
		}
	case chan *model.CostUpdate:
		if cost, ok := converted.(*model.CostUpdate); ok {
			select {
			case c <- cost:
				sm.logger.Debug("sent "+eventType+" event to GraphQL subscription",
					zap.String("subscription_id", subscription.ID),
					zap.String("event_id", event.ID))
			case <-subscription.Context.Done():
				return
			default:
				sm.logger.Warn(eventType+" subscription channel full, dropping event",
					zap.String("subscription_id", subscription.ID))
			}
		}
	}
}

// cleanupMetricsSubscription handles cleanup when metrics subscription ends
func (sm *GraphQLSubscriptionManager) cleanupMetricsSubscription(subscription *GraphQLSubscription, ch chan *model.MetricsUpdate) {
	close(ch)
	sm.subscriptionsMux.Lock()
	delete(sm.subscriptions, subscription.ID)
	sm.subscriptionsMux.Unlock()
}

// handleMetricsEvent processes a single metrics event
func (sm *GraphQLSubscriptionManager) handleMetricsEvent(subscription *GraphQLSubscription, event *streaming.InternalEvent, ch chan *model.MetricsUpdate, categories, services []string, threshold *float64) {
	subscription.LastActivity = time.Now()

	metricsUpdate := sm.converter.ConvertToMetricsUpdate(event)
	if metricsUpdate == nil {
		return
	}

	if !sm.shouldSendMetricsUpdate(metricsUpdate, categories, services, threshold) {
		return
	}

	sm.sendMetricsUpdate(subscription, event, ch, metricsUpdate)
}

// shouldSendMetricsUpdate determines if a metrics update should be sent based on filters
func (sm *GraphQLSubscriptionManager) shouldSendMetricsUpdate(update *model.MetricsUpdate, categories, services []string, threshold *float64) bool {
	if !sm.matchesCategories(update, categories) {
		return false
	}

	if !sm.matchesServices(update, services) {
		return false
	}

	if !sm.meetsThreshold(update, threshold) {
		return false
	}

	return true
}

// matchesCategories checks if the update matches the category filter
func (sm *GraphQLSubscriptionManager) matchesCategories(update *model.MetricsUpdate, categories []string) bool {
	if err := common.ValidateSliceNotEmpty("categories", categories); err != nil {
		return true
	}

	for _, category := range categories {
		if update.SubscriptionCategory == category {
			return true
		}
	}
	return false
}

// matchesServices checks if the update matches the service filter
func (sm *GraphQLSubscriptionManager) matchesServices(update *model.MetricsUpdate, services []string) bool {
	if err := common.ValidateSliceNotEmpty("services", services); err != nil {
		return true
	}

	for _, service := range services {
		if update.ServiceName == service {
			return true
		}
	}
	return false
}

// meetsThreshold checks if the update meets the threshold requirements
func (sm *GraphQLSubscriptionManager) meetsThreshold(update *model.MetricsUpdate, threshold *float64) bool {
	if threshold == nil {
		return true
	}
	return update.Sum >= *threshold || update.Max >= *threshold
}

// sendMetricsUpdate attempts to send the metrics update through the channel
func (sm *GraphQLSubscriptionManager) sendMetricsUpdate(subscription *GraphQLSubscription, event *streaming.InternalEvent, ch chan *model.MetricsUpdate, metricsUpdate *model.MetricsUpdate) {
	select {
	case ch <- metricsUpdate:
		sm.logMetricsEventSent(subscription, event, metricsUpdate)
	case <-subscription.Context.Done():
		return
	default:
		sm.logMetricsEventDropped(subscription, metricsUpdate)
	}
}

// logMetricsEventSent logs when a metrics event is successfully sent
func (sm *GraphQLSubscriptionManager) logMetricsEventSent(subscription *GraphQLSubscription, event *streaming.InternalEvent, metricsUpdate *model.MetricsUpdate) {
	sm.logger.Debug("sent metrics event to GraphQL subscription",
		zap.String("subscription_id", subscription.ID),
		zap.String("event_id", event.ID),
		zap.String("metric_type", metricsUpdate.MetricType),
		zap.String("service", metricsUpdate.ServiceName),
		zap.String("category", metricsUpdate.SubscriptionCategory))
}

// logMetricsEventDropped logs when a metrics event is dropped due to full channel
func (sm *GraphQLSubscriptionManager) logMetricsEventDropped(subscription *GraphQLSubscription, metricsUpdate *model.MetricsUpdate) {
	sm.logger.Warn("metrics subscription channel full, dropping event",
		zap.String("subscription_id", subscription.ID),
		zap.String("metric_id", metricsUpdate.MetricID))
}
