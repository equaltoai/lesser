package graph

import (
	"context"
	"errors"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/services/relationships"
	"go.uber.org/zap"
)

// Stub implementations for missing mutations that are defined in the schema but not yet fully implemented

// UnblockActor is the resolver for the unblockActor field.
func (r *mutationResolver) UnblockActor(ctx context.Context, id string) (bool, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return false, err
	}

	// Get the relationships service
	relationshipsService := r.Registry.Relationships()
	if relationshipsService == nil {
		return false, errors.New("relationships service is not available")
	}

	// Unblock the actor using the relationships service
	_, err = relationshipsService.Unblock(ctx, &relationships.UnblockCommand{
		BlockerID: username,
		BlockedID: id,
	})
	if err != nil {
		r.Logger.Error("Failed to unblock actor",
			zap.String("user", username),
			zap.String("target_id", id),
			zap.Error(err))
		return false, err
	}

	// Track cost using centralized tracker
	r.trackDynamoOperation(ctx, "write", 1)

	r.Logger.Info("unblocked actor successfully",
		zap.String("user", username),
		zap.String("target_id", id))

	return true, nil
}

// UnfollowActor is the resolver for the unfollowActor field.
func (r *mutationResolver) UnfollowActor(ctx context.Context, id string) (bool, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return false, err
	}

	// Get the relationships service
	relationshipsService := r.Registry.Relationships()
	if relationshipsService == nil {
		return false, errors.New("relationships service is not available")
	}

	// Unfollow the actor using the relationships service
	_, err = relationshipsService.Unfollow(ctx, &relationships.UnfollowCommand{
		FollowerID:  username,
		FollowingID: id,
	})
	if err != nil {
		r.Logger.Error("Failed to unfollow actor",
			zap.String("user", username),
			zap.String("target_id", id),
			zap.Error(err))
		return false, err
	}

	// Track cost using centralized tracker
	r.trackDynamoOperation(ctx, "write", 1)

	r.Logger.Info("unfollowed actor successfully",
		zap.String("user", username),
		zap.String("target_id", id))

	return true, nil
}

// UnmuteActor is the resolver for the unmuteActor field.
func (r *mutationResolver) UnmuteActor(ctx context.Context, id string) (bool, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return false, err
	}

	// Get the relationships service
	relationshipsService := r.Registry.Relationships()
	if relationshipsService == nil {
		return false, errors.New("relationships service is not available")
	}

	// Unmute the actor using the relationships service
	_, err = relationshipsService.Unmute(ctx, &relationships.UnmuteCommand{
		MuterID: username,
		MutedID: id,
	})
	if err != nil {
		r.Logger.Error("Failed to unmute actor",
			zap.String("user", username),
			zap.String("target_id", id),
			zap.Error(err))
		return false, err
	}

	// Track cost using centralized tracker
	r.trackDynamoOperation(ctx, "write", 1)

	r.Logger.Info("unmuted actor successfully",
		zap.String("user", username),
		zap.String("target_id", id))

	return true, nil
}

// NOTE: UnfollowHashtag and UpdateHashtagNotifications are now implemented in mutation_resolvers_hashtags.go

// UpdateStreamingPreferences is the resolver for the updateStreamingPreferences field.
func (r *mutationResolver) UpdateStreamingPreferences(ctx context.Context, input model.StreamingPreferencesInput) (*model.UserPreferences, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	// Get the user repository
	userRepo := r.Registry.GetStorage().User()
	if userRepo == nil {
		return nil, errors.New("user repository is not available")
	}

	// Create preferences map from input
	preferencesMap := map[string]interface{}{
		"streaming_default_quality": string(input.DefaultQuality),
		"streaming_auto_quality":    input.AutoQuality,
		"streaming_preload_next":    input.PreloadNext,
		"streaming_data_saver":      input.DataSaver,
	}

	// Update preferences
	err = userRepo.UpdatePreferences(ctx, username, preferencesMap)
	if err != nil {
		r.Logger.Error("Failed to update streaming preferences",
			zap.String("user", username),
			zap.Error(err))
		return nil, err
	}

	// Track cost using centralized tracker
	r.trackDynamoOperation(ctx, "write", 1)

	r.Logger.Info("streaming preferences updated successfully",
		zap.String("user", username))

	// Return updated preferences
	return &model.UserPreferences{
		ActorID: username,
		Streaming: &model.StreamingPreferences{
			DefaultQuality: input.DefaultQuality,
			AutoQuality:    input.AutoQuality,
			PreloadNext:    input.PreloadNext,
			DataSaver:      input.DataSaver,
		},
		// Return default values for other preference groups
		Notifications: &model.NotificationPreferences{
			Email:  false, // Email notifications not supported by Lesser
			Push:   true,
			InApp:  true,
			Digest: model.DigestFrequencyNever,
		},
		Privacy: &model.PrivacyPreferences{
			DefaultVisibility: model.VisibilityPublic,
			Indexable:         true,
			ShowOnlineStatus:  true,
		},
	}, nil
}

// WithdrawFromQuotes is the resolver for the withdrawFromQuotes field.
func (r *mutationResolver) WithdrawFromQuotes(ctx context.Context, noteID string) (*model.WithdrawQuotePayload, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	// Get the quotes service
	quotesService := r.Registry.Quotes()
	if quotesService == nil {
		r.Logger.Error("quotes service not available")
		return nil, errors.New("quotes service is not available")
	}

	// Withdraw quotes
	note, withdrawnCount, err := quotesService.WithdrawFromQuotes(ctx, noteID, username)
	if err != nil {
		r.Logger.Error("Failed to withdraw from quotes",
			zap.String("user", username),
			zap.String("note_id", noteID),
			zap.Error(err))
		return nil, err
	}

	// Track cost using centralized tracker
	r.trackDynamoOperation(ctx, "write", int64(withdrawnCount))

	r.Logger.Info("withdrew from quotes successfully",
		zap.String("user", username),
		zap.String("note_id", noteID),
		zap.Int("withdrawn_count", withdrawnCount))

	// Convert note to Object for GraphQL response
	var noteObject *model.Object
	if note != nil {
		noteObject = r.convertStatusToObject(ctx, note)
	}

	return &model.WithdrawQuotePayload{
		Success:        true,
		Note:           noteObject,
		WithdrawnCount: withdrawnCount,
	}, nil
}
