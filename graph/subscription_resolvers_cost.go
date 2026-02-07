package graph

import (
	"context"

	"github.com/equaltoai/lesser/graph/model"
	"go.uber.org/zap"
)

// NOTE: imports intentionally omitted. Run gofmt/goimports and add any
// required imports after generating these files.

// BudgetAlerts implements SubscriptionResolver
func (r *subscriptionResolver) BudgetAlerts(ctx context.Context, domain *string) (<-chan *model.BudgetAlert, error) {
	username, err := r.requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	// Use SubscriptionManager for consistent subscription handling
	sm := r.SubscriptionManager
	if sm == nil {
		r.Logger.Error("subscription manager not available for budget alerts")
		ch := make(chan *model.BudgetAlert)
		close(ch)
		return ch, ErrSubscriptionManagerNotRunning
	}

	if !sm.IsRunning() {
		r.Logger.Error("subscription manager not running for budget alerts")
		ch := make(chan *model.BudgetAlert)
		close(ch)
		return ch, ErrSubscriptionManagerNotRunning
	}

	alertsChan, err := sm.SubscribeToBudgetAlerts(ctx, username, domain)
	if err != nil {
		r.Logger.Error("failed to create budget alerts subscription",
			zap.String("user", username),
			zap.Error(err))
		return nil, err
	}

	r.Logger.Info("Started budget alerts subscription",
		zap.String("user", username),
		zap.Bool("filtered", domain != nil))

	return alertsChan, nil
}

// CostAlerts implements SubscriptionResolver
func (r *subscriptionResolver) CostAlerts(ctx context.Context, thresholdUSD float64) (<-chan *model.CostAlert, error) {
	username, err := r.requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	// Use SubscriptionManager for consistent subscription handling
	sm := r.SubscriptionManager
	if sm == nil {
		r.Logger.Error("subscription manager not available for cost alerts")
		ch := make(chan *model.CostAlert)
		close(ch)
		return ch, ErrSubscriptionManagerNotRunning
	}

	if !sm.IsRunning() {
		r.Logger.Error("subscription manager not running for cost alerts")
		ch := make(chan *model.CostAlert)
		close(ch)
		return ch, ErrSubscriptionManagerNotRunning
	}

	alertsChan, err := sm.SubscribeToCostAlerts(ctx, username, thresholdUSD)
	if err != nil {
		r.Logger.Error("failed to create cost alerts subscription",
			zap.String("user", username),
			zap.Error(err))
		return nil, err
	}

	r.Logger.Info("Started cost alerts subscription",
		zap.String("user", username),
		zap.Float64("threshold", thresholdUSD))

	return alertsChan, nil
}

// CostUpdates implements SubscriptionResolver
func (r *subscriptionResolver) CostUpdates(ctx context.Context, threshold *int) (<-chan *model.CostUpdate, error) {
	username, err := r.requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	// Use SubscriptionManager for consistent subscription handling
	sm := r.SubscriptionManager
	if sm == nil {
		r.Logger.Error("subscription manager not available for cost updates")
		ch := make(chan *model.CostUpdate)
		close(ch)
		return ch, ErrSubscriptionManagerNotRunning
	}

	if !sm.IsRunning() {
		r.Logger.Error("subscription manager not running for cost updates")
		ch := make(chan *model.CostUpdate)
		close(ch)
		return ch, ErrSubscriptionManagerNotRunning
	}

	updatesChan, err := sm.SubscribeToCostUpdates(ctx, username, threshold)
	if err != nil {
		r.Logger.Error("failed to create cost updates subscription",
			zap.String("user", username),
			zap.Error(err))
		return nil, err
	}

	r.Logger.Info("Started cost updates subscription", zap.String("user", username))
	return updatesChan, nil
}

// MetricsUpdates implements SubscriptionResolver
func (r *subscriptionResolver) MetricsUpdates(ctx context.Context, categories []string, services []string, threshold *float64) (<-chan *model.MetricsUpdate, error) {
	username, err := r.requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	// Use SubscriptionManager for consistent subscription handling
	sm := r.SubscriptionManager
	if sm == nil {
		r.Logger.Error("subscription manager not available for metrics updates")
		ch := make(chan *model.MetricsUpdate)
		close(ch)
		return ch, ErrSubscriptionManagerNotRunning
	}

	if !sm.IsRunning() {
		r.Logger.Error("subscription manager not running for metrics updates")
		ch := make(chan *model.MetricsUpdate)
		close(ch)
		return ch, ErrSubscriptionManagerNotRunning
	}

	updateChan, err := sm.SubscribeToMetricsUpdates(ctx, username, categories, services, threshold)
	if err != nil {
		r.Logger.Error("failed to create metrics updates subscription",
			zap.String("user", username),
			zap.Error(err))
		return nil, err
	}

	r.Logger.Info("Started metrics updates subscription",
		zap.String("user", username))

	return updateChan, nil
}

// PerformanceAlert implements SubscriptionResolver
func (r *subscriptionResolver) PerformanceAlert(ctx context.Context, severity model.AlertSeverity) (<-chan *model.PerformanceAlert, error) {
	username, err := r.requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	// Use SubscriptionManager for consistent subscription handling
	sm := r.SubscriptionManager
	if sm == nil {
		r.Logger.Error("subscription manager not available for performance alerts")
		ch := make(chan *model.PerformanceAlert)
		close(ch)
		return ch, ErrSubscriptionManagerNotRunning
	}

	if !sm.IsRunning() {
		r.Logger.Error("subscription manager not running for performance alerts")
		ch := make(chan *model.PerformanceAlert)
		close(ch)
		return ch, ErrSubscriptionManagerNotRunning
	}

	alertChan, err := sm.SubscribeToPerformanceAlerts(ctx, username, severity)
	if err != nil {
		r.Logger.Error("failed to create performance alerts subscription",
			zap.String("user", username),
			zap.Error(err))
		return nil, err
	}

	r.Logger.Info("Started performance alerts subscription",
		zap.String("user", username),
		zap.String("severity", string(severity)))

	return alertChan, nil
}
