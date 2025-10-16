package graph

import (
	"context"
	"errors"
	"fmt"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/services/relationships"
	"go.uber.org/zap"
)

// FollowActor is the resolver for the followActor field.
func (r *mutationResolver) FollowActor(ctx context.Context, id string) (*activitypub.Activity, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	// Get the relationships service
	relationshipsService := r.Registry.Relationships()
	if relationshipsService == nil {
		return nil, errors.New("relationships service is not available")
	}

	// Follow the actor
	_, err = relationshipsService.Follow(ctx, &relationships.FollowCommand{
		FollowerID:  username,
		FollowingID: id,
	})
	if err != nil {
		r.Logger.Error("Failed to follow actor",
			zap.String("user", username),
			zap.String("target_id", id),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to follow actor"), err)
	}

	// Track cost using centralized tracker
	r.trackDynamoOperation(ctx, "write", 1)

	// Return the Follow activity (construct from result data)
	activityID := fmt.Sprintf("%s/follows/%s", username, id)
	return &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			ID:   activityID,
			Type: "Follow",
		},
		Actor:  username,
		Object: id,
	}, nil
}

// BlockActor is the resolver for the blockActor field.
func (r *mutationResolver) BlockActor(ctx context.Context, id string) (*model.Relationship, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	// Get the relationships service
	relationshipsService := r.Registry.Relationships()
	if relationshipsService == nil {
		return nil, errors.New("relationships service is not available")
	}

	// Block the actor using the relationships package types
	result, err := relationshipsService.Block(ctx, &relationships.BlockCommand{
		BlockerID: username,
		BlockedID: id,
	})
	if err != nil {
		r.Logger.Error("Failed to block actor",
			zap.String("user", username),
			zap.String("target_id", id),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to block actor"), err)
	}

	// Track cost using centralized tracker
	r.trackDynamoOperation(ctx, "write", 1)

	// Return the updated relationship
	return &model.Relationship{
		ID:        id,
		Following: result.Relationship.Following,
		Blocking:  result.Relationship.Blocking,
		Muting:    result.Relationship.Muting,
	}, nil
}

// MuteActor is the resolver for the muteActor field.
func (r *mutationResolver) MuteActor(ctx context.Context, id string, notifications *bool) (*model.Relationship, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	// Get the relationships service
	relationshipsService := r.Registry.Relationships()
	if relationshipsService == nil {
		return nil, errors.New("relationships service is not available")
	}

	// Mute the actor
	muteNotifications := true
	if notifications != nil {
		muteNotifications = *notifications
	}

	_, err = relationshipsService.Mute(ctx, &relationships.MuteCommand{
		MuterID:  username,
		MutedID:  id,
		Duration: nil, // Permanent mute
	})
	if err != nil {
		r.Logger.Error("Failed to mute actor",
			zap.String("user", username),
			zap.String("target_id", id),
			zap.Bool("notifications", muteNotifications),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to mute actor"), err)
	}

	// Track cost using centralized tracker
	r.trackDynamoOperation(ctx, "write", 1)

	// Return the updated relationship
	return &model.Relationship{
		ID:                  id,
		Muting:              true,
		MutingNotifications: muteNotifications,
	}, nil
}

// UnblockActor is the resolver for the unblockActor field - implemented in schema.resolvers.go

// UnfollowActor is the resolver for the unfollowActor field - implemented in schema.resolvers.go

// UnmuteActor is the resolver for the unmuteActor field - implemented in schema.resolvers.go

// UpdateRelationship is the resolver for the updateRelationship field.
func (r *mutationResolver) UpdateRelationship(ctx context.Context, id string, input model.UpdateRelationshipInput) (*model.Relationship, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	// TODO: Implement UpdateRelationship in the relationships service
	// For now, return a stub response
	r.Logger.Warn("UpdateRelationship not fully implemented yet",
		zap.String("user", username),
		zap.String("target_id", id))

	// Track cost using centralized tracker
	r.trackDynamoOperation(ctx, "write", 1)

	return nil, errors.New("UpdateRelationship not yet implemented - relationships service needs to be extended")
}
