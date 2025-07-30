package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/google/uuid"
	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"
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
}

// NewScheduledStatusRepository creates a new scheduled status repository
func NewScheduledStatusRepository(db core.DB, tableName string, logger *zap.Logger) *ScheduledStatusRepository {
	return &ScheduledStatusRepository{
		db:        db,
		tableName: tableName,
		logger:    logger,
	}
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
	pk := fmt.Sprintf("USER#%s#SCHEDULED", username)
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
func (r *ScheduledStatusRepository) GetScheduledStatusMedia(ctx context.Context, id string) ([]any, error) {
	// Get the scheduled status first to access its media IDs
	scheduled, err := r.GetScheduledStatus(ctx, id)
	if err != nil {
		return nil, err
	}

	// If no media IDs, return empty slice
	if len(scheduled.MediaIDs) == 0 {
		return []any{}, nil
	}

	// Query for media attachments based on the media IDs
	// This is a simplified implementation - in practice you'd query the media table
	// For now, return empty slice as the legacy implementation seems to use a different pattern
	r.logger.Info("getting scheduled status media",
		zap.String("scheduled_status_id", id),
		zap.Strings("media_ids", scheduled.MediaIDs))

	// TODO: Implement proper media attachment queries when media repository is available
	return []any{}, nil
}
