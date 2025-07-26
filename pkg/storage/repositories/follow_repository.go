package repositories

import (
	"context"
	"fmt"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// FollowRepository implements follow relationship operations using DynamORM
type FollowRepository struct {
	db        core.DB
	tableName string
	logger    *zap.Logger
}

// NewFollowRepository creates a new follow repository
func NewFollowRepository(db core.DB, tableName string, logger *zap.Logger) *FollowRepository {
	return &FollowRepository{
		db:        db,
		tableName: tableName,
		logger:    logger,
	}
}

// CreateFollow creates a new follow relationship
func (r *FollowRepository) CreateFollow(ctx context.Context, followerUsername, followedUsername, activityID string) error {
	follow := models.NewFollow(followerUsername, followedUsername, activityID)

	if err := r.db.WithContext(ctx).Model(follow).Create(); err != nil {
		// Check if it's a duplicate key error
		if errors.IsConditionFailed(err) {
			r.logger.Debug("follow relationship already exists",
				zap.String("follower", followerUsername),
				zap.String("followed", followedUsername))
			return nil
		}
		r.logger.Error("failed to create follow",
			zap.String("follower", followerUsername),
			zap.String("followed", followedUsername),
			zap.Error(err))
		return fmt.Errorf("failed to create follow: %w", err)
	}

	r.logger.Info("created follow relationship",
		zap.String("follower", followerUsername),
		zap.String("followed", followedUsername),
		zap.String("activity_id", activityID))

	return nil
}

// AcceptFollow updates a follow relationship to accepted state
func (r *FollowRepository) AcceptFollow(ctx context.Context, followerUsername, followedUsername string) error {
	// First, get the follow to ensure it exists
	follow, err := r.GetFollow(ctx, followerUsername, followedUsername)
	if err != nil {
		return err
	}

	// Update the state
	follow.Accept()

	// Update in database
	if err := r.db.WithContext(ctx).Model(follow).
		Where("PK", "=", follow.PK).
		Where("SK", "=", follow.SK).
		Update(); err != nil {
		r.logger.Error("failed to accept follow",
			zap.String("follower", followerUsername),
			zap.String("followed", followedUsername),
			zap.Error(err))
		return fmt.Errorf("failed to accept follow: %w", err)
	}

	r.logger.Info("accepted follow relationship",
		zap.String("follower", followerUsername),
		zap.String("followed", followedUsername))

	return nil
}

// RejectFollow updates a follow relationship to rejected state
func (r *FollowRepository) RejectFollow(ctx context.Context, followerUsername, followedUsername string) error {
	// First, get the follow to ensure it exists
	follow, err := r.GetFollow(ctx, followerUsername, followedUsername)
	if err != nil {
		return err
	}

	// Update the state
	follow.Reject()

	// Update in database
	if err := r.db.WithContext(ctx).Model(follow).
		Where("PK", "=", follow.PK).
		Where("SK", "=", follow.SK).
		Update(); err != nil {
		r.logger.Error("failed to reject follow",
			zap.String("follower", followerUsername),
			zap.String("followed", followedUsername),
			zap.Error(err))
		return fmt.Errorf("failed to reject follow: %w", err)
	}

	r.logger.Info("rejected follow relationship",
		zap.String("follower", followerUsername),
		zap.String("followed", followedUsername))

	return nil
}

// RemoveFollow deletes a follow relationship
func (r *FollowRepository) RemoveFollow(ctx context.Context, followerUsername, followedUsername string) error {
	follow := &models.Follow{
		PK: fmt.Sprintf("follow#%s", followerUsername),
		SK: fmt.Sprintf("following#%s", followedUsername),
	}

	if err := r.db.WithContext(ctx).Model(follow).
		Where("PK", "=", follow.PK).
		Where("SK", "=", follow.SK).
		Delete(); err != nil {
		if errors.IsNotFound(err) {
			r.logger.Debug("follow relationship not found",
				zap.String("follower", followerUsername),
				zap.String("followed", followedUsername))
			return nil
		}
		r.logger.Error("failed to remove follow",
			zap.String("follower", followerUsername),
			zap.String("followed", followedUsername),
			zap.Error(err))
		return fmt.Errorf("failed to remove follow: %w", err)
	}

	r.logger.Info("removed follow relationship",
		zap.String("follower", followerUsername),
		zap.String("followed", followedUsername))

	return nil
}

// GetFollow retrieves a specific follow relationship
func (r *FollowRepository) GetFollow(ctx context.Context, followerUsername, followedUsername string) (*models.Follow, error) {
	var follow models.Follow
	
	query := r.db.WithContext(ctx).Model(&follow).
		Where("PK", "=", fmt.Sprintf("follow#%s", followerUsername)).
		Where("SK", "=", fmt.Sprintf("following#%s", followedUsername))

	if err := query.First(&follow); err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("follow relationship not found")
		}
		return nil, fmt.Errorf("failed to get follow: %w", err)
	}

	return &follow, nil
}

// GetFollowers retrieves all followers for a user
func (r *FollowRepository) GetFollowers(ctx context.Context, username string, limit int, cursor string) ([]*models.Follow, string, error) {
	query := r.db.WithContext(ctx).Model(&models.Follow{}).
		Index("gsi1-index").
		Where("GSI1PK", "=", fmt.Sprintf("follow#%s", username)).
		Limit(limit)

	if cursor != "" {
		query = query.Cursor(cursor)
	}

	var follows []models.Follow
	err := query.All(&follows)
	nextCursor := "" // TODO: implement pagination
	if err != nil {
		return nil, "", fmt.Errorf("failed to scan followers: %w", err)
	}

	// Convert to pointer slice
	result := make([]*models.Follow, len(follows))
	for i := range follows {
		result[i] = &follows[i]
	}

	return result, nextCursor, nil
}

// GetFollowing retrieves all users that a user is following
func (r *FollowRepository) GetFollowing(ctx context.Context, username string, limit int, cursor string) ([]*models.Follow, string, error) {
	query := r.db.WithContext(ctx).Model(&models.Follow{}).
		Where("PK", "=", fmt.Sprintf("follow#%s", username)).
		Limit(limit)

	if cursor != "" {
		query = query.Cursor(cursor)
	}

	var follows []models.Follow
	err := query.All(&follows)
	nextCursor := "" // TODO: implement pagination
	if err != nil {
		return nil, "", fmt.Errorf("failed to scan following: %w", err)
	}

	// Convert to pointer slice
	result := make([]*models.Follow, len(follows))
	for i := range follows {
		result[i] = &follows[i]
	}

	return result, nextCursor, nil
}

// GetPendingFollows retrieves pending follow requests for a user
func (r *FollowRepository) GetPendingFollows(ctx context.Context, username string, limit int, cursor string) ([]*models.Follow, string, error) {
	query := r.db.WithContext(ctx).Model(&models.Follow{}).
		Index("gsi2-index").
		Where("GSI2PK", "=", fmt.Sprintf("follow#state#%s", models.FollowStatePending)).
		Filter("FollowedUsername", "=", username).
		Limit(limit)

	if cursor != "" {
		query = query.Cursor(cursor)
	}

	var follows []models.Follow
	err := query.All(&follows)
	nextCursor := "" // TODO: implement pagination
	if err != nil {
		return nil, "", fmt.Errorf("failed to scan pending follows: %w", err)
	}

	// Convert to pointer slice
	result := make([]*models.Follow, len(follows))
	for i := range follows {
		result[i] = &follows[i]
	}

	return result, nextCursor, nil
}

// IsFollowing checks if one user follows another
func (r *FollowRepository) IsFollowing(ctx context.Context, followerUsername, followedUsername string) (bool, error) {
	follow, err := r.GetFollow(ctx, followerUsername, followedUsername)
	if err != nil {
		if err.Error() == "follow relationship not found" {
			return false, nil
		}
		return false, err
	}

	return follow.State == models.FollowStateAccepted, nil
}