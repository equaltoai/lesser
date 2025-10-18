package graph

import (
	"context"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/trust"
	"go.uber.org/zap"
)

// NOTE: imports intentionally omitted. Run gofmt/goimports and add any
// required imports after generating these files.

// RelationshipUpdates is the resolver for the relationshipUpdates field.
func (r *subscriptionResolver) RelationshipUpdates(ctx context.Context, actorID *string) (<-chan *model.RelationshipUpdate, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	r.Logger.Info("Relationship updates subscription started",
		zap.String("user", username))

	// Use SubscriptionManager for consistent subscription handling
	sm := r.SubscriptionManager
	if sm == nil {
		r.Logger.Error("subscription manager not available for relationship updates")
		ch := make(chan *model.RelationshipUpdate)
		close(ch)
		return ch, ErrSubscriptionManagerNotRunning
	}

	if !sm.IsRunning() {
		r.Logger.Error("subscription manager not running for relationship updates")
		ch := make(chan *model.RelationshipUpdate)
		close(ch)
		return ch, ErrSubscriptionManagerNotRunning
	}

	relationshipChan, err := sm.SubscribeToRelationshipUpdates(ctx, username, actorID)
	if err != nil {
		r.Logger.Error("failed to create relationship subscription",
			zap.String("user", username),
			zap.Error(err))
		return nil, err
	}

	r.Logger.Info("started relationship subscription",
		zap.String("user", username))

	return relationshipChan, nil
}

// TrustUpdates implements SubscriptionResolver
func (r *subscriptionResolver) TrustUpdates(ctx context.Context, actorID string) (<-chan *trust.TrustEdge, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	r.Logger.Info("Started trust updates subscription",
		zap.String("user", username),
		zap.String("actor", actorID))

	// Use SubscriptionManager for consistent subscription handling
	sm := r.SubscriptionManager
	if sm == nil {
		r.Logger.Error("subscription manager not available for trust updates")
		ch := make(chan *trust.TrustEdge)
		close(ch)
		return ch, ErrSubscriptionManagerNotRunning
	}

	if !sm.IsRunning() {
		r.Logger.Error("subscription manager not running for trust updates")
		ch := make(chan *trust.TrustEdge)
		close(ch)
		return ch, ErrSubscriptionManagerNotRunning
	}

	trustChan, err := sm.SubscribeToTrustUpdates(ctx, actorID)
	if err != nil {
		r.Logger.Error("failed to create trust subscription",
			zap.String("user", username),
			zap.String("actor", actorID),
			zap.Error(err))
		return nil, err
	}

	r.Logger.Info("started trust subscription",
		zap.String("user", username),
		zap.String("actor", actorID))

	return trustChan, nil
}
