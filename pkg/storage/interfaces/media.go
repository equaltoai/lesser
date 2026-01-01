// Package interfaces defines the repository interfaces for the Lesser application.
package interfaces

import (
	"context"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
)

// MediaRepository defines the interface for media/attachment operations.
// This handles file uploads, processing, and CDN management.
type MediaRepository interface {
	// Core media operations
	CreateMedia(ctx context.Context, media *models.Media) error
	GetMedia(ctx context.Context, mediaID string) (*models.Media, error)
	UpdateMedia(ctx context.Context, media *models.Media) error
	DeleteMedia(ctx context.Context, mediaID string) error

	// Media queries
	GetMediaByUser(ctx context.Context, userID string, limit int) ([]*models.Media, error)
	GetMediaByStatus(ctx context.Context, status string, limit int) ([]*models.Media, error)
	GetMediaByContentType(ctx context.Context, contentType string, limit int) ([]*models.Media, error)
	GetUserMediaLegacy(ctx context.Context, username string) ([]any, error)

	// User media queries (paginated)
	GetUserMedia(ctx context.Context, userID string, opts PaginationOptions) (*PaginatedResult[*models.Media], error)
	GetUserMediaByType(ctx context.Context, userID, contentType string, opts PaginationOptions) (*PaginatedResult[*models.Media], error)
	GetUnusedMedia(ctx context.Context, olderThan time.Time, opts PaginationOptions) (*PaginatedResult[*models.Media], error)

	// Media processing
	MarkMediaProcessing(ctx context.Context, mediaID string) error
	MarkMediaReady(ctx context.Context, mediaID string) error
	MarkMediaFailed(ctx context.Context, mediaID, errorMsg string) error
	GetPendingMedia(ctx context.Context, opts PaginationOptions) (*PaginatedResult[*models.Media], error)
	GetProcessingMedia(ctx context.Context, opts PaginationOptions) (*PaginatedResult[*models.Media], error)

	// Media variants and thumbnails
	AddMediaVariant(ctx context.Context, mediaID, variantName string, variant models.MediaVariant) error
	GetMediaVariant(ctx context.Context, mediaID, variantName string) (*models.MediaVariant, error)
	DeleteMediaVariant(ctx context.Context, mediaID, variantName string) error

	// Media attachment updates
	UpdateMediaAttachment(ctx context.Context, mediaID string, updates map[string]any) error
	UnmarkAllMediaAsSensitive(ctx context.Context, username string) error

	// Media usage tracking
	MarkMediaUsed(ctx context.Context, mediaID string) error
	GetMediaUsageStats(ctx context.Context, mediaID string) (usageCount int, lastUsed *time.Time, err error)

	// Content moderation
	SetMediaModeration(ctx context.Context, mediaID string, isNSFW bool, score float64, labels []string) error
	GetModerationPendingMedia(ctx context.Context, opts PaginationOptions) (*PaginatedResult[*models.Media], error)

	// Batch operations
	GetMediaByIDs(ctx context.Context, mediaIDs []string) ([]*models.Media, error)
	DeleteExpiredMedia(ctx context.Context, expiredBefore time.Time) (int64, error)

	// Storage and CDN operations
	GetMediaStorageUsage(ctx context.Context, userID string) (int64, error)
	GetTotalStorageUsage(ctx context.Context) (int64, error)

	// Media job operations
	CreateMediaJob(ctx context.Context, job *models.MediaJob) error
	GetMediaJob(ctx context.Context, jobID string) (*models.MediaJob, error)
	UpdateMediaJob(ctx context.Context, job *models.MediaJob) error
	DeleteMediaJob(ctx context.Context, jobID string) error
	GetJobsByStatus(ctx context.Context, status string, limit int) ([]*models.MediaJob, error)
	GetJobsByUser(ctx context.Context, username string, limit int) ([]*models.MediaJob, error)

	// User media configuration
	CreateUserMediaConfig(ctx context.Context, config *models.UserMediaConfig) error
	GetUserMediaConfig(ctx context.Context, userID string) (*models.UserMediaConfig, error)
	GetUserMediaConfigByUsername(ctx context.Context, username string) (*models.UserMediaConfig, error)
	UpdateUserMediaConfig(ctx context.Context, config *models.UserMediaConfig) error
	DeleteUserMediaConfig(ctx context.Context, userID string) error

	// Media spending tracking
	CreateMediaSpending(ctx context.Context, spending *models.MediaSpending) error
	GetMediaSpending(ctx context.Context, userID, period string) (*models.MediaSpending, error)
	UpdateMediaSpending(ctx context.Context, spending *models.MediaSpending) error
	GetMediaSpendingByTimeRange(ctx context.Context, userID string, periodType string, limit int) ([]*models.MediaSpending, error)
	GetOrCreateMediaSpending(ctx context.Context, userID, period, periodType string) (*models.MediaSpending, error)

	// Spending transactions
	CreateMediaSpendingTransaction(ctx context.Context, transaction *models.MediaSpendingTransaction) error
	GetMediaSpendingTransactions(ctx context.Context, userID string, limit int) ([]*models.MediaSpendingTransaction, error)
	AddSpendingTransaction(ctx context.Context, transaction *models.MediaSpendingTransaction) error

	// Transcoding job operations
	CreateTranscodingJob(ctx context.Context, job *models.TranscodingJob) error
	GetTranscodingJob(ctx context.Context, jobID string) (*models.TranscodingJob, error)
	UpdateTranscodingJob(ctx context.Context, job *models.TranscodingJob) error
	GetTranscodingJobsByUser(ctx context.Context, userID string, limit int) ([]*models.TranscodingJob, error)
	GetTranscodingJobsByMedia(ctx context.Context, mediaID string, limit int) ([]*models.TranscodingJob, error)
	GetTranscodingJobsByStatus(ctx context.Context, status string, limit int) ([]*models.TranscodingJob, error)
	DeleteTranscodingJob(ctx context.Context, jobID string) error
	GetTranscodingCostsByUser(ctx context.Context, userID string, timeRange string) (map[string]int64, error)

	// Dependencies
	SetDependencies(deps map[string]interface{})
}
