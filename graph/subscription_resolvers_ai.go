package graph

import (
	"context"

	"github.com/equaltoai/lesser/graph/model"
	"go.uber.org/zap"
)

// NOTE: imports intentionally omitted. Run gofmt/goimports and add any
// required imports after generating these files.

// AiAnalysisUpdates implements SubscriptionResolver
func (r *subscriptionResolver) AiAnalysisUpdates(ctx context.Context, objectID *string) (<-chan *model.AIAnalysis, error) {
	username, err := r.requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	// Use SubscriptionManager for consistent subscription handling
	sm := r.SubscriptionManager
	if sm == nil {
		r.Logger.Error("subscription manager not available for AI analysis updates")
		ch := make(chan *model.AIAnalysis)
		close(ch)
		return ch, ErrSubscriptionManagerNotRunning
	}

	if !sm.IsRunning() {
		r.Logger.Error("subscription manager not running for AI analysis updates")
		ch := make(chan *model.AIAnalysis)
		close(ch)
		return ch, ErrSubscriptionManagerNotRunning
	}

	updatesChan, err := sm.SubscribeToAIAnalysisUpdates(ctx, objectID)
	if err != nil {
		r.Logger.Error("failed to create AI analysis updates subscription",
			zap.String("user", username),
			zap.Error(err))
		return nil, err
	}

	r.Logger.Info("Started AI analysis updates subscription",
		zap.String("user", username),
		zap.Bool("filtered", objectID != nil))

	return updatesChan, nil
}

// ThreatIntelligence implements SubscriptionResolver
func (r *subscriptionResolver) ThreatIntelligence(ctx context.Context) (<-chan *model.ThreatAlert, error) {
	username, err := r.requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	sm := r.SubscriptionManager
	if sm == nil || !sm.IsRunning() {
		r.Logger.Error("subscription manager not available or not running")
		ch := make(chan *model.ThreatAlert)
		close(ch)
		return ch, ErrSubscriptionManagerNotRunning
	}

	alertChan, err := sm.SubscribeToThreatIntelligence(ctx, username)
	if err != nil {
		r.Logger.Error("failed to create threat intelligence subscription",
			zap.String("user", username),
			zap.Error(err))
		return nil, err
	}

	r.Logger.Info("started threat intelligence subscription", zap.String("user", username))
	return alertChan, nil
}
