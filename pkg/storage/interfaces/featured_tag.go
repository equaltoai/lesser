// Package interfaces defines the repository interfaces for the Lesser application.
package interfaces

import (
	"context"

	"github.com/equaltoai/lesser/pkg/storage"
)

// FeaturedTagRepository defines the interface for featured tag operations.
// This handles user-featured hashtags for profile display and tag suggestions.
type FeaturedTagRepository interface {
	// ===== Core Featured Tag Operations =====

	// CreateFeaturedTag creates a new featured tag for a user
	CreateFeaturedTag(ctx context.Context, tag *storage.FeaturedTag) error

	// DeleteFeaturedTag removes a featured tag
	DeleteFeaturedTag(ctx context.Context, username, name string) error

	// GetFeaturedTags returns all featured tags for a user
	GetFeaturedTags(ctx context.Context, username string) ([]*storage.FeaturedTag, error)

	// ===== Tag Suggestions =====

	// GetTagSuggestions returns suggested tags based on user's usage
	GetTagSuggestions(ctx context.Context, username string, limit int) ([]string, error)
}
