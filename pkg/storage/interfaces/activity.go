// Package interfaces defines the repository interfaces for the Lesser application.
package interfaces //nolint:revive // Standard interfaces package name

import (
	"context"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
)

// ActivityRepository defines the interface for ActivityPub activity operations.
// This handles activity lifecycle, inbox/outbox management, collections, and federation tracking.
type ActivityRepository interface {
	// ===== Core Activity Operations =====

	// CreateActivity stores an activity in the database
	CreateActivity(ctx context.Context, activity *activitypub.Activity) error

	// GetActivity retrieves an activity by ID
	GetActivity(ctx context.Context, id string) (*activitypub.Activity, error)

	// ===== Inbox/Outbox Operations =====

	// GetInboxActivities retrieves inbox activities for a user with pagination
	GetInboxActivities(ctx context.Context, username string, limit int, cursor string) ([]*activitypub.Activity, string, error)

	// GetOutboxActivities retrieves activities created by a user with pagination
	GetOutboxActivities(ctx context.Context, username string, limit int, cursor string) ([]*activitypub.Activity, string, error)

	// ===== Collection Operations =====

	// GetCollection retrieves a collection for an actor (inbox, outbox, followers, following, liked)
	GetCollection(ctx context.Context, username, collectionType string, limit int, cursor string) (*activitypub.OrderedCollectionPage, error)

	// ===== Analytics and Metrics Operations =====

	// GetWeeklyActivity retrieves weekly activity statistics
	GetWeeklyActivity(ctx context.Context, weekTimestamp int64) (*storage.WeeklyActivity, error)

	// RecordActivity records general activity metrics
	RecordActivity(ctx context.Context, activityType string, actorID string, timestamp time.Time) error

	// GetHashtagActivity retrieves activities related to a hashtag since a given time
	GetHashtagActivity(ctx context.Context, hashtag string, since time.Time) ([]*storage.Activity, error)

	// ===== Federation Operations =====

	// RecordFederationActivity records federation activity metrics
	RecordFederationActivity(ctx context.Context, activity *storage.FederationActivity) error
}
