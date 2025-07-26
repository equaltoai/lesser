package repositories

import (
	"context"
	"fmt"

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