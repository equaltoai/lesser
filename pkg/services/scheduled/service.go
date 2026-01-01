// Package scheduled provides scheduled status management services for the Lesser ActivityPub server.
//
// This service handles all operations related to scheduled statuses including:
// - Creating and scheduling posts for future publication
// - Managing scheduled status updates and deletions
// - Processing scheduled statuses at their scheduled time
// - Handling media attachments for scheduled posts
package scheduled

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	svcErrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/equaltoai/lesser/pkg/streaming"
	"go.uber.org/zap"
)

type scheduledStatusRepository interface {
	CreateScheduledStatus(ctx context.Context, scheduled *storage.ScheduledStatus) error
	GetScheduledStatus(ctx context.Context, id string) (*storage.ScheduledStatus, error)
	GetScheduledStatuses(ctx context.Context, username string, limit int, cursor string) ([]*storage.ScheduledStatus, string, error)
	UpdateScheduledStatus(ctx context.Context, scheduled *storage.ScheduledStatus) error
	DeleteScheduledStatus(ctx context.Context, id string) error
	GetScheduledStatusMedia(ctx context.Context, scheduledStatusID string) ([]*models.Media, error)
}

type mediaRepository interface {
	GetMedia(ctx context.Context, mediaID string) (*models.Media, error)
}

var (
	_ scheduledStatusRepository = (*repositories.ScheduledStatusRepository)(nil)
	_ mediaRepository          = (*repositories.MediaRepository)(nil)
)

// Service provides business logic for scheduled status operations
type Service struct {
	scheduledRepo scheduledStatusRepository
	statusRepo    interfaces.StatusRepository
	mediaRepo     mediaRepository
	publisher     streaming.Publisher
	logger        *zap.Logger
	domain        string
}

// NewService creates a new scheduled status service
func NewService(
	scheduledRepo scheduledStatusRepository,
	statusRepo interfaces.StatusRepository,
	mediaRepo mediaRepository,
	publisher streaming.Publisher,
	logger *zap.Logger,
	domain string,
) *Service {
	return &Service{
		scheduledRepo: scheduledRepo,
		statusRepo:    statusRepo,
		mediaRepo:     mediaRepo,
		publisher:     publisher,
		logger:        logger,
		domain:        domain,
	}
}

// Query and Command types for CQRS pattern

// GetScheduledStatusQuery contains parameters for retrieving a single scheduled status
type GetScheduledStatusQuery struct {
	ID       string `json:"id" validate:"required"`
	Username string `json:"username"` // For ownership verification
}

// ListScheduledStatusesQuery contains parameters for listing scheduled statuses
type ListScheduledStatusesQuery struct {
	Username   string                       `json:"username" validate:"required"`
	Pagination interfaces.PaginationOptions `json:"pagination"`
}

// CreateScheduledStatusCommand contains data needed to create a scheduled status
type CreateScheduledStatusCommand struct {
	Username      string         `json:"username" validate:"required"`
	Status        string         `json:"status" validate:"required,min=1,max=500"`
	MediaIDs      []string       `json:"media_ids"`
	Sensitive     bool           `json:"sensitive"`
	SpoilerText   string         `json:"spoiler_text"`
	Visibility    string         `json:"visibility"`
	Language      string         `json:"language"`
	InReplyToID   string         `json:"in_reply_to_id"`
	Poll          map[string]any `json:"poll"`
	ScheduledAt   time.Time      `json:"scheduled_at" validate:"required"`
	ApplicationID string         `json:"application_id"`
}

// UpdateScheduledStatusCommand contains data needed to update a scheduled status
type UpdateScheduledStatusCommand struct {
	ID          string     `json:"id" validate:"required"`
	Username    string     `json:"username" validate:"required"` // For ownership verification
	ScheduledAt *time.Time `json:"scheduled_at"`
}

// DeleteScheduledStatusCommand contains data needed to delete a scheduled status
type DeleteScheduledStatusCommand struct {
	ID       string `json:"id" validate:"required"`
	Username string `json:"username" validate:"required"` // For ownership verification
}

// PublishScheduledStatusCommand contains data needed to publish a scheduled status
type PublishScheduledStatusCommand struct {
	ID string `json:"id" validate:"required"`
}

// Result types

// StatusResult contains a single scheduled status
type StatusResult struct {
	ScheduledStatus  *storage.ScheduledStatus `json:"scheduled_status"`
	MediaAttachments []*models.Media          `json:"media_attachments,omitempty"`
	Events           []*streaming.Event       `json:"events"`
}

// StatusListResult contains multiple scheduled statuses
type StatusListResult struct {
	ScheduledStatuses []*storage.ScheduledStatus          `json:"scheduled_statuses"`
	Pagination        *interfaces.PaginatedResult[string] `json:"pagination"`
	Events            []*streaming.Event                  `json:"events"`
}

// GetScheduledStatus retrieves a single scheduled status by ID
func (s *Service) GetScheduledStatus(ctx context.Context, query *GetScheduledStatusQuery) (*StatusResult, error) {
	s.logger.Info("getting scheduled status",
		zap.String("id", query.ID),
		zap.String("username", query.Username))

	// Get the scheduled status
	scheduled, err := s.scheduledRepo.GetScheduledStatus(ctx, query.ID)
	if err != nil {
		s.logger.Error("failed to get scheduled status",
			zap.String("id", query.ID),
			zap.Error(err))
		return nil, errors.Join(svcErrors.ErrGetStatus, err)
	}

	// Verify ownership if username provided
	if query.Username != "" && scheduled.Username != query.Username {
		return nil, svcErrors.ErrGetStatus // Don't reveal it exists
	}

	// Check if already published
	if scheduled.Published {
		return nil, svcErrors.ErrGetStatus
	}

	// Get media attachments if any
	var mediaAttachments []*models.Media
	if err := common.ValidateSliceNotEmpty("scheduled.MediaIDs", scheduled.MediaIDs); err == nil {
		mediaAttachments, err = s.getMediaAttachments(ctx, scheduled.MediaIDs)
		if err != nil {
			s.logger.Warn("failed to get media attachments",
				zap.String("scheduled_id", query.ID),
				zap.Error(err))
		}
	}

	return &StatusResult{
		ScheduledStatus:  scheduled,
		MediaAttachments: mediaAttachments,
		Events:           nil,
	}, nil
}

// ListScheduledStatuses retrieves all scheduled statuses for a user
func (s *Service) ListScheduledStatuses(ctx context.Context, query *ListScheduledStatusesQuery) (*StatusListResult, error) {
	s.logger.Info("listing scheduled statuses",
		zap.String("username", query.Username),
		zap.Int("limit", query.Pagination.Limit))

	// Set default limit if not provided
	limit := query.Pagination.Limit
	if limit <= 0 {
		limit = 20
	}

	// Get scheduled statuses from repository
	statuses, nextCursor, err := s.scheduledRepo.GetScheduledStatuses(
		ctx,
		query.Username,
		limit,
		query.Pagination.Cursor,
	)
	if err != nil {
		s.logger.Error("failed to get scheduled statuses",
			zap.String("username", query.Username),
			zap.Error(err))
		return nil, errors.Join(svcErrors.ErrGetStatuses, err)
	}

	// Filter out published statuses (should already be done by repo, but double-check)
	unpublished := make([]*storage.ScheduledStatus, 0, len(statuses))
	for _, status := range statuses {
		if !status.Published {
			unpublished = append(unpublished, status)
		}
	}

	// Build pagination result
	pagination := &interfaces.PaginatedResult[string]{
		Items:      make([]string, len(unpublished)),
		NextCursor: nextCursor,
		HasMore:    nextCursor != "",
	}
	for i, status := range unpublished {
		pagination.Items[i] = status.ID
	}

	return &StatusListResult{
		ScheduledStatuses: unpublished,
		Pagination:        pagination,
		Events:            nil,
	}, nil
}

// CreateScheduledStatus creates a new scheduled status
func (s *Service) CreateScheduledStatus(ctx context.Context, cmd *CreateScheduledStatusCommand) (*StatusResult, error) {
	s.logger.Info("creating scheduled status",
		zap.String("username", cmd.Username),
		zap.Time("scheduled_at", cmd.ScheduledAt))

	// Validate scheduled time
	if err := s.validateScheduledTime(cmd.ScheduledAt); err != nil {
		return nil, err
	}

	// Validate media attachments if provided
	if err := common.ValidateSliceNotEmpty("cmd.MediaIDs", cmd.MediaIDs); err == nil {
		if err := s.validateMediaAttachments(ctx, cmd.MediaIDs); err != nil {
			return nil, err
		}
	}

	// Create the scheduled status
	scheduled := &storage.ScheduledStatus{
		Username:      cmd.Username,
		Status:        cmd.Status,
		MediaIDs:      cmd.MediaIDs,
		Sensitive:     cmd.Sensitive,
		SpoilerText:   cmd.SpoilerText,
		Visibility:    cmd.Visibility,
		Language:      cmd.Language,
		InReplyToID:   cmd.InReplyToID,
		Poll:          cmd.Poll,
		ScheduledAt:   cmd.ScheduledAt,
		ApplicationID: cmd.ApplicationID,
		Published:     false,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	// Set default visibility if not provided
	if err := common.ValidateRequiredParam("scheduled.Visibility", scheduled.Visibility); err != nil {
		scheduled.Visibility = "public"
	}

	// Create in repository
	if err := s.scheduledRepo.CreateScheduledStatus(ctx, scheduled); err != nil {
		s.logger.Error("failed to create scheduled status",
			zap.String("username", scheduled.Username),
			zap.Error(err))
		return nil, errors.Join(svcErrors.ErrCreateScheduledStatus, err)
	}

	// Get media attachments for response
	var mediaAttachments []*models.Media
	if err := common.ValidateSliceNotEmpty("scheduled.MediaIDs", scheduled.MediaIDs); err == nil {
		mediaAttachments, _ = s.getMediaAttachments(ctx, scheduled.MediaIDs)
	}

	// Emit events for real-time updates
	events := s.emitScheduledStatusCreatedEvents(ctx, scheduled)

	return &StatusResult{
		ScheduledStatus:  scheduled,
		MediaAttachments: mediaAttachments,
		Events:           events,
	}, nil
}

// UpdateScheduledStatus updates a scheduled status (currently only scheduled time)
func (s *Service) UpdateScheduledStatus(ctx context.Context, cmd *UpdateScheduledStatusCommand) (*StatusResult, error) {
	s.logger.Info("updating scheduled status",
		zap.String("id", cmd.ID),
		zap.String("username", cmd.Username))

	// Get existing scheduled status
	existing, err := s.scheduledRepo.GetScheduledStatus(ctx, cmd.ID)
	if err != nil {
		s.logger.Error("failed to get scheduled status for update",
			zap.String("id", cmd.ID),
			zap.Error(err))
		return nil, errors.Join(svcErrors.ErrGetStatus, err)
	}

	// Verify ownership
	if existing.Username != cmd.Username {
		return nil, svcErrors.ErrGetStatus // Don't reveal it exists
	}

	// Check if already published
	if existing.Published {
		return nil, svcErrors.ErrUpdateStatus
	}

	// Update scheduled time if provided
	updated := false
	if cmd.ScheduledAt != nil {
		if err := s.validateScheduledTime(*cmd.ScheduledAt); err != nil {
			return nil, err
		}
		existing.ScheduledAt = *cmd.ScheduledAt
		updated = true
	}

	if !updated {
		// No changes
		return &StatusResult{
			ScheduledStatus: existing,
			Events:          nil,
		}, nil
	}

	existing.UpdatedAt = time.Now()

	// Update in repository
	if err := s.scheduledRepo.UpdateScheduledStatus(ctx, existing); err != nil {
		s.logger.Error("failed to update scheduled status",
			zap.String("id", cmd.ID),
			zap.Error(err))
		return nil, errors.Join(svcErrors.ErrUpdateStatus, err)
	}

	// Get media attachments for response
	var mediaAttachments []*models.Media
	if err := common.ValidateSliceNotEmpty("existing.MediaIDs", existing.MediaIDs); err == nil {
		mediaAttachments, _ = s.getMediaAttachments(ctx, existing.MediaIDs)
	}

	// Emit events for real-time updates
	events := s.emitScheduledStatusUpdatedEvents(ctx, existing)

	return &StatusResult{
		ScheduledStatus:  existing,
		MediaAttachments: mediaAttachments,
		Events:           events,
	}, nil
}

// DeleteScheduledStatus deletes a scheduled status
func (s *Service) DeleteScheduledStatus(ctx context.Context, cmd *DeleteScheduledStatusCommand) error {
	s.logger.Info("deleting scheduled status",
		zap.String("id", cmd.ID),
		zap.String("username", cmd.Username))

	// Get existing scheduled status
	existing, err := s.scheduledRepo.GetScheduledStatus(ctx, cmd.ID)
	if err != nil {
		s.logger.Error("failed to get scheduled status for deletion",
			zap.String("id", cmd.ID),
			zap.Error(err))
		return errors.Join(svcErrors.ErrGetStatus, err)
	}

	// Verify ownership
	if existing.Username != cmd.Username {
		return svcErrors.ErrGetStatus // Don't reveal it exists
	}

	// Check if already published
	if existing.Published {
		return svcErrors.ErrDeleteStatus
	}

	// Delete from repository
	if err := s.scheduledRepo.DeleteScheduledStatus(ctx, cmd.ID); err != nil {
		s.logger.Error("failed to delete scheduled status",
			zap.String("id", cmd.ID),
			zap.Error(err))
		return errors.Join(svcErrors.ErrDeleteStatus, err)
	}

	// Emit events for real-time updates
	s.emitScheduledStatusDeletedEvents(ctx, existing)

	return nil
}

// PublishScheduledStatus publishes a scheduled status immediately or at its scheduled time
func (s *Service) PublishScheduledStatus(ctx context.Context, cmd *PublishScheduledStatusCommand) error {
	s.logger.Info("publishing scheduled status",
		zap.String("id", cmd.ID))

	// Get the scheduled status
	scheduled, err := s.scheduledRepo.GetScheduledStatus(ctx, cmd.ID)
	if err != nil {
		s.logger.Error("failed to get scheduled status for publication",
			zap.String("id", cmd.ID),
			zap.Error(err))
		return errors.Join(svcErrors.ErrGetStatus, err)
	}

	// Check if already published
	if scheduled.Published {
		return svcErrors.ErrGetStatus
	}

	// Mark as published
	scheduled.Published = true
	now := time.Now()
	scheduled.PublishedAt = &now
	scheduled.UpdatedAt = now

	// Update in repository
	if err := s.scheduledRepo.UpdateScheduledStatus(ctx, scheduled); err != nil {
		s.logger.Error("failed to update scheduled status for publication",
			zap.String("id", cmd.ID),
			zap.Error(err))
		return errors.Join(svcErrors.ErrUpdateStatus, err)
	}

	// Note: Actual status creation should be handled by a separate service
	// This method just marks the scheduled status as published

	// Emit events
	s.emitScheduledStatusPublishedEvents(ctx, scheduled)

	return nil
}

// GetScheduledMediaAttachments retrieves media attachments for a scheduled status
func (s *Service) GetScheduledMediaAttachments(ctx context.Context, scheduledStatusID string) ([]*models.Media, error) {
	s.logger.Info("getting scheduled status media attachments",
		zap.String("scheduled_status_id", scheduledStatusID))

	mediaItems, err := s.scheduledRepo.GetScheduledStatusMedia(ctx, scheduledStatusID)
	if err != nil {
		s.logger.Error("failed to get scheduled status media",
			zap.String("scheduled_status_id", scheduledStatusID),
			zap.Error(err))
		return nil, errors.Join(svcErrors.ErrGetStatuses, err)
	}

	return mediaItems, nil
}

// Helper methods

// validateScheduledTime validates that the scheduled time is in the future
func (s *Service) validateScheduledTime(scheduledAt time.Time) error {
	// Must be at least 5 minutes in the future
	minTime := time.Now().Add(5 * time.Minute)
	if scheduledAt.Before(minTime) {
		return svcErrors.ErrScheduledTimeInPast
	}

	// Must not be more than 1 year in the future
	maxTime := time.Now().Add(365 * 24 * time.Hour)
	if scheduledAt.After(maxTime) {
		return svcErrors.ErrValidationFailed
	}

	return nil
}

// validateMediaAttachments validates that media attachments exist
func (s *Service) validateMediaAttachments(ctx context.Context, mediaIDs []string) error {
	if err := common.ValidateSliceLength("mediaIDs", mediaIDs, 4); err != nil {
		return svcErrors.ErrValidationFailed
	}

	for _, mediaID := range mediaIDs {
		// Validate media ID format
		if err := common.ValidateRequiredParam("mediaID", mediaID); err != nil {
			return svcErrors.ErrValidationFailed
		}

		// Check if media exists and is accessible
		media, err := s.mediaRepo.GetMedia(ctx, mediaID)
		if err != nil {
			s.logger.Error("failed to get media attachment",
				zap.String("media_id", mediaID),
				zap.Error(err))
			return errors.Join(svcErrors.ErrMediaAttachmentNotFound, err)
		}

		// Validate media is in ready state
		if media.Status != "ready" && media.Status != "completed" {
			s.logger.Error("media attachment not ready",
				zap.String("media_id", mediaID),
				zap.String("status", media.Status))
			return svcErrors.ErrMediaAttachmentNotReady
		}

		// Check media hasn't expired
		if media.ExpiresAt > 0 && time.Now().Unix() > media.ExpiresAt {
			s.logger.Error("media attachment expired",
				zap.String("media_id", mediaID),
				zap.Int64("expires_at", media.ExpiresAt))
			return svcErrors.ErrMediaAttachmentExpired
		}

		s.logger.Debug("validated media attachment",
			zap.String("media_id", mediaID),
			zap.String("content_type", media.ContentType),
			zap.String("status", media.Status))
	}

	return nil
}

// getMediaAttachments retrieves media attachments by IDs
func (s *Service) getMediaAttachments(ctx context.Context, mediaIDs []string) ([]*models.Media, error) {
	mediaItems := make([]*models.Media, 0, len(mediaIDs))

	for _, mediaID := range mediaIDs {
		media, err := s.mediaRepo.GetMedia(ctx, mediaID)
		if err != nil {
			s.logger.Error("failed to get media attachment",
				zap.String("media_id", mediaID),
				zap.Error(err))
			return nil, errors.Join(svcErrors.ErrRetrieveMediaAttachment, err)
		}

		// Only include ready/completed media
		if media.Status == "ready" || media.Status == "completed" {
			mediaItems = append(mediaItems, media)
			s.logger.Debug("retrieved media attachment",
				zap.String("media_id", mediaID),
				zap.String("content_type", media.ContentType))
		} else {
			s.logger.Warn("skipping media attachment not ready",
				zap.String("media_id", mediaID),
				zap.String("status", media.Status))
		}
	}

	return mediaItems, nil
}

// Event emission methods

func (s *Service) emitScheduledStatusCreatedEvents(ctx context.Context, scheduled *storage.ScheduledStatus) []*streaming.Event {
	if s.publisher == nil {
		return nil
	}

	event := &streaming.Event{
		Type:      "scheduled_status.created",
		Stream:    fmt.Sprintf("user:%s", scheduled.Username),
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"id":           scheduled.ID,
			"scheduled_at": scheduled.ScheduledAt,
		},
	}

	// Publish to user's stream
	if err := s.publisher.PublishToUser(ctx, scheduled.Username, event); err != nil {
		s.logger.Warn("failed to publish scheduled status created event",
			zap.String("id", scheduled.ID),
			zap.Error(err))
	}

	return []*streaming.Event{event}
}

func (s *Service) emitScheduledStatusUpdatedEvents(ctx context.Context, scheduled *storage.ScheduledStatus) []*streaming.Event {
	if s.publisher == nil {
		return nil
	}

	event := &streaming.Event{
		Type:      "scheduled_status.updated",
		Stream:    fmt.Sprintf("user:%s", scheduled.Username),
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"id":           scheduled.ID,
			"scheduled_at": scheduled.ScheduledAt,
		},
	}

	// Publish to user's stream
	if err := s.publisher.PublishToUser(ctx, scheduled.Username, event); err != nil {
		s.logger.Warn("failed to publish scheduled status updated event",
			zap.String("id", scheduled.ID),
			zap.Error(err))
	}

	return []*streaming.Event{event}
}

func (s *Service) emitScheduledStatusDeletedEvents(ctx context.Context, scheduled *storage.ScheduledStatus) {
	if s.publisher == nil {
		return
	}

	event := &streaming.Event{
		Type:      "scheduled_status.deleted",
		Stream:    fmt.Sprintf("user:%s", scheduled.Username),
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"id": scheduled.ID,
		},
	}

	// Publish to user's stream
	if err := s.publisher.PublishToUser(ctx, scheduled.Username, event); err != nil {
		s.logger.Warn("failed to publish scheduled status deleted event",
			zap.String("id", scheduled.ID),
			zap.Error(err))
	}
}

func (s *Service) emitScheduledStatusPublishedEvents(ctx context.Context, scheduled *storage.ScheduledStatus) {
	if s.publisher == nil {
		return
	}

	event := &streaming.Event{
		Type:      "scheduled_status.published",
		Stream:    fmt.Sprintf("user:%s", scheduled.Username),
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"id":           scheduled.ID,
			"published_at": scheduled.PublishedAt,
		},
	}

	// Publish to user's stream
	if err := s.publisher.PublishToUser(ctx, scheduled.Username, event); err != nil {
		s.logger.Warn("failed to publish scheduled status published event",
			zap.String("id", scheduled.ID),
			zap.Error(err))
	}
}
