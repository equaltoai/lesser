package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm/batch"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"
)

// NotificationQueryHelper provides common query patterns for notifications
type NotificationQueryHelper struct {
	db        core.DB
	tableName string
	logger    *zap.Logger
}

// NewNotificationQueryHelper creates a new notification query helper
func NewNotificationQueryHelper(db core.DB, tableName string, logger *zap.Logger) *NotificationQueryHelper {
	return &NotificationQueryHelper{
		db:        db,
		tableName: tableName,
		logger:    logger,
	}
}

// buildPaginatedNotificationQuery builds a paginated query for notifications with common patterns
func (h *NotificationQueryHelper) buildPaginatedNotificationQuery(ctx context.Context, pk string, opts interfaces.PaginationOptions, additionalFilters map[string]interface{}) core.Query {
	// Validate and set default pagination options
	if opts.Limit <= 0 {
		opts.Limit = 20
	}
	if opts.Limit > 100 {
		opts.Limit = 100
	}

	// Start building the query
	query := h.db.WithContext(ctx).Model(&models.Notification{}).
		Where("PK", "=", pk).
		OrderBy("SK", "DESC")

	// Apply additional filters
	for key, value := range additionalFilters {
		query = query.Filter(key, "=", value)
	}

	// Resume from the supplied cursor value when available
	if opts.Cursor != "" {
		query = query.Where("SK", "<", opts.Cursor)
	}

	// Get one more item than requested to determine if there are more results
	query = query.Limit(opts.Limit + 1)

	return query
}

// executePaginatedNotificationQuery executes a paginated notification query and builds the result
func (h *NotificationQueryHelper) executePaginatedNotificationQuery(query core.Query, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Notification], error) {
	var notifications []models.Notification
	err := query.All(&notifications)
	if err != nil {
		return nil, err
	}

	// Build result
	result := &interfaces.PaginatedResult[*models.Notification]{
		Items: make([]*models.Notification, 0, len(notifications)),
		Total: -1, // Not calculated
	}

	// Generate next cursor
	if len(notifications) > opts.Limit {
		// We got more results than requested, so there are more pages
		result.NextCursor = notifications[opts.Limit-1].SK
		result.HasMore = true
		notifications = notifications[:opts.Limit] // Trim to requested limit
	}

	// Convert to pointers
	for i := range notifications {
		result.Items = append(result.Items, &notifications[i])
	}

	return result, nil
}

// GetPaginatedNotifications retrieves notifications with pagination using common patterns
func (h *NotificationQueryHelper) GetPaginatedNotifications(ctx context.Context, userID string, opts interfaces.PaginationOptions, additionalFilters map[string]interface{}) (*interfaces.PaginatedResult[*models.Notification], error) {
	pk := "USER#" + userID
	query := h.buildPaginatedNotificationQuery(ctx, pk, opts, additionalFilters)

	result, err := h.executePaginatedNotificationQuery(query, opts)
	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, EntityNotification, "paginated query")
	}

	return result, nil
}

// CostTrackingQueryHelper provides common query patterns for cost tracking
type CostTrackingQueryHelper struct {
	db        core.DB
	tableName string
	logger    *zap.Logger
}

// NewCostTrackingQueryHelper creates a new cost tracking query helper
func NewCostTrackingQueryHelper(db core.DB, tableName string, logger *zap.Logger) *CostTrackingQueryHelper {
	return &CostTrackingQueryHelper{
		db:        db,
		tableName: tableName,
		logger:    logger,
	}
}

// buildTimeRangeQuery builds a time range query for cost tracking with common patterns
func (h *CostTrackingQueryHelper) buildTimeRangeQuery(ctx context.Context, gsiIndex, gsiPKValue, skPrefix string, startTime, endTime time.Time, limit int) (core.Query, error) {
	startSK := fmt.Sprintf("%s#%s", skPrefix, startTime.Format(common.CompactTimeFormat))
	endSK := fmt.Sprintf("%s#%s", skPrefix, endTime.Format(common.CompactTimeFormat))

	// Extract GSI number from index name (e.g., "gsi1" -> "1")
	gsiNumber := gsiIndex[len(gsiIndex)-1:]
	gsiPKField := fmt.Sprintf("gsi%sPK", gsiNumber)
	gsiSKField := fmt.Sprintf("gsi%sSK", gsiNumber)

	query := h.db.WithContext(ctx).Model(&models.NotificationCostTracking{}).
		Index(gsiIndex).
		Where(gsiPKField, "=", gsiPKValue).
		Where(gsiSKField, ">=", startSK).
		Where(gsiSKField, "<=", endSK).
		OrderBy(gsiSKField, "DESC").
		Limit(limit)

	return query, nil
}

// GetCostTrackingByTimeRange retrieves cost tracking records within a time range using common patterns
func (h *CostTrackingQueryHelper) GetCostTrackingByTimeRange(ctx context.Context, gsiIndex, gsiPKValue, skPrefix string, startTime, endTime time.Time, limit int) ([]*models.NotificationCostTracking, error) {
	var trackingRecords []*models.NotificationCostTracking

	query, err := h.buildTimeRangeQuery(ctx, gsiIndex, gsiPKValue, skPrefix, startTime, endTime, limit)
	if err != nil {
		return nil, err
	}

	err = query.All(&trackingRecords)
	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, "cost tracking", "time range query")
	}

	return trackingRecords, nil
}

// BatchOperationHelper provides common batch operation patterns
type BatchOperationHelper struct {
	db        core.DB
	tableName string
	logger    *zap.Logger
}

// NewBatchOperationHelper creates a new batch operation helper
func NewBatchOperationHelper(db core.DB, tableName string, logger *zap.Logger) *BatchOperationHelper {
	return &BatchOperationHelper{
		db:        db,
		tableName: tableName,
		logger:    logger,
	}
}

// BatchCreateItems creates multiple items efficiently using common patterns
func (h *BatchOperationHelper) BatchCreateItems(ctx context.Context, items []interface{}, itemType string) error {
	if err := common.ValidateSliceNotEmpty(itemType, items); err != nil {
		return nil
	}

	// Prepare all items based on their type
	for _, item := range items {
		switch v := item.(type) {
		case *models.Notification:
			if err := v.BeforeCreate(); err != nil {
				return ErrorHandler.HandleCreateError(err, EntityNotification, "preparation")
			}
		case *models.Timeline:
			if err := v.BeforeCreate(); err != nil {
				return ErrorHandler.HandleCreateError(err, EntityTimelineEntry, "preparation")
			}
		default:
			itemTypeStr := fmt.Sprintf("%T", v)
			h.logger.Error("unsupported item type for batch creation", zap.String("type", itemTypeStr))
			return ErrorHandler.HandleCreateError(storage.ErrInvalidInput, "batch items", itemTypeStr)
		}
	}

	// Convert to []any for batch operations
	anyItems := make([]any, len(items))
	copy(anyItems, items)

	// Use batch writer for efficient bulk creation
	batchWriter := batch.NewBatchWriter(h.db, batch.BatchWriterConfig{
		BatchSize: batch.DefaultBatchSize,
		Logger:    h.logger,
	})

	result, err := batchWriter.WriteItems(ctx, anyItems)
	if err != nil {
		return ErrorHandler.HandleCreateError(err, itemType, "batch operation")
	}

	// Check if any items failed
	if result.FailedItems > 0 {
		h.logger.Warn(fmt.Sprintf("some %s failed to create", itemType),
			zap.Int("failed_items", result.FailedItems),
			zap.Int("total_items", result.TotalItems),
		)
		// For non-critical items, we'll continue even with some failures
	}

	return nil
}
