package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/theory-cloud/tabletheory/pkg/core"
	"go.uber.org/zap"
)

// HashtagFollowUpdateConfig holds configuration for hashtag follow updates
type HashtagFollowUpdateConfig struct {
	Operation   string // "notification", "mute", "unmute"
	BoolValue   *bool  // For notification setting or mute/unmute
	ErrorPrefix string // Error message prefix
}

// updateHashtagFollowSetting is a generic helper for updating hashtag follow settings
func updateHashtagFollowSetting(
	ctx context.Context,
	db core.DB,
	logger *zap.Logger,
	userID, hashtag string,
	config HashtagFollowUpdateConfig,
) error {
	tagLower := normalizeHashtagName(hashtag)
	if tagLower == "" {
		return ErrorHandler.HandleUpdateError(storage.ErrInvalidInput, EntityHashtag, "empty hashtag")
	}
	now := time.Now().UTC()
	pk := fmt.Sprintf("user#%s", userID)
	sk := fmt.Sprintf("hashtag#%s", tagLower)

	// Get existing follow
	var existingFollow models.HashtagFollow
	err := db.WithContext(ctx).Model(&models.HashtagFollow{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(&existingFollow)

	if err != nil {
		logger.Error(fmt.Sprintf("failed to get hashtag follow for %s", config.Operation),
			zap.String("user_id", userID),
			zap.String("hashtag", tagLower),
			zap.Error(err))
		return ErrorHandler.HandleGetError(err, EntityHashtag, fmt.Sprintf("follow %s#%s", userID, tagLower))
	}

	// Update the appropriate field based on operation
	switch config.Operation {
	case "notification":
		if config.BoolValue != nil {
			existingFollow.NotificationsEnabled = *config.BoolValue
		}
	case "mute":
		existingFollow.Muted = true
	case "unmute":
		existingFollow.Muted = false
	default:
		logger.Error("unknown operation type for hashtag follow",
			zap.String("operation", config.Operation),
			zap.String("user_id", userID),
			zap.String("hashtag", tagLower))
		return ErrorHandler.HandleUpdateError(storage.ErrInvalidInput, EntityHashtag, fmt.Sprintf("follow %s#%s operation %s", userID, tagLower, config.Operation))
	}

	existingFollow.UpdatedAt = now

	// Save by recreating (DynamORM pattern)
	err = db.WithContext(ctx).Model(&existingFollow).Create()
	if err != nil {
		logger.Error(fmt.Sprintf("failed to %s hashtag", config.Operation),
			zap.String("user_id", userID),
			zap.String("hashtag", tagLower),
			zap.Error(err))
		return ErrorHandler.HandleUpdateError(err, EntityHashtag, fmt.Sprintf("follow %s#%s", userID, tagLower))
	}

	return nil
}
