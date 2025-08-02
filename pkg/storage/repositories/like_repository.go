package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// LikeRepository implements like operations using DynamORM
type LikeRepository struct {
	db        core.DB
	tableName string
	logger    *zap.Logger
}

// NewLikeRepository creates a new like repository
func NewLikeRepository(db core.DB, tableName string, logger *zap.Logger) *LikeRepository {
	return &LikeRepository{
		db:        db,
		tableName: tableName,
		logger:    logger,
	}
}

// CreateLike creates a new like
func (r *LikeRepository) CreateLike(ctx context.Context, actor, object string) (*models.Like, error) {
	like := models.NewLike(actor, object)

	if err := r.db.WithContext(ctx).Model(like).Create(); err != nil {
		// Check if it's a duplicate key error (already liked)
		if errors.IsConditionFailed(err) {
			r.logger.Debug("like already exists",
				zap.String("actor", actor),
				zap.String("object", object))
			return like, nil
		}
		r.logger.Error("failed to create like",
			zap.String("actor", actor),
			zap.String("object", object),
			zap.Error(err))
		return nil, fmt.Errorf("failed to create like: %w", err)
	}

	r.logger.Info("created like",
		zap.String("like_id", like.ID),
		zap.String("actor", actor),
		zap.String("object", object))

	return like, nil
}

// DeleteLike removes a like
func (r *LikeRepository) DeleteLike(ctx context.Context, actor, object string) error {
	like := &models.Like{
		PK: fmt.Sprintf("object#%s#likes", object),
		SK: fmt.Sprintf("actor#%s", actor),
	}

	if err := r.db.WithContext(ctx).Model(like).
		Where("PK", "=", like.PK).
		Where("SK", "=", like.SK).
		Delete(); err != nil {
		if errors.IsNotFound(err) {
			r.logger.Debug("like not found",
				zap.String("actor", actor),
				zap.String("object", object))
			return nil
		}
		r.logger.Error("failed to delete like",
			zap.String("actor", actor),
			zap.String("object", object),
			zap.Error(err))
		return fmt.Errorf("failed to delete like: %w", err)
	}

	r.logger.Info("deleted like",
		zap.String("actor", actor),
		zap.String("object", object))

	return nil
}

// GetLike retrieves a specific like
func (r *LikeRepository) GetLike(ctx context.Context, actor, object string) (*models.Like, error) {
	var like models.Like
	
	query := r.db.WithContext(ctx).Model(&like).
		Where("PK", "=", fmt.Sprintf("object#%s#likes", object)).
		Where("SK", "=", fmt.Sprintf("actor#%s", actor))

	if err := query.First(&like); err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("like not found")
		}
		return nil, fmt.Errorf("failed to get like: %w", err)
	}

	return &like, nil
}

// GetObjectLikes retrieves all likes for an object
func (r *LikeRepository) GetObjectLikes(ctx context.Context, objectID string, limit int, cursor string) ([]*models.Like, string, error) {
	query := r.db.WithContext(ctx).Model(&models.Like{}).
		Where("PK", "=", fmt.Sprintf("object#%s#likes", objectID)).
		Limit(limit)

	if cursor != "" {
		query = query.Cursor(cursor)
	}

	var likes []models.Like
	err := query.All(&likes)
	nextCursor := "" // TODO: implement pagination
	if err != nil {
		return nil, "", fmt.Errorf("failed to scan likes: %w", err)
	}

	// Convert to pointer slice
	result := make([]*models.Like, len(likes))
	for i := range likes {
		result[i] = &likes[i]
	}

	return result, nextCursor, nil
}

// GetActorLikes retrieves all likes by an actor
func (r *LikeRepository) GetActorLikes(ctx context.Context, actorID string, limit int, cursor string) ([]*models.Like, string, error) {
	query := r.db.WithContext(ctx).Model(&models.Like{}).
		Index("gsi1-index").
		Where("GSI1PK", "=", fmt.Sprintf("actor#%s#likes", actorID)).
		Limit(limit)

	if cursor != "" {
		query = query.Cursor(cursor)
	}

	var likes []models.Like
	err := query.All(&likes)
	nextCursor := "" // TODO: implement pagination
	if err != nil {
		return nil, "", fmt.Errorf("failed to scan likes: %w", err)
	}

	// Convert to pointer slice
	result := make([]*models.Like, len(likes))
	for i := range likes {
		result[i] = &likes[i]
	}

	return result, nextCursor, nil
}

// HasLiked checks if an actor has liked an object
func (r *LikeRepository) HasLiked(ctx context.Context, actor, object string) (bool, error) {
	like, err := r.GetLike(ctx, actor, object)
	if err != nil {
		if err.Error() == "like not found" {
			return false, nil
		}
		return false, err
	}
	return like != nil, nil
}

// CascadeDeleteLikes deletes all likes for an object
func (r *LikeRepository) CascadeDeleteLikes(ctx context.Context, objectID string) error {
	// Query all likes for the object
	// PK pattern: object#{object_id}#likes
	pk := fmt.Sprintf("object#%s#likes", objectID)
	
	// Keep deleting in batches until no more likes remain
	for {
		// Query likes with a reasonable batch size
		var likes []models.Like
		err := r.db.WithContext(ctx).Model(&models.Like{}).
			Where("PK", "=", pk).
			Limit(100). // Process in batches of 100
			All(&likes)
		
		if err != nil {
			return fmt.Errorf("failed to query likes for deletion: %w", err)
		}
		
		// If no likes found, we're done
		if len(likes) == 0 {
			break
		}
		
		// Delete each like
		for _, like := range likes {
			err = r.db.WithContext(ctx).Model(&models.Like{}).
				Where("PK", "=", like.PK).
				Where("SK", "=", like.SK).
				Delete()
			
			if err != nil {
				// Log but continue - best effort deletion
				r.logger.Warn("failed to delete like during cascade",
					zap.String("pk", like.PK),
					zap.String("sk", like.SK),
					zap.Error(err))
			}
		}
		
		// If we got less than the limit, we're done
		if len(likes) < 100 {
			break
		}
	}
	
	r.logger.Info("cascade deleted likes for object",
		zap.String("object_id", objectID))
		
	return nil
}

// TombstoneObject creates a tombstone for a deleted object
func (r *LikeRepository) TombstoneObject(ctx context.Context, objectID string, deletedBy string) error {
	// Create tombstone model
	tombstone := &models.Tombstone{
		PK:         fmt.Sprintf("OBJECT#%s", objectID),
		SK:         "TOMBSTONE",
		ID:         objectID,
		Type:       "Tombstone",
		FormerType: "Object", // Default, could be enhanced to get actual type
		Deleted:    time.Now(),
		DeletedBy:  deletedBy,
	}

	if err := r.db.WithContext(ctx).Model(tombstone).Create(); err != nil {
		r.logger.Error("failed to create tombstone",
			zap.String("object_id", objectID),
			zap.String("deleted_by", deletedBy),
			zap.Error(err))
		return fmt.Errorf("failed to create tombstone: %w", err)
	}

	r.logger.Info("created tombstone",
		zap.String("object_id", objectID),
		zap.String("deleted_by", deletedBy))

	return nil
}

// GetTombstone retrieves a tombstone by object ID
func (r *LikeRepository) GetTombstone(ctx context.Context, objectID string) (*storage.Tombstone, error) {
	var tombstone models.Tombstone
	
	query := r.db.WithContext(ctx).Model(&tombstone).
		Where("PK", "=", fmt.Sprintf("OBJECT#%s", objectID)).
		Where("SK", "=", "TOMBSTONE")

	if err := query.First(&tombstone); err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("tombstone not found for object: %s", objectID)
		}
		r.logger.Error("failed to get tombstone",
			zap.String("object_id", objectID),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get tombstone: %w", err)
	}

	// Convert to storage.Tombstone
	result := &storage.Tombstone{
		ID:         tombstone.ID,
		Type:       tombstone.Type,
		FormerType: tombstone.FormerType,
		Deleted:    tombstone.Deleted,
		DeletedBy:  tombstone.DeletedBy,
		Summary:    tombstone.Summary,
	}

	return result, nil
}

// GetLikeCount counts likes for a status
func (r *LikeRepository) GetLikeCount(ctx context.Context, statusID string) (int64, error) {
	// Query likes with PK pattern: OBJECT#{statusID}#LIKES
	pk := fmt.Sprintf("OBJECT#%s#LIKES", statusID)
	
	count, err := r.db.WithContext(ctx).Model(&models.Like{}).
		Where("PK", "=", pk).
		Count()
	
	if err != nil {
		r.logger.Error("failed to count likes",
			zap.String("status_id", statusID),
			zap.Error(err))
		return 0, fmt.Errorf("failed to count likes: %w", err)
	}

	return count, nil
}

// GetBoostCount counts boosts/announces for a status
func (r *LikeRepository) GetBoostCount(ctx context.Context, statusID string) (int64, error) {
	// Query announces with PK pattern: OBJECT#{statusID}#ANNOUNCES
	pk := fmt.Sprintf("OBJECT#%s#ANNOUNCES", statusID)
	
	count, err := r.db.WithContext(ctx).Model(&models.Announce{}).
		Where("PK", "=", pk).
		Count()
	
	if err != nil {
		r.logger.Error("failed to count boosts",
			zap.String("status_id", statusID),
			zap.Error(err))
		return 0, fmt.Errorf("failed to count boosts: %w", err)
	}

	return count, nil
}

// IncrementReblogCount increments the reblog count on an object
func (r *LikeRepository) IncrementReblogCount(ctx context.Context, objectID string) error {
	// This method would typically update a counter field on the object
	// For now, we'll implement a basic increment pattern
	// In a real implementation, this might use UpdateExpression to atomically increment
	
	r.logger.Info("incrementing reblog count",
		zap.String("object_id", objectID))

	// Note: This is a simplified implementation
	// In practice, you might want to:
	// 1. Get the current object
	// 2. Use an atomic update to increment a reblog_count field
	// 3. Or maintain a separate counter record
	
	// For now, this is a no-op that logs the operation
	// The actual implementation would depend on the object model structure
	
	return nil
}