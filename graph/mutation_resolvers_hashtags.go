package graph

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	"go.uber.org/zap"
)

// FollowHashtag is the resolver for the followHashtag field.
func (r *mutationResolver) FollowHashtag(ctx context.Context, hashtag string, notifyLevel *model.NotificationLevel) (*model.HashtagFollowPayload, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	// Get hashtag service
	hashtagService := r.Registry.Hashtags()
	if hashtagService == nil {
		r.Logger.Error("hashtag service not available")
		return &model.HashtagFollowPayload{Success: false}, fmt.Errorf("hashtag service not available")
	}

	// Build notification settings from input
	var settings *storage.HashtagNotificationSettings
	if notifyLevel != nil {
		level := "all" //nolint:goconst // Notification level, not query type
		if *notifyLevel == model.NotificationLevelFollowing || *notifyLevel == model.NotificationLevelMutuals {
			level = common.RelationshipFollowing
		}

		settings = &storage.HashtagNotificationSettings{
			Level:   level,
			Muted:   false,
			Filters: []*storage.NotificationFilter{},
		}
	}

	// Call service to follow
	resultHashtag, err := hashtagService.FollowHashtag(ctx, username, hashtag, settings)
	if err != nil {
		r.Logger.Error("failed to follow hashtag",
			zap.String("user", username),
			zap.String("hashtag", hashtag),
			zap.Error(err))
		return &model.HashtagFollowPayload{Success: false}, fmt.Errorf("failed to follow hashtag: %w", err)
	}

	// Track costs
	r.trackDynamoOperation(ctx, "write", 1)

	// Convert the result using the converter
	return &model.HashtagFollowPayload{
		Success: true,
		Hashtag: r.convertHashtagToModel(ctx, resultHashtag, username),
	}, nil
}

// UnfollowHashtag is the resolver for the unfollowHashtag field.
func (r *mutationResolver) UnfollowHashtag(ctx context.Context, hashtag string) (*model.UnfollowHashtagPayload, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	// Get hashtag service
	hashtagService := r.Registry.Hashtags()
	if hashtagService == nil {
		r.Logger.Error("hashtag service not available")
		return &model.UnfollowHashtagPayload{Success: false}, fmt.Errorf("hashtag service not available")
	}

	// Call service to unfollow
	resultHashtag, err := hashtagService.UnfollowHashtag(ctx, username, hashtag)
	if err != nil {
		r.Logger.Error("failed to unfollow hashtag",
			zap.String("user", username),
			zap.String("hashtag", hashtag),
			zap.Error(err))
		return &model.UnfollowHashtagPayload{Success: false}, fmt.Errorf("failed to unfollow hashtag: %w", err)
	}

	// Track costs
	r.trackDynamoOperation(ctx, "write", 1)

	// Convert the result using the converter
	return &model.UnfollowHashtagPayload{
		Success: true,
		Hashtag: r.convertHashtagToModel(ctx, resultHashtag, username),
	}, nil
}

// UpdateHashtagNotifications is the resolver for the updateHashtagNotifications field.
func (r *mutationResolver) UpdateHashtagNotifications(ctx context.Context, hashtag string, settings model.HashtagNotificationSettingsInput) (*model.UpdateHashtagNotificationsPayload, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	// Get hashtag repository for updating settings
	hashtagRepo := r.Storage.Hashtag()
	if hashtagRepo == nil {
		r.Logger.Error("hashtag repository not available")
		return &model.UpdateHashtagNotificationsPayload{Success: false}, fmt.Errorf("hashtag repository not available")
	}

	// Convert GraphQL input to storage model
	level := "all" //nolint:goconst // Notification level, not query type
	if settings.Level == model.NotificationLevelFollowing || settings.Level == model.NotificationLevelMutuals {
		level = common.RelationshipFollowing
	}

	storageSettings := &storage.HashtagNotificationSettings{
		UserID:  username,
		Hashtag: hashtag,
		Level:   level,
		Muted:   false,
		Filters: []*storage.NotificationFilter{},
	}

	// Convert filters if provided
	if settings.Filters != nil {
		storageSettings.Filters = make([]*storage.NotificationFilter, 0, len(settings.Filters))
		for _, filter := range settings.Filters {
			if filter != nil {
				storageSettings.Filters = append(storageSettings.Filters, &storage.NotificationFilter{
					Types:        []string{filter.Type},
					ExcludeTypes: []string{filter.Value},
				})
			}
		}
	}

	// Update settings in storage
	err = hashtagRepo.UpdateHashtagNotificationSettings(ctx, username, hashtag, storageSettings)
	if err != nil {
		r.Logger.Error("failed to update hashtag notification settings",
			zap.String("user", username),
			zap.String("hashtag", hashtag),
			zap.Error(err))
		return &model.UpdateHashtagNotificationsPayload{Success: false}, fmt.Errorf("failed to update notification settings: %w", err)
	}

	// Track costs
	r.trackDynamoOperation(ctx, "write", 1)

	// Fetch fresh hashtag data
	hashtagService := r.Registry.Hashtags()
	if hashtagService == nil {
		r.Logger.Warn("hashtag service not available for fetching updated hashtag")
		return &model.UpdateHashtagNotificationsPayload{
			Success:  true,
			Settings: r.convertInputToSettingsModel(settings),
		}, nil
	}

	freshHashtag, err := hashtagService.GetHashtag(ctx, hashtag, username)
	if err != nil {
		r.Logger.Error("failed to fetch hashtag after notification update",
			zap.String("hashtag", hashtag),
			zap.Error(err))
		return &model.UpdateHashtagNotificationsPayload{
			Success:  true,
			Settings: r.convertInputToSettingsModel(settings),
		}, nil
	}

	return &model.UpdateHashtagNotificationsPayload{
		Success:  true,
		Hashtag:  r.convertHashtagToModel(ctx, freshHashtag, username),
		Settings: r.convertInputToSettingsModel(settings),
	}, nil
}

// MuteHashtag is the resolver for the muteHashtag field.
func (r *mutationResolver) MuteHashtag(ctx context.Context, hashtag string, until *model.Time) (*model.MuteHashtagPayload, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	// Get hashtag service
	hashtagService := r.Registry.Hashtags()
	if hashtagService == nil {
		r.Logger.Error("hashtag service not available")
		return &model.MuteHashtagPayload{Success: false}, fmt.Errorf("hashtag service not available")
	}

	// Convert until time if provided
	var untilTime *model.Time
	if until != nil {
		untilTime = until
	}

	// Call service to mute
	var resultHashtag *model.Hashtag
	if untilTime != nil {
		t := time.Time(*untilTime)
		ht, err := hashtagService.MuteHashtag(ctx, username, hashtag, &t)
		if err != nil {
			r.Logger.Error("failed to mute hashtag",
				zap.String("user", username),
				zap.String("hashtag", hashtag),
				zap.Error(err))
			return &model.MuteHashtagPayload{Success: false}, fmt.Errorf("failed to mute hashtag: %w", err)
		}
		resultHashtag = r.convertHashtagToModel(ctx, ht, username)
	} else {
		ht, err := hashtagService.MuteHashtag(ctx, username, hashtag, nil)
		if err != nil {
			r.Logger.Error("failed to mute hashtag",
				zap.String("user", username),
				zap.String("hashtag", hashtag),
				zap.Error(err))
			return &model.MuteHashtagPayload{Success: false}, fmt.Errorf("failed to mute hashtag: %w", err)
		}
		resultHashtag = r.convertHashtagToModel(ctx, ht, username)
	}

	// Track costs
	r.trackDynamoOperation(ctx, "write", 1)

	return &model.MuteHashtagPayload{
		Success: true,
		Hashtag: resultHashtag,
	}, nil
}

// convertInputToSettingsModel converts GraphQL input to model output
func (r *mutationResolver) convertInputToSettingsModel(input model.HashtagNotificationSettingsInput) *model.HashtagNotificationSettings {
	result := &model.HashtagNotificationSettings{
		Level:   input.Level,
		Muted:   false,
		Filters: []*model.NotificationFilter{},
	}

	if input.Filters != nil {
		result.Filters = make([]*model.NotificationFilter, 0, len(input.Filters))
		for _, filter := range input.Filters {
			if filter != nil {
				result.Filters = append(result.Filters, &model.NotificationFilter{
					Type:  filter.Type,
					Value: filter.Value,
				})
			}
		}
	}

	return result
}
