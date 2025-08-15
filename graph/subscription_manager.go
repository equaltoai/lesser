package graph

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/moderation"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/equaltoai/lesser/pkg/trust"
	"go.uber.org/zap"
)

// Stream name constants
const (
	StreamNamePublic = "public"
)

// GraphQLSubscriptionManager manages lifecycle of all GraphQL subscriptions
// and connects to the stream-router's event bus for real-time updates
//
//nolint:revive // Named this way for clarity in a codebase with multiple subscription types
type GraphQLSubscriptionManager struct {
	logger           *zap.Logger
	eventBus         *streaming.EventBus
	converter        *EventConverter
	subscriptions    map[string]*GraphQLSubscription
	subscriptionsMux sync.RWMutex
	running          bool
	runningMux       sync.RWMutex
}

// GraphQLSubscription represents an active GraphQL subscription
//
//nolint:revive // Named this way for clarity in a codebase with multiple subscription types
type GraphQLSubscription struct {
	ID            string
	Type          string                 // timeline, notification, cost, etc.
	UserID        string                 // User context
	Params        map[string]interface{} // Subscription parameters
	Filter        *streaming.EventFilter // Event bus filter
	Subscriber    *streaming.Subscriber  // Event bus subscriber
	OutputChannel interface{}            // Typed output channel
	Context       context.Context        // Subscription context
	Cancel        context.CancelFunc     // Cancel function
	Created       time.Time
	LastActivity  time.Time
}

// NewGraphQLSubscriptionManager creates a new GraphQL subscription manager
func NewGraphQLSubscriptionManager(eventBus *streaming.EventBus, logger *zap.Logger) *GraphQLSubscriptionManager {
	if logger == nil {
		logger = zap.NewNop()
	}

	converter := NewEventConverter(logger)

	return &GraphQLSubscriptionManager{
		logger:        logger,
		eventBus:      eventBus,
		converter:     converter,
		subscriptions: make(map[string]*GraphQLSubscription),
		running:       false,
	}
}

// Start starts the subscription manager
func (sm *GraphQLSubscriptionManager) Start(ctx context.Context) error {
	sm.runningMux.Lock()
	defer sm.runningMux.Unlock()

	if sm.running {
		return fmt.Errorf("subscription manager is already running")
	}

	sm.running = true

	// Start cleanup routine
	go sm.cleanupLoop(ctx)

	sm.logger.Info("GraphQL subscription manager started")
	return nil
}

// Stop stops the subscription manager and cleans up all subscriptions
func (sm *GraphQLSubscriptionManager) Stop() error {
	sm.runningMux.Lock()
	defer sm.runningMux.Unlock()

	if !sm.running {
		return nil
	}

	sm.running = false

	// Cancel all active subscriptions
	sm.subscriptionsMux.Lock()
	for _, sub := range sm.subscriptions {
		sm.cleanupSubscription(sub)
	}
	sm.subscriptions = make(map[string]*GraphQLSubscription)
	sm.subscriptionsMux.Unlock()

	sm.logger.Info("GraphQL subscription manager stopped")
	return nil
}

// IsRunning returns whether the subscription manager is running
func (sm *GraphQLSubscriptionManager) IsRunning() bool {
	sm.runningMux.RLock()
	defer sm.runningMux.RUnlock()
	return sm.running
}

// SubscribeToTimeline subscribes to timeline updates using the event bus
func (sm *GraphQLSubscriptionManager) SubscribeToTimeline(ctx context.Context, username string, timelineType model.TimelineType) (<-chan *model.Object, error) {
	if !sm.IsRunning() {
		return nil, fmt.Errorf("subscription manager is not running")
	}

	// Create output channel
	ch := make(chan *model.Object, 100)

	// Determine stream name based on timeline type
	var streamName string
	switch timelineType {
	case model.TimelineTypeHome:
		streamName = fmt.Sprintf("user:%s", username)
	case model.TimelineTypePublic:
		streamName = StreamNamePublic
	case model.TimelineTypeLocal:
		streamName = "public:local"
	case model.TimelineTypeDirect:
		streamName = fmt.Sprintf("direct:%s", username)
	default:
		streamName = fmt.Sprintf("user:%s", username)
	}

	// Create event filter for status events
	filter := &streaming.EventFilter{
		Types: []streaming.EventType{
			streaming.EventTypeStatus,
			streaming.EventTypeStatusUpdate,
		},
		Streams:     []string{streamName},
		MinPriority: streaming.PriorityNormal,
	}

	subscriptionID := fmt.Sprintf("timeline_%s_%s_%d", username, timelineType, time.Now().UnixNano())

	// Use event bus for timeline subscription
	if sm.eventBus == nil || !sm.eventBus.IsRunning() {
		return nil, fmt.Errorf("event bus is not available for timeline subscription")
	}

	return sm.createEventBusSubscription(ctx, subscriptionID, "timeline", username, filter, ch)
}

// SubscribeToNotifications subscribes to notification events using the event bus
func (sm *GraphQLSubscriptionManager) SubscribeToNotifications(ctx context.Context, username string) (<-chan *model.Notification, error) {
	if !sm.IsRunning() {
		return nil, fmt.Errorf("subscription manager is not running")
	}

	ch := make(chan *model.Notification, 50)

	filter := &streaming.EventFilter{
		Types: []streaming.EventType{
			streaming.EventTypeNotification,
		},
		UserID:      username,
		MinPriority: streaming.PriorityHigh,
	}

	subscriptionID := fmt.Sprintf("notifications_%s_%d", username, time.Now().UnixNano())

	// Use event bus for notification subscription
	if sm.eventBus == nil || !sm.eventBus.IsRunning() {
		return nil, fmt.Errorf("event bus is not available for notification subscription")
	}

	return sm.createNotificationEventBusSubscription(ctx, subscriptionID, username, filter, ch)
}

// SubscribeToCostUpdates subscribes to cost update events using the event bus
func (sm *GraphQLSubscriptionManager) SubscribeToCostUpdates(ctx context.Context, username string, threshold *int) (<-chan *model.CostUpdate, error) {
	if !sm.IsRunning() {
		return nil, fmt.Errorf("subscription manager is not running")
	}

	ch := make(chan *model.CostUpdate, 20)

	filter := &streaming.EventFilter{
		Types: []streaming.EventType{
			streaming.EventTypeCostUpdate,
			streaming.EventTypeCostAlert,
		},
		UserID:      username,
		MinPriority: streaming.PriorityNormal,
	}

	subscriptionID := fmt.Sprintf("cost_%s_%d", username, time.Now().UnixNano())

	// Use event bus for cost update subscription
	if sm.eventBus == nil || !sm.eventBus.IsRunning() {
		return nil, fmt.Errorf("event bus is not available for cost update subscription")
	}

	return sm.createCostEventBusSubscription(ctx, subscriptionID, username, filter, ch, threshold)
}

// SubscribeToModerationEvents subscribes to moderation events using the event bus
func (sm *GraphQLSubscriptionManager) SubscribeToModerationEvents(ctx context.Context, actorID *string) (<-chan *moderation.ModerationDecision, error) {
	if !sm.IsRunning() {
		return nil, fmt.Errorf("subscription manager is not running")
	}

	ch := make(chan *moderation.ModerationDecision, 50)

	filter := &streaming.EventFilter{
		Types: []streaming.EventType{
			streaming.EventTypeModeration,
			streaming.EventTypeModerationFlag,
			streaming.EventTypeModerationReview,
		},
		MinPriority: streaming.PriorityHigh,
	}

	if actorID != nil && *actorID != "" {
		filter.ActorID = *actorID
	}

	subscriptionID := fmt.Sprintf("moderation_%s_%d", getStringValue(actorID), time.Now().UnixNano())

	// Use event bus for moderation subscription
	if sm.eventBus == nil || !sm.eventBus.IsRunning() {
		return nil, fmt.Errorf("event bus is not available for moderation subscription")
	}

	return sm.createModerationEventBusSubscription(ctx, subscriptionID, actorID, filter, ch)
}

// SubscribeToTrustUpdates subscribes to trust score updates using the event bus
func (sm *GraphQLSubscriptionManager) SubscribeToTrustUpdates(ctx context.Context, actorID string) (<-chan *trust.TrustEdge, error) {
	if !sm.IsRunning() {
		return nil, fmt.Errorf("subscription manager is not running")
	}

	ch := make(chan *trust.TrustEdge, 20)

	filter := &streaming.EventFilter{
		Types: []streaming.EventType{
			streaming.EventTypeTrustUpdate,
			streaming.EventTypeReputationUpdate,
			streaming.EventTypeVouchUpdate,
		},
		ActorID:     actorID,
		MinPriority: streaming.PriorityNormal,
	}

	subscriptionID := fmt.Sprintf("trust_%s_%d", actorID, time.Now().UnixNano())

	// Use event bus for trust subscription
	if sm.eventBus == nil || !sm.eventBus.IsRunning() {
		return nil, fmt.Errorf("event bus is not available for trust subscription")
	}

	return sm.createTrustEventBusSubscription(ctx, subscriptionID, actorID, filter, ch)
}

// SubscribeToAIAnalysis subscribes to AI analysis updates using the event bus
func (sm *GraphQLSubscriptionManager) SubscribeToAIAnalysis(ctx context.Context, objectID *string) (<-chan *model.AIAnalysis, error) {
	if !sm.IsRunning() {
		return nil, fmt.Errorf("subscription manager is not running")
	}

	ch := make(chan *model.AIAnalysis, 20)

	filter := &streaming.EventFilter{
		Types: []streaming.EventType{
			streaming.EventTypeAIAnalysis,
			streaming.EventTypeAIClassification,
			streaming.EventTypeAIModeration,
		},
		MinPriority: streaming.PriorityNormal,
	}

	if objectID != nil && *objectID != "" {
		if filter.Metadata == nil {
			filter.Metadata = make(map[string]string)
		}
		filter.Metadata["target_id"] = *objectID
	}

	subscriptionID := fmt.Sprintf("ai_%s_%d", getStringValue(objectID), time.Now().UnixNano())

	// Use event bus for AI analysis subscription
	if sm.eventBus == nil || !sm.eventBus.IsRunning() {
		return nil, fmt.Errorf("event bus is not available for AI analysis subscription")
	}

	return sm.createAIEventBusSubscription(ctx, subscriptionID, objectID, filter, ch)
}

// SubscribeToHashtagActivity subscribes to hashtag activity using the event bus
func (sm *GraphQLSubscriptionManager) SubscribeToHashtagActivity(ctx context.Context, username string, hashtags []string) (<-chan *model.HashtagActivityUpdate, error) {
	if !sm.IsRunning() {
		return nil, fmt.Errorf("subscription manager is not running")
	}

	if len(hashtags) == 0 {
		return nil, fmt.Errorf("at least one hashtag must be specified")
	}

	ch := make(chan *model.HashtagActivityUpdate, 100)

	filter := &streaming.EventFilter{
		Types: []streaming.EventType{
			streaming.EventTypeHashtagUpdate,
			streaming.EventTypeHashtagTrend,
		},
		MinPriority: streaming.PriorityNormal,
	}

	// Add hashtag metadata filters
	filter.Metadata = make(map[string]string)
	for i, hashtag := range hashtags {
		filter.Metadata[fmt.Sprintf("hashtag_%d", i)] = hashtag
	}

	subscriptionID := fmt.Sprintf("hashtag_%s_%d", username, time.Now().UnixNano())

	// Use event bus for hashtag activity subscription
	if sm.eventBus == nil || !sm.eventBus.IsRunning() {
		return nil, fmt.Errorf("event bus is not available for hashtag activity subscription")
	}

	return sm.createHashtagEventBusSubscription(ctx, subscriptionID, username, hashtags, filter, ch)
}

// SubscribeToQuoteActivity subscribes to quote activity using the event bus
func (sm *GraphQLSubscriptionManager) SubscribeToQuoteActivity(ctx context.Context, username string, noteID string, noteObj any) (<-chan *model.QuoteActivityUpdate, error) {
	if !sm.IsRunning() {
		return nil, fmt.Errorf("subscription manager is not running")
	}

	if noteID == "" {
		return nil, fmt.Errorf("noteID cannot be empty")
	}
	if username == "" {
		return nil, fmt.Errorf("username cannot be empty")
	}

	ch := make(chan *model.QuoteActivityUpdate, 50)

	filter := &streaming.EventFilter{
		Types: []streaming.EventType{
			streaming.EventTypeStatus, // Quotes are statuses that reference the original
		},
		Streams:     []string{fmt.Sprintf("quote:%s", noteID)},
		MinPriority: streaming.PriorityNormal,
	}

	subscriptionID := fmt.Sprintf("quote_%s_%s_%d", username, noteID, time.Now().UnixNano())

	// Use event bus for quote activity subscription
	if sm.eventBus == nil || !sm.eventBus.IsRunning() {
		return nil, fmt.Errorf("event bus is not available for quote activity subscription")
	}

	return sm.createQuoteEventBusSubscription(ctx, subscriptionID, username, noteID, noteObj, filter, ch)
}

// SubscribeToMetricsUpdates subscribes to real-time metrics updates using the event bus
func (sm *GraphQLSubscriptionManager) SubscribeToMetricsUpdates(ctx context.Context, username string, categories []string, services []string, threshold *float64) (<-chan *model.MetricsUpdate, error) {
	if !sm.IsRunning() {
		return nil, fmt.Errorf("subscription manager is not running")
	}

	ch := make(chan *model.MetricsUpdate, 100)

	// Create filter for metrics events
	filter := &streaming.EventFilter{
		Types: []streaming.EventType{
			streaming.EventTypeMetricsUpdate,
		},
		MinPriority: streaming.PriorityNormal,
	}

	// Build streams for metrics filtering
	streams := []string{"metrics:global"} // Always include global metrics
	if len(categories) > 0 {
		for _, category := range categories {
			streams = append(streams, fmt.Sprintf("metrics:%s", category))
			if username != "" {
				streams = append(streams, fmt.Sprintf("metrics:%s:user:%s", category, username))
			}
		}
	}
	if len(services) > 0 {
		for _, service := range services {
			streams = append(streams, fmt.Sprintf("metrics:service:%s", service))
		}
	}
	if username != "" {
		streams = append(streams, fmt.Sprintf("metrics:user:%s", username))
	}

	filter.Streams = streams

	// Add category metadata filters if specified
	if len(categories) > 0 {
		if filter.Metadata == nil {
			filter.Metadata = make(map[string]string)
		}
		filter.Metadata["subscription_type"] = SubscriptionTypeMetrics
	}

	subscriptionID := fmt.Sprintf("metrics_%s_%d", username, time.Now().UnixNano())

	// Use event bus for metrics subscription
	if sm.eventBus == nil || !sm.eventBus.IsRunning() {
		return nil, fmt.Errorf("event bus is not available for metrics subscription")
	}

	return sm.createMetricsEventBusSubscription(ctx, subscriptionID, username, categories, services, threshold, filter, ch)
}

// Helper function to get string value from pointer
func getStringValue(ptr *string) string {
	if ptr == nil {
		return ""
	}
	return *ptr
}

// cleanupLoop periodically cleans up inactive subscriptions
func (sm *GraphQLSubscriptionManager) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			sm.cleanupInactiveSubscriptions()
		case <-ctx.Done():
			return
		}
	}
}

// cleanupInactiveSubscriptions removes subscriptions that have been cancelled or are stale
func (sm *GraphQLSubscriptionManager) cleanupInactiveSubscriptions() {
	sm.subscriptionsMux.Lock()
	defer sm.subscriptionsMux.Unlock()

	cutoff := time.Now().Add(-5 * time.Minute)
	toRemove := make([]string, 0)

	for id, sub := range sm.subscriptions {
		select {
		case <-sub.Context.Done():
			// Context is cancelled, remove subscription
			toRemove = append(toRemove, id)
		default:
			// Check if subscription is stale
			if sub.LastActivity.Before(cutoff) {
				toRemove = append(toRemove, id)
			}
		}
	}

	for _, id := range toRemove {
		if sub, exists := sm.subscriptions[id]; exists {
			sm.cleanupSubscription(sub)
			delete(sm.subscriptions, id)
			sm.logger.Debug("cleaned up inactive GraphQL subscription",
				zap.String("subscription_id", id),
				zap.String("type", sub.Type))
		}
	}

	if len(toRemove) > 0 {
		sm.logger.Info("GraphQL subscription cleanup completed",
			zap.Int("removed_subscriptions", len(toRemove)),
			zap.Int("active_subscriptions", len(sm.subscriptions)))
	}
}

// cleanupSubscription cleans up a single subscription
func (sm *GraphQLSubscriptionManager) cleanupSubscription(sub *GraphQLSubscription) {
	if sub.Cancel != nil {
		sub.Cancel()
	}
	if sub.Subscriber != nil && sm.eventBus != nil {
		_ = sm.eventBus.Unsubscribe(sub.Subscriber.ID)
	}
}

// GetStats returns statistics about active subscriptions
func (sm *GraphQLSubscriptionManager) GetStats() map[string]interface{} {
	sm.subscriptionsMux.RLock()
	defer sm.subscriptionsMux.RUnlock()

	stats := make(map[string]interface{})
	stats["total_subscriptions"] = len(sm.subscriptions)
	stats["event_bus_available"] = sm.eventBus != nil && sm.eventBus.IsRunning()

	// Count by type
	typeCounts := make(map[string]int)
	for _, sub := range sm.subscriptions {
		typeCounts[sub.Type]++
	}
	stats["by_type"] = typeCounts

	return stats
}
