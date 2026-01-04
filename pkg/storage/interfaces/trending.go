// Package interfaces defines the repository interfaces for the Lesser application.
package interfaces //nolint:revive // Standard interfaces package name

import (
	"context"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
)

// TrendingRepository defines the interface for trending and analytics operations.
// This handles trending hashtags, statuses, links, and engagement metrics.
type TrendingRepository interface {
	// Recording operations

	// RecordHashtagUsage records when a hashtag is used in a status
	RecordHashtagUsage(ctx context.Context, hashtag string, statusID string, authorID string) error

	// RecordStatusEngagement records engagement on a status (like, boost, reply)
	RecordStatusEngagement(ctx context.Context, statusID string, engagementType string, userID string) error

	// RecordLinkShare records when a link is shared in a status
	RecordLinkShare(ctx context.Context, linkURL string, statusID string, authorID string) error

	// Trending retrieval operations

	// GetTrendingHashtags returns the top trending hashtags since the given time
	GetTrendingHashtags(ctx context.Context, since time.Time, limit int) ([]*storage.TrendingHashtag, error)

	// GetTrendingStatuses returns the top trending statuses since the given time
	GetTrendingStatuses(ctx context.Context, since time.Time, limit int) ([]*storage.TrendingStatus, error)

	// GetTrendingLinks returns the top trending links since the given time
	GetTrendingLinks(ctx context.Context, since time.Time, limit int) ([]*storage.TrendingLink, error)

	// Recent retrieval operations (no trending calculation)

	// GetRecentHashtags returns recent hashtags since the given time
	GetRecentHashtags(ctx context.Context, since time.Time, limit int) ([]*storage.TrendingHashtag, error)

	// GetRecentStatusesWithEngagement returns recent statuses with engagement since the given time
	GetRecentStatusesWithEngagement(ctx context.Context, since time.Time, limit int) ([]*storage.TrendingStatus, error)

	// GetRecentLinks returns recent links since the given time
	GetRecentLinks(ctx context.Context, since time.Time, limit int) ([]*storage.TrendingLink, error)

	// Engagement metrics operations

	// StoreEngagementMetrics stores engagement metrics for a status
	StoreEngagementMetrics(ctx context.Context, metrics *storage.EngagementMetrics) error

	// GetEngagementMetrics retrieves stored engagement metrics for a status
	GetEngagementMetrics(ctx context.Context, statusID string) (*storage.EngagementMetrics, error)

	// Trend storage operations

	// StoreHashtagTrend stores a hashtag trend record
	StoreHashtagTrend(ctx context.Context, trend any) error

	// StoreStatusTrend stores a status trend record
	StoreStatusTrend(ctx context.Context, trend any) error

	// StoreLinkTrend stores a link trend record
	StoreLinkTrend(ctx context.Context, trend any) error

	// SetStatusRepository sets the status repository dependency for cross-repository operations
	SetStatusRepository(statusRepo interface{})
}
