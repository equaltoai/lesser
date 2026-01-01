// Package interfaces defines the repository interfaces for the Lesser application.
package interfaces

import (
	"context"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
)

// TimelineFilters represents filters for timeline queries
type TimelineFilters struct {
	OnlyMedia      bool   // Only show entries with media
	ExcludeReplies bool   // Exclude reply entries
	ExcludeBoosts  bool   // Exclude boost/announce entries
	Language       string // Filter by language
	MinID          string // Minimum entry ID (for pagination)
	MaxID          string // Maximum entry ID (for pagination)
}

// TimelineRepository defines the interface for timeline operations.
// This handles timeline entry management for home, public, list, direct, and hashtag timelines.
type TimelineRepository interface {
	// Core timeline entry operations
	CreateTimelineEntry(ctx context.Context, entry *models.Timeline) error
	CreateTimelineEntries(ctx context.Context, entries []*models.Timeline) error
	GetTimelineEntry(ctx context.Context, timelineType, timelineID, entryID string, timelineAt time.Time) (*models.Timeline, error)
	UpdateTimelineEntry(ctx context.Context, entry *models.Timeline) error
	DeleteTimelineEntry(ctx context.Context, timelineType, timelineID, entryID string, timelineAt time.Time) error

	// Timeline retrieval by type
	GetHomeTimeline(ctx context.Context, username string, limit int, cursor string) ([]*models.Timeline, string, error)
	GetPublicTimeline(ctx context.Context, local bool, limit int, cursor string) ([]*models.Timeline, string, error)
	GetListTimeline(ctx context.Context, listID string, limit int, cursor string) ([]*models.Timeline, string, error)
	GetDirectTimeline(ctx context.Context, username string, limit int, cursor string) ([]*models.Timeline, string, error)
	GetHashtagTimeline(ctx context.Context, hashtag string, local bool, limit int, cursor string) ([]*models.Timeline, string, error)

	// Timeline retrieval by index
	GetTimelineEntriesByPost(ctx context.Context, postID string, limit int, cursor string) ([]*models.Timeline, string, error)
	GetTimelineEntriesByActor(ctx context.Context, actorID string, limit int, cursor string) ([]*models.Timeline, string, error)
	GetTimelineEntriesByVisibility(ctx context.Context, visibility string, limit int, cursor string) ([]*models.Timeline, string, error)
	GetTimelineEntriesByLanguage(ctx context.Context, language string, limit int, cursor string) ([]*models.Timeline, string, error)

	// Advanced timeline queries
	GetTimelineEntriesInRange(ctx context.Context, timelineType, timelineID string, startTime, endTime time.Time, limit int) ([]*models.Timeline, error)
	GetTimelineEntriesWithFilters(ctx context.Context, timelineType, timelineID string, filters TimelineFilters, limit int, cursor string) ([]*models.Timeline, string, error)
	CountTimelineEntries(ctx context.Context, timelineType, timelineID string) (int, error)

	// Batch operations
	DeleteTimelineEntriesByPost(ctx context.Context, postID string) error
	DeleteExpiredTimelineEntries(ctx context.Context, before time.Time) error
	RemoveFromTimelines(ctx context.Context, objectID string) error

	// Conversation support (timeline interface compatibility)
	GetConversations(ctx context.Context, username string, limit int, cursor string) ([]*models.Conversation, string, error)
}
