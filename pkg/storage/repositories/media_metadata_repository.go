package repositories

import (
	"context"
	stdErrors "errors"
	"fmt"
	"time"

	"github.com/pay-theory/dynamorm/pkg/core"
	dynamormerrors "github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/cost"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

// MediaMetadataRepository handles media metadata operations using enhanced DynamORM patterns
type MediaMetadataRepository struct {
	*EnhancedBaseRepository[*models.MediaMetadata]
}

// NewMediaMetadataRepository creates a new media metadata repository with enhanced functionality and cost tracking
func NewMediaMetadataRepository(db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService) *MediaMetadataRepository {
	// Create enhanced repository for media metadata operations
	enhancedRepo := NewEnhancedBaseRepository[*models.MediaMetadata](db, tableName, logger, costService, "MediaMetadataRepository", "media_metadata")

	// Set up enhanced services for media metadata operations
	enhancedRepo.SetValidationService(NewDefaultValidationService())
	enhancedRepo.SetPermissionService(NewDefaultPermissionService())
	enhancedRepo.SetCachingService(NewInMemoryCachingService()) // Cache media metadata for performance
	enhancedRepo.SetEventService(NewDefaultEventService())

	return &MediaMetadataRepository{
		EnhancedBaseRepository: enhancedRepo,
	}
}

// CreateMediaMetadata creates a new media metadata record
func (r *MediaMetadataRepository) CreateMediaMetadata(ctx context.Context, metadata *models.MediaMetadata) error {
	r.logger.Debug("creating media metadata",
		zap.String("media_id", metadata.MediaID),
		zap.String("status", metadata.Status))

	if err := metadata.BeforeCreate(); err != nil {
		return fmt.Errorf("%w for creation: %w", ErrMediaMetadataPrepareFailed, err)
	}

	return r.ValidateAndCreate(ctx, metadata)
}

// GetMediaMetadata retrieves media metadata by media ID
func (r *MediaMetadataRepository) GetMediaMetadata(ctx context.Context, mediaID string) (*models.MediaMetadata, error) {
	r.logger.Debug("getting media metadata",
		zap.String("media_id", mediaID))

	var metadata models.MediaMetadata
	err := r.Get(ctx, fmt.Sprintf("MEDIA#%s", mediaID), "METADATA", &metadata)
	if err != nil {
		var appErr *apperrors.AppError
		if stdErrors.As(err, &appErr) && appErr.Code == apperrors.CodeNotFound {
			return nil, fmt.Errorf("%w: %s", ErrMediaMetadataNotFound, mediaID)
		}
		if dynamormerrors.IsNotFound(err) {
			return nil, fmt.Errorf("%w: %s", ErrMediaMetadataNotFound, mediaID)
		}
		return nil, fmt.Errorf("%w: %w", ErrMediaMetadataQueryFailed, err)
	}

	return &metadata, nil
}

// UpdateMediaMetadata updates an existing media metadata record
func (r *MediaMetadataRepository) UpdateMediaMetadata(ctx context.Context, metadata *models.MediaMetadata) error {
	r.logger.Debug("updating media metadata",
		zap.String("media_id", metadata.MediaID),
		zap.String("status", metadata.Status))

	if err := metadata.BeforeUpdate(); err != nil {
		return fmt.Errorf("%w for update: %w", ErrMediaMetadataPrepareFailed, err)
	}

	return r.Update(ctx, metadata)
}

// GetMediaMetadataByStatus retrieves media metadata records by processing status using GSI
func (r *MediaMetadataRepository) GetMediaMetadataByStatus(ctx context.Context, status string, limit int) ([]*models.MediaMetadata, error) {
	r.logger.Debug("getting media metadata by status",
		zap.String("status", status),
		zap.Int("limit", limit))

	// Use BaseRepository's GetDB() for complex GSI queries that BaseRepository doesn't directly support
	var metadataList []*models.MediaMetadata
	query := r.GetDB().WithContext(ctx).Model(&models.MediaMetadata{}).
		Index("gsi1").
		Where("gsi1PK", "=", fmt.Sprintf("STATUS#%s", status))

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.Scan(&metadataList)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrMediaMetadataStatusQueryFailed, err)
	}

	// Track cost for GSI query
	if r.GetCostService() != nil {
		itemCount := int64(len(metadataList))
		estimatedRU := itemCount
		if estimatedRU == 0 {
			estimatedRU = 1
		}
		operation := cost.DynamoOperation{
			Type:               "Query",
			TableName:          r.tableName,
			ConsumedReadUnits:  estimatedRU,
			ConsumedWriteUnits: 0,
			ItemCount:          itemCount,
			Timestamp:          time.Now(),
			OperationID:        fmt.Sprintf("media_metadata_byStatus_%d", time.Now().UnixNano()),
		}
		if trackErr := r.GetCostService().TrackDynamoOperation(ctx, operation); trackErr != nil {
			r.logger.Warn("failed to track media metadata by status cost", zap.Error(trackErr))
		}
	}

	return metadataList, nil
}

// GetPendingMediaMetadata retrieves media metadata records that need processing
func (r *MediaMetadataRepository) GetPendingMediaMetadata(ctx context.Context, limit int) ([]*models.MediaMetadata, error) {
	return r.GetMediaMetadataByStatus(ctx, "pending", limit)
}

// GetProcessingMediaMetadata retrieves media metadata records currently being processed
func (r *MediaMetadataRepository) GetProcessingMediaMetadata(ctx context.Context, limit int) ([]*models.MediaMetadata, error) {
	return r.GetMediaMetadataByStatus(ctx, "processing", limit)
}

// MarkProcessingStarted marks media metadata as processing started
// This is media processing business logic - preserve complete functionality
func (r *MediaMetadataRepository) MarkProcessingStarted(ctx context.Context, mediaID string) error {
	metadata, err := r.GetMediaMetadata(ctx, mediaID)
	if err != nil {
		// Create new metadata record if it doesn't exist
		metadata = &models.MediaMetadata{
			MediaID: mediaID,
			Status:  "processing",
		}
		return r.CreateMediaMetadata(ctx, metadata)
	}

	metadata.SetProcessing()
	return r.UpdateMediaMetadata(ctx, metadata)
}

// MarkProcessingComplete marks media metadata as processing complete with results
// This is media processing business logic - preserve ALL functionality including thumbnail/EXIF data
func (r *MediaMetadataRepository) MarkProcessingComplete(ctx context.Context, mediaID string, result ProcessingResult) error {
	metadata, err := r.GetMediaMetadata(ctx, mediaID)
	if err != nil {
		if !stdErrors.Is(err, ErrMediaMetadataNotFound) {
			return err
		}
		metadata = &models.MediaMetadata{MediaID: mediaID}
	}

	// Update metadata with processing results - this is the core media processing functionality
	metadata.SetComplete()
	metadata.Duration = float64(result.Duration) / 1000.0 // Convert ms to seconds
	metadata.Width = result.Width
	metadata.Height = result.Height
	metadata.Blurhash = result.Blurhash
	metadata.FileSize = int64(result.FileSize)

	// Set available qualities based on processing result - critical for media delivery
	if metadata.AvailableQualities == nil {
		metadata.AvailableQualities = make([]string, 0)
	}

	for quality := range result.Sizes {
		// Add quality if not already present
		found := false
		for _, existing := range metadata.AvailableQualities {
			if existing == quality {
				found = true
				break
			}
		}
		if !found {
			metadata.AvailableQualities = append(metadata.AvailableQualities, quality)
		}
	}

	if stdErrors.Is(err, ErrMediaMetadataNotFound) {
		return r.CreateMediaMetadata(ctx, metadata)
	}
	return r.UpdateMediaMetadata(ctx, metadata)
}

// MarkProcessingFailed marks media metadata as processing failed
// This is media processing business logic - preserve ALL error handling functionality
func (r *MediaMetadataRepository) MarkProcessingFailed(ctx context.Context, mediaID string, errorMsg string) error {
	metadata, err := r.GetMediaMetadata(ctx, mediaID)
	if err != nil {
		if !stdErrors.Is(err, ErrMediaMetadataNotFound) {
			return err
		}
		metadata = &models.MediaMetadata{MediaID: mediaID}
	}

	metadata.SetFailed()
	// Store error message in a custom field if needed, or just log it
	r.logger.Error("media processing failed",
		zap.String("media_id", mediaID),
		zap.String("error", errorMsg))

	if stdErrors.Is(err, ErrMediaMetadataNotFound) {
		return r.CreateMediaMetadata(ctx, metadata)
	}
	return r.UpdateMediaMetadata(ctx, metadata)
}

// DeleteMediaMetadata deletes media metadata by media ID
func (r *MediaMetadataRepository) DeleteMediaMetadata(ctx context.Context, mediaID string) error {
	r.logger.Debug("deleting media metadata",
		zap.String("media_id", mediaID))

	return r.Delete(ctx, fmt.Sprintf("MEDIA#%s", mediaID), "METADATA")
}

// CleanupExpiredMetadata cleans up expired media metadata records
// This is media cleanup business logic - preserve ALL cleanup functionality for failed media
func (r *MediaMetadataRepository) CleanupExpiredMetadata(ctx context.Context) error {
	r.logger.Debug("cleaning up expired media metadata")

	// Find failed media older than 7 days
	cutoffTime := time.Now().Add(-7 * 24 * time.Hour)

	var expiredMetadata []*models.MediaMetadata
	// Use GetDB() for complex GSI query with date range
	err := r.GetDB().WithContext(ctx).Model(&models.MediaMetadata{}).
		Index("gsi1").
		Where("gsi1PK", "=", "STATUS#failed").
		Where("gsi1SK", "<", fmt.Sprintf("PROCESSED#%s", cutoffTime.Format(time.RFC3339))).
		Limit(100). // Process in batches
		Scan(&expiredMetadata)

	if err != nil {
		return fmt.Errorf("%w: %w", ErrExpiredMediaMetadataQueryFailed, err)
	}

	// Delete expired records - use BaseRepository Delete method
	for _, metadata := range expiredMetadata {
		if err := r.DeleteMediaMetadata(ctx, metadata.MediaID); err != nil {
			r.logger.Error("failed to delete expired media metadata",
				zap.String("media_id", metadata.MediaID),
				zap.Error(err))
		}
	}

	r.logger.Info("cleaned up expired media metadata",
		zap.Int("count", len(expiredMetadata)))

	// Track cleanup operation cost
	if r.GetCostService() != nil {
		operation := cost.DynamoOperation{
			Type:               "BatchDelete",
			TableName:          r.tableName,
			ConsumedReadUnits:  int64(len(expiredMetadata)), // Estimate 1 RU per item found
			ConsumedWriteUnits: int64(len(expiredMetadata)), // 1 WU per item deleted
			ItemCount:          int64(len(expiredMetadata)),
			Timestamp:          time.Now(),
			OperationID:        fmt.Sprintf("media_metadata_cleanup_%d", time.Now().UnixNano()),
		}
		if trackErr := r.GetCostService().TrackDynamoOperation(ctx, operation); trackErr != nil {
			r.logger.Warn("failed to track media cleanup cost", zap.Error(trackErr))
		}
	}

	return nil
}

// ProcessingResult represents the result of media processing
// This structure is critical for media processing pipeline - preserves ALL functionality
type ProcessingResult struct {
	Width    int                 `json:"width"`
	Height   int                 `json:"height"`
	Duration int                 `json:"duration"` // Duration in milliseconds
	FileSize int                 `json:"file_size"`
	Blurhash string              `json:"blurhash"`
	Sizes    map[string]SizeInfo `json:"sizes"`
}

// SizeInfo contains information about a processed media size
// This is critical for thumbnail generation and media optimization - preserves ALL functionality
type SizeInfo struct {
	Width  int    `json:"width"`
	Height int    `json:"height"`
	S3Key  string `json:"s3_key"`
	URL    string `json:"url"`
}
