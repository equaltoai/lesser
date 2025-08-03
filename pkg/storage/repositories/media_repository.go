package repositories

import (
	"context"
	"fmt"

	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/storage/models"
)

// MediaRepository handles media and media job operations using DynamORM
type MediaRepository struct {
	db        core.DB
	tableName string
	logger    *zap.Logger
}

// NewMediaRepository creates a new MediaRepository
func NewMediaRepository(db core.DB, tableName string, logger *zap.Logger) *MediaRepository {
	return &MediaRepository{
		db:        db,
		tableName: tableName,
		logger:    logger,
	}
}

// CreateMediaJob creates a new media processing job
func (r *MediaRepository) CreateMediaJob(ctx context.Context, job *models.MediaJob) error {
	r.logger.Debug("creating media job",
		zap.String("job_id", job.JobID),
		zap.String("media_id", job.MediaID),
		zap.String("username", job.Username))

	if err := job.BeforeCreate(); err != nil {
		return fmt.Errorf("failed to prepare job for creation: %w", err)
	}

	return r.db.WithContext(ctx).Model(job).Create()
}

// GetMediaJob retrieves a media job by ID
func (r *MediaRepository) GetMediaJob(ctx context.Context, jobID string) (*models.MediaJob, error) {
	r.logger.Debug("getting media job", zap.String("job_id", jobID))

	var job models.MediaJob
	err := r.db.WithContext(ctx).Model(&models.MediaJob{}).
		Where("PK", "=", fmt.Sprintf("JOB#%s", jobID)).
		Where("SK", "=", fmt.Sprintf("JOB#%s", jobID)).
		First(&job)
	
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("media job not found: %s", jobID)
		}
		return nil, fmt.Errorf("failed to get media job: %w", err)
	}

	return &job, nil
}

// UpdateMediaJob updates an existing media job
func (r *MediaRepository) UpdateMediaJob(ctx context.Context, job *models.MediaJob) error {
	r.logger.Debug("updating media job",
		zap.String("job_id", job.JobID),
		zap.String("status", job.Status))

	if err := job.BeforeUpdate(); err != nil {
		return fmt.Errorf("failed to prepare job for update: %w", err)
	}

	return r.db.WithContext(ctx).Model(job).Update()
}

// GetJobsByStatus retrieves jobs by status
func (r *MediaRepository) GetJobsByStatus(ctx context.Context, status string, limit int) ([]*models.MediaJob, error) {
	r.logger.Debug("getting jobs by status",
		zap.String("status", status),
		zap.Int("limit", limit))

	var jobs []*models.MediaJob
	query := r.db.WithContext(ctx).Model(&models.MediaJob{}).
		Where("GSI2PK", "=", fmt.Sprintf("STATUS#%s", status))

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.Scan(&jobs)
	if err != nil {
		return nil, fmt.Errorf("failed to get jobs by status: %w", err)
	}

	return jobs, nil
}

// GetJobsByUser retrieves jobs for a specific user
func (r *MediaRepository) GetJobsByUser(ctx context.Context, username string, limit int) ([]*models.MediaJob, error) {
	r.logger.Debug("getting jobs by user",
		zap.String("username", username),
		zap.Int("limit", limit))

	var jobs []*models.MediaJob
	query := r.db.WithContext(ctx).Model(&models.MediaJob{}).
		Where("GSI1PK", "=", fmt.Sprintf("USER_JOBS#%s", username))

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.Scan(&jobs)
	if err != nil {
		return nil, fmt.Errorf("failed to get jobs by user: %w", err)
	}

	return jobs, nil
}

// CreateMedia creates a new media record
func (r *MediaRepository) CreateMedia(ctx context.Context, media *models.Media) error {
	r.logger.Debug("creating media record",
		zap.String("media_id", media.MediaID),
		zap.String("user_id", media.UserID),
		zap.String("content_type", media.ContentType))

	if err := media.BeforeCreate(); err != nil {
		return fmt.Errorf("failed to prepare media for creation: %w", err)
	}

	return r.db.WithContext(ctx).Model(media).Create()
}

// GetMedia retrieves a media record by ID
func (r *MediaRepository) GetMedia(ctx context.Context, mediaID string) (*models.Media, error) {
	r.logger.Debug("getting media", zap.String("media_id", mediaID))

	var media models.Media
	err := r.db.WithContext(ctx).Model(&models.Media{}).
		Where("PK", "=", fmt.Sprintf("media#%s", mediaID)).
		Where("SK", "=", "version#original").
		First(&media)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("media not found: %s", mediaID)
		}
		return nil, fmt.Errorf("failed to get media: %w", err)
	}

	return &media, nil
}

// UpdateMedia updates an existing media record
func (r *MediaRepository) UpdateMedia(ctx context.Context, media *models.Media) error {
	r.logger.Debug("updating media",
		zap.String("media_id", media.MediaID),
		zap.String("status", media.Status))

	if err := media.BeforeUpdate(); err != nil {
		return fmt.Errorf("failed to prepare media for update: %w", err)
	}

	return r.db.WithContext(ctx).Model(media).Update()
}

// GetMediaByUser retrieves media records for a specific user
func (r *MediaRepository) GetMediaByUser(ctx context.Context, userID string, limit int) ([]*models.Media, error) {
	r.logger.Debug("getting media by user",
		zap.String("user_id", userID),
		zap.Int("limit", limit))

	var mediaList []*models.Media
	query := r.db.WithContext(ctx).Model(&models.Media{}).
		Where("GSI1PK", "=", fmt.Sprintf("USER_MEDIA#%s", userID))

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.Scan(&mediaList)
	if err != nil {
		return nil, fmt.Errorf("failed to get media by user: %w", err)
	}

	return mediaList, nil
}

// GetMediaByStatus retrieves media records by processing status
func (r *MediaRepository) GetMediaByStatus(ctx context.Context, status string, limit int) ([]*models.Media, error) {
	r.logger.Debug("getting media by status",
		zap.String("status", status),
		zap.Int("limit", limit))

	var mediaList []*models.Media
	query := r.db.WithContext(ctx).Model(&models.Media{}).
		Where("GSI2PK", "=", fmt.Sprintf("MEDIA_STATUS#%s", status))

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.Scan(&mediaList)
	if err != nil {
		return nil, fmt.Errorf("failed to get media by status: %w", err)
	}

	return mediaList, nil
}

// GetMediaByContentType retrieves media records by content type
func (r *MediaRepository) GetMediaByContentType(ctx context.Context, contentType string, limit int) ([]*models.Media, error) {
	r.logger.Debug("getting media by content type",
		zap.String("content_type", contentType),
		zap.Int("limit", limit))

	var mediaList []*models.Media
	query := r.db.WithContext(ctx).Model(&models.Media{}).
		Where("GSI3PK", "=", fmt.Sprintf("CONTENT_TYPE#%s", contentType))

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.Scan(&mediaList)
	if err != nil {
		return nil, fmt.Errorf("failed to get media by content type: %w", err)
	}

	return mediaList, nil
}

// DeleteMediaJob deletes a media job
func (r *MediaRepository) DeleteMediaJob(ctx context.Context, jobID string) error {
	r.logger.Debug("deleting media job", zap.String("job_id", jobID))

	return r.db.WithContext(ctx).Model(&models.MediaJob{}).
		Where("PK", "=", fmt.Sprintf("JOB#%s", jobID)).
		Where("SK", "=", fmt.Sprintf("JOB#%s", jobID)).
		Delete()
}

// DeleteMedia deletes a media record
func (r *MediaRepository) DeleteMedia(ctx context.Context, mediaID string) error {
	r.logger.Debug("deleting media", zap.String("media_id", mediaID))

	return r.db.WithContext(ctx).Model(&models.Media{}).
		Where("PK", "=", fmt.Sprintf("media#%s", mediaID)).
		Where("SK", "=", "version#original").
		Delete()
}

// GetUserMedia retrieves media records for a user (for interface compatibility)
func (r *MediaRepository) GetUserMedia(ctx context.Context, username string) ([]any, error) {
	r.logger.Debug("getting user media", zap.String("username", username))

	mediaList, err := r.GetMediaByUser(ctx, username, 0)
	if err != nil {
		return nil, err
	}

	// Convert to []any for interface compatibility
	result := make([]any, len(mediaList))
	for i, media := range mediaList {
		result[i] = media
	}

	return result, nil
}

// UpdateMediaAttachment updates a media attachment (for interface compatibility)
func (r *MediaRepository) UpdateMediaAttachment(ctx context.Context, mediaID string, updates map[string]any) error {
	r.logger.Debug("updating media attachment", 
		zap.String("media_id", mediaID),
		zap.Any("updates", updates))

	media, err := r.GetMedia(ctx, mediaID)
	if err != nil {
		return err
	}

	// Apply updates to the media model
	for key, value := range updates {
		switch key {
		case "description":
			if desc, ok := value.(string); ok {
				media.Description = desc
				r.logger.Debug("updated media description", 
					zap.String("media_id", mediaID),
					zap.String("description", desc))
			}
		case "focus":
			if focus, ok := value.(string); ok {
				media.Focus = focus
				r.logger.Debug("updated media focus", 
					zap.String("media_id", mediaID),
					zap.String("focus", focus))
			}
		case "sensitive":
			if sensitive, ok := value.(bool); ok {
				media.IsNSFW = sensitive
				r.logger.Debug("updated media sensitivity", 
					zap.String("media_id", mediaID),
					zap.Bool("is_nsfw", sensitive))
			}
		}
	}

	return r.UpdateMedia(ctx, media)
}

// UnmarkAllMediaAsSensitive unmarks all media for a user as non-sensitive
func (r *MediaRepository) UnmarkAllMediaAsSensitive(ctx context.Context, username string) error {
	r.logger.Debug("unmarking all media as sensitive", zap.String("username", username))

	mediaList, err := r.GetMediaByUser(ctx, username, 0)
	if err != nil {
		return err
	}

	for _, media := range mediaList {
		media.IsNSFW = false
		if err := r.UpdateMedia(ctx, media); err != nil {
			r.logger.Error("failed to update media sensitivity",
				zap.String("media_id", media.MediaID),
				zap.Error(err))
			// Continue with other media
		}
	}

	return nil
}