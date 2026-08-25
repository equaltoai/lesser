package graph

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/moderation"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/equaltoai/lesser/pkg/trust"
	"go.uber.org/zap"
)

// Stream name constants
const (
	StreamNamePublic = "public"

	subscriptionTypeConversation = "conversation"
)

// Context key for WebSocket connection ID
// Note: contextKey is already defined in dataloader.go, so we reuse it
const (
	contextKeyConnectionID = contextKey("ws_connection_id")
)

// GraphQLSubscriptionManager manages lifecycle of all GraphQL subscriptions
// using DynamoDB-backed persistence and queue-based event delivery
//
//nolint:revive // Named this way for clarity in a codebase with multiple subscription types
type GraphQLSubscriptionManager struct {
	logger           *zap.Logger
	connRepo         interfaces.StreamingConnectionRepository
	publisher        streaming.Publisher
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
	ConnectionID  string                 // WebSocket connection ID
	Params        map[string]interface{} // Subscription parameters
	Streams       []string               // Stream names subscribed to
	OutputChannel interface{}            // Typed output channel
	Context       context.Context        // Subscription context
	Cancel        context.CancelFunc     // Cancel function
	Created       time.Time
	LastActivity  time.Time
}

// NewGraphQLSubscriptionManager creates a new GraphQL subscription manager with DynamoDB backing
func NewGraphQLSubscriptionManager(
	connRepo interfaces.StreamingConnectionRepository,
	publisher streaming.Publisher,
	logger *zap.Logger,
) *GraphQLSubscriptionManager {
	if logger == nil {
		logger = zap.NewNop()
	}

	converter := NewEventConverter(logger)

	return &GraphQLSubscriptionManager{
		logger:        logger,
		connRepo:      connRepo,
		publisher:     publisher,
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
		return ErrSubscriptionManagerAlreadyRunning
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

// Helper functions for WebSocket context and subscription management

// WithConnectionID adds connection ID to context
func WithConnectionID(ctx context.Context, connectionID string) context.Context {
	if connectionID == "" {
		return ctx
	}
	return context.WithValue(ctx, contextKeyConnectionID, connectionID)
}

// connectionIDFromContext extracts connection ID from context
func connectionIDFromContext(ctx context.Context) string {
	if connectionID, ok := ctx.Value(contextKeyConnectionID).(string); ok {
		return connectionID
	}
	return ""
}

// createSubscriptionRecord writes a subscription to DynamoDB for the given streams
func (sm *GraphQLSubscriptionManager) createSubscriptionRecord(
	ctx context.Context,
	subscriptionID, userID string,
	streams []string,
) (string, error) {
	if sm.connRepo == nil {
		sm.logger.Error("subscription repository unavailable",
			zap.String("subscription_id", subscriptionID),
			zap.String("user_id", userID))
		return "", errors.ServiceUnavailable("subscription repository")
	}
	connectionID := sm.resolveConnectionID(ctx, userID)
	sm.logger.Info("subscription context resolved",
		zap.String("subscription_id", subscriptionID),
		zap.String("connection_id", connectionID),
		zap.String("user_id", userID),
		zap.Strings("streams", streams))
	if connectionID == "" {
		sm.logger.Error("unable to resolve connection id for subscription",
			zap.String("subscription_id", subscriptionID),
			zap.String("user_id", userID),
			zap.Strings("streams", streams))
		return "", errors.ServiceUnavailable("websocket connection")
	}

	// Create subscription records for each stream using a background context so we
	// are not coupled to gqlgen's request lifecycle (which cancels quickly). Some
	// deployments route DynamoDB through VPC endpoints that add initial connection
	// latency, so give the write a bit more headroom while still bounding it.
	persistCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Preserve log context by attaching connectionID information manually.
	for _, stream := range streams {
		start := time.Now()
		sm.logger.Info("persisting subscription stream",
			zap.String("subscription_id", subscriptionID),
			zap.String("connection_id", connectionID),
			zap.String("user_id", userID),
			zap.String("stream", stream))

		if err := sm.connRepo.WriteSubscription(persistCtx, connectionID, userID, stream); err != nil {
			sm.logger.Error("failed to persist subscription record",
				zap.String("subscription_id", subscriptionID),
				zap.String("connection_id", connectionID),
				zap.String("user_id", userID),
				zap.String("stream", stream),
				zap.Error(err),
				zap.Duration("elapsed", time.Since(start)),
				zap.String("context_err", fmt.Sprint(persistCtx.Err())))
			return "", fmt.Errorf("failed to create subscription for stream %s: %w", stream, err)
		}

		sm.logger.Info("persisted subscription stream",
			zap.String("subscription_id", subscriptionID),
			zap.String("connection_id", connectionID),
			zap.String("user_id", userID),
			zap.String("stream", stream),
			zap.Duration("elapsed", time.Since(start)))
	}

	sm.logger.Info("created subscription records",
		zap.String("subscription_id", subscriptionID),
		zap.String("connection_id", connectionID),
		zap.String("user_id", userID),
		zap.Strings("streams", streams))

	return connectionID, nil
}

// resolveConnectionID attempts to determine the websocket connection ID for a subscription.
// Primary source is the context (set by the graphql-ws handler). If missing—such as during
// resolver instrumentation or atypical gqlgen lifecycles—we fall back to the connection
// repository to locate an active connection for the subscribing user.
func (sm *GraphQLSubscriptionManager) resolveConnectionID(ctx context.Context, userID string) string {
	if id := connectionIDFromContext(ctx); id != "" {
		return id
	}

	if sm.connRepo == nil || userID == "" {
		return ""
	}

	lookupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	connections, err := sm.connRepo.GetConnectionsByUser(lookupCtx, userID)
	if err != nil {
		sm.logger.Warn("failed to lookup connections for subscription",
			zap.String("user_id", userID),
			zap.Error(err))
		return ""
	}

	if len(connections) == 0 {
		return ""
	}

	for _, conn := range connections {
		if conn.State == models.ConnectionStateConnected {
			return conn.ConnectionID
		}
	}

	// Fall back to the first connection if none are actively connected.
	return connections[0].ConnectionID
}

// deleteSubscriptionRecords removes subscriptions from DynamoDB for the given streams
func (sm *GraphQLSubscriptionManager) deleteSubscriptionRecords(
	ctx context.Context,
	connectionID string,
	streams []string,
) error {
	if connectionID == "" {
		return nil // No connection ID means nothing to delete
	}

	if ctx == nil {
		ctx = context.Background()
	}

	deleteCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	for _, stream := range streams {
		if err := sm.connRepo.DeleteSubscription(deleteCtx, connectionID, stream); err != nil {
			sm.logger.Warn("failed to delete subscription",
				zap.String("connection_id", connectionID),
				zap.String("stream", stream),
				zap.Error(err))
			// Continue deleting other streams even if one fails
		}
	}

	return nil
}

// createGenericSubscription is a helper that creates a subscription with the given parameters
func (sm *GraphQLSubscriptionManager) createGenericSubscription(
	ctx context.Context,
	subscriptionType, userID string,
	streams []string,
	channelBuffer int,
	params map[string]interface{},
) (interface{}, string, error) {
	subscriptionID := fmt.Sprintf("%s_%s_%d", subscriptionType, userID, time.Now().UnixNano())

	// Create subscription record in DynamoDB
	connectionID, err := sm.createSubscriptionRecord(ctx, subscriptionID, userID, streams)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create %s subscription: %w", subscriptionType, err)
	}

	// Create channel based on type - we'll use interface{} and let caller cast
	var ch interface{}
	switch subscriptionType {
	case ServiceTypeTimeline:
		ch = make(chan *model.Object, channelBuffer)
	case "notification":
		ch = make(chan *model.Notification, channelBuffer)
	case EventTypeCost:
		ch = make(chan *model.CostUpdate, channelBuffer)
	case "moderation":
		ch = make(chan *moderation.ModerationDecision, channelBuffer)
	case ServiceTypeTrust:
		ch = make(chan *trust.TrustEdge, channelBuffer)
	case "ai":
		ch = make(chan *model.AIAnalysis, channelBuffer)
	case TimelineTypeHashtag:
		ch = make(chan *model.HashtagActivityUpdate, channelBuffer)
	case QuoteType:
		ch = make(chan *model.QuoteActivityUpdate, channelBuffer)
	case SubscriptionTypeMetrics:
		ch = make(chan *model.MetricsUpdate, channelBuffer)
	case TimelineTypeList:
		ch = make(chan *model.ListUpdate, channelBuffer)
	case subscriptionTypeConversation:
		ch = make(chan *model.Conversation, channelBuffer)
	case "federation":
		ch = make(chan *model.FederationHealthUpdate, channelBuffer)
	case streaming.CategoryRelationship:
		ch = make(chan *model.RelationshipUpdate, channelBuffer)
	case "budget":
		ch = make(chan *model.BudgetAlert, channelBuffer)
	case "moderation_alerts":
		ch = make(chan *model.ModerationAlert, channelBuffer)
	case "moderation_queue":
		ch = make(chan *model.ModerationItem, channelBuffer)
	case "cost_alerts":
		ch = make(chan *model.CostAlert, channelBuffer)
	case "performance":
		ch = make(chan *model.PerformanceAlert, channelBuffer)
	case "threat":
		ch = make(chan *model.ThreatAlert, channelBuffer)
	case "infrastructure":
		ch = make(chan *model.InfrastructureEvent, channelBuffer)
	default:
		ch = make(chan interface{}, channelBuffer)
	}

	var subscriptionParams map[string]interface{}
	if len(params) > 0 {
		subscriptionParams = make(map[string]interface{}, len(params))
		for key, value := range params {
			subscriptionParams[key] = value
		}
	}

	// Store subscription in memory for channel management
	//nolint:gosec // cancel is retained on GraphQLSubscription and invoked when the subscription or connection is removed.
	subCtx, cancel := context.WithCancel(ctx)
	sub := &GraphQLSubscription{
		ID:            subscriptionID,
		Type:          subscriptionType,
		UserID:        userID,
		ConnectionID:  connectionID,
		Streams:       streams,
		OutputChannel: ch,
		Context:       subCtx,
		Cancel:        cancel,
		Params:        subscriptionParams,
		Created:       time.Now(),
		LastActivity:  time.Now(),
	}

	sm.subscriptionsMux.Lock()
	sm.subscriptions[subscriptionID] = sub
	sm.subscriptionsMux.Unlock()

	sm.logger.Info("subscription created",
		zap.String("subscription_id", subscriptionID),
		zap.String("type", subscriptionType),
		zap.String("user_id", userID),
		zap.Strings("streams", streams),
		zap.Any("params", subscriptionParams))

	return ch, subscriptionID, nil
}

// SubscribeToTimeline subscribes to timeline updates via DynamoDB-backed subscriptions
func (sm *GraphQLSubscriptionManager) SubscribeToTimeline(
	ctx context.Context,
	username string,
	timelineType model.TimelineType,
	actorUsername *string,
	hashtag *string,
	listID *string,
) (<-chan *model.Object, error) {
	if !sm.IsRunning() {
		return nil, ErrSubscriptionManagerNotRunning
	}

	streamName, err := timelineStreamName(username, timelineType, timelineRoutingInputs{
		actorUsername: actorUsername,
		hashtag:       hashtag,
		listID:        listID,
	})
	if err != nil {
		return nil, err
	}

	streams := []string{streamName}
	ch, _, err := sm.createGenericSubscription(ctx, ServiceTypeTimeline, username, streams, 100, nil)
	if err != nil {
		return nil, err
	}

	return ch.(chan *model.Object), nil
}

// SubscribeToNotifications subscribes to notification events via DynamoDB-backed subscriptions
func (sm *GraphQLSubscriptionManager) SubscribeToNotifications(ctx context.Context, username string) (<-chan *model.Notification, error) {
	if !sm.IsRunning() {
		return nil, ErrSubscriptionManagerNotRunning
	}

	streams := []string{fmt.Sprintf("user:%s:notifications", username)}
	ch, _, err := sm.createGenericSubscription(ctx, "notification", username, streams, 50, nil)
	if err != nil {
		return nil, err
	}

	return ch.(chan *model.Notification), nil
}

// SubscribeToCostUpdates subscribes to cost update events via DynamoDB-backed subscriptions
func (sm *GraphQLSubscriptionManager) SubscribeToCostUpdates(ctx context.Context, username string, threshold *int) (<-chan *model.CostUpdate, error) {
	if !sm.IsRunning() {
		return nil, ErrSubscriptionManagerNotRunning
	}

	var subscriptionParams map[string]interface{}
	if threshold != nil {
		subscriptionParams = map[string]interface{}{
			"threshold": *threshold,
		}
	}

	streams := []string{fmt.Sprintf("%s:%s", EventTypeCost, username)}
	ch, _, err := sm.createGenericSubscription(ctx, EventTypeCost, username, streams, 20, subscriptionParams)
	if err != nil {
		return nil, err
	}

	return ch.(chan *model.CostUpdate), nil
}

// SubscribeToModerationEvents subscribes to moderation events via DynamoDB-backed subscriptions
func (sm *GraphQLSubscriptionManager) SubscribeToModerationEvents(ctx context.Context, actorID *string) (<-chan *moderation.ModerationDecision, error) {
	if !sm.IsRunning() {
		return nil, ErrSubscriptionManagerNotRunning
	}

	userID := getStringValue(actorID)
	if userID == "" {
		userID = "global"
	}

	streams := []string{fmt.Sprintf("moderation:%s", userID)}
	ch, _, err := sm.createGenericSubscription(ctx, "moderation", userID, streams, 50, nil)
	if err != nil {
		return nil, err
	}

	return ch.(chan *moderation.ModerationDecision), nil
}

// SubscribeToTrustUpdates subscribes to trust score updates via DynamoDB-backed subscriptions
func (sm *GraphQLSubscriptionManager) SubscribeToTrustUpdates(ctx context.Context, actorID string) (<-chan *trust.TrustEdge, error) {
	if !sm.IsRunning() {
		return nil, ErrSubscriptionManagerNotRunning
	}

	streams := []string{fmt.Sprintf("trust:%s", actorID)}
	ch, _, err := sm.createGenericSubscription(ctx, ServiceTypeTrust, actorID, streams, 20, nil)
	if err != nil {
		return nil, err
	}

	return ch.(chan *trust.TrustEdge), nil
}

// SubscribeToAIAnalysis subscribes to AI analysis updates via DynamoDB-backed subscriptions
func (sm *GraphQLSubscriptionManager) SubscribeToAIAnalysis(ctx context.Context, objectID *string) (<-chan *model.AIAnalysis, error) {
	if !sm.IsRunning() {
		return nil, ErrSubscriptionManagerNotRunning
	}

	userID := getStringValue(objectID)
	if userID == "" {
		userID = "global"
	}

	streams := []string{fmt.Sprintf("ai:%s", userID)}
	ch, _, err := sm.createGenericSubscription(ctx, "ai", userID, streams, 20, nil)
	if err != nil {
		return nil, err
	}

	return ch.(chan *model.AIAnalysis), nil
}

// SubscribeToHashtagActivity subscribes to hashtag activity via DynamoDB-backed subscriptions
func (sm *GraphQLSubscriptionManager) SubscribeToHashtagActivity(ctx context.Context, username string, hashtags []string) (<-chan *model.HashtagActivityUpdate, error) {
	if !sm.IsRunning() {
		return nil, ErrSubscriptionManagerNotRunning
	}

	if err := common.ValidateSliceNotEmpty("hashtags", hashtags); err != nil {
		return nil, ErrAtLeastOneHashtagRequired
	}

	// Create stream names for each hashtag
	streams := make([]string, len(hashtags))
	for i, hashtag := range hashtags {
		streams[i] = fmt.Sprintf("%s:%s", TimelineTypeHashtag, hashtag)
	}

	ch, _, err := sm.createGenericSubscription(ctx, TimelineTypeHashtag, username, streams, 100, nil)
	if err != nil {
		return nil, err
	}

	return ch.(chan *model.HashtagActivityUpdate), nil
}

// SubscribeToQuoteActivity subscribes to quote activity via DynamoDB-backed subscriptions
func (sm *GraphQLSubscriptionManager) SubscribeToQuoteActivity(ctx context.Context, username string, noteID string) (<-chan *model.QuoteActivityUpdate, error) {
	if !sm.IsRunning() {
		return nil, ErrSubscriptionManagerNotRunning
	}

	if err := common.ValidateRequiredParam("noteID", noteID); err != nil {
		return nil, ErrNoteIDCannotBeEmpty
	}
	if err := common.ValidateRequiredParam("username", username); err != nil {
		return nil, ErrUsernameCannotBeEmpty
	}

	streams := []string{fmt.Sprintf("%s:%s", QuoteType, noteID)}
	ch, _, err := sm.createGenericSubscription(ctx, QuoteType, username, streams, 50, nil)
	if err != nil {
		return nil, err
	}

	return ch.(chan *model.QuoteActivityUpdate), nil
}

// SubscribeToMetricsUpdates subscribes to real-time metrics updates via DynamoDB-backed subscriptions
func (sm *GraphQLSubscriptionManager) SubscribeToMetricsUpdates(ctx context.Context, username string, categories []string, services []string, threshold *float64) (<-chan *model.MetricsUpdate, error) {
	if !sm.IsRunning() {
		return nil, ErrSubscriptionManagerNotRunning
	}

	var subscriptionParams map[string]interface{}
	if threshold != nil {
		subscriptionParams = map[string]interface{}{
			"threshold": *threshold,
		}
	}

	// Build streams for metrics filtering
	streams := []string{fmt.Sprintf("%s:global", SubscriptionTypeMetrics)} // Always include global metrics
	if err := common.ValidateSliceNotEmpty("categories", categories); err == nil {
		for _, category := range categories {
			streams = append(streams, fmt.Sprintf("%s:%s", SubscriptionTypeMetrics, category))
			if username != "" {
				streams = append(streams, fmt.Sprintf("%s:%s:user:%s", SubscriptionTypeMetrics, category, username))
			}
		}
	}
	if err := common.ValidateSliceNotEmpty("services", services); err == nil {
		for _, service := range services {
			streams = append(streams, fmt.Sprintf("%s:service:%s", SubscriptionTypeMetrics, service))
		}
	}
	if username != "" {
		streams = append(streams, fmt.Sprintf("%s:user:%s", SubscriptionTypeMetrics, username))
	}

	ch, _, err := sm.createGenericSubscription(ctx, SubscriptionTypeMetrics, username, streams, 100, subscriptionParams)
	if err != nil {
		return nil, err
	}

	return ch.(chan *model.MetricsUpdate), nil
}

// SubscribeToListActivity subscribes to list activity updates via DynamoDB-backed subscriptions
func (sm *GraphQLSubscriptionManager) SubscribeToListActivity(ctx context.Context, username string, listID string) (<-chan *model.ListUpdate, error) {
	if !sm.IsRunning() {
		return nil, ErrSubscriptionManagerNotRunning
	}

	if err := common.ValidateRequiredParam("listID", listID); err != nil {
		return nil, ErrListIDCannotBeEmpty
	}
	if err := common.ValidateRequiredParam("username", username); err != nil {
		return nil, ErrUsernameCannotBeEmpty
	}

	streams := []string{fmt.Sprintf("%s:%s", TimelineTypeList, listID)}
	ch, _, err := sm.createGenericSubscription(ctx, TimelineTypeList, username, streams, 100, nil)
	if err != nil {
		return nil, err
	}

	return ch.(chan *model.ListUpdate), nil
}

// SubscribeToConversation subscribes to conversation updates via DynamoDB-backed subscriptions
func (sm *GraphQLSubscriptionManager) SubscribeToConversation(ctx context.Context, username string) (<-chan *model.Conversation, error) {
	if !sm.IsRunning() {
		return nil, ErrSubscriptionManagerNotRunning
	}

	if err := common.ValidateRequiredParam("username", username); err != nil {
		return nil, ErrUsernameCannotBeEmpty
	}

	streams := []string{fmt.Sprintf("conversation:%s", username)}
	ch, _, err := sm.createGenericSubscription(ctx, subscriptionTypeConversation, username, streams, 100, nil)
	if err != nil {
		return nil, err
	}

	return ch.(chan *model.Conversation), nil
}

// SubscribeToFederationHealth subscribes to federation health updates via DynamoDB-backed subscriptions
func (sm *GraphQLSubscriptionManager) SubscribeToFederationHealth(ctx context.Context, username string, domain *string) (<-chan *model.FederationHealthUpdate, error) {
	if !sm.IsRunning() {
		return nil, ErrSubscriptionManagerNotRunning
	}

	if err := common.ValidateRequiredParam("username", username); err != nil {
		return nil, ErrUsernameCannotBeEmpty
	}

	// Build streams based on domain filter
	var streams []string
	if domain != nil && *domain != "" {
		streams = []string{fmt.Sprintf("federation:%s", *domain)}
	} else {
		streams = []string{"federation:health"}
	}

	ch, _, err := sm.createGenericSubscription(ctx, "federation", username, streams, 100, nil)
	if err != nil {
		return nil, err
	}

	return ch.(chan *model.FederationHealthUpdate), nil
}

// SubscribeToRelationshipUpdates subscribes to relationship updates via DynamoDB-backed subscriptions
func (sm *GraphQLSubscriptionManager) SubscribeToRelationshipUpdates(ctx context.Context, username string, actorID *string) (<-chan *model.RelationshipUpdate, error) {
	if !sm.IsRunning() {
		return nil, ErrSubscriptionManagerNotRunning
	}

	if err := common.ValidateRequiredParam("username", username); err != nil {
		return nil, ErrUsernameCannotBeEmpty
	}

	// Determine stream target - either specific actor or user's relationships
	streamTarget := username
	if actorID != nil && *actorID != "" {
		streamTarget = *actorID
	}

	streams := []string{fmt.Sprintf("%s:%s", streaming.CategoryRelationship, streamTarget)}
	ch, _, err := sm.createGenericSubscription(ctx, streaming.CategoryRelationship, username, streams, 100, nil)
	if err != nil {
		return nil, err
	}

	return ch.(chan *model.RelationshipUpdate), nil
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

	if err := common.ValidateSliceNotEmpty("toRemove", toRemove); err == nil {
		sm.logger.Info("GraphQL subscription cleanup completed",
			zap.Int("removed_subscriptions", len(toRemove)),
			zap.Int("active_subscriptions", len(sm.subscriptions)))
	}
}

// cleanupSubscription cleans up a single subscription and removes from DynamoDB
func (sm *GraphQLSubscriptionManager) cleanupSubscription(sub *GraphQLSubscription) {
	if sub.Cancel != nil {
		sub.Cancel()
	}

	// Delete subscription records from DynamoDB
	if sub.ConnectionID != "" && sub.Streams != nil {
		ctx := context.Background()
		if err := sm.deleteSubscriptionRecords(ctx, sub.ConnectionID, sub.Streams); err != nil {
			sm.logger.Warn("failed to delete subscription records during cleanup",
				zap.String("subscription_id", sub.ID),
				zap.String("connection_id", sub.ConnectionID),
				zap.Error(err))
		}
	}
}

// SubscribeToBudgetAlerts subscribes to budget alert updates via DynamoDB-backed subscriptions
func (sm *GraphQLSubscriptionManager) SubscribeToBudgetAlerts(ctx context.Context, username string, domain *string) (<-chan *model.BudgetAlert, error) {
	if !sm.IsRunning() {
		return nil, ErrSubscriptionManagerNotRunning
	}

	if err := common.ValidateRequiredParam("username", username); err != nil {
		return nil, ErrUsernameCannotBeEmpty
	}

	// Build stream name based on domain
	var streamName string
	if domain != nil {
		streamName = fmt.Sprintf("budget_alerts:%s", *domain)
	} else {
		streamName = fmt.Sprintf("budget_alerts:%s", username)
	}

	streams := []string{streamName}
	ch, _, err := sm.createGenericSubscription(ctx, "budget", username, streams, 100, nil)
	if err != nil {
		return nil, err
	}

	return ch.(chan *model.BudgetAlert), nil
}

// SubscribeToModerationAlerts subscribes to moderation alert updates via DynamoDB-backed subscriptions
func (sm *GraphQLSubscriptionManager) SubscribeToModerationAlerts(ctx context.Context, username string, severity *model.ModerationSeverity) (<-chan *model.ModerationAlert, error) {
	if !sm.IsRunning() {
		return nil, ErrSubscriptionManagerNotRunning
	}

	var subscriptionParams map[string]interface{}
	if severity != nil {
		subscriptionParams = map[string]interface{}{
			"severity": *severity,
		}
	}

	if err := common.ValidateRequiredParam("username", username); err != nil {
		return nil, ErrUsernameCannotBeEmpty
	}

	// Build stream names for moderation alerts
	streams := []string{"moderation:alerts", fmt.Sprintf("moderation:alerts:%s", username)}

	ch, _, err := sm.createGenericSubscription(ctx, "moderation_alerts", username, streams, 100, subscriptionParams)
	if err != nil {
		return nil, err
	}

	return ch.(chan *model.ModerationAlert), nil
}

// SubscribeToCostAlerts subscribes to cost alert updates via DynamoDB-backed subscriptions
func (sm *GraphQLSubscriptionManager) SubscribeToCostAlerts(ctx context.Context, username string, thresholdUSD float64) (<-chan *model.CostAlert, error) {
	if !sm.IsRunning() {
		return nil, ErrSubscriptionManagerNotRunning
	}

	subscriptionParams := map[string]interface{}{
		"threshold": thresholdUSD,
	}

	if err := common.ValidateRequiredParam("username", username); err != nil {
		return nil, ErrUsernameCannotBeEmpty
	}

	streamName := fmt.Sprintf("cost_alerts:%s", username)
	streams := []string{streamName}

	ch, _, err := sm.createGenericSubscription(ctx, "cost_alerts", username, streams, 100, subscriptionParams)
	if err != nil {
		return nil, err
	}

	return ch.(chan *model.CostAlert), nil
}

// SubscribeToPerformanceAlerts subscribes to performance alert updates via DynamoDB-backed subscriptions
func (sm *GraphQLSubscriptionManager) SubscribeToPerformanceAlerts(ctx context.Context, username string, severity model.AlertSeverity) (<-chan *model.PerformanceAlert, error) {
	if !sm.IsRunning() {
		return nil, ErrSubscriptionManagerNotRunning
	}

	subscriptionParams := map[string]interface{}{
		"severity": severity,
	}

	if err := common.ValidateRequiredParam("username", username); err != nil {
		return nil, ErrUsernameCannotBeEmpty
	}

	streamName := fmt.Sprintf("performance:%s", username)
	streams := []string{streamName}

	ch, _, err := sm.createGenericSubscription(ctx, "performance", username, streams, 100, subscriptionParams)
	if err != nil {
		return nil, err
	}

	return ch.(chan *model.PerformanceAlert), nil
}

// SubscribeToThreatIntelligence subscribes to threat intelligence updates via DynamoDB-backed subscriptions
func (sm *GraphQLSubscriptionManager) SubscribeToThreatIntelligence(ctx context.Context, username string) (<-chan *model.ThreatAlert, error) {
	if !sm.IsRunning() {
		return nil, ErrSubscriptionManagerNotRunning
	}

	if err := common.ValidateRequiredParam("username", username); err != nil {
		return nil, ErrUsernameCannotBeEmpty
	}

	streamName := fmt.Sprintf("threat:%s", username)
	streams := []string{streamName}

	ch, _, err := sm.createGenericSubscription(ctx, "threat", username, streams, 100, nil)
	if err != nil {
		return nil, err
	}

	return ch.(chan *model.ThreatAlert), nil
}

// SubscribeToInfrastructureEvents subscribes to infrastructure event updates via DynamoDB-backed subscriptions
func (sm *GraphQLSubscriptionManager) SubscribeToInfrastructureEvents(ctx context.Context, username string) (<-chan *model.InfrastructureEvent, error) {
	if !sm.IsRunning() {
		return nil, ErrSubscriptionManagerNotRunning
	}

	if err := common.ValidateRequiredParam("username", username); err != nil {
		return nil, ErrUsernameCannotBeEmpty
	}

	streamName := fmt.Sprintf("infrastructure:%s", username)
	streams := []string{streamName}

	ch, _, err := sm.createGenericSubscription(ctx, "infrastructure", username, streams, 100, nil)
	if err != nil {
		return nil, err
	}

	return ch.(chan *model.InfrastructureEvent), nil
}

// SubscribeToModerationQueueUpdate subscribes to moderation queue updates via DynamoDB-backed subscriptions
func (sm *GraphQLSubscriptionManager) SubscribeToModerationQueueUpdate(ctx context.Context, username string, priority *model.Priority) (<-chan *model.ModerationItem, error) {
	if !sm.IsRunning() {
		return nil, ErrSubscriptionManagerNotRunning
	}

	if err := common.ValidateRequiredParam("username", username); err != nil {
		return nil, ErrUsernameCannotBeEmpty
	}

	// Construct stream names
	// Subscribe to general queue and optionally specific priority
	streams := []string{"moderation:queue"}
	if priority != nil {
		streams = append(streams, fmt.Sprintf("moderation:queue:%s", *priority))
	}

	var subscriptionParams map[string]interface{}
	if priority != nil {
		subscriptionParams = map[string]interface{}{
			"priority": *priority,
		}
	}

	ch, _, err := sm.createGenericSubscription(ctx, "moderation_queue", username, streams, 100, subscriptionParams)
	if err != nil {
		return nil, err
	}

	return ch.(chan *model.ModerationItem), nil
}

// GetStats returns statistics about active subscriptions
func (sm *GraphQLSubscriptionManager) GetStats() map[string]interface{} {
	sm.subscriptionsMux.RLock()
	defer sm.subscriptionsMux.RUnlock()

	stats := make(map[string]interface{})
	stats["total_subscriptions"] = len(sm.subscriptions)
	stats["repository_available"] = sm.connRepo != nil
	stats["publisher_available"] = sm.publisher != nil

	// Count by type
	typeCounts := make(map[string]int)
	for _, sub := range sm.subscriptions {
		typeCounts[sub.Type]++
	}
	stats["by_type"] = typeCounts

	return stats
}
