package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/storage/models"
)

// MediaMetadataRepository handles media metadata operations using DynamORM
type MediaMetadataRepository struct {
	db     core.DB
	logger *zap.Logger
}

// NewMediaMetadataRepository creates a new media metadata repository
func NewMediaMetadataRepository(db core.DB, logger *zap.Logger) *MediaMetadataRepository {
	return &MediaMetadataRepository{
		db:     db,
		logger: logger,
	}
}

// CreateMediaMetadata creates a new media metadata record
func (r *MediaMetadataRepository) CreateMediaMetadata(ctx context.Context, metadata *models.MediaMetadata) error {
	r.logger.Debug("creating media metadata",
		zap.String("media_id", metadata.MediaID),
		zap.String("status", metadata.Status))

	if err := metadata.BeforeCreate(); err != nil {
		return fmt.Errorf("failed to prepare media metadata for creation: %w", err)
	}

	return r.db.WithContext(ctx).Model(metadata).Create()
}

// GetMediaMetadata retrieves media metadata by media ID
func (r *MediaMetadataRepository) GetMediaMetadata(ctx context.Context, mediaID string) (*models.MediaMetadata, error) {
	r.logger.Debug("getting media metadata",
		zap.String("media_id", mediaID))

	var metadata models.MediaMetadata
	err := r.db.WithContext(ctx).Model(&models.MediaMetadata{}).
		Where("PK", "=", fmt.Sprintf("MEDIA#%s", mediaID)).
		Where("SK", "=", "METADATA").
		First(&metadata)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("media metadata not found: %s", mediaID)
		}
		return nil, fmt.Errorf("failed to get media metadata: %w", err)
	}

	return &metadata, nil
}

// UpdateMediaMetadata updates an existing media metadata record
func (r *MediaMetadataRepository) UpdateMediaMetadata(ctx context.Context, metadata *models.MediaMetadata) error {
	r.logger.Debug("updating media metadata",
		zap.String("media_id", metadata.MediaID),
		zap.String("status", metadata.Status))

	if err := metadata.BeforeUpdate(); err != nil {
		return fmt.Errorf("failed to prepare media metadata for update: %w", err)
	}

	return r.db.WithContext(ctx).Model(metadata).Update()
}

// GetMediaMetadataByStatus retrieves media metadata records by processing status
func (r *MediaMetadataRepository) GetMediaMetadataByStatus(ctx context.Context, status string, limit int) ([]*models.MediaMetadata, error) {
	r.logger.Debug("getting media metadata by status",
		zap.String("status", status),
		zap.Int("limit", limit))

	var metadataList []*models.MediaMetadata
	query := r.db.WithContext(ctx).Model(&models.MediaMetadata{}).
		Index("gsi1-index").
		Where("GSI1PK", "=", fmt.Sprintf("STATUS#%s", status))

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.Scan(&metadataList)
	if err != nil {
		return nil, fmt.Errorf("failed to get media metadata by status: %w", err)
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
func (r *MediaMetadataRepository) MarkProcessingComplete(ctx context.Context, mediaID string, result ProcessingResult) error {
	metadata, err := r.GetMediaMetadata(ctx, mediaID)
	if err != nil {
		// Create new metadata record if it doesn't exist
		metadata = &models.MediaMetadata{
			MediaID: mediaID,
		}
	}

	// Update metadata with processing results
	metadata.SetComplete()
	metadata.Duration = float64(result.Duration) / 1000.0 // Convert ms to seconds
	metadata.Width = result.Width
	metadata.Height = result.Height
	metadata.Blurhash = result.Blurhash
	metadata.FileSize = int64(result.FileSize)

	// Set available qualities based on processing result
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

	if metadata.PK == "" {
		return r.CreateMediaMetadata(ctx, metadata)
	}
	return r.UpdateMediaMetadata(ctx, metadata)
}

// MarkProcessingFailed marks media metadata as processing failed
func (r *MediaMetadataRepository) MarkProcessingFailed(ctx context.Context, mediaID string, errorMsg string) error {
	metadata, err := r.GetMediaMetadata(ctx, mediaID)
	if err != nil {
		// Create new metadata record if it doesn't exist
		metadata = &models.MediaMetadata{
			MediaID: mediaID,
		}
	}

	metadata.SetFailed()
	// Store error message in a custom field if needed, or just log it
	r.logger.Error("media processing failed",
		zap.String("media_id", mediaID),
		zap.String("error", errorMsg))

	if metadata.PK == "" {
		return r.CreateMediaMetadata(ctx, metadata)
	}
	return r.UpdateMediaMetadata(ctx, metadata)
}

// DeleteMediaMetadata deletes media metadata by media ID
func (r *MediaMetadataRepository) DeleteMediaMetadata(ctx context.Context, mediaID string) error {
	r.logger.Debug("deleting media metadata",
		zap.String("media_id", mediaID))

	metadata := &models.MediaMetadata{
		PK: fmt.Sprintf("MEDIA#%s", mediaID),
		SK: "METADATA",
	}

	return r.db.WithContext(ctx).Model(metadata).Delete()
}

// CleanupExpiredMetadata cleans up expired media metadata records
func (r *MediaMetadataRepository) CleanupExpiredMetadata(ctx context.Context) error {
	r.logger.Debug("cleaning up expired media metadata")

	// Find failed media older than 7 days
	cutoffTime := time.Now().Add(-7 * 24 * time.Hour)

	var expiredMetadata []*models.MediaMetadata
	err := r.db.WithContext(ctx).Model(&models.MediaMetadata{}).
		Index("gsi1-index").
		Where("GSI1PK", "=", "STATUS#failed").
		Where("GSI1SK", "<", fmt.Sprintf("PROCESSED#%s", cutoffTime.Format(time.RFC3339))).
		Limit(100). // Process in batches
		Scan(&expiredMetadata)

	if err != nil {
		return fmt.Errorf("failed to find expired media metadata: %w", err)
	}

	// Delete expired records
	for _, metadata := range expiredMetadata {
		if err := r.DeleteMediaMetadata(ctx, metadata.MediaID); err != nil {
			r.logger.Error("failed to delete expired media metadata",
				zap.String("media_id", metadata.MediaID),
				zap.Error(err))
		}
	}

	r.logger.Info("cleaned up expired media metadata",
		zap.Int("count", len(expiredMetadata)))

	return nil
}

// ProcessingResult represents the result of media processing
type ProcessingResult struct {
	Width    int                 `json:"width"`
	Height   int                 `json:"height"`
	Duration int                 `json:"duration"` // Duration in milliseconds
	FileSize int                 `json:"file_size"`
	Blurhash string              `json:"blurhash"`
	Sizes    map[string]SizeInfo `json:"sizes"`
}

// SizeInfo contains information about a processed media size
type SizeInfo struct {
	Width  int    `json:"width"`
	Height int    `json:"height"`
	S3Key  string `json:"s3_key"`
	URL    string `json:"url"`
}
