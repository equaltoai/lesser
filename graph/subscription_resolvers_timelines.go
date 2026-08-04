package graph

import (
	"context"
	"strings"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/services/lists"
	"go.uber.org/zap"
)

// NOTE: imports intentionally omitted. Run gofmt/goimports and add any
// required imports after generating these files.

// ====================================================================
// SUBSCRIPTION RESOLVERS
// ====================================================================

// ActivityStream is the resolver for the activityStream field.
func (r *subscriptionResolver) ActivityStream(ctx context.Context, types []model.ActivityType) (<-chan *activitypub.Activity, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

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
func (r *subscriptionResolver) TimelineUpdates(
	ctx context.Context,
	timelineType model.TimelineType,
	actorUsername *string,
	hashtag *string,
	listID *string,
) (<-chan *model.Object, error) {
	username := r.optionalAuth(ctx)
	inputs := timelineRoutingInputs{actorUsername: actorUsername, hashtag: hashtag, listID: listID}

	// Validate routing before authorization so malformed operations fail closed
	// without being mistaken for a credential problem.
	if _, err := validateTimelineRoutingInputs(timelineType, inputs); err != nil {
		return nil, err
	}
	if !timelineAllowsAnonymous(timelineType) && username == "" {
		return nil, ErrAuthenticationRequired
	}
	if timelineType == model.TimelineTypeList {
		if r.Registry == nil || r.Registry.Lists() == nil {
			return nil, ErrListNotFoundOrAccessDenied
		}
		if _, err := r.Registry.Lists().GetList(ctx, &lists.GetListQuery{
			ListID:   strings.TrimSpace(*listID),
			ViewerID: username,
		}); err != nil {
			return nil, ErrListNotFoundOrAccessDenied
		}
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

	timelineChan, err := sm.SubscribeToTimelineUpdates(ctx, username, timelineType, actorUsername, hashtag, listID)
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

func timelineAllowsAnonymous(timelineType model.TimelineType) bool {
	switch timelineType {
	case model.TimelineTypePublic, model.TimelineTypeLocal, model.TimelineTypeActor, model.TimelineTypeHashtag:
		return true
	default:
		return false
	}
}
