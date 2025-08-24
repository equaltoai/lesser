// Package repositories provides repository implementations for data access layer
package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/google/uuid"
	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"
)

// MediaRepositoryInterface defines the interface for media operations
type MediaRepositoryInterface interface {
	GetMedia(ctx context.Context, mediaID string) (*models.Media, error)
}

// Constants for media types to avoid repetition
const (
	MediaTypeImage   = "image"
	MediaTypeVideo   = "video"
	MediaTypeAudio   = "audio"
	MediaTypeGifv    = "gifv"
	MediaTypeUnknown = "unknown"
)

// Helper function to convert from models.ScheduledStatus to storage.ScheduledStatus
func (r *ScheduledStatusRepository) modelToStorage(model *models.ScheduledStatus) *storage.ScheduledStatus {
	return &storage.ScheduledStatus{
		ID:            model.ID,
		Username:      model.Username,
		Status:        model.Status,
		MediaIDs:      model.MediaIDs,
		Sensitive:     model.Sensitive,
		SpoilerText:   model.SpoilerText,
		Visibility:    model.Visibility,
		Language:      model.Language,
		InReplyToID:   model.InReplyToID,
		Poll:          model.Poll,
		ScheduledAt:   model.ScheduledAt,
		CreatedAt:     model.CreatedAt,
		UpdatedAt:     model.UpdatedAt,
		Published:     model.Published,
		PublishedAt:   model.PublishedAt,
		ApplicationID: model.ApplicationID,
	}
}

// Helper function to convert from storage.ScheduledStatus to models.ScheduledStatus
func (r *ScheduledStatusRepository) storageToModel(scheduled *storage.ScheduledStatus) *models.ScheduledStatus {
	model := &models.ScheduledStatus{
		ID:            scheduled.ID,
		Username:      scheduled.Username,
		Status:        scheduled.Status,
		MediaIDs:      scheduled.MediaIDs,
		Sensitive:     scheduled.Sensitive,
		SpoilerText:   scheduled.SpoilerText,
		Visibility:    scheduled.Visibility,
		Language:      scheduled.Language,
		InReplyToID:   scheduled.InReplyToID,
		Poll:          scheduled.Poll,
		ScheduledAt:   scheduled.ScheduledAt,
		CreatedAt:     scheduled.CreatedAt,
		UpdatedAt:     scheduled.UpdatedAt,
		Published:     scheduled.Published,
		PublishedAt:   scheduled.PublishedAt,
		ApplicationID: scheduled.ApplicationID,
	}
	_ = model.UpdateKeys() // Ignore error as this is internal model operation
	return model
}

// ScheduledStatusRepository handles scheduled status operations using enhanced DynamORM patterns
type ScheduledStatusRepository struct {
	*EnhancedBaseRepository[*models.ScheduledStatus]
	mediaRepo MediaRepositoryInterface // Add media repository dependency
}

// NewScheduledStatusRepository creates a new scheduled status repository with enhanced functionality and cost tracking
func NewScheduledStatusRepository(db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService) *ScheduledStatusRepository {
	// Create enhanced repository for scheduled status operations
	enhancedRepo := NewEnhancedBaseRepository[*models.ScheduledStatus](db, tableName, logger, costService, "ScheduledStatusRepository", "scheduled_status")

	// Set up enhanced services for scheduled status operations
	enhancedRepo.SetValidationService(NewDefaultValidationService())
	enhancedRepo.SetPermissionService(NewDefaultPermissionService())
	enhancedRepo.SetCachingService(NewInMemoryCachingService()) // Cache scheduled statuses
	enhancedRepo.SetEventService(NewDefaultEventService())

	return &ScheduledStatusRepository{
		EnhancedBaseRepository: enhancedRepo,
	}
}

// SetMediaRepository sets the media repository dependency
func (r *ScheduledStatusRepository) SetMediaRepository(mediaRepo MediaRepositoryInterface) {
	r.mediaRepo = mediaRepo
}

// CreateScheduledStatus creates a new scheduled status
func (r *ScheduledStatusRepository) CreateScheduledStatus(ctx context.Context, scheduled *storage.ScheduledStatus) error {
	r.logger.Info("creating scheduled status",
		zap.String("username", scheduled.Username),
		zap.Time("scheduled_at", scheduled.ScheduledAt))

	// Generate ID if not provided
	if err := common.ValidateRequiredParam("scheduled_id", scheduled.ID); err != nil {
		scheduled.ID = uuid.New().String()
	}

	// Set timestamps
	if scheduled.CreatedAt.IsZero() {
		scheduled.CreatedAt = time.Now()
	}
	if scheduled.UpdatedAt.IsZero() {
		scheduled.UpdatedAt = scheduled.CreatedAt
	}

	// Create DynamORM model
	scheduledModel := r.storageToModel(scheduled)

	// Create the scheduled status using enhanced validation and creation
	err := r.ValidateAndCreate(ctx, scheduledModel)
	if err != nil {
		return ErrorHandler.HandleCreateError(err, EntityScheduledStatus, scheduled.ID)
	}

	// Update the original with any generated values
	scheduled.ID = scheduledModel.ID
	scheduled.CreatedAt = scheduledModel.CreatedAt
	scheduled.UpdatedAt = scheduledModel.UpdatedAt

	return nil
}

// GetScheduledStatus retrieves a scheduled status by ID
func (r *ScheduledStatusRepository) GetScheduledStatus(ctx context.Context, id string) (*storage.ScheduledStatus, error) {
	// Since we don't know the username, we need to do a scan with a filter
	// This is less efficient than the legacy implementation but follows DynamORM patterns
	var scheduledModels []*models.ScheduledStatus

	err := r.db.WithContext(ctx).Model(&models.ScheduledStatus{}).
		Where("SK", "=", fmt.Sprintf("ID#%s", id)).
		All(&scheduledModels)
	if err != nil {
		return nil, ErrorHandler.HandleGetError(err, EntityScheduledStatus, id)
	}

	if err := common.ValidateSliceNotEmpty("scheduled_models", scheduledModels); err != nil {
		return nil, ErrorHandler.HandleGetError(err, EntityScheduledStatus, id)
	}

	return r.modelToStorage(scheduledModels[0]), nil
}

// GetScheduledStatuses retrieves scheduled statuses for a user
func (r *ScheduledStatusRepository) GetScheduledStatuses(ctx context.Context, username string, limit int, cursor string) ([]*storage.ScheduledStatus, string, error) {
	pk := fmt.Sprintf(storage.UserScheduledKey, username)
	query := r.db.WithContext(ctx).Model(&models.ScheduledStatus{}).
		Where("PK", "=", pk).
		OrderBy("SK", "ASC") // Ordered by ID

	// Handle cursor-based pagination
	if cursor != "" {
		query = query.Where("SK", ">", cursor)
	}

	// Get one more item than requested to determine if there are more results
	query = query.Limit(limit + 1)

	var scheduledModels []*models.ScheduledStatus
	err := query.All(&scheduledModels)
	if err != nil {
		return nil, "", ErrorHandler.HandleQueryError(err, EntityScheduledStatus, "user scheduling")
	}

	// Filter out published statuses and convert to storage models
	statuses := make([]*storage.ScheduledStatus, 0, len(scheduledModels))
	for _, model := range scheduledModels {
		if !model.Published {
			statuses = append(statuses, r.modelToStorage(model))
		}
	}

	// Generate next cursor
	var nextCursor string
	if err := common.ValidateSliceLength("statuses", statuses, limit); err != nil {
		// We got more results than requested, so there are more pages
		nextCursor = scheduledModels[limit-1].SK
		statuses = statuses[:limit] // Trim to requested limit
	}

	return statuses, nextCursor, nil
}

// UpdateScheduledStatus updates a scheduled status
func (r *ScheduledStatusRepository) UpdateScheduledStatus(ctx context.Context, scheduled *storage.ScheduledStatus) error {
	// First get the existing status to ensure it exists
	existing, err := r.GetScheduledStatus(ctx, scheduled.ID)
	if err != nil {
		return err
	}

	// Preserve username if not set
	if err := common.ValidateRequiredParam("scheduled_username", scheduled.Username); err != nil {
		scheduled.Username = existing.Username
	}

	// Update timestamp
	scheduled.UpdatedAt = time.Now()

	// Create updated model
	scheduledModel := r.storageToModel(scheduled)

	// Use BaseRepository update
	err = r.Update(ctx, scheduledModel)
	if err != nil {
		return ErrorHandler.HandleUpdateError(err, EntityScheduledStatus, scheduled.ID)
	}

	return nil
}

// DeleteScheduledStatus deletes a scheduled status
func (r *ScheduledStatusRepository) DeleteScheduledStatus(ctx context.Context, id string) error {
	// First get the status to find the username
	status, err := r.GetScheduledStatus(ctx, id)
	if err != nil {
		return err
	}

	// Use BaseRepository delete with keys
	pk := fmt.Sprintf("USER#%s#SCHEDULED", status.Username)
	sk := fmt.Sprintf("ID#%s", id)

	err = r.Delete(ctx, pk, sk)
	if err != nil {
		return ErrorHandler.HandleDeleteError(err, EntityScheduledStatus, id)
	}

	return nil
}

// GetDueScheduledStatuses retrieves scheduled statuses that are due to be published
func (r *ScheduledStatusRepository) GetDueScheduledStatuses(ctx context.Context, before time.Time, limit int) ([]*storage.ScheduledStatus, error) {
	gsi1sk := fmt.Sprintf("TIME#%s", before.Format(time.RFC3339Nano))

	query := r.db.WithContext(ctx).Model(&models.ScheduledStatus{}).
		Index("gsi1").
		Where("GSI1PK", "=", "SCHEDULED#DUE").
		Where("GSI1SK", "<", gsi1sk).
		OrderBy("GSI1SK", "ASC").
		Limit(limit)

	var scheduledModels []*models.ScheduledStatus
	err := query.All(&scheduledModels)
	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, EntityScheduledStatus, "due scheduling")
	}

	// Filter out published statuses and convert to storage models
	statuses := make([]*storage.ScheduledStatus, 0, len(scheduledModels))
	for _, model := range scheduledModels {
		if !model.Published {
			statuses = append(statuses, r.modelToStorage(model))
		}
	}

	return statuses, nil
}

// MarkScheduledStatusPublished marks a scheduled status as published
func (r *ScheduledStatusRepository) MarkScheduledStatusPublished(ctx context.Context, id string) error {
	// First get the status to find the username
	status, err := r.GetScheduledStatus(ctx, id)
	if err != nil {
		return err
	}

	// Update the status to mark as published
	now := time.Now()
	status.Published = true
	status.PublishedAt = &now
	status.UpdatedAt = now

	// Create model for update
	scheduledModel := r.storageToModel(status)

	err = r.Update(ctx, scheduledModel)
	if err != nil {
		return ErrorHandler.HandleUpdateError(err, EntityScheduledStatus, id)
	}

	return nil
}

// GetScheduledStatusMedia gets media for scheduled status
func (r *ScheduledStatusRepository) GetScheduledStatusMedia(ctx context.Context, id string) ([]*models.Media, error) {
	// Get the scheduled status first to access its media IDs
	scheduled, err := r.GetScheduledStatus(ctx, id)
	if err != nil {
		return nil, err
	}

	// If no media IDs, return empty slice
	if err := common.ValidateSliceNotEmpty("media_ids", scheduled.MediaIDs); err != nil {
		return []*models.Media{}, nil
	}

	r.logger.Debug("retrieving media attachments for scheduled status",
		zap.String("scheduled_status_id", id),
		zap.Strings("media_ids", scheduled.MediaIDs),
		zap.Int("media_count", len(scheduled.MediaIDs)))

	// Check if media repository is available
	if r.mediaRepo == nil {
		r.logger.Warn("media repository not available, returning empty media list",
			zap.String("scheduled_status_id", id))
		return []*models.Media{}, nil
	}

	// Retrieve media attachments by querying each media ID
	mediaAttachments := make([]*models.Media, 0, len(scheduled.MediaIDs))
	retrievedCount := 0
	errorCount := 0

	for order, mediaID := range scheduled.MediaIDs {
		// Attempt to get media by ID
		media, err := r.mediaRepo.GetMedia(ctx, mediaID)
		if err != nil {
			r.logger.Warn("failed to retrieve media attachment",
				zap.String("scheduled_status_id", id),
				zap.String("media_id", mediaID),
				zap.Int("order", order),
				zap.Error(err))
			errorCount++
			continue // Continue with other media rather than failing entirely
		}

		// Add media directly to attachments
		mediaAttachments = append(mediaAttachments, media)
		retrievedCount++

		r.logger.Debug("successfully retrieved media attachment",
			zap.String("media_id", mediaID),
			zap.String("content_type", media.ContentType),
			zap.Int("order", order))
	}

	// Log summary of media retrieval
	r.logger.Info("completed media retrieval for scheduled status",
		zap.String("scheduled_status_id", id),
		zap.Int("requested_count", len(scheduled.MediaIDs)),
		zap.Int("retrieved_count", retrievedCount),
		zap.Int("error_count", errorCount))

	return mediaAttachments, nil
}

// convertToMediaAttachment converts a Media model to Mastodon API compatible attachment

// convertVariants converts media variants to API format

// calculateAspect calculates aspect ratio for media

// parseFocus parses focus point string into API format
