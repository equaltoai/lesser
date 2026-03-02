package repositories

import (
	"context"
	"fmt"
	"time"

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

// deleteOldHashtagTrendRecordsBatch deletes HashtagTrend model records
func deleteOldHashtagTrendRecordsBatch(
	_ context.Context,
	_ core.DB,
	logger *zap.Logger,
	before time.Time,
	_ BatchDeleteConfig,
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
	_ context.Context,
	_ core.DB,
	logger *zap.Logger,
	before time.Time,
	_ BatchDeleteConfig,
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
	_ context.Context,
	_ core.DB,
	logger *zap.Logger,
	before time.Time,
	_ BatchDeleteConfig,
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
