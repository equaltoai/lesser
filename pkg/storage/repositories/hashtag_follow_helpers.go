package repositories

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
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
	tagLower := strings.ToLower(strings.TrimPrefix(hashtag, "#"))
	now := time.Now()

	// Get existing follow
	var existingFollow models.HashtagFollow
	err := db.Model(&models.HashtagFollow{}).
		Where("PK", "=", fmt.Sprintf("USER#%s", userID)).
		Where("SK", "=", fmt.Sprintf("HASHTAG_FOLLOW#%s", tagLower)).
		First(&existingFollow)

	if err != nil {
		logger.Error(fmt.Sprintf("failed to get hashtag follow for %s", config.Operation),
			zap.String("user_id", userID),
			zap.String("hashtag", tagLower),
			zap.Error(err))
		return fmt.Errorf("failed to get hashtag follow: %w", err)
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
		return fmt.Errorf("unknown operation: %s", config.Operation)
	}

	existingFollow.UpdatedAt = now

	// Save by recreating (DynamORM pattern)
	err = db.Model(&existingFollow).Create()
	if err != nil {
		logger.Error(fmt.Sprintf("failed to %s hashtag", config.Operation),
			zap.String("user_id", userID),
			zap.String("hashtag", tagLower),
			zap.Error(err))
		return fmt.Errorf("failed to %s hashtag: %w", config.Operation, err)
	}

	return nil
}