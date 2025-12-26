package graph

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
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

// AcceptFollowRequest accepts a pending follow request for the current viewer.
func (r *mutationResolver) AcceptFollowRequest(ctx context.Context, accountID string) (*model.Relationship, error) {
	return r.resolveFollowRequest(ctx, accountID, true)
}

// RejectFollowRequest rejects a pending follow request for the current viewer.
func (r *mutationResolver) RejectFollowRequest(ctx context.Context, accountID string) (*model.Relationship, error) {
	return r.resolveFollowRequest(ctx, accountID, false)
}

func (r *mutationResolver) resolveFollowRequest(ctx context.Context, accountID string, accept bool) (*model.Relationship, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	if err := common.ValidateRequiredParam("accountID", accountID); err != nil {
		return nil, err
	}

	relationshipsService := r.Registry.Relationships()
	if relationshipsService == nil {
		return nil, errors.New("relationships service is not available")
	}

	accountID = strings.TrimSpace(accountID)

	var (
		result *relationships.RelationshipResult
		action = "reject"
	)

	if accept {
		action = "accept"
		result, err = relationshipsService.AcceptFollowRequest(ctx, &relationships.AcceptFollowRequestCommand{
			RequesterID: username,
			FollowerID:  accountID,
		})
	} else {
		result, err = relationshipsService.RejectFollowRequest(ctx, &relationships.RejectFollowRequestCommand{
			RequesterID: username,
			FollowerID:  accountID,
		})
	}
	if err != nil {
		r.Logger.Error("Failed to handle follow request",
			zap.String("action", action),
			zap.String("user", username),
			zap.String("account_id", accountID),
			zap.Error(err))
		return nil, errors.Join(fmt.Errorf("failed to %s follow request", action), err)
	}

	r.trackDynamoOperation(ctx, "write", 1)

	return r.convertRelationshipToGraphQL(result.Relationship), nil
}

// AddDomainBlock blocks a domain for the current viewer.
func (r *mutationResolver) AddDomainBlock(ctx context.Context, domain string) (bool, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return false, err
	}
	if err := common.ValidateRequiredParam("domain", domain); err != nil {
		return false, err
	}

	relationshipsService := r.Registry.Relationships()
	if relationshipsService == nil {
		return false, errors.New("relationships service is not available")
	}

	if err := relationshipsService.AddDomainBlock(ctx, &relationships.AddDomainBlockCommand{
		UserID: username,
		Domain: strings.TrimSpace(domain),
	}); err != nil {
		r.Logger.Error("Failed to add domain block",
			zap.String("user", username),
			zap.String("domain", domain),
			zap.Error(err))
		return false, errors.Join(errors.New("failed to add domain block"), err)
	}

	r.trackDynamoOperation(ctx, "write", 1)
	return true, nil
}

// RemoveDomainBlock unblocks a domain for the current viewer.
func (r *mutationResolver) RemoveDomainBlock(ctx context.Context, domain string) (bool, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return false, err
	}
	if err := common.ValidateRequiredParam("domain", domain); err != nil {
		return false, err
	}

	relationshipsService := r.Registry.Relationships()
	if relationshipsService == nil {
		return false, errors.New("relationships service is not available")
	}

	if err := relationshipsService.RemoveDomainBlock(ctx, &relationships.RemoveDomainBlockCommand{
		UserID: strings.TrimSpace(username),
		Domain: strings.TrimSpace(domain),
	}); err != nil {
		r.Logger.Error("Failed to remove domain block",
			zap.String("user", username),
			zap.String("domain", domain),
			zap.Error(err))
		return false, errors.Join(errors.New("failed to remove domain block"), err)
	}

	r.trackDynamoOperation(ctx, "write", 1)
	return true, nil
}
