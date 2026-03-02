package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/theory-cloud/tabletheory/pkg/core"
	"go.uber.org/zap"
)

// BatchDeleteConfig holds configuration for batch deletion operations
type BatchDeleteConfig struct {
	ModelType   string // "hashtag_trend", "trending_hashtag", "hashtag_usage"
	ErrorPrefix string // Error message prefix
	BatchSize   int    // Batch size for deletion
	QueryLimit  int    // Limit for initial query
	FilterField string // Field to filter on (e.g., "UpdatedAt", "UsedAt")
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
	// IMPORTANT:
	// TableTheory's `.Scan(...)` issues a DynamoDB Scan, and the previous implementation used a
	// non-key attribute filter (`UpdatedAt < before`) which is a table-wide scan. Trend records
	// already write TTLs, so expiration is handled by DynamoDB TTL.
	//
	// This helper remains for compatibility but is now a no-op to prevent scan-based deletion.
	logger.Info("skipping manual hashtag trend cleanup (ttl handles expiration)",
		zap.Time("before", before),
	)
	return 0, nil

}

// deleteOldTrendingHashtagRecordsBatch deletes TrendingHashtag model records
func deleteOldTrendingHashtagRecordsBatch(
	ctx context.Context,
	db core.DB,
	logger *zap.Logger,
	before time.Time,
	config BatchDeleteConfig,
) (int, error) {
	// IMPORTANT:
	// This was previously implemented as a scan (`Filter(...).Scan(...)`) which is a table-wide scan.
	// Trending hashtag records are TTL-driven; expiration is handled by DynamoDB TTL.
	//
	// This helper remains for compatibility but is now a no-op to prevent scan-based deletion.
	logger.Info("skipping manual trending hashtag cleanup (ttl handles expiration)",
		zap.Time("before", before),
	)
	return 0, nil

}

// deleteOldHashtagUsageRecordsBatch removes expired hashtag usage records
func deleteOldHashtagUsageRecordsBatch(
	ctx context.Context,
	db core.DB,
	logger *zap.Logger,
	before time.Time,
	config BatchDeleteConfig,
) (int, error) {
	// IMPORTANT:
	// This was previously implemented as a scan (`Filter(...).Scan(...)`) which is a table-wide scan.
	// Hashtag usage records are TTL-driven; expiration is handled by DynamoDB TTL.
	//
	// This helper remains for compatibility but is now a no-op to prevent scan-based deletion.
	logger.Info("skipping manual hashtag usage cleanup (ttl handles expiration)",
		zap.Time("before", before),
	)
	return 0, nil

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
