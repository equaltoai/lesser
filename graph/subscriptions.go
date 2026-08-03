package graph

import (
	"context"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/moderation"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/equaltoai/lesser/pkg/trust"
	"go.uber.org/zap"
)

// SubscriptionManager provides the GraphQL subscription API
// This is a wrapper around GraphQLSubscriptionManager to maintain compatibility
type SubscriptionManager struct {
	manager *GraphQLSubscriptionManager
	logger  *zap.Logger
}

// NewSubscriptionManager creates a new subscription manager with DynamoDB-backed persistence
func NewSubscriptionManager(
	connRepo interfaces.StreamingConnectionRepository,
	publisher streaming.Publisher,
	logger *zap.Logger,
) *SubscriptionManager {
	if logger == nil {
		logger = zap.NewNop()
	}

	manager := NewGraphQLSubscriptionManager(connRepo, publisher, logger)

	return &SubscriptionManager{
		manager: manager,
		logger:  logger,
	}
}

// Start starts the subscription manager
func (sm *SubscriptionManager) Start(ctx context.Context) error {
	return sm.manager.Start(ctx)
}

// Stop stops the subscription manager
func (sm *SubscriptionManager) Stop() error {
	return sm.manager.Stop()
}

// IsRunning returns whether the subscription manager is running
func (sm *SubscriptionManager) IsRunning() bool {
	return sm.manager.IsRunning()
}

// SubscribeToActivityStream creates a channel for activity stream updates
// NOTE: This method creates ActivityPub activities, not GraphQL Objects
func (sm *SubscriptionManager) SubscribeToActivityStream(ctx context.Context, username string, activityTypes []model.ActivityType) (<-chan *activitypub.Activity, error) {
	// For now, convert timeline subscription to activities
	objectsCh, err := sm.manager.SubscribeToTimeline(ctx, username, model.TimelineTypeHome, nil, nil, nil)
	if err != nil {
		return nil, err
	}

	// Create activity channel and convert objects to activities
	activityCh := make(chan *activitypub.Activity, 100)

	go func() {
		defer close(activityCh)

		for obj := range objectsCh {
			// Convert Object to Activity (simplified conversion)
			activity := &activitypub.Activity{
				BaseObject: activitypub.BaseObject{
					ID:   obj.ID,
					Type: activitypub.CreateType,
				},
				Actor:  obj.Actor.ID,
				Object: obj,
			}

			// Filter by activity types if specified
			if len(activityTypes) > 0 {
				typeMatch := false
				for _, t := range activityTypes {
					if string(t) == activity.Type {
						typeMatch = true
						break
					}
				}
				if !typeMatch {
					continue
				}
			}

			select {
			case activityCh <- activity:
			case <-ctx.Done():
				return
			}
		}
	}()

	return activityCh, nil
}

// SubscribeToTimelineUpdates creates a channel for timeline updates using event bus
func (sm *SubscriptionManager) SubscribeToTimelineUpdates(
	ctx context.Context,
	username string,
	timelineType model.TimelineType,
	actorUsername *string,
	hashtag *string,
	listID *string,
) (<-chan *model.Object, error) {
	return sm.manager.SubscribeToTimeline(ctx, username, timelineType, actorUsername, hashtag, listID)
}

// SubscribeToCostUpdates creates a channel for cost updates using event bus
func (sm *SubscriptionManager) SubscribeToCostUpdates(ctx context.Context, username string, threshold *int) (<-chan *model.CostUpdate, error) {
	return sm.manager.SubscribeToCostUpdates(ctx, username, threshold)
}

// SubscribeToModerationEvents creates a channel for moderation events using event bus
func (sm *SubscriptionManager) SubscribeToModerationEvents(ctx context.Context, actorID *string) (<-chan *moderation.ModerationDecision, error) {
	return sm.manager.SubscribeToModerationEvents(ctx, actorID)
}

// SubscribeToTrustUpdates creates a channel for trust score updates using event bus
func (sm *SubscriptionManager) SubscribeToTrustUpdates(ctx context.Context, actorID string) (<-chan *trust.TrustEdge, error) {
	return sm.manager.SubscribeToTrustUpdates(ctx, actorID)
}

// SubscribeToNotifications creates a channel for notification updates using event bus
func (sm *SubscriptionManager) SubscribeToNotifications(ctx context.Context, username string) (<-chan *model.Notification, error) {
	return sm.manager.SubscribeToNotifications(ctx, username)
}

// SubscribeToAIAnalysisUpdates creates a channel for AI analysis updates using event bus
func (sm *SubscriptionManager) SubscribeToAIAnalysisUpdates(ctx context.Context, objectID *string) (<-chan *model.AIAnalysis, error) {
	return sm.manager.SubscribeToAIAnalysis(ctx, objectID)
}

// SubscribeToHashtagActivity creates a channel for hashtag activity updates using event bus
func (sm *SubscriptionManager) SubscribeToHashtagActivity(ctx context.Context, username string, hashtags []string) (<-chan *model.HashtagActivityUpdate, error) {
	return sm.manager.SubscribeToHashtagActivity(ctx, username, hashtags)
}

// SubscribeToMetricsUpdates creates a channel for real-time metrics updates using event bus
func (sm *SubscriptionManager) SubscribeToMetricsUpdates(ctx context.Context, username string, categories []string, services []string, threshold *float64) (<-chan *model.MetricsUpdate, error) {
	return sm.manager.SubscribeToMetricsUpdates(ctx, username, categories, services, threshold)
}

// SubscribeToQuoteActivity creates a channel for quote activity updates using event bus
func (sm *SubscriptionManager) SubscribeToQuoteActivity(ctx context.Context, username string, noteID string) (<-chan *model.QuoteActivityUpdate, error) {
	return sm.manager.SubscribeToQuoteActivity(ctx, username, noteID)
}

// SubscribeToListActivity creates a channel for list activity updates using event bus
func (sm *SubscriptionManager) SubscribeToListActivity(ctx context.Context, username string, listID string) (<-chan *model.ListUpdate, error) {
	return sm.manager.SubscribeToListActivity(ctx, username, listID)
}

// SubscribeToConversation creates a channel for conversation updates using event bus
func (sm *SubscriptionManager) SubscribeToConversation(ctx context.Context, username string) (<-chan *model.Conversation, error) {
	return sm.manager.SubscribeToConversation(ctx, username)
}

// SubscribeToFederationHealth creates a channel for federation health updates using event bus
func (sm *SubscriptionManager) SubscribeToFederationHealth(ctx context.Context, username string, domain *string) (<-chan *model.FederationHealthUpdate, error) {
	return sm.manager.SubscribeToFederationHealth(ctx, username, domain)
}

// SubscribeToRelationshipUpdates creates a channel for relationship updates using event bus
func (sm *SubscriptionManager) SubscribeToRelationshipUpdates(ctx context.Context, username string, actorID *string) (<-chan *model.RelationshipUpdate, error) {
	return sm.manager.SubscribeToRelationshipUpdates(ctx, username, actorID)
}

// SubscribeToBudgetAlerts creates a channel for budget alert updates using event bus
func (sm *SubscriptionManager) SubscribeToBudgetAlerts(ctx context.Context, username string, domain *string) (<-chan *model.BudgetAlert, error) {
	return sm.manager.SubscribeToBudgetAlerts(ctx, username, domain)
}

// SubscribeToModerationAlerts creates a channel for moderation alert updates using event bus
func (sm *SubscriptionManager) SubscribeToModerationAlerts(ctx context.Context, username string, severity *model.ModerationSeverity) (<-chan *model.ModerationAlert, error) {
	return sm.manager.SubscribeToModerationAlerts(ctx, username, severity)
}

// SubscribeToCostAlerts creates a channel for cost alert updates using event bus
func (sm *SubscriptionManager) SubscribeToCostAlerts(ctx context.Context, username string, thresholdUSD float64) (<-chan *model.CostAlert, error) {
	return sm.manager.SubscribeToCostAlerts(ctx, username, thresholdUSD)
}

// SubscribeToPerformanceAlerts creates a channel for performance alert updates using event bus
func (sm *SubscriptionManager) SubscribeToPerformanceAlerts(ctx context.Context, username string, severity model.AlertSeverity) (<-chan *model.PerformanceAlert, error) {
	return sm.manager.SubscribeToPerformanceAlerts(ctx, username, severity)
}

// SubscribeToThreatIntelligence creates a channel for threat intelligence updates using event bus
func (sm *SubscriptionManager) SubscribeToThreatIntelligence(ctx context.Context, username string) (<-chan *model.ThreatAlert, error) {
	return sm.manager.SubscribeToThreatIntelligence(ctx, username)
}

// SubscribeToInfrastructureEvents creates a channel for infrastructure event updates using event bus
func (sm *SubscriptionManager) SubscribeToInfrastructureEvents(ctx context.Context, username string) (<-chan *model.InfrastructureEvent, error) {
	return sm.manager.SubscribeToInfrastructureEvents(ctx, username)
}

// SubscribeToModerationQueueUpdate creates a channel for moderation queue updates using event bus
func (sm *SubscriptionManager) SubscribeToModerationQueueUpdate(ctx context.Context, username string, priority *model.Priority) (<-chan *model.ModerationItem, error) {
	return sm.manager.SubscribeToModerationQueueUpdate(ctx, username, priority)
}

// GetStats returns statistics about active subscriptions
func (sm *SubscriptionManager) GetStats() map[string]interface{} {
	return sm.manager.GetStats()
}
