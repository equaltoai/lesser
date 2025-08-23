package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	dynamoerrors "github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// BatchDeleteConfig holds configuration for batch deletion operations
type BatchDeleteConfig struct {
	ModelType     string // "hashtag_trend", "trending_hashtag", "hashtag_usage"
	ErrorPrefix   string // Error message prefix
	BatchSize     int    // Batch size for deletion
	QueryLimit    int    // Limit for initial query
	FilterField   string // Field to filter on (e.g., "UpdatedAt", "UsedAt")
}

// deleteOldRecordsBatch is a generic helper for batch deletion of old records
func deleteOldRecordsBatch(
	ctx context.Context,
	db core.DB,
	logger *zap.Logger,
	before time.Time,
	config BatchDeleteConfig,
) (int, error) {
	switch config.ModelType {
	case "hashtag_trend":
		return deleteOldHashtagTrendRecordsBatch(ctx, db, logger, before, config)
	case "trending_hashtag":
		return deleteOldTrendingHashtagRecordsBatch(ctx, db, logger, before, config)
	case "hashtag_usage":
		return deleteOldHashtagUsageRecordsBatch(ctx, db, logger, before, config)
	default:
		return 0, ErrorHandler.HandleQueryError(fmt.Errorf("%w: %s", ErrHashtagBatchUnknownModelType, config.ModelType), EntityHashtag, config.ModelType)
	}
}

// processModelBatchDelete handles batch deletion with validation and logging for any model slice
func processModelBatchDelete[T any](ctx context.Context, db core.DB, logger *zap.Logger, models []*T, batchSize int, modelName string) int {
	if err := common.ValidateSliceNotEmpty("models", models); err != nil {
		return 0
	}

	// Convert to []any for batch operations
	items := make([]any, len(models))
	for i, model := range models {
		items[i] = model
	}

	deletedCount := processBatchDelete(ctx, db, logger, items, batchSize)
	logger.Debug(fmt.Sprintf("deleted %s records", modelName), zap.Int("count", deletedCount))
	return deletedCount
}

// deleteOldHashtagTrendRecordsBatch deletes HashtagTrend model records
func deleteOldHashtagTrendRecordsBatch(
	ctx context.Context,
	db core.DB,
	logger *zap.Logger,
	before time.Time,
	config BatchDeleteConfig,
) (int, error) {
	var trends []*models.HashtagTrend
	
	// Query old trend records using Filter and Scan
	err := db.WithContext(ctx).Model(&models.HashtagTrend{}).
		Filter(config.FilterField, "<", before.Format(time.RFC3339)).
		Limit(config.QueryLimit).
		Scan(&trends)

	if err != nil {
		if dynamoerrors.IsNotFound(err) {
			return 0, nil
		}
		return 0, ErrorHandler.HandleQueryError(err, EntityHashtag, "old hashtag trends scan")
	}

	// Use batch delete for efficiency
	deletedCount := processModelBatchDelete(ctx, db, logger, trends, config.BatchSize, "hashtag trend")
	return deletedCount, nil
}

// deleteOldTrendingHashtagRecordsBatch deletes TrendingHashtag model records
func deleteOldTrendingHashtagRecordsBatch(
	ctx context.Context,
	db core.DB,
	logger *zap.Logger,
	before time.Time,
	config BatchDeleteConfig,
) (int, error) {
	var trends []*models.TrendingHashtag

	// Query old trending hashtag records
	err := db.WithContext(ctx).Model(&models.TrendingHashtag{}).
		Filter(config.FilterField, "<", before.Format(time.RFC3339)).
		Limit(config.QueryLimit).
		Scan(&trends)

	if err != nil {
		if dynamoerrors.IsNotFound(err) {
			return 0, nil
		}
		return 0, ErrorHandler.HandleQueryError(err, EntityHashtag, "old trending hashtags scan")
	}

	// Batch delete trending hashtags
	deletedCount := processModelBatchDelete(ctx, db, logger, trends, config.BatchSize, "trending hashtag")
	return deletedCount, nil
}

// deleteOldHashtagUsageRecordsBatch removes expired hashtag usage records
func deleteOldHashtagUsageRecordsBatch(
	ctx context.Context,
	db core.DB,
	logger *zap.Logger,
	before time.Time,
	config BatchDeleteConfig,
) (int, error) {
	var usageRecords []*models.HashtagUsage

	// Query old usage records that haven't been cleaned up by TTL
	err := db.WithContext(ctx).Model(&models.HashtagUsage{}).
		Filter(config.FilterField, "<", before.Format(time.RFC3339)).
		Limit(config.QueryLimit).
		Scan(&usageRecords)

	if err != nil {
		if dynamoerrors.IsNotFound(err) {
			return 0, nil
		}
		return 0, ErrorHandler.HandleQueryError(err, EntityHashtag, "old hashtag usage scan")
	}

	// Batch delete usage records
	deletedCount := processModelBatchDelete(ctx, db, logger, usageRecords, config.BatchSize, "hashtag usage")
	return deletedCount, nil
}

// processBatchDelete performs batch delete processing
func processBatchDelete(ctx context.Context, db core.DB, logger *zap.Logger, items []any, batchSize int) int {
	var deletedCount int

	for i := 0; i < len(items); i += batchSize {
		end := i + batchSize
		if end > len(items) {
			end = len(items)
		}

		batchItems := items[i:end]
		err := deleteBatch(ctx, db, batchItems)
		if err != nil {
			logger.Warn("failed to delete batch",
				zap.Int("batch_start", i),
				zap.Int("batch_size", len(batchItems)),
				zap.Error(err))
		} else {
			deletedCount += len(batchItems)
		}
	}

	return deletedCount
}

// deleteBatch performs batch delete using DynamORM
func deleteBatch(ctx context.Context, db core.DB, items []any) error {
	if err := common.ValidateSliceNotEmpty("items", items); err != nil {
		return nil
	}

	// Use DynamORM batch delete - delete items individually
	for _, item := range items {
		if err := db.WithContext(ctx).Model(item).Delete(); err != nil {
			// Continue with other items rather than failing the whole batch
			// Note: Individual delete failures don't stop the batch process
			continue
		}
	}

	return nil
}