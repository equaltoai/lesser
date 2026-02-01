package repositories

import (
	"context"
	stdErrors "errors"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/cost"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/theory-cloud/tabletheory/pkg/core"
	"github.com/theory-cloud/tabletheory/pkg/errors"
	"go.uber.org/zap"
)

// MuteRepository implements mute operations using enhanced DynamORM patterns
type MuteRepository struct {
	*EnhancedBaseRepository[*models.Mute]
	// Keep direct access to fields that domain methods need
	logger *zap.Logger
	db     core.DB
}

// NewMuteRepository creates a new mute repository with enhanced functionality
func NewMuteRepository(db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService) *MuteRepository {
	// Create enhanced repository optimized for mute operations
	enhancedRepo := NewEnhancedBaseRepository[*models.Mute](db, tableName, logger, costService, "MuteRepository", "mute")

	// Set up enhanced services for mute operations
	enhancedRepo.SetValidationService(NewDefaultValidationService())
	enhancedRepo.SetPermissionService(NewDefaultPermissionService())
	enhancedRepo.SetCachingService(NewInMemoryCachingService()) // Mute status frequently checked
	enhancedRepo.SetEventService(NewDefaultEventService())      // Important for moderation notifications

	return &MuteRepository{
		EnhancedBaseRepository: enhancedRepo,
		logger:                 logger,
		db:                     db,
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

	// Use enhanced validation and creation with automatic permission checking and event emission
	if err := r.ValidateAndCreate(ctx, mute); err != nil {
		var appErr *apperrors.AppError
		// Check if it's a duplicate mute
		if stdErrors.As(err, &appErr) && appErr.Code == apperrors.CodeAlreadyExists {
			r.logger.Debug("mute relationship already exists",
				zap.String("muter", muterActor),
				zap.String("muted", mutedActor),
				zap.Bool("validation_enabled", r.HasValidation()),
				zap.Bool("events_enabled", r.HasEvents()))
			return nil // Idempotent - don't fail if mute already exists
		}
		if errors.IsConditionFailed(err) {
			r.logger.Debug("mute relationship already exists",
				zap.String("muter", muterActor),
				zap.String("muted", mutedActor),
				zap.Bool("validation_enabled", r.HasValidation()),
				zap.Bool("events_enabled", r.HasEvents()))
			return nil // Idempotent - don't fail if mute already exists
		}
		r.logger.Error("failed to create mute with enhanced validation",
			zap.String("muter", muterActor),
			zap.String("muted", mutedActor),
			zap.Bool("validation_enabled", r.HasValidation()),
			zap.Bool("events_enabled", r.HasEvents()),
			zap.Error(err))
		return ErrorHandler.HandleCreateError(err, EntityMute, fmt.Sprintf("%s muting %s", muterActor, mutedActor))
	}

	r.logger.Info("created mute relationship with enhanced patterns",
		zap.String("mute_id", fmt.Sprintf("%s:%s", muterActor, mutedActor)),
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
		var appErr *apperrors.AppError
		if stdErrors.As(err, &appErr) && appErr.Code == apperrors.CodeNotFound {
			return false, nil
		}
		if errors.IsNotFound(err) || strings.Contains(strings.ToLower(err.Error()), "not found") {
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
		IndexName:   "gsi1",                  // Use gsi1 for reverse lookup
		PKFormat:    "MUTED#%s",              // gsi1PK format
		SKField:     "gsi1SK",                // Sort key field for gsi1
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

	count, err := r.db.WithContext(ctx).Model(&models.Mute{}).
		Index("gsi1").
		Where("gsi1PK", "=", fmt.Sprintf("MUTED#%s", mutedUsername)).
		Count()
	if err != nil {
		r.logger.Error("failed to count users who muted actor",
			zap.String("muted_actor", mutedActor),
			zap.Error(err))
		return 0, ErrorHandler.HandleQueryError(err, EntityMute, fmt.Sprintf("count users who muted %s", mutedActor))
	}

	return int(count), nil
}
