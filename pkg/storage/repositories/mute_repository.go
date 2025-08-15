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

// MuteRepository implements mute operations using DynamORM
type MuteRepository struct {
	db        core.DB
	tableName string
	logger    *zap.Logger
}

// NewMuteRepository creates a new mute repository
func NewMuteRepository(db core.DB, tableName string, logger *zap.Logger) *MuteRepository {
	return &MuteRepository{
		db:        db,
		tableName: tableName,
		logger:    logger,
	}
}

// CreateMute creates a new mute relationship
func (r *MuteRepository) CreateMute(ctx context.Context, muterActor, mutedActor, activityID string, hideNotifications bool, _ *time.Duration) error {
	mute := &models.Mute{
		Actor:             muterActor,
		Object:            mutedActor,
		ID:                activityID,
		HideNotifications: hideNotifications,
		Published:         time.Now().UTC(),
		CreatedAt:         time.Now().UTC(),
	}

	// BeforeCreate will set keys and other fields
	if err := mute.BeforeCreate(); err != nil {
		r.logger.Error("failed to prepare mute for creation",
			zap.String("muter", muterActor),
			zap.String("muted", mutedActor),
			zap.Error(err))
		return fmt.Errorf("failed to prepare mute: %w", err)
	}

	if err := r.db.WithContext(ctx).Model(mute).Create(); err != nil {
		// Check if it's a duplicate mute
		if errors.IsConditionFailed(err) {
			r.logger.Debug("mute relationship already exists",
				zap.String("muter", muterActor),
				zap.String("muted", mutedActor))
			return nil // Idempotent - don't fail if mute already exists
		}
		r.logger.Error("failed to create mute",
			zap.String("muter", muterActor),
			zap.String("muted", mutedActor),
			zap.Error(err))
		return fmt.Errorf("failed to create mute: %w", err)
	}

	r.logger.Info("created mute relationship",
		zap.String("muter", muterActor),
		zap.String("muted", mutedActor),
		zap.String("activity_id", activityID))

	return nil
}

// DeleteMute removes a mute relationship (for Undo Mute)
func (r *MuteRepository) DeleteMute(ctx context.Context, muterActor, mutedActor string) error {
	// Extract usernames for key generation
	muterUsername := extractUsernameFromActor(muterActor)
	mutedUsername := extractUsernameFromActor(mutedActor)

	mute := &models.Mute{
		PK: fmt.Sprintf("MUTE#%s", muterUsername),
		SK: fmt.Sprintf("MUTED#%s", mutedUsername),
	}

	if err := r.db.WithContext(ctx).Model(mute).
		Where("PK", "=", mute.PK).
		Where("SK", "=", mute.SK).
		Delete(); err != nil {
		if errors.IsNotFound(err) {
			r.logger.Debug("mute not found for deletion",
				zap.String("muter", muterActor),
				zap.String("muted", mutedActor))
			return nil // Idempotent - don't fail if mute doesn't exist
		}
		r.logger.Error("failed to delete mute",
			zap.String("muter", muterActor),
			zap.String("muted", mutedActor),
			zap.Error(err))
		return fmt.Errorf("failed to delete mute: %w", err)
	}

	r.logger.Info("deleted mute relationship",
		zap.String("muter", muterActor),
		zap.String("muted", mutedActor))

	return nil
}

// IsMuted checks if one actor has muted another
func (r *MuteRepository) IsMuted(ctx context.Context, muterActor, mutedActor string) (bool, error) {
	// Extract usernames for key generation
	muterUsername := extractUsernameFromActor(muterActor)
	mutedUsername := extractUsernameFromActor(mutedActor)

	var mute models.Mute

	err := r.db.WithContext(ctx).Model(&mute).
		Where("PK", "=", fmt.Sprintf("MUTE#%s", muterUsername)).
		Where("SK", "=", fmt.Sprintf("MUTED#%s", mutedUsername)).
		First(&mute)

	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		r.logger.Error("failed to check mute status",
			zap.String("muter", muterActor),
			zap.String("muted", mutedActor),
			zap.Error(err))
		return false, fmt.Errorf("failed to check mute status: %w", err)
	}

	return true, nil
}

// GetMutedUsers returns a list of users muted by the given actor
func (r *MuteRepository) GetMutedUsers(ctx context.Context, muterActor string, limit int, cursor string) ([]string, string, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	muterUsername := extractUsernameFromActor(muterActor)

	query := r.db.WithContext(ctx).Model(&models.Mute{}).
		Where("PK", "=", fmt.Sprintf("MUTE#%s", muterUsername)).
		Limit(limit)

	if cursor != "" {
		query = query.Cursor(cursor)
	}

	// Get one more item than requested to determine if there are more results
	query = query.Limit(limit + 1)

	var mutes []models.Mute
	err := query.All(&mutes)
	if err != nil {
		r.logger.Error("failed to get muted users",
			zap.String("muter", muterActor),
			zap.Error(err))
		return nil, "", fmt.Errorf("failed to get muted users: %w", err)
	}

	// Generate next cursor
	var nextCursor string
	if len(mutes) > limit {
		// We got more results than requested, so there are more pages
		nextCursor = mutes[limit-1].SK
		mutes = mutes[:limit] // Trim to requested limit
	}

	// Extract muted actor IDs
	mutedUsers := make([]string, len(mutes))
	for i, mute := range mutes {
		mutedUsers[i] = mute.Object
	}

	return mutedUsers, nextCursor, nil
}

// GetUsersWhoMuted returns a list of users who have muted the given actor
func (r *MuteRepository) GetUsersWhoMuted(ctx context.Context, mutedActor string, limit int, cursor string) ([]string, string, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	mutedUsername := extractUsernameFromActor(mutedActor)

	query := r.db.WithContext(ctx).Model(&models.Mute{}).
		Index("GSI1").
		Where("GSI1PK", "=", fmt.Sprintf("MUTED#%s", mutedUsername)).
		Limit(limit)

	if cursor != "" {
		query = query.Cursor(cursor)
	}

	// Get one more item than requested to determine if there are more results
	query = query.Limit(limit + 1)

	var mutes []models.Mute
	err := query.All(&mutes)
	if err != nil {
		r.logger.Error("failed to get users who muted actor",
			zap.String("muted_actor", mutedActor),
			zap.Error(err))
		return nil, "", fmt.Errorf("failed to get users who muted actor: %w", err)
	}

	// Generate next cursor
	var nextCursor string
	if len(mutes) > limit {
		// We got more results than requested, so there are more pages
		nextCursor = mutes[limit-1].GSI1SK
		mutes = mutes[:limit] // Trim to requested limit
	}

	// Extract muter actor IDs
	muters := make([]string, len(mutes))
	for i, mute := range mutes {
		muters[i] = mute.Actor
	}

	return muters, nextCursor, nil
}

// GetMute retrieves a specific mute relationship
func (r *MuteRepository) GetMute(ctx context.Context, muterActor, mutedActor string) (*storage.Mute, error) {
	// Extract usernames for key generation
	muterUsername := extractUsernameFromActor(muterActor)
	mutedUsername := extractUsernameFromActor(mutedActor)

	var mute models.Mute

	err := r.db.WithContext(ctx).Model(&mute).
		Where("PK", "=", fmt.Sprintf("MUTE#%s", muterUsername)).
		Where("SK", "=", fmt.Sprintf("MUTED#%s", mutedUsername)).
		First(&mute)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("mute not found")
		}
		r.logger.Error("failed to get mute",
			zap.String("muter", muterActor),
			zap.String("muted", mutedActor),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get mute: %w", err)
	}

	// Convert to storage.Mute
	return &storage.Mute{
		Actor:             mute.Actor,
		Object:            mute.Object,
		ID:                mute.ID,
		HideNotifications: mute.HideNotifications,
		Published:         mute.Published,
		CreatedAt:         mute.CreatedAt,
	}, nil
}

// CountMutedUsers returns the number of users muted by the given actor
func (r *MuteRepository) CountMutedUsers(ctx context.Context, muterActor string) (int, error) {
	muterUsername := extractUsernameFromActor(muterActor)

	count, err := r.db.WithContext(ctx).Model(&models.Mute{}).
		Where("PK", "=", fmt.Sprintf("MUTE#%s", muterUsername)).
		Count()

	if err != nil {
		r.logger.Error("failed to count muted users",
			zap.String("muter", muterActor),
			zap.Error(err))
		return 0, fmt.Errorf("failed to count muted users: %w", err)
	}

	return int(count), nil
}

// CountUsersWhoMuted returns the number of users who have muted the given actor
func (r *MuteRepository) CountUsersWhoMuted(ctx context.Context, mutedActor string) (int, error) {
	mutedUsername := extractUsernameFromActor(mutedActor)

	count, err := r.db.WithContext(ctx).Model(&models.Mute{}).
		Index("GSI1").
		Where("GSI1PK", "=", fmt.Sprintf("MUTED#%s", mutedUsername)).
		Count()

	if err != nil {
		r.logger.Error("failed to count users who muted actor",
			zap.String("muted_actor", mutedActor),
			zap.Error(err))
		return 0, fmt.Errorf("failed to count users who muted actor: %w", err)
	}

	return int(count), nil
}

