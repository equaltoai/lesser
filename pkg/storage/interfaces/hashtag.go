// Package interfaces defines the repository interfaces for the Lesser application.
package interfaces //nolint:revive // Standard interfaces package name

import (
	"context"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

// HashtagRepository defines the interface for hashtag operations.
// This handles hashtag indexing, trending, and user hashtag follows.
type HashtagRepository interface {
	// IndexHashtag indexes a hashtag when used in a status
	IndexHashtag(ctx context.Context, hashtag string, statusID string, authorID string, visibility string) error

	// IndexStatusHashtags indexes a status with its hashtags for efficient search
	IndexStatusHashtags(ctx context.Context, statusID string, authorID string, authorHandle string, statusURL string, content string, hashtags []string, published time.Time, visibility string) error

	// RemoveStatusFromHashtagIndex removes a status from all hashtag indexes
	RemoveStatusFromHashtagIndex(ctx context.Context, statusID string) error

	// GetHashtagInfo retrieves information about a specific hashtag
	GetHashtagInfo(ctx context.Context, hashtag string) (*storage.Hashtag, error)

	// GetHashtagUsageHistory retrieves recent usage history for a hashtag
	GetHashtagUsageHistory(ctx context.Context, hashtag string, days int) ([]int64, error)

	// GetHashtagActivity retrieves activities for a hashtag since a specific time
	GetHashtagActivity(ctx context.Context, hashtag string, since time.Time) ([]*storage.Activity, error)

	// GetHashtagStats retrieves hashtag statistics
	GetHashtagStats(ctx context.Context, hashtag string) (any, error)

	// GetHashtagTimelineAdvanced retrieves hashtag timeline with advanced filtering
	GetHashtagTimelineAdvanced(ctx context.Context, hashtag string, maxID *string, limit int, visibility string) ([]*storage.StatusSearchResult, error)

	// GetMultiHashtagTimeline retrieves timeline for multiple hashtags
	GetMultiHashtagTimeline(ctx context.Context, hashtags []string, maxID *string, limit int, userID string) ([]*storage.StatusSearchResult, error)

	// GetSuggestedHashtags gets suggested hashtags for a user
	GetSuggestedHashtags(ctx context.Context, userID string, limit int) ([]*storage.HashtagSearchResult, error)

	// FollowHashtag creates a hashtag follow relationship
	FollowHashtag(ctx context.Context, userID, hashtag string) error

	// UnfollowHashtag removes a hashtag follow relationship
	UnfollowHashtag(ctx context.Context, userID, hashtag string) error

	// IsFollowingHashtag checks if a user is following a hashtag
	IsFollowingHashtag(ctx context.Context, userID, hashtag string) (bool, error)

	// GetHashtagFollow retrieves the hashtag follow record for a user
	GetHashtagFollow(ctx context.Context, userID string, hashtag string) (*models.HashtagFollow, error)

	// GetHashtagMute retrieves the hashtag mute record for a user
	GetHashtagMute(ctx context.Context, userID string, hashtag string) (*models.HashtagMute, error)
}
