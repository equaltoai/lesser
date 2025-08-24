package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// MuteRepository implements mute operations using BaseRepository and DynamORM
type MuteRepository struct {
	*BaseRepository[*models.Mute]
	// Keep direct access to fields that domain methods need
	logger *zap.Logger
	db     core.DB
}

// NewMuteRepository creates a new mute repository with cost tracking
func NewMuteRepository(db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService) *MuteRepository {
	return &MuteRepository{
		BaseRepository: NewBaseRepositoryWithCostTracking[*models.Mute](db, tableName, logger, costService, "mute"),
		logger:         logger,
		db:             db,
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
		return ErrorHandler.HandleCreateError(err, EntityMute, fmt.Sprintf("%s muting %s", muterActor, mutedActor))
	}

	// Use BaseRepository Create method
	if err := r.Create(ctx, mute); err != nil {
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
		return ErrorHandler.HandleCreateError(err, EntityMute, fmt.Sprintf("%s muting %s", muterActor, mutedActor))
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

	pk := fmt.Sprintf("MUTE#%s", muterUsername)
	sk := fmt.Sprintf("MUTED#%s", mutedUsername)

	err := r.Delete(ctx, pk, sk)
	if err != nil {
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
		return ErrorHandler.HandleDeleteError(err, EntityMute, fmt.Sprintf("%s unmuting %s", muterActor, mutedActor))
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

	pk := fmt.Sprintf("MUTE#%s", muterUsername)
	sk := fmt.Sprintf("MUTED#%s", mutedUsername)

	var mute models.Mute
	err := r.Get(ctx, pk, sk, &mute)
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		r.logger.Error("failed to check mute status",
			zap.String("muter", muterActor),
			zap.String("muted", mutedActor),
			zap.Error(err))
		return false, ErrorHandler.HandleQueryError(err, EntityMute, fmt.Sprintf("mute check %s->%s", muterActor, mutedActor))
	}

	return true, nil
}

// GetMutedUsers returns a list of users muted by the given actor
func (r *MuteRepository) GetMutedUsers(ctx context.Context, muterActor string, limit int, cursor string) ([]string, string, error) {
	muterUsername := extractUsernameFromActor(muterActor)

	config := RelationshipPaginationConfig{
		IndexName:   "",            // Use main table
		PKFormat:    "MUTE#%s",     // PK format
		SKField:     "SK",          // Sort key field
		ActorField:  "Object",      // Extract muted users (Object field)
		ErrorPrefix: "muted users", // Error message prefix
	}

	return getPaginatedMuteList(ctx, r.db, r.logger, muterUsername, limit, cursor, config)
}

// GetUsersWhoMuted returns a list of users who have muted the given actor
func (r *MuteRepository) GetUsersWhoMuted(ctx context.Context, mutedActor string, limit int, cursor string) ([]string, string, error) {
	mutedUsername := extractUsernameFromActor(mutedActor)

	config := RelationshipPaginationConfig{
		IndexName:   "GSI1",                  // Use GSI1 for reverse lookup
		PKFormat:    "MUTED#%s",              // GSI1PK format
		SKField:     "GSI1SK",                // Sort key field for GSI1
		ActorField:  "Actor",                 // Extract muter users (Actor field)
		ErrorPrefix: "users who muted actor", // Error message prefix
	}

	return getPaginatedMuteList(ctx, r.db, r.logger, mutedUsername, limit, cursor, config)
}

// GetMute retrieves a specific mute relationship
func (r *MuteRepository) GetMute(ctx context.Context, muterActor, mutedActor string) (*storage.Mute, error) {
	// Extract usernames for key generation
	muterUsername := extractUsernameFromActor(muterActor)
	mutedUsername := extractUsernameFromActor(mutedActor)

	pk := fmt.Sprintf("MUTE#%s", muterUsername)
	sk := fmt.Sprintf("MUTED#%s", mutedUsername)

	var mute models.Mute
	err := r.Get(ctx, pk, sk, &mute)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, ErrorHandler.HandleGetError(err, EntityMute, fmt.Sprintf("%s->%s", muterActor, mutedActor))
		}
		r.logger.Error("failed to get mute",
			zap.String("muter", muterActor),
			zap.String("muted", mutedActor),
			zap.Error(err))
		return nil, ErrorHandler.HandleGetError(err, EntityMute, fmt.Sprintf("%s->%s", muterActor, mutedActor))
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
	pk := fmt.Sprintf("MUTE#%s", muterUsername)

	count, err := r.Count(ctx, pk)
	if err != nil {
		r.logger.Error("failed to count muted users",
			zap.String("muter", muterActor),
			zap.Error(err))
		return 0, ErrorHandler.HandleQueryError(err, EntityMute, fmt.Sprintf("count muted users for %s", muterActor))
	}

	return count, nil
}

// CountUsersWhoMuted returns the number of users who have muted the given actor
func (r *MuteRepository) CountUsersWhoMuted(ctx context.Context, mutedActor string) (int, error) {
	mutedUsername := extractUsernameFromActor(mutedActor)

	// Use QueryGSI from BaseRepository to count on GSI1
	mutes, err := r.QueryGSI(ctx, "GSI1", fmt.Sprintf("MUTED#%s", mutedUsername), 0)
	if err != nil {
		r.logger.Error("failed to count users who muted actor",
			zap.String("muted_actor", mutedActor),
			zap.Error(err))
		return 0, ErrorHandler.HandleQueryError(err, EntityMute, fmt.Sprintf("count users who muted %s", mutedActor))
	}

	return len(mutes), nil
}
