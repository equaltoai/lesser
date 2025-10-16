package graph

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/streaming"
	"go.uber.org/zap"
)

// NOTE: imports intentionally omitted. Run gofmt/goimports and add any
// required imports after generating these files.

// BudgetAlerts implements SubscriptionResolver
func (r *subscriptionResolver) BudgetAlerts(ctx context.Context, domain *string) (<-chan *model.BudgetAlert, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	// Create channel for budget alerts
	alertsChan := make(chan *model.BudgetAlert, 100)

	// Use EventBus for real-time budget alerts - NO POLLING, REAL EVENTS ONLY
	eventBus := r.Registry.EventBus()
	if eventBus == nil {
		return nil, ErrEventBusUnavailable
	}

	// Subscribe to budget alert events via EventBus
	var streamName string
	if domain != nil {
		streamName = fmt.Sprintf("budget_alerts:%s", *domain)
	} else {
		streamName = fmt.Sprintf("budget_alerts:%s", username)
	}

	eventChan, err := eventBus.Subscribe(ctx, streamName)
	if err != nil {
		return nil, errors.Join(errors.New("failed to subscribe to budget alerts"), err)
	}

	// Forward events to GraphQL channel
	go func() {
		defer close(alertsChan)
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-eventChan:
				if !ok {
					return
				}

				// Convert event to BudgetAlert
				if alert, ok := event.(*model.BudgetAlert); ok {
					// Enhance alert with calculated level and message using helper functions
					if alert.BudgetUsd > 0 && alert.SpentUsd >= 0 {
						alert.AlertLevel = r.getBudgetAlertLevel(alert.SpentUsd, alert.BudgetUsd)
						// Update percentage for consistency
						alert.PercentUsed = (alert.SpentUsd / alert.BudgetUsd) * 100

						// Generate descriptive message using helper function
						alertMessage := r.getBudgetAlertMessage(alert.SpentUsd, alert.BudgetUsd)
						r.Logger.Info("Processing budget alert",
							zap.String("domain", alert.Domain),
							zap.String("alert_level", string(alert.AlertLevel)),
							zap.String("message", alertMessage))
					}

					select {
					case alertsChan <- alert:
					case <-ctx.Done():
						return
					}
				}
			}
		}
	}()

	r.Logger.Info("Started budget alerts subscription",
		zap.String("user", username),
		zap.Bool("filtered", domain != nil))

	return alertsChan, nil
}

// CostAlerts implements SubscriptionResolver
func (r *subscriptionResolver) CostAlerts(ctx context.Context, thresholdUSD float64) (<-chan *model.CostAlert, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	// Create channel for cost alerts
	alertsChan := make(chan *model.CostAlert, 100)

	// Use EventBus to subscribe to cost events - NO POLLING, REAL EVENTS ONLY
	eventBus := r.Registry.EventBus()
	if eventBus == nil {
		return nil, ErrEventBusUnavailable
	}

	// Subscribe to cost events via EventBus
	streamName := fmt.Sprintf("cost_events:%s", username)
	eventChan, err := eventBus.Subscribe(ctx, streamName)
	if err != nil {
		return nil, errors.Join(errors.New("failed to subscribe to cost alerts"), err)
	}

	// Forward events to GraphQL channel
	go func() {
		defer close(alertsChan)
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-eventChan:
				if !ok {
					return
				}

				// The event is already the payload data (CostEventPayload) after conversion
				// by the GraphQL EventBus adapter
				if costPayload, ok := event.(*streaming.CostEventPayload); ok {
					// Check if cost exceeds threshold
					if costPayload.CostUSD > thresholdUSD {
						alert := &model.CostAlert{
							ID:        fmt.Sprintf("cost_alert_%d", time.Now().UnixNano()),
							Type:      "service_threshold",
							Amount:    costPayload.CostUSD,
							Threshold: thresholdUSD,
							Domain:    stringPtr(costPayload.TenantID),
							Message: fmt.Sprintf("Cost alert for %s: $%.2f exceeded threshold $%.2f",
								costPayload.Service, costPayload.CostUSD, thresholdUSD),
							Timestamp: model.Time(costPayload.Timestamp),
						}

						select {
						case alertsChan <- alert:
						case <-ctx.Done():
							return
						}
					}
				}
			}
		}
	}()

	r.Logger.Info("Started cost alerts subscription",
		zap.String("user", username),
		zap.Float64("threshold", thresholdUSD))

	return alertsChan, nil
}

// CostUpdates implements SubscriptionResolver
func (r *subscriptionResolver) CostUpdates(ctx context.Context, _ *int) (<-chan *model.CostUpdate, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	updatesChan := make(chan *model.CostUpdate, 100)
	eventBus := r.Registry.EventBus()

	if eventBus == nil {
		r.startFallbackCostTracking(ctx, updatesChan)
		return updatesChan, nil
	}

	if err := r.startEventBusCostTracking(ctx, eventBus, username, updatesChan); err != nil {
		return nil, err
	}

	r.Logger.Info("Started cost updates subscription", zap.String("user", username))
	return updatesChan, nil
}

// MetricsUpdates implements SubscriptionResolver
func (r *subscriptionResolver) MetricsUpdates(ctx context.Context, _ []string, _ []string, _ *float64) (<-chan *model.MetricsUpdate, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	updateChan := make(chan *model.MetricsUpdate, 100)

	// For now, return empty channel
	// This would be implemented with real metrics streaming
	go func() {
		<-ctx.Done()
		close(updateChan)
	}()

	r.Logger.Info("Started metrics updates subscription",
		zap.String("user", username))

	return updateChan, nil
}

// PerformanceAlert implements SubscriptionResolver
func (r *subscriptionResolver) PerformanceAlert(ctx context.Context, severity model.AlertSeverity) (<-chan *model.PerformanceAlert, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	alertChan := make(chan *model.PerformanceAlert, 100)

	// For now, return empty channel
	// This would be implemented with real performance monitoring
	go func() {
		<-ctx.Done()
		close(alertChan)
	}()

	r.Logger.Info("Started performance alerts subscription",
		zap.String("user", username),
		zap.String("severity", string(severity)))

	return alertChan, nil
}
