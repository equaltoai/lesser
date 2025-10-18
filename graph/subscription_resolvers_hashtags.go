package graph

import (
	"context"
	"fmt"

	"github.com/equaltoai/lesser/graph/model"
	"go.uber.org/zap"
)

// HashtagActivity implements SubscriptionResolver using the SubscriptionManager pattern
func (r *subscriptionResolver) HashtagActivity(ctx context.Context, hashtags []string) (<-chan *model.HashtagActivityUpdate, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	if len(hashtags) == 0 {
		return nil, fmt.Errorf("at least one hashtag required")
	}

	// Check if SubscriptionManager is available
	sm := r.SubscriptionManager
	if sm == nil {
		r.Logger.Error("subscription manager not available for hashtag activity")
		activityChan := make(chan *model.HashtagActivityUpdate)
		close(activityChan)
		return activityChan, fmt.Errorf("subscription manager not available")
	}

	if !sm.IsRunning() {
		r.Logger.Error("subscription manager not running for hashtag activity")
		activityChan := make(chan *model.HashtagActivityUpdate)
		close(activityChan)
		return activityChan, fmt.Errorf("subscription manager not running")
	}

	// Use the SubscriptionManager's built-in method for hashtag activity
	activityChan, err := sm.SubscribeToHashtagActivity(ctx, username, hashtags)
	if err != nil {
		r.Logger.Error("failed to create hashtag activity subscription",
			zap.String("user", username),
			zap.Strings("hashtags", hashtags),
			zap.Error(err))
		return nil, fmt.Errorf("failed to create subscription: %w", err)
	}

	r.Logger.Info("started hashtag activity subscription",
		zap.String("user", username),
		zap.Int("hashtag_count", len(hashtags)))

	return activityChan, nil
}
