package graph

import (
	"context"
	"errors"

	"github.com/equaltoai/lesser/graph/model"
	"go.uber.org/zap"
)

// FollowHashtag is the resolver for the followHashtag field.
func (r *mutationResolver) FollowHashtag(ctx context.Context, hashtag string, _ *model.NotificationLevel) (*model.HashtagFollowPayload, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	// TODO: Implement FollowHashtag in the hashtags service
	// For now, return a stub response
	r.Logger.Warn("FollowHashtag not fully implemented yet",
		zap.String("user", username),
		zap.String("hashtag", hashtag))

	// Track cost using centralized tracker
	r.trackDynamoOperation(ctx, "write", 1)

	return &model.HashtagFollowPayload{
		Success: false,
		Hashtag: nil,
	}, errors.New("FollowHashtag not yet implemented - hashtags service needs to be extended")
}

// MuteHashtag is the resolver for the muteHashtag field.
func (r *mutationResolver) MuteHashtag(ctx context.Context, hashtag string, _ *model.Time) (*model.MuteHashtagPayload, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	// TODO: Implement MuteHashtag in the hashtags service
	// For now, return a stub response
	r.Logger.Warn("MuteHashtag not fully implemented yet",
		zap.String("user", username),
		zap.String("hashtag", hashtag))

	// Track cost using centralized tracker
	r.trackDynamoOperation(ctx, "write", 1)

	return &model.MuteHashtagPayload{
		Success: false,
		Hashtag: nil,
	}, errors.New("MuteHashtag not yet implemented - hashtags service needs to be extended")
}

// UnfollowHashtag is the resolver for the unfollowHashtag field - implemented in schema.resolvers.go

// UpdateHashtagNotifications is the resolver for the updateHashtagNotifications field - implemented in schema.resolvers.go
