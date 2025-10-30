package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// LikeRepository implements like operations using enhanced DynamORM patterns
type LikeRepository struct {
	*EnhancedBaseRepository[*models.Like]
}

// NewLikeRepository creates a new like repository with enhanced functionality
func NewLikeRepository(db core.DB, tableName string, logger *zap.Logger) *LikeRepository {
	// Create enhanced repository optimized for like operations
	enhancedRepo := NewEnhancedBaseRepository[*models.Like](db, tableName, logger, nil, "LikeRepository", "like")

	// Set up enhanced services for like operations
	enhancedRepo.SetValidationService(NewDefaultValidationService())
	enhancedRepo.SetPermissionService(NewDefaultPermissionService())
	enhancedRepo.SetCachingService(NewInMemoryCachingService()) // Likes are frequently checked
	enhancedRepo.SetEventService(NewDefaultEventService())      // Important for notifications

	return &LikeRepository{
		EnhancedBaseRepository: enhancedRepo,
	}
}

// NewLikeRepositoryWithCostTracking creates a new like repository with cost tracking
func NewLikeRepositoryWithCostTracking(db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService) *LikeRepository {
	// Create enhanced repository with cost tracking
	enhancedRepo := NewEnhancedBaseRepository[*models.Like](db, tableName, logger, costService, "LikeRepository", "like")

	// Set up enhanced services for like operations
	enhancedRepo.SetValidationService(NewDefaultValidationService())
	enhancedRepo.SetPermissionService(NewDefaultPermissionService())
	enhancedRepo.SetCachingService(NewInMemoryCachingService()) // Likes are frequently checked
	enhancedRepo.SetEventService(NewDefaultEventService())      // Important for notifications

	return &LikeRepository{
		EnhancedBaseRepository: enhancedRepo,
	}
}

// CreateLike creates a new like with enhanced validation and event emission
func (r *LikeRepository) CreateLike(ctx context.Context, actor, object, statusAuthorID string) (*models.Like, error) {
	like := models.NewLike(actor, object, statusAuthorID)

	// Use enhanced validation and creation with automatic permission checking and event emission
	if err := r.ValidateAndCreate(ctx, like); err != nil {
		// Check if it's a duplicate key error (already liked)
		if errors.IsConditionFailed(err) {
			r.logger.Debug("like already exists",
				zap.String("actor", actor),
				zap.String("object", object),
				zap.String("status_author_id", statusAuthorID),
				zap.Bool("validation_enabled", r.HasValidation()),
				zap.Bool("events_enabled", r.HasEvents()))
			return like, nil
		}
		r.logger.Error("failed to create like with enhanced validation",
			zap.String("actor", actor),
			zap.String("object", object),
			zap.String("status_author_id", statusAuthorID),
			zap.Bool("validation_enabled", r.HasValidation()),
			zap.Bool("events_enabled", r.HasEvents()),
			zap.Error(err))
		return nil, ErrorHandler.HandleCreateError(err, "like", fmt.Sprintf("%s:%s", actor, object))
	}

	r.logger.Info("created like with enhanced patterns",
		zap.String("like_id", like.ID),
		zap.String("actor", actor),
		zap.String("object", object),
		zap.String("status_author_id", statusAuthorID))

	return like, nil
}

// DeleteLike removes a like
func (r *LikeRepository) DeleteLike(ctx context.Context, actor, object string) error {
	pk := fmt.Sprintf("object#%s#likes", object)
	sk := fmt.Sprintf("actor#%s", actor)

	return DeleteEntityWithLogging(ctx, r.BaseRepository, pk, sk, "like", map[string]string{
		"actor":  actor,
		"object": object,
	})
}

// GetLike retrieves a specific like
func (r *LikeRepository) GetLike(ctx context.Context, actor, object string) (*models.Like, error) {
	pk := fmt.Sprintf("object#%s#likes", object)
	sk := fmt.Sprintf("actor#%s", actor)

	like := &models.Like{}
	if err := r.Get(ctx, pk, sk, like); err != nil {
		if err.Error() == fmt.Sprintf("item not found: pk=%s, sk=%s", pk, sk) {
			return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, "like", fmt.Sprintf("%s:%s", actor, object))
		}
		return nil, ErrorHandler.HandleGetError(err, "like", fmt.Sprintf("%s:%s", actor, object))
	}

	return like, nil
}

// GetObjectLikes retrieves all likes for an object
func (r *LikeRepository) GetObjectLikes(ctx context.Context, objectID string, limit int, cursor string) ([]*models.Like, string, error) {
	pk := fmt.Sprintf("object#%s#likes", objectID)

	opts := BasePaginationOptions{
		Limit:  limit,
		Cursor: cursor,
		Order:  "ASC",
	}

	result, err := r.FindWithPagination(ctx, pk, opts)
	if err != nil {
		return nil, "", ErrorHandler.HandleQueryError(err, "status like", fmt.Sprintf("object %s", objectID))
	}

	// Convert to pointer slice
	likePtrs := make([]*models.Like, len(result.Items))
	copy(likePtrs, result.Items)

	return likePtrs, result.NextCursor, nil
}

// GetActorLikes retrieves all likes by an actor
func (r *LikeRepository) GetActorLikes(ctx context.Context, actorID string, limit int, cursor string) ([]*models.Like, string, error) {
	config := CollectionQueryConfig{
		IndexName:   "gsi1-index",
		LogName:     "actor likes",
		ErrorPrefix: "get actor likes",
		GSIConfig: &GSIQueryConfig{
			PKField:   "GSI1PK",
			SKField:   "GSI1SK",
			PKValue:   "actor#%s#likes",
			UseCursor: true,
			OrderBy:   "ASC",
		},
	}

	converter := func(likes []*models.Like) ([]*models.Like, error) {
		return likes, nil
	}

	return QueryCollectionWithConversion(ctx, r.BaseRepository, config, actorID, limit, cursor, converter)
}

// CountActorLikes returns the total number of likes by an actor
func (r *LikeRepository) CountActorLikes(ctx context.Context, actorID string) (int64, error) {
	count, err := r.db.WithContext(ctx).Model(&models.Like{}).
		Index("gsi1-index").
		Where("gsI1PK", "=", fmt.Sprintf("actor#%s#likes", actorID)).
		Count()
	if err != nil {
		return 0, ErrorHandler.HandleQueryError(err, "like", fmt.Sprintf("actor %s count", actorID))
	}
	return count, nil
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
		likes, err := r.Query(ctx, pk, 100)
		if err != nil {
			return ErrorHandler.HandleQueryError(err, "like", fmt.Sprintf("cascade delete object %s", objectID))
		}

		// If no likes found, we're done
		if err := common.ValidateSliceNotEmpty("likes", likes); err != nil {
			break
		}

		// Prepare keys for batch deletion
		keys := make([]struct{ PK, SK string }, len(likes))
		for i, like := range likes {
			keys[i] = struct{ PK, SK string }{PK: like.GetPK(), SK: like.GetSK()}
		}

		// Use batch delete
		if err := r.BatchDelete(ctx, keys); err != nil {
			r.logger.Warn("failed to batch delete likes during cascade",
				zap.String("object_id", objectID),
				zap.Int("count", len(keys)),
				zap.Error(err))
			// Continue with individual deletes as fallback
			for _, key := range keys {
				if delErr := r.Delete(ctx, key.PK, key.SK); delErr != nil {
					r.logger.Warn("failed to delete like during cascade",
						zap.String("pk", key.PK),
						zap.String("sk", key.SK),
						zap.Error(delErr))
				}
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

	// Use direct DynamORM call since we need a different model type
	if err := r.db.WithContext(ctx).Model(tombstone).Create(); err != nil {
		r.logger.Error("failed to create tombstone",
			zap.String("object_id", objectID),
			zap.String("deleted_by", deletedBy),
			zap.Error(err))
		return ErrorHandler.HandleCreateError(err, "tombstone", objectID)
	}

	r.logger.Info("created tombstone",
		zap.String("object_id", objectID),
		zap.String("deleted_by", deletedBy))

	return nil
}

// GetTombstone retrieves a tombstone by object ID
func (r *LikeRepository) GetTombstone(ctx context.Context, objectID string) (*storage.Tombstone, error) {
	var tombstone models.Tombstone

	// Use direct DynamORM call since we need a different model type
	query := r.db.WithContext(ctx).Model(&tombstone).
		Where("PK", "=", fmt.Sprintf("OBJECT#%s", objectID)).
		Where("SK", "=", "TOMBSTONE")

	if err := query.First(&tombstone); err != nil {
		if errors.IsNotFound(err) {
			return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, "tombstone", objectID)
		}
		r.logger.Error("failed to get tombstone",
			zap.String("object_id", objectID),
			zap.Error(err))
		return nil, ErrorHandler.HandleGetError(err, "tombstone", objectID)
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
	// Query likes with PK pattern: object#{statusID}#likes (note: different case from other methods)
	pk := fmt.Sprintf("object#%s#likes", statusID)

	count, err := r.Count(ctx, pk)
	if err != nil {
		r.logger.Error("failed to count likes",
			zap.String("status_id", statusID),
			zap.Error(err))
		return 0, ErrorHandler.HandleQueryError(err, "like", fmt.Sprintf("status %s count", statusID))
	}

	return int64(count), nil
}

// GetBoostCount counts boosts/announces for a status
func (r *LikeRepository) GetBoostCount(ctx context.Context, statusID string) (int64, error) {
	// Query announces with PK pattern: OBJECT#{statusID}#ANNOUNCES
	pk := fmt.Sprintf("OBJECT#%s#ANNOUNCES", statusID)

	// Use direct DynamORM call since we're working with Announce model, not Like
	count, err := r.db.WithContext(ctx).Model(&models.Announce{}).
		Where("PK", "=", pk).
		Count()

	if err != nil {
		r.logger.Error("failed to count boosts",
			zap.String("status_id", statusID),
			zap.Error(err))
		return 0, ErrorHandler.HandleQueryError(err, "boost", fmt.Sprintf("status %s count", statusID))
	}

	return count, nil
}

// IncrementReblogCount increments the reblog count on a status
func (r *LikeRepository) IncrementReblogCount(ctx context.Context, objectID string) error {
	// Use DynamORM's atomic increment functionality
	pk := fmt.Sprintf("status#%s", objectID)
	sk := fmt.Sprintf("status#%s", objectID)

	// Create a partial status with just the keys and the field to increment
	statusUpdate := &models.Status{
		PK: pk,
		SK: sk,
	}

	// Use DynamORM's increment functionality for atomic updates (working with Status model, not Like)
	err := r.db.WithContext(ctx).Model(statusUpdate).
		Update("ReblogCount", "ReblogCount + 1")

	if err != nil {
		r.logger.Error("failed to increment reblog count",
			zap.String("object_id", objectID),
			zap.Error(err))
		return ErrorHandler.HandleUpdateError(err, "reblog count", objectID)
	}

	r.logger.Info("incremented reblog count",
		zap.String("object_id", objectID))

	return nil
}

// HasReblogged checks if a user has reblogged/boosted a specific status
func (r *LikeRepository) HasReblogged(ctx context.Context, actorID, statusID string) (bool, error) {
	// Query for an announce with the specific pattern
	pk := fmt.Sprintf("OBJECT#%s#ANNOUNCES", statusID)
	sk := fmt.Sprintf("ACTOR#%s", actorID)

	// Use direct DynamORM call since we're working with Announce model, not Like
	var announces []models.Announce
	err := r.db.WithContext(ctx).Model(&models.Announce{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		Limit(1).
		All(&announces)

	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil // Not found = not reblogged
		}
		r.logger.Error("failed to check if reblogged",
			zap.String("actor_id", actorID),
			zap.String("status_id", statusID),
			zap.Error(err))
		return false, ErrorHandler.HandleQueryError(err, "reblog status", fmt.Sprintf("actor %s status %s", actorID, statusID))
	}

	return len(announces) > 0, nil
}

// Storage interface compatibility methods - these delegate to the main methods above

// CountForObject provides Storage interface compatibility for CountObjectLikes
func (r *LikeRepository) CountForObject(ctx context.Context, objectID string) (int64, error) {
	return r.GetLikeCount(ctx, objectID)
}

// GetForObject provides Storage interface compatibility for GetObjectLikes
func (r *LikeRepository) GetForObject(ctx context.Context, objectID string, limit int, cursor string) ([]*models.Like, string, error) {
	return r.GetObjectLikes(ctx, objectID, limit, cursor)
}

// GetLikedObjects provides Storage interface compatibility (already exists as GetActorLikes with different name)
func (r *LikeRepository) GetLikedObjects(ctx context.Context, actorID string, limit int, cursor string) ([]*models.Like, string, error) {
	return r.GetActorLikes(ctx, actorID, limit, cursor)
}
