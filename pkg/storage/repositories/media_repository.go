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
