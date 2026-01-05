// Package interfaces defines the repository interfaces for the Lesser application.
package interfaces //nolint:revive // Standard interfaces package name

import (
	"context"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

// LikeRepository defines the interface for like operations.
// This handles likes/favorites for statuses and other content.
type LikeRepository interface {
	// CreateLike creates a new like
	CreateLike(ctx context.Context, actor, object, statusAuthorID string) (*models.Like, error)

	// DeleteLike removes a like
	DeleteLike(ctx context.Context, actor, object string) error

	// GetLike retrieves a specific like
	GetLike(ctx context.Context, actor, object string) (*models.Like, error)

	// GetObjectLikes retrieves all likes for an object with pagination
	GetObjectLikes(ctx context.Context, objectID string, limit int, cursor string) ([]*models.Like, string, error)

	// GetActorLikes retrieves all likes by an actor with pagination
	GetActorLikes(ctx context.Context, actorID string, limit int, cursor string) ([]*models.Like, string, error)

	// CountActorLikes returns the total number of likes by an actor
	CountActorLikes(ctx context.Context, actorID string) (int64, error)

	// HasLiked checks if an actor has liked an object
	HasLiked(ctx context.Context, actor, object string) (bool, error)

	// CascadeDeleteLikes deletes all likes for an object
	CascadeDeleteLikes(ctx context.Context, objectID string) error

	// TombstoneObject creates a tombstone for a deleted object
	TombstoneObject(ctx context.Context, objectID string, deletedBy string) error

	// GetTombstone retrieves a tombstone by object ID
	GetTombstone(ctx context.Context, objectID string) (*storage.Tombstone, error)

	// GetLikeCount counts likes for a status
	GetLikeCount(ctx context.Context, statusID string) (int64, error)

	// GetBoostCount counts boosts/announces for a status
	GetBoostCount(ctx context.Context, statusID string) (int64, error)

	// IncrementReblogCount increments the reblog count on a status
	IncrementReblogCount(ctx context.Context, objectID string) error

	// HasReblogged checks if a user has reblogged/boosted a specific status
	HasReblogged(ctx context.Context, actorID, statusID string) (bool, error)

	// Storage interface compatibility methods

	// CountForObject provides Storage interface compatibility for CountObjectLikes
	CountForObject(ctx context.Context, objectID string) (int64, error)

	// GetForObject provides Storage interface compatibility for GetObjectLikes
	GetForObject(ctx context.Context, objectID string, limit int, cursor string) ([]*models.Like, string, error)

	// GetLikedObjects provides Storage interface compatibility
	GetLikedObjects(ctx context.Context, actorID string, limit int, cursor string) ([]*models.Like, string, error)
}
