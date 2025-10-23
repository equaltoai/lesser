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
func (r *BaseRepository[T]) QueryGSIWithTimeRangeHelper(ctx context.Context, indexName, gsiPK, gsiSK, pkValue string, startTime, endTime time.Time, limit int, cursor string, order string, operationName string) ([]T, string, error) {
	var results []T

	safeLimit, clamped, usedDefault := clampLimit(limit, defaultBaseQueryLimit, maxBaseQueryLimit)
	if clamped && r.logger != nil {
		fields := []zap.Field{
			zap.String("operation", operationName),
			zap.String("index", indexName),
			zap.String("pk_value", pkValue),
			zap.Int("requested_limit", limit),
			zap.Int("applied_limit", safeLimit),
		}
		message := "time-range GSI query limit clamped"
		if usedDefault {
			message = "time-range GSI query applied default limit"
		}
		r.logger.Warn(message, fields...)
	}

	if order == "" {
		order = SortOrderDesc
	}

	startSK := startTime.Format(time.RFC3339)
	endSK := endTime.Format(time.RFC3339)

	query := r.db.WithContext(ctx).Model(new(T)).
		Index(indexName).
		Where(gsiPK, "=", pkValue).
		Where(gsiSK, ">=", startSK).
		Where(gsiSK, "<=", endSK).
		OrderBy(gsiSK, order).
		Limit(safeLimit + 1)

	if cursor != "" {
		if order == SortOrderDesc {
			query = query.Where(gsiSK, "<", cursor)
		} else {
			query = query.Where(gsiSK, ">", cursor)
		}
	}

	err := query.All(&results)

	if err != nil {
		r.logger.Error(fmt.Sprintf("failed to %s", operationName),
			zap.Error(err),
			zap.String("pk_value", pkValue),
			zap.Time("startTime", startTime),
			zap.Time("endTime", endTime))
		return nil, "", MapErrorWithContext(err, fmt.Sprintf("failed to %s", operationName))
	}

	hasMore := len(results) > safeLimit
	if hasMore {
		results = results[:safeLimit]
	}

	nextCursor := ""
	if hasMore && len(results) > 0 {
		if cursorValue, ok := extractStringField(results[len(results)-1], gsiSK); ok {
			nextCursor = cursorValue
		}
	}

	// Track cost for federation-specific queries
	if r.costService != nil {
		readUnits := int64(len(results))
		if readUnits == 0 {
			readUnits = 1
		}
		if err := r.TrackRead(ctx, operationName, readUnits); err != nil {
			r.logger.Warn("failed to track read operation", zap.Error(err))
		}
	}

	return results, nextCursor, nil
}
