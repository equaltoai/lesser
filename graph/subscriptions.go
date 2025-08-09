package graph

import (
	"context"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/moderation"
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

// NewSubscriptionManager creates a new subscription manager with event bus integration
func NewSubscriptionManager(logger *zap.Logger) *SubscriptionManager {
	if logger == nil {
		logger = zap.NewNop()
	}

	// Try to get the global event bus from stream-router
	eventBus := getGlobalStreamRouterEventBus()
	if eventBus != nil {
		logger.Info("GraphQL subscriptions connected to stream-router event bus")
	} else {
		logger.Warn("Stream-router event bus not available, using polling fallback")
	}

	manager := NewGraphQLSubscriptionManager(eventBus, logger)

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
	objectsCh, err := sm.manager.SubscribeToTimeline(ctx, username, model.TimelineTypeHome)
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
func (sm *SubscriptionManager) SubscribeToTimelineUpdates(ctx context.Context, username string, timelineType model.TimelineType) (<-chan *model.Object, error) {
	return sm.manager.SubscribeToTimeline(ctx, username, timelineType)
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
func (sm *SubscriptionManager) SubscribeToQuoteActivity(ctx context.Context, username string, noteID string, noteObj any) (<-chan *model.QuoteActivityUpdate, error) {
	return sm.manager.SubscribeToQuoteActivity(ctx, username, noteID, noteObj)
}

// GetStats returns statistics about active subscriptions
func (sm *SubscriptionManager) GetStats() map[string]interface{} {
	return sm.manager.GetStats()
}

// getGlobalStreamRouterEventBus tries to access the stream-router's global event bus
// This connects to the stream-router Lambda's event bus for real-time events
func getGlobalStreamRouterEventBus() *streaming.EventBus {
	// Use the global event bus from the streaming package
	// This will be initialized if the stream-router has started its event bus
	// Note: In separate Lambda deployments, this will return nil and we'll fall back to polling
	return streaming.GetGlobalEventBus(zap.NewNop())
}

