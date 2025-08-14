package repositories

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

// MediaRepository handles media and media job operations using DynamORM
type MediaRepository struct {
	db        core.DB
	tableName string
	logger    *zap.Logger
	deps      map[string]interface{} // Dependencies for cross-repository operations
}

// NewMediaRepository creates a new MediaRepository
func NewMediaRepository(db core.DB, tableName string, logger *zap.Logger) *MediaRepository {
	return &MediaRepository{
		db:        db,
		tableName: tableName,
		logger:    logger,
		deps:      make(map[string]interface{}),
	}
}

// SetDependencies sets repository dependencies for cross-repo operations
func (r *MediaRepository) SetDependencies(deps map[string]interface{}) {
	r.deps = deps
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

// GetUserMediaLegacy retrieves media records for a user (for legacy interface compatibility)
func (r *MediaRepository) GetUserMediaLegacy(ctx context.Context, username string) ([]any, error) {
	r.logger.Debug("getting user media legacy", zap.String("username", username))

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

// === User Media Configuration Methods ===

// CreateUserMediaConfig creates a new user media configuration
func (r *MediaRepository) CreateUserMediaConfig(ctx context.Context, config *models.UserMediaConfig) error {
	r.logger.Debug("creating user media config",
		zap.String("user_id", config.UserID),
		zap.String("username", config.Username),
		zap.String("plan_tier", config.PlanTier))

	if err := config.BeforeCreate(); err != nil {
		return fmt.Errorf("failed to prepare user media config for creation: %w", err)
	}

	return r.db.WithContext(ctx).Model(config).Create()
}

// GetUserMediaConfig retrieves a user's media configuration
func (r *MediaRepository) GetUserMediaConfig(ctx context.Context, userID string) (*models.UserMediaConfig, error) {
	r.logger.Debug("getting user media config", zap.String("user_id", userID))

	var config models.UserMediaConfig
	err := r.db.WithContext(ctx).Model(&models.UserMediaConfig{}).
		Where("PK", "=", fmt.Sprintf("USER_MEDIA_CONFIG#%s", userID)).
		Where("SK", "=", "CONFIG").
		First(&config)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("user media config not found: %s", userID)
		}
		return nil, fmt.Errorf("failed to get user media config: %w", err)
	}

	return &config, nil
}

// GetUserMediaConfigByUsername retrieves a user's media configuration by username
func (r *MediaRepository) GetUserMediaConfigByUsername(ctx context.Context, username string) (*models.UserMediaConfig, error) {
	r.logger.Debug("getting user media config by username", zap.String("username", username))

	// Try to resolve username to userID using user repository dependency
	if userRepo, ok := r.deps["user"].(interface {
		GetUserIDByUsername(ctx context.Context, username string) (string, error)
	}); ok {
		userID, err := userRepo.GetUserIDByUsername(ctx, username)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve username to userID: %w", err)
		}
		return r.GetUserMediaConfig(ctx, userID)
	}

	// Fallback: scan for config by username (less efficient)
	var config models.UserMediaConfig
	err := r.db.WithContext(ctx).Model(&models.UserMediaConfig{}).
		Filter("Username", "=", username).
		First(&config)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil // Config doesn't exist
		}
		return nil, fmt.Errorf("failed to get user media config by username: %w", err)
	}

	return &config, nil
}

// UpdateUserMediaConfig updates an existing user media configuration
func (r *MediaRepository) UpdateUserMediaConfig(ctx context.Context, config *models.UserMediaConfig) error {
	r.logger.Debug("updating user media config",
		zap.String("user_id", config.UserID),
		zap.String("plan_tier", config.PlanTier))

	if err := config.BeforeUpdate(); err != nil {
		return fmt.Errorf("failed to prepare user media config for update: %w", err)
	}

	return r.db.WithContext(ctx).Model(config).Update()
}

// DeleteUserMediaConfig deletes a user's media configuration
func (r *MediaRepository) DeleteUserMediaConfig(ctx context.Context, userID string) error {
	r.logger.Debug("deleting user media config", zap.String("user_id", userID))

	return r.db.WithContext(ctx).Model(&models.UserMediaConfig{}).
		Where("PK", "=", fmt.Sprintf("USER_MEDIA_CONFIG#%s", userID)).
		Where("SK", "=", "CONFIG").
		Delete()
}

// === Media Spending Tracking Methods ===

// CreateMediaSpending creates a new media spending record
func (r *MediaRepository) CreateMediaSpending(ctx context.Context, spending *models.MediaSpending) error {
	r.logger.Debug("creating media spending record",
		zap.String("user_id", spending.UserID),
		zap.String("period", spending.Period),
		zap.String("period_type", spending.PeriodType))

	if err := spending.BeforeCreate(); err != nil {
		return fmt.Errorf("failed to prepare media spending for creation: %w", err)
	}

	return r.db.WithContext(ctx).Model(spending).Create()
}

// GetMediaSpending retrieves a media spending record for a user and period
func (r *MediaRepository) GetMediaSpending(ctx context.Context, userID, period string) (*models.MediaSpending, error) {
	r.logger.Debug("getting media spending",
		zap.String("user_id", userID),
		zap.String("period", period))

	var spending models.MediaSpending
	err := r.db.WithContext(ctx).Model(&models.MediaSpending{}).
		Where("PK", "=", fmt.Sprintf("MEDIA_SPENDING#%s", userID)).
		Where("SK", "=", fmt.Sprintf("PERIOD#%s", period)).
		First(&spending)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("media spending record not found: %s/%s", userID, period)
		}
		return nil, fmt.Errorf("failed to get media spending: %w", err)
	}

	return &spending, nil
}

// UpdateMediaSpending updates an existing media spending record
func (r *MediaRepository) UpdateMediaSpending(ctx context.Context, spending *models.MediaSpending) error {
	r.logger.Debug("updating media spending",
		zap.String("user_id", spending.UserID),
		zap.String("period", spending.Period),
		zap.Int64("total_spend_micros", spending.TotalSpendMicros))

	if err := spending.BeforeUpdate(); err != nil {
		return fmt.Errorf("failed to prepare media spending for update: %w", err)
	}

	return r.db.WithContext(ctx).Model(spending).Update()
}

// GetMediaSpendingByTimeRange retrieves spending records for a user within a time range
func (r *MediaRepository) GetMediaSpendingByTimeRange(ctx context.Context, userID string, periodType string, limit int) ([]*models.MediaSpending, error) {
	r.logger.Debug("getting media spending by time range",
		zap.String("user_id", userID),
		zap.String("period_type", periodType),
		zap.Int("limit", limit))

	var spendingList []*models.MediaSpending
	query := r.db.WithContext(ctx).Model(&models.MediaSpending{}).
		Where("PK", "=", fmt.Sprintf("MEDIA_SPENDING#%s", userID)).
		Where("SK", "BEGINS_WITH", "PERIOD#")

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.Scan(&spendingList)
	if err != nil {
		return nil, fmt.Errorf("failed to get media spending by time range: %w", err)
	}

	// Filter by period type if specified
	if periodType != "" {
		filtered := make([]*models.MediaSpending, 0)
		for _, spending := range spendingList {
			if spending.PeriodType == periodType {
				filtered = append(filtered, spending)
			}
		}
		spendingList = filtered
	}

	return spendingList, nil
}

// CreateMediaSpendingTransaction creates a new spending transaction
func (r *MediaRepository) CreateMediaSpendingTransaction(ctx context.Context, transaction *models.MediaSpendingTransaction) error {
	r.logger.Debug("creating media spending transaction",
		zap.String("user_id", transaction.UserID),
		zap.String("category", transaction.Category),
		zap.Int64("cost_micros", transaction.CostMicros))

	if err := transaction.BeforeCreate(); err != nil {
		return fmt.Errorf("failed to prepare spending transaction for creation: %w", err)
	}

	return r.db.WithContext(ctx).Model(transaction).Create()
}

// GetMediaSpendingTransactions retrieves spending transactions for a user
func (r *MediaRepository) GetMediaSpendingTransactions(ctx context.Context, userID string, limit int) ([]*models.MediaSpendingTransaction, error) {
	r.logger.Debug("getting media spending transactions",
		zap.String("user_id", userID),
		zap.Int("limit", limit))

	var transactions []*models.MediaSpendingTransaction
	query := r.db.WithContext(ctx).Model(&models.MediaSpendingTransaction{}).
		Where("PK", "=", fmt.Sprintf("SPENDING_TXN#%s", userID)).
		Where("SK", "BEGINS_WITH", "TXN#")

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.Scan(&transactions)
	if err != nil {
		return nil, fmt.Errorf("failed to get spending transactions: %w", err)
	}

	return transactions, nil
}

// GetOrCreateMediaSpending gets an existing spending record or creates a new one
func (r *MediaRepository) GetOrCreateMediaSpending(ctx context.Context, userID, period, periodType string) (*models.MediaSpending, error) {
	r.logger.Debug("getting or creating media spending",
		zap.String("user_id", userID),
		zap.String("period", period),
		zap.String("period_type", periodType))

	// Try to get existing record
	spending, err := r.GetMediaSpending(ctx, userID, period)
	if err == nil {
		return spending, nil
	}

	// If not found, create a new one
	if errors.IsNotFound(err) || strings.Contains(err.Error(), "not found") {
		spending = &models.MediaSpending{
			UserID:     userID,
			Period:     period,
			PeriodType: periodType,
		}

		if err := r.CreateMediaSpending(ctx, spending); err != nil {
			return nil, fmt.Errorf("failed to create new spending record: %w", err)
		}

		return spending, nil
	}

	return nil, fmt.Errorf("failed to get or create spending record: %w", err)
}

// AddSpendingTransaction adds a transaction and updates the spending record
func (r *MediaRepository) AddSpendingTransaction(ctx context.Context, transaction *models.MediaSpendingTransaction) error {
	r.logger.Debug("adding spending transaction",
		zap.String("user_id", transaction.UserID),
		zap.String("category", transaction.Category),
		zap.Int64("cost_micros", transaction.CostMicros))

	// Create the transaction record
	if err := r.CreateMediaSpendingTransaction(ctx, transaction); err != nil {
		return fmt.Errorf("failed to create spending transaction: %w", err)
	}

	// Determine the period based on the transaction timestamp
	period := transaction.CreatedAt.Format(common.MonthFormat) // Monthly period
	periodType := PeriodMonthly

	// Get or create the spending record
	spending, err := r.GetOrCreateMediaSpending(ctx, transaction.UserID, period, periodType)
	if err != nil {
		return fmt.Errorf("failed to get or create spending record: %w", err)
	}

	// Add the transaction to the spending record
	spending.AddSpending(transaction)

	// Update the spending record
	if err := r.UpdateMediaSpending(ctx, spending); err != nil {
		return fmt.Errorf("failed to update spending record: %w", err)
	}

	return nil
}

// === Transcoding Job Tracking Methods ===

// CreateTranscodingJob creates a new transcoding job record
func (r *MediaRepository) CreateTranscodingJob(ctx context.Context, job *models.TranscodingJob) error {
	r.logger.Debug("creating transcoding job",
		zap.String("job_id", job.JobID),
		zap.String("media_id", job.MediaID),
		zap.String("user_id", job.UserID),
		zap.String("job_type", job.JobType))

	if err := job.BeforeCreate(); err != nil {
		return fmt.Errorf("failed to prepare transcoding job for creation: %w", err)
	}

	return r.db.WithContext(ctx).Model(job).Create()
}

// GetTranscodingJob retrieves a transcoding job by ID
func (r *MediaRepository) GetTranscodingJob(ctx context.Context, jobID string) (*models.TranscodingJob, error) {
	r.logger.Debug("getting transcoding job", zap.String("job_id", jobID))

	var job models.TranscodingJob
	err := r.db.WithContext(ctx).Model(&models.TranscodingJob{}).
		Where("PK", "=", fmt.Sprintf("TRANSCODING_JOB#%s", jobID)).
		Where("SK", "=", "JOB_METRICS").
		First(&job)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("transcoding job not found: %s", jobID)
		}
		return nil, fmt.Errorf("failed to get transcoding job: %w", err)
	}

	return &job, nil
}

// UpdateTranscodingJob updates an existing transcoding job
func (r *MediaRepository) UpdateTranscodingJob(ctx context.Context, job *models.TranscodingJob) error {
	r.logger.Debug("updating transcoding job",
		zap.String("job_id", job.JobID),
		zap.String("status", job.Status),
		zap.Int64("total_cost_micros", job.TotalCostMicros))

	if err := job.BeforeUpdate(); err != nil {
		return fmt.Errorf("failed to prepare transcoding job for update: %w", err)
	}

	return r.db.WithContext(ctx).Model(job).Update()
}

// GetTranscodingJobsByUser retrieves transcoding jobs for a specific user
func (r *MediaRepository) GetTranscodingJobsByUser(ctx context.Context, userID string, limit int) ([]*models.TranscodingJob, error) {
	r.logger.Debug("getting transcoding jobs by user",
		zap.String("user_id", userID),
		zap.Int("limit", limit))

	var jobs []*models.TranscodingJob
	query := r.db.WithContext(ctx).Model(&models.TranscodingJob{}).
		Where("GSI1PK", "=", fmt.Sprintf("USER_TRANSCODING#%s", userID))

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.Scan(&jobs)
	if err != nil {
		return nil, fmt.Errorf("failed to get transcoding jobs by user: %w", err)
	}

	return jobs, nil
}

// GetTranscodingJobsByMedia retrieves transcoding jobs for a specific media item
func (r *MediaRepository) GetTranscodingJobsByMedia(ctx context.Context, mediaID string, limit int) ([]*models.TranscodingJob, error) {
	r.logger.Debug("getting transcoding jobs by media",
		zap.String("media_id", mediaID),
		zap.Int("limit", limit))

	var jobs []*models.TranscodingJob
	query := r.db.WithContext(ctx).Model(&models.TranscodingJob{}).
		Where("GSI2PK", "=", fmt.Sprintf("MEDIA_TRANSCODING#%s", mediaID))

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.Scan(&jobs)
	if err != nil {
		return nil, fmt.Errorf("failed to get transcoding jobs by media: %w", err)
	}

	return jobs, nil
}

// GetTranscodingJobsByStatus retrieves transcoding jobs by status
func (r *MediaRepository) GetTranscodingJobsByStatus(ctx context.Context, status string, limit int) ([]*models.TranscodingJob, error) {
	r.logger.Debug("getting transcoding jobs by status",
		zap.String("status", status),
		zap.Int("limit", limit))

	var jobs []*models.TranscodingJob
	// Use scan with filter for status queries. In production, consider adding a GSI
	// for frequently queried statuses to improve performance
	query := r.db.WithContext(ctx).Model(&models.TranscodingJob{}).
		Where("status", "=", status)

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.All(&jobs)
	if err != nil {
		return nil, fmt.Errorf("failed to get transcoding jobs by status: %w", err)
	}

	// Filter results to match requested limit more precisely if needed
	if limit > 0 && len(jobs) > limit {
		jobs = jobs[:limit]
	}

	return jobs, nil
}

// DeleteTranscodingJob deletes a transcoding job
func (r *MediaRepository) DeleteTranscodingJob(ctx context.Context, jobID string) error {
	r.logger.Debug("deleting transcoding job", zap.String("job_id", jobID))

	return r.db.WithContext(ctx).Model(&models.TranscodingJob{}).
		Where("PK", "=", fmt.Sprintf("TRANSCODING_JOB#%s", jobID)).
		Where("SK", "=", "JOB_METRICS").
		Delete()
}

// GetTranscodingCostsByUser retrieves aggregated transcoding costs for a user
func (r *MediaRepository) GetTranscodingCostsByUser(ctx context.Context, userID string, timeRange string) (map[string]int64, error) {
	r.logger.Debug("getting transcoding costs by user",
		zap.String("user_id", userID),
		zap.String("time_range", timeRange))

	jobs, err := r.GetTranscodingJobsByUser(ctx, userID, 0) // Get all jobs
	if err != nil {
		return nil, err
	}

	// Aggregate costs by service
	aggregatedCosts := make(map[string]int64)
	totalCost := int64(0)

	for _, job := range jobs {
		// Filter by time range if specified
		if timeRange != "" && !r.isWithinTimeRange(job.StartedAt, timeRange) {
			continue
		}

		totalCost += job.TotalCostMicros
		for service, cost := range job.CostBreakdown {
			aggregatedCosts[service] += cost
		}
	}

	aggregatedCosts["total"] = totalCost
	return aggregatedCosts, nil
}

// isWithinTimeRange checks if a timestamp is within the specified time range
func (r *MediaRepository) isWithinTimeRange(timestamp time.Time, timeRange string) bool {
	now := time.Now()
	switch timeRange {
	case "day":
		return timestamp.After(now.Add(-24 * time.Hour))
	case "week":
		return timestamp.After(now.Add(-7 * 24 * time.Hour))
	case "month":
		return timestamp.After(now.Add(-30 * 24 * time.Hour))
	case "year":
		return timestamp.After(now.Add(-365 * 24 * time.Hour))
	default:
		return true // No filter
	}
}

// === MISSING INTERFACE METHODS IMPLEMENTATION ===

// MarkMediaProcessing marks a media item as currently being processed
func (r *MediaRepository) MarkMediaProcessing(ctx context.Context, mediaID string) error {
	r.logger.Debug("marking media as processing", zap.String("media_id", mediaID))

	media, err := r.GetMedia(ctx, mediaID)
	if err != nil {
		return err
	}

	media.SetProcessing()
	return r.UpdateMedia(ctx, media)
}

// MarkMediaReady marks a media item as successfully processed and ready
func (r *MediaRepository) MarkMediaReady(ctx context.Context, mediaID string) error {
	r.logger.Debug("marking media as ready", zap.String("media_id", mediaID))

	media, err := r.GetMedia(ctx, mediaID)
	if err != nil {
		return err
	}

	media.SetProcessed()
	return r.UpdateMedia(ctx, media)
}

// MarkMediaFailed marks a media item as failed with an error message
func (r *MediaRepository) MarkMediaFailed(ctx context.Context, mediaID, errorMsg string) error {
	r.logger.Debug("marking media as failed", 
		zap.String("media_id", mediaID),
		zap.String("error", errorMsg))

	media, err := r.GetMedia(ctx, mediaID)
	if err != nil {
		return err
	}

	media.SetFailed(errorMsg)
	return r.UpdateMedia(ctx, media)
}

// GetPendingMedia retrieves media items with pending status
func (r *MediaRepository) GetPendingMedia(ctx context.Context, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Media], error) {
	r.logger.Debug("getting pending media", 
		zap.Int("limit", opts.Limit))

	return r.getMediaByStatus(ctx, models.StatusPending, opts)
}

// GetProcessingMedia retrieves media items with processing status
func (r *MediaRepository) GetProcessingMedia(ctx context.Context, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Media], error) {
	r.logger.Debug("getting processing media", 
		zap.Int("limit", opts.Limit))

	return r.getMediaByStatus(ctx, models.StatusProcessing, opts)
}

// AddMediaVariant adds a variant to a media item
func (r *MediaRepository) AddMediaVariant(ctx context.Context, mediaID, variantName string, variant models.MediaVariant) error {
	r.logger.Debug("adding media variant",
		zap.String("media_id", mediaID),
		zap.String("variant_name", variantName))

	media, err := r.GetMedia(ctx, mediaID)
	if err != nil {
		return err
	}

	media.AddVariant(variantName, variant)
	return r.UpdateMedia(ctx, media)
}

// GetMediaVariant retrieves a specific variant of a media item
func (r *MediaRepository) GetMediaVariant(ctx context.Context, mediaID, variantName string) (*models.MediaVariant, error) {
	r.logger.Debug("getting media variant",
		zap.String("media_id", mediaID),
		zap.String("variant_name", variantName))

	media, err := r.GetMedia(ctx, mediaID)
	if err != nil {
		return nil, err
	}

	variant, exists := media.GetVariant(variantName)
	if !exists {
		return nil, fmt.Errorf("variant '%s' not found for media '%s'", variantName, mediaID)
	}

	return &variant, nil
}

// DeleteMediaVariant removes a variant from a media item
func (r *MediaRepository) DeleteMediaVariant(ctx context.Context, mediaID, variantName string) error {
	r.logger.Debug("deleting media variant",
		zap.String("media_id", mediaID),
		zap.String("variant_name", variantName))

	media, err := r.GetMedia(ctx, mediaID)
	if err != nil {
		return err
	}

	if media.Variants == nil {
		return fmt.Errorf("no variants found for media '%s'", mediaID)
	}

	if _, exists := media.Variants[variantName]; !exists {
		return fmt.Errorf("variant '%s' not found for media '%s'", variantName, mediaID)
	}

	delete(media.Variants, variantName)
	media.UpdatedAt = time.Now()

	return r.UpdateMedia(ctx, media)
}

// GetUserMedia retrieves media for a user with pagination (interface compatible)
func (r *MediaRepository) GetUserMedia(ctx context.Context, userID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Media], error) {
	r.logger.Debug("getting user media with pagination", 
		zap.String("user_id", userID),
		zap.Int("limit", opts.Limit))

	return r.getUserMediaWithOptions(ctx, userID, opts, "")
}

// GetUserMediaByType retrieves media for a user filtered by content type
func (r *MediaRepository) GetUserMediaByType(ctx context.Context, userID, contentType string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Media], error) {
	r.logger.Debug("getting user media by type", 
		zap.String("user_id", userID),
		zap.String("content_type", contentType),
		zap.Int("limit", opts.Limit))

	return r.getUserMediaWithOptions(ctx, userID, opts, contentType)
}

// GetUnusedMedia retrieves media that hasn't been used since a specific time
func (r *MediaRepository) GetUnusedMedia(ctx context.Context, olderThan time.Time, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Media], error) {
	r.logger.Debug("getting unused media", 
		zap.Time("older_than", olderThan),
		zap.Int("limit", opts.Limit))

	// Query all media and filter by usage
	var mediaList []*models.Media
	query := r.db.WithContext(ctx).Model(&models.Media{})

	if opts.Limit > 0 {
		query = query.Limit(opts.Limit * 2) // Get more to account for filtering
	}

	err := query.All(&mediaList)
	if err != nil {
		return nil, fmt.Errorf("failed to get unused media: %w", err)
	}

	// Filter for unused media
	filteredMedia := make([]*models.Media, 0)
	for _, media := range mediaList {
		// Check if media is unused (never used or last used before olderThan)
		if media.UsageCount == 0 || (media.LastUsedAt != nil && media.LastUsedAt.Before(olderThan)) {
			filteredMedia = append(filteredMedia, media)
		}
	}

	// Apply pagination to filtered results
	start := 0
	if opts.Cursor != "" {
		// Simple offset-based pagination for this query
		if offset := r.parseCursor(opts.Cursor); offset > 0 && offset < len(filteredMedia) {
			start = offset
		}
	}

	end := len(filteredMedia)
	if opts.Limit > 0 && start+opts.Limit < end {
		end = start + opts.Limit
	}

	resultItems := filteredMedia[start:end]
	nextCursor := ""
	hasMore := false
	if end < len(filteredMedia) {
		nextCursor = r.encodeCursor(end)
		hasMore = true
	}

	return &interfaces.PaginatedResult[*models.Media]{
		Items:      resultItems,
		NextCursor: nextCursor,
		HasMore:    hasMore,
		Total:      int64(len(filteredMedia)),
	}, nil
}

// MarkMediaUsed marks a media item as used (increments usage count)
func (r *MediaRepository) MarkMediaUsed(ctx context.Context, mediaID string) error {
	r.logger.Debug("marking media as used", zap.String("media_id", mediaID))

	media, err := r.GetMedia(ctx, mediaID)
	if err != nil {
		return err
	}

	media.MarkUsed()
	return r.UpdateMedia(ctx, media)
}

// GetMediaUsageStats returns usage statistics for a media item
func (r *MediaRepository) GetMediaUsageStats(ctx context.Context, mediaID string) (usageCount int, lastUsed *time.Time, err error) {
	r.logger.Debug("getting media usage stats", zap.String("media_id", mediaID))

	media, err := r.GetMedia(ctx, mediaID)
	if err != nil {
		return 0, nil, err
	}

	return media.UsageCount, media.LastUsedAt, nil
}

// SetMediaModeration sets moderation results for a media item
func (r *MediaRepository) SetMediaModeration(ctx context.Context, mediaID string, isNSFW bool, score float64, labels []string) error {
	r.logger.Debug("setting media moderation",
		zap.String("media_id", mediaID),
		zap.Bool("is_nsfw", isNSFW),
		zap.Float64("score", score))

	media, err := r.GetMedia(ctx, mediaID)
	if err != nil {
		return err
	}

	media.SetModeration(isNSFW, score, labels)
	return r.UpdateMedia(ctx, media)
}

// === HELPER METHODS ===

// getMediaByStatus retrieves media by status with pagination
func (r *MediaRepository) getMediaByStatus(ctx context.Context, status string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Media], error) {
	var mediaList []*models.Media
	query := r.db.WithContext(ctx).Model(&models.Media{}).
		Where("GSI2PK", "=", fmt.Sprintf("MEDIA_STATUS#%s", status))

	if opts.Limit > 0 {
		query = query.Limit(opts.Limit)
	}

	// Add cursor-based pagination if provided
	if opts.Cursor != "" {
		query = query.Where("GSI2SK", ">", opts.Cursor)
	}

	err := query.Scan(&mediaList)
	if err != nil {
		return nil, fmt.Errorf("failed to get media by status '%s': %w", status, err)
	}

	// Build pagination result
	nextCursor := ""
	hasMore := false
	if len(mediaList) > 0 && opts.Limit > 0 && len(mediaList) == opts.Limit {
		// Use the last item's sort key as the next cursor
		lastItem := mediaList[len(mediaList)-1]
		nextCursor = lastItem.GSI2SK
		hasMore = true
	}

	return &interfaces.PaginatedResult[*models.Media]{
		Items:      mediaList,
		NextCursor: nextCursor,
		HasMore:    hasMore,
		Total:      -1, // Not calculated for performance
	}, nil
}

// getUserMediaWithOptions retrieves user media with optional content type filter
func (r *MediaRepository) getUserMediaWithOptions(ctx context.Context, userID string, opts interfaces.PaginationOptions, contentType string) (*interfaces.PaginatedResult[*models.Media], error) {
	var mediaList []*models.Media
	query := r.db.WithContext(ctx).Model(&models.Media{}).
		Where("GSI1PK", "=", fmt.Sprintf("USER_MEDIA#%s", userID))

	// Apply content type filter if provided
	if contentType != "" {
		// Normalize content type for filtering
		contentTypeKey := strings.Split(contentType, "/")[0] // "image", "video", "audio"
		query = query.Filter("ContentType", "BEGINS_WITH", contentTypeKey)
	}

	if opts.Limit > 0 {
		query = query.Limit(opts.Limit)
	}

	// Add cursor-based pagination if provided
	if opts.Cursor != "" {
		query = query.Where("GSI1SK", ">", opts.Cursor)
	}

	err := query.Scan(&mediaList)
	if err != nil {
		return nil, fmt.Errorf("failed to get user media: %w", err)
	}

	// Build pagination result
	nextCursor := ""
	hasMore := false
	if len(mediaList) > 0 && opts.Limit > 0 && len(mediaList) == opts.Limit {
		// Use the last item's sort key as the next cursor
		lastItem := mediaList[len(mediaList)-1]
		nextCursor = lastItem.GSI1SK
		hasMore = true
	}

	return &interfaces.PaginatedResult[*models.Media]{
		Items:      mediaList,
		NextCursor: nextCursor,
		HasMore:    hasMore,
		Total:      -1, // Not calculated for performance
	}, nil
}

// parseCursor parses a simple numeric cursor (for unused media query)
func (r *MediaRepository) parseCursor(cursor string) int {
	// Simple implementation for demonstration
	// In production, use proper cursor encoding/decoding
	if cursor == "" {
		return 0
	}
	// This is a simplified implementation - in production you'd want proper cursor handling
	return 0
}

// encodeCursor encodes a simple numeric cursor
func (r *MediaRepository) encodeCursor(offset int) string {
	// Simple implementation for demonstration
	// In production, use proper cursor encoding
	return fmt.Sprintf("%d", offset)
}

// GetModerationPendingMedia retrieves media items that need moderation review
func (r *MediaRepository) GetModerationPendingMedia(ctx context.Context, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Media], error) {
	r.logger.Debug("getting moderation pending media", 
		zap.Int("limit", opts.Limit))

	// Query media that needs moderation (has no moderation score or labels)
	var mediaList []*models.Media
	query := r.db.WithContext(ctx).Model(&models.Media{}).
		Filter("ModerationScore", "=", 0.0) // Assuming 0.0 means not moderated yet

	if opts.Limit > 0 {
		query = query.Limit(opts.Limit)
	}

	// Add cursor-based pagination if provided
	if opts.Cursor != "" {
		query = query.Where("CreatedAt", ">", opts.Cursor)
	}

	err := query.All(&mediaList)
	if err != nil {
		return nil, fmt.Errorf("failed to get moderation pending media: %w", err)
	}

	// Build pagination result
	nextCursor := ""
	hasMore := false
	if len(mediaList) > 0 && opts.Limit > 0 && len(mediaList) == opts.Limit {
		// Use the last item's creation time as the next cursor
		lastItem := mediaList[len(mediaList)-1]
		nextCursor = lastItem.CreatedAt.Format(time.RFC3339)
		hasMore = true
	}

	return &interfaces.PaginatedResult[*models.Media]{
		Items:      mediaList,
		NextCursor: nextCursor,
		HasMore:    hasMore,
		Total:      -1, // Not calculated for performance
	}, nil
}

// GetMediaByIDs retrieves multiple media items by their IDs
func (r *MediaRepository) GetMediaByIDs(ctx context.Context, mediaIDs []string) ([]*models.Media, error) {
	r.logger.Debug("getting media by IDs", zap.Int("count", len(mediaIDs)))

	if len(mediaIDs) == 0 {
		return []*models.Media{}, nil
	}

	var mediaList []*models.Media
	
	// Use batch get for efficiency
	// Note: DynamORM might not support batch operations directly,
	// so we'll fetch them individually for now
	for _, mediaID := range mediaIDs {
		media, err := r.GetMedia(ctx, mediaID)
		if err != nil {
			if errors.IsNotFound(err) {
				// Skip not found items
				continue
			}
			return nil, fmt.Errorf("failed to get media %s: %w", mediaID, err)
		}
		mediaList = append(mediaList, media)
	}

	return mediaList, nil
}

// DeleteExpiredMedia deletes media items that have expired
func (r *MediaRepository) DeleteExpiredMedia(ctx context.Context, expiredBefore time.Time) (int64, error) {
	r.logger.Debug("deleting expired media", zap.Time("expired_before", expiredBefore))

	// Query all media with TTL that has expired
	var mediaList []*models.Media
	query := r.db.WithContext(ctx).Model(&models.Media{}).
		Filter("ExpiresAt", "<", expiredBefore.Unix())

	err := query.All(&mediaList)
	if err != nil {
		return 0, fmt.Errorf("failed to query expired media: %w", err)
	}

	deletedCount := int64(0)
	for _, media := range mediaList {
		if err := r.DeleteMedia(ctx, media.MediaID); err != nil {
			r.logger.Error("failed to delete expired media",
				zap.String("media_id", media.MediaID),
				zap.Error(err))
			// Continue with other deletions
		} else {
			deletedCount++
		}
	}

	r.logger.Info("deleted expired media", 
		zap.Int64("deleted_count", deletedCount),
		zap.Int("total_expired", len(mediaList)))

	return deletedCount, nil
}

// GetMediaStorageUsage returns the total storage used by a user's media
func (r *MediaRepository) GetMediaStorageUsage(ctx context.Context, userID string) (int64, error) {
	r.logger.Debug("getting media storage usage", zap.String("user_id", userID))

	// Get all user media
	mediaList, err := r.GetMediaByUser(ctx, userID, 0)
	if err != nil {
		return 0, fmt.Errorf("failed to get user media for storage calculation: %w", err)
	}

	totalSize := int64(0)
	for _, media := range mediaList {
		totalSize += media.GetTotalSize() // Includes variants
	}

	return totalSize, nil
}

// GetTotalStorageUsage returns the total storage used by all media in the system
func (r *MediaRepository) GetTotalStorageUsage(ctx context.Context) (int64, error) {
	r.logger.Debug("getting total storage usage")

	// This is an expensive operation - consider caching or aggregation in production
	var mediaList []*models.Media
	err := r.db.WithContext(ctx).Model(&models.Media{}).All(&mediaList)
	if err != nil {
		return 0, fmt.Errorf("failed to get all media for total storage calculation: %w", err)
	}

	totalSize := int64(0)
	for _, media := range mediaList {
		totalSize += media.GetTotalSize() // Includes variants
	}

	r.logger.Info("calculated total storage usage", zap.Int64("total_bytes", totalSize))
	return totalSize, nil
}
