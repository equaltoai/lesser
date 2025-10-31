package graph

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

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
	result, err := relationshipsService.Follow(ctx, &relationships.FollowCommand{
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

	activity := result.Activity
	if activity == nil {
		activityID := fmt.Sprintf("%s/follows/%s", username, url.PathEscape(id))
		now := time.Now().UTC()
		activity = &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				ID:        activityID,
				Type:      activitypub.FollowType,
				Published: &now,
			},
			Actor:  username,
			Object: id,
		}
	} else if activity.Published == nil {
		now := time.Now().UTC()
		activity.Published = &now
	}

	return activity, nil

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

	// Get relationships service
	relationshipsService := r.Registry.Relationships()
	if relationshipsService == nil {
		r.Logger.Error("relationships service not available")
		return nil, ErrModerationUnavailable // Use existing error pattern from graph/errors.go
	}

	// Build command from GraphQL input
	cmd := &relationships.UpdateRelationshipCommand{
		FollowerID:  username,
		FollowingID: id, // The id parameter is the target user ID
		Notify:      input.Notify,
		ShowReblogs: input.ShowReblogs,
		Note:        input.Note,
	}

	// Handle languages - convert from []string to *[]string if provided
	if len(input.Languages) > 0 {
		cmd.Languages = &input.Languages
	}

	// Update the relationship
	relationshipData, err := relationshipsService.UpdateRelationship(ctx, cmd)
	if err != nil {
		r.Logger.Error("failed to update relationship",
			zap.String("follower", username),
			zap.String("following", id),
			zap.Error(err))
		return nil, err
	}

	// Track cost
	r.trackDynamoOperation(ctx, "write", 1)

	// Convert to GraphQL model
	gqlRelationship := r.convertRelationshipToGraphQL(relationshipData)

	r.Logger.Info("relationship updated successfully",
		zap.String("follower", username),
		zap.String("following", id))

	return gqlRelationship, nil
}
