package repositories

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// BatchCreateHelper performs batch creation for any model type with common validation and logging
func (r *BaseRepository[T]) BatchCreateHelper(ctx context.Context, items []T, entityType string) error {
	if len(items) == 0 {
		return nil
	}

	// Prepare all items
	for _, item := range items {
		if err := item.UpdateKeys(); err != nil {
			return ErrorHandler.HandleCreateError(err, entityType, item.GetPK())
		}
	}

	// Convert to []any for DynamORM batch operations
	batchItems := make([]any, len(items))
	for i, item := range items {
		batchItems[i] = item
	}

	// Use DynamORM's batch create functionality
	err := r.db.WithContext(ctx).Model(new(T)).BatchCreate(batchItems)
	if err != nil {
		r.logger.Error("batch create failed",
			zap.String("entity_type", entityType),
			zap.Int("count", len(items)),
			zap.Error(err))
		return ErrorHandler.HandleCreateError(err, entityType, "batch")
	}

	r.logger.Debug("batch created items",
		zap.String("entity_type", entityType),
		zap.Int("count", len(items)))

	return nil
}

// QueryGSIWithTimeRangeHelper provides common GSI query pattern with time range filtering
func (r *BaseRepository[T]) QueryGSIWithTimeRangeHelper(ctx context.Context, indexName, gsiPK, gsiSK, pkValue string, startTime, endTime time.Time, limit int, operationName string) ([]T, error) {
	var results []T

	startSK := startTime.Format(time.RFC3339)
	endSK := endTime.Format(time.RFC3339)

	query := r.db.WithContext(ctx).Model(new(T)).
		Index(indexName).
		Where(gsiPK, "=", pkValue).
		Where(gsiSK, ">=", startSK).
		Where(gsiSK, "<=", endSK).
		OrderBy(gsiSK, "DESC").
		Limit(limit)

	err := query.All(&results)

	// Track cost for federation-specific queries
	if r.costService != nil {
		if err := r.TrackRead(ctx, operationName, int64(len(results))); err != nil {
			r.logger.Warn("failed to track read operation", zap.Error(err))
		}
	}

	if err != nil {
		r.logger.Error(fmt.Sprintf("failed to %s", operationName),
			zap.Error(err),
			zap.String("pk_value", pkValue),
			zap.Time("startTime", startTime),
			zap.Time("endTime", endTime))
		return nil, MapErrorWithContext(err, fmt.Sprintf("failed to %s", operationName))
	}

	return results, nil
}
