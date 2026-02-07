package graph

import (
	"context"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/moderation"
	"go.uber.org/zap"
)

// NOTE: imports intentionally omitted. Run gofmt/goimports and add any
// required imports after generating these files.

// ModerationEvents implements SubscriptionResolver
func (r *subscriptionResolver) ModerationEvents(ctx context.Context, actorID *string) (<-chan *moderation.ModerationDecision, error) {
	username, err := r.requireModeratorOrAdmin(ctx)
	if err != nil {
		return nil, err
	}

	// Use SubscriptionManager for consistent subscription handling
	sm := r.SubscriptionManager
	if sm == nil {
		r.Logger.Error("subscription manager not available for moderation events")
		ch := make(chan *moderation.ModerationDecision)
		close(ch)
		return ch, ErrSubscriptionManagerNotRunning
	}

	if !sm.IsRunning() {
		r.Logger.Error("subscription manager not running for moderation events")
		ch := make(chan *moderation.ModerationDecision)
		close(ch)
		return ch, ErrSubscriptionManagerNotRunning
	}

	eventChan, err := sm.SubscribeToModerationEvents(ctx, actorID)
	if err != nil {
		r.Logger.Error("failed to create moderation events subscription",
			zap.String("user", username),
			zap.Error(err))
		return nil, err
	}

	r.Logger.Info("Started moderation events subscription",
		zap.String("user", username))

	return eventChan, nil
}

// ModerationAlerts implements SubscriptionResolver
func (r *subscriptionResolver) ModerationAlerts(ctx context.Context, severity *model.ModerationSeverity) (<-chan *model.ModerationAlert, error) {
	username, err := r.requireModeratorOrAdmin(ctx)
	if err != nil {
		return nil, err
	}

	// Use SubscriptionManager for consistent subscription handling
	sm := r.SubscriptionManager
	if sm == nil {
		r.Logger.Error("subscription manager not available for moderation alerts")
		ch := make(chan *model.ModerationAlert)
		close(ch)
		return ch, ErrSubscriptionManagerNotRunning
	}

	if !sm.IsRunning() {
		r.Logger.Error("subscription manager not running for moderation alerts")
		ch := make(chan *model.ModerationAlert)
		close(ch)
		return ch, ErrSubscriptionManagerNotRunning
	}

	alertsChan, err := sm.SubscribeToModerationAlerts(ctx, username, severity)
	if err != nil {
		r.Logger.Error("failed to create moderation alerts subscription",
			zap.String("user", username),
			zap.Error(err))
		return nil, err
	}

	r.Logger.Info("Started moderation alerts subscription",
		zap.String("user", username),
		zap.Bool("filtered", severity != nil))

	return alertsChan, nil
}

// ModerationQueueUpdate implements SubscriptionResolver
func (r *subscriptionResolver) ModerationQueueUpdate(ctx context.Context, priority *model.Priority) (<-chan *model.ModerationItem, error) {
	username, err := r.requireModeratorOrAdmin(ctx)
	if err != nil {
		return nil, err
	}

	// Use SubscriptionManager for consistent subscription handling
	sm := r.SubscriptionManager
	if sm == nil {
		r.Logger.Error("subscription manager not available for moderation queue")
		ch := make(chan *model.ModerationItem)
		close(ch)
		return ch, ErrSubscriptionManagerNotRunning
	}

	if !sm.IsRunning() {
		r.Logger.Error("subscription manager not running for moderation queue")
		ch := make(chan *model.ModerationItem)
		close(ch)
		return ch, ErrSubscriptionManagerNotRunning
	}

	queueChan, err := sm.SubscribeToModerationQueueUpdate(ctx, username, priority)
	if err != nil {
		r.Logger.Error("failed to create moderation queue subscription",
			zap.String("user", username),
			zap.Error(err))
		return nil, err
	}

	r.Logger.Info("Started moderation queue subscription",
		zap.String("user", username),
		zap.Any("priority", priority))

	return queueChan, nil
}
