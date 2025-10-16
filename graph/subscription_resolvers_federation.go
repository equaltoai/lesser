package graph

import (
	"context"

	"github.com/equaltoai/lesser/graph/model"
	"go.uber.org/zap"
)

// NOTE: imports intentionally omitted. Run gofmt/goimports and add any
// required imports after generating these files.

// FederationHealthUpdates implements SubscriptionResolver
func (r *subscriptionResolver) FederationHealthUpdates(ctx context.Context, domain *string) (<-chan *model.FederationHealthUpdate, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	r.Logger.Info("Started federation health subscription",
		zap.String("user", username),
		zap.Bool("filtered", domain != nil))

	// Use SubscriptionManager for consistent subscription handling
	sm := r.SubscriptionManager
	if sm == nil {
		r.Logger.Error("subscription manager not available for federation health updates")
		ch := make(chan *model.FederationHealthUpdate)
		close(ch)
		return ch, ErrSubscriptionManagerNotRunning
	}

	if !sm.IsRunning() {
		r.Logger.Error("subscription manager not running for federation health updates")
		ch := make(chan *model.FederationHealthUpdate)
		close(ch)
		return ch, ErrSubscriptionManagerNotRunning
	}

	federationChan, err := sm.SubscribeToFederationHealth(ctx, username, domain)
	if err != nil {
		r.Logger.Error("failed to create federation health subscription",
			zap.String("user", username),
			zap.Error(err))
		return nil, err
	}

	r.Logger.Info("started federation health subscription",
		zap.String("user", username))

	return federationChan, nil
}

// InfrastructureEvent implements SubscriptionResolver
func (r *subscriptionResolver) InfrastructureEvent(ctx context.Context) (<-chan *model.InfrastructureEvent, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	// Use SubscriptionManager for consistent subscription handling
	sm := r.SubscriptionManager
	if sm == nil {
		r.Logger.Error("subscription manager not available for infrastructure events")
		ch := make(chan *model.InfrastructureEvent)
		close(ch)
		return ch, ErrSubscriptionManagerNotRunning
	}

	if !sm.IsRunning() {
		r.Logger.Error("subscription manager not running for infrastructure events")
		ch := make(chan *model.InfrastructureEvent)
		close(ch)
		return ch, ErrSubscriptionManagerNotRunning
	}

	eventChan, err := sm.SubscribeToInfrastructureEvents(ctx, username)
	if err != nil {
		r.Logger.Error("failed to create infrastructure events subscription",
			zap.String("user", username),
			zap.Error(err))
		return nil, err
	}

	r.Logger.Info("Started infrastructure events subscription",
		zap.String("user", username))

	return eventChan, nil
}
