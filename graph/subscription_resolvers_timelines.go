package graph

import (
	"context"
	"errors"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"go.uber.org/zap"
)

// NOTE: imports intentionally omitted. Run gofmt/goimports and add any
// required imports after generating these files.

// ====================================================================
// SUBSCRIPTION RESOLVERS
// ====================================================================

// ActivityStream is the resolver for the activityStream field.
func (r *subscriptionResolver) ActivityStream(ctx context.Context, types []model.ActivityType) (<-chan *activitypub.Activity, error) {
	username := r.optionalAuth(ctx)

	r.Logger.Info("Activity stream subscription started",
		zap.String("user", username),
		zap.Int("typeCount", len(types)))

	// Use SubscriptionManager for consistent subscription handling
	sm := r.SubscriptionManager
	if sm == nil {
		r.Logger.Error("subscription manager not available for activity stream")
		ch := make(chan *activitypub.Activity)
		close(ch)
		return ch, ErrSubscriptionManagerNotRunning
	}

	if !sm.IsRunning() {
		r.Logger.Error("subscription manager not running for activity stream")
		ch := make(chan *activitypub.Activity)
		close(ch)
		return ch, ErrSubscriptionManagerNotRunning
	}

	activityChan, err := sm.SubscribeToActivityStream(ctx, username, types)
	if err != nil {
		r.Logger.Error("failed to create activity stream subscription",
			zap.String("user", username),
			zap.Error(err))
		return nil, err
	}

	r.Logger.Info("started activity stream subscription",
		zap.String("user", username),
		zap.Int("type_filters", len(types)))

	return activityChan, nil
}

// TimelineUpdates is the resolver for the timelineUpdates field.
func (r *subscriptionResolver) TimelineUpdates(ctx context.Context, timelineType model.TimelineType, listID *string) (<-chan *model.Object, error) {
	username := r.optionalAuth(ctx)

	// Validate request
	if timelineType == model.TimelineTypeList && listID == nil {
		return nil, errors.New("listId is required for list timeline type")
	}
	if timelineType != model.TimelineTypePublic && username == "" {
		return nil, errors.New("authentication required for this timeline type")
	}

	r.Logger.Info("Timeline updates subscription started",
		zap.String("user", username),
		zap.String("type", string(timelineType)))

	// Use SubscriptionManager for consistent subscription handling
	sm := r.SubscriptionManager
	if sm == nil {
		r.Logger.Error("subscription manager not available for timeline updates")
		ch := make(chan *model.Object)
		close(ch)
		return ch, ErrSubscriptionManagerNotRunning
	}

	if !sm.IsRunning() {
		r.Logger.Error("subscription manager not running for timeline updates")
		ch := make(chan *model.Object)
		close(ch)
		return ch, ErrSubscriptionManagerNotRunning
	}

	// Ensure the WebSocket connection ID is attached so DynamoDB subscription records can be created.
	ctx = WithConnectionID(ctx, r.getConnectionID(ctx))

	timelineChan, err := sm.SubscribeToTimelineUpdates(ctx, username, timelineType)
	if err != nil {
		r.Logger.Error("failed to create timeline subscription",
			zap.String("user", username),
			zap.String("type", string(timelineType)),
			zap.Error(err))
		return nil, err
	}

	r.Logger.Info("started timeline subscription",
		zap.String("user", username),
		zap.String("type", string(timelineType)))

	return timelineChan, nil
}
