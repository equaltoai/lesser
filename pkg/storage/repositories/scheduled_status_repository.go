// Package repositories provides repository implementations for data access layer
package repositories

import (
	"context"
	"fmt"
	"strings"
	"time"

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
	model.UpdateKeys()
	return model
}

// ScheduledStatusRepository handles scheduled status operations using DynamORM
type ScheduledStatusRepository struct {
	db        core.DB
	tableName string
	logger    *zap.Logger
	mediaRepo MediaRepositoryInterface // Add media repository dependency
}

// NewScheduledStatusRepository creates a new scheduled status repository
func NewScheduledStatusRepository(db core.DB, tableName string, logger *zap.Logger) *ScheduledStatusRepository {
	return &ScheduledStatusRepository{
		db:        db,
		tableName: tableName,
		logger:    logger,
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
	if scheduled.ID == "" {
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

	// Create the scheduled status using DynamORM
	err := r.db.WithContext(ctx).Model(scheduledModel).Create()
	if err != nil {
		return fmt.Errorf("failed to create scheduled status: %w", err)
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
	var scheduledModels []models.ScheduledStatus

	err := r.db.WithContext(ctx).Model(&models.ScheduledStatus{}).
		Where("SK", "=", fmt.Sprintf("ID#%s", id)).
		All(&scheduledModels)
	if err != nil {
		return nil, fmt.Errorf("failed to scan for scheduled status: %w", err)
	}

	if len(scheduledModels) == 0 {
		return nil, fmt.Errorf("scheduled status not found")
	}

	return r.modelToStorage(&scheduledModels[0]), nil
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
		return nil, "", fmt.Errorf("failed to get scheduled statuses: %w", err)
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
	if len(statuses) > limit {
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
	if scheduled.Username == "" {
		scheduled.Username = existing.Username
	}

	// Update timestamp
	scheduled.UpdatedAt = time.Now()

	// Create updated model
	scheduledModel := r.storageToModel(scheduled)

	// Use DynamORM update - this will update all fields
	err = r.db.WithContext(ctx).Model(scheduledModel).Update()
	if err != nil {
		return fmt.Errorf("failed to update scheduled status: %w", err)
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

	// Create model with keys for deletion
	scheduledModel := &models.ScheduledStatus{
		PK: fmt.Sprintf("USER#%s#SCHEDULED", status.Username),
		SK: fmt.Sprintf("ID#%s", id),
	}

	err = r.db.WithContext(ctx).Model(scheduledModel).Delete()
	if err != nil {
		return fmt.Errorf("failed to delete scheduled status: %w", err)
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
		return nil, fmt.Errorf("failed to get due scheduled statuses: %w", err)
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

	err = r.db.WithContext(ctx).Model(scheduledModel).Update()
	if err != nil {
		return fmt.Errorf("failed to mark scheduled status as published: %w", err)
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
	if len(scheduled.MediaIDs) == 0 {
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
func (r *ScheduledStatusRepository) convertToMediaAttachment(media *models.Media, _ int) map[string]any {
	// Determine media type based on content type
	mediaType := MediaTypeUnknown
	if media.IsImage() {
		mediaType = MediaTypeImage
	} else if media.IsVideo() {
		mediaType = MediaTypeVideo
	} else if media.IsAudio() {
		mediaType = MediaTypeAudio
	}

	// Handle GIF images as gifv type
	if media.ContentType == "image/gif" {
		mediaType = MediaTypeGifv
	}

	// Build attachment object compatible with Mastodon API
	attachment := map[string]any{
		"id":          media.MediaID,
		"type":        mediaType,
		"url":         media.CDNUrl,
		"preview_url": media.CDNUrl, // Use same URL for preview by default
		"remote_url":  nil,          // For local media, this is nil
		"description": media.Description,
		"meta": map[string]any{
			"original": map[string]any{
				"width":  media.Width,
				"height": media.Height,
				"size":   fmt.Sprintf("%dx%d", media.Width, media.Height),
				"aspect": r.calculateAspect(media.Width, media.Height),
			},
		},
		"blurhash": media.Blurhash,
		"focus":    r.parseFocus(media.Focus),
	}

	// Add duration for video/audio
	if mediaType == MediaTypeVideo || mediaType == MediaTypeAudio {
		if meta, ok := attachment["meta"].(map[string]any); ok {
			if original, ok := meta["original"].(map[string]any); ok {
				original["duration"] = media.Duration
			}
		}
	}

	// Add variants/thumbnails if available
	if len(media.Variants) > 0 {
		attachment["variants"] = r.convertVariants(media.Variants)
	}

	// Add preview variant for videos/images
	if mediaType == MediaTypeVideo || mediaType == MediaTypeImage {
		if thumbnail, exists := media.GetVariant("thumbnail"); exists {
			attachment["preview_url"] = thumbnail.CDNUrl
			if meta, ok := attachment["meta"].(map[string]any); ok {
				meta["small"] = map[string]any{
					"width":  thumbnail.Width,
					"height": thumbnail.Height,
					"size":   fmt.Sprintf("%dx%d", thumbnail.Width, thumbnail.Height),
					"aspect": r.calculateAspect(thumbnail.Width, thumbnail.Height),
				}
			}
		}
	}

	return attachment
}

// convertVariants converts media variants to API format
func (r *ScheduledStatusRepository) convertVariants(variants map[string]models.MediaVariant) map[string]any {
	apiVariants := make(map[string]any)
	for name, variant := range variants {
		apiVariants[name] = map[string]any{
			"url":          variant.CDNUrl,
			"content_type": variant.ContentType,
			"width":        variant.Width,
			"height":       variant.Height,
			"file_size":    variant.FileSize,
		}
	}
	return apiVariants
}

// calculateAspect calculates aspect ratio for media
func (r *ScheduledStatusRepository) calculateAspect(width, height int) float64 {
	if height == 0 {
		return 1.0
	}
	return float64(width) / float64(height)
}

// parseFocus parses focus point string into API format
func (r *ScheduledStatusRepository) parseFocus(focus string) map[string]float64 {
	if focus == "" {
		return map[string]float64{"x": 0.0, "y": 0.0}
	}

	parts := strings.Split(focus, ",")
	if len(parts) != 2 {
		return map[string]float64{"x": 0.0, "y": 0.0}
	}

	var x, y float64
	if _, err := fmt.Sscanf(parts[0], "%f", &x); err != nil {
		x = 0.0
	}
	if _, err := fmt.Sscanf(parts[1], "%f", &y); err != nil {
		y = 0.0
	}

	return map[string]float64{"x": x, "y": y}
}
