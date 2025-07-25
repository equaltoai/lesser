package cost

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"
)

// DynamORMCostTracker wraps a DynamORM client with cost tracking capabilities
type DynamORMCostTracker struct {
	*Tracker // Embed existing cost tracker
	client   core.DB
	logger   *zap.Logger
	mu       sync.RWMutex
}

// NewDynamORMCostTracker creates a new cost tracking wrapper for DynamORM
func NewDynamORMCostTracker(client core.DB, logger *zap.Logger) *DynamORMCostTracker {
	return &DynamORMCostTracker{
		Tracker: New(),
		client:  client,
		logger:  logger,
	}
}

// NewDynamORMCostTrackerWithRequest creates a new cost tracking wrapper with request context
func NewDynamORMCostTrackerWithRequest(client core.DB, requestID, operationType string, logger *zap.Logger) *DynamORMCostTracker {
	return &DynamORMCostTracker{
		Tracker: NewWithRequest(requestID, operationType),
		client:  client,
		logger:  logger,
	}
}

// TrackOperation wraps a DynamORM operation with cost tracking
func (ct *DynamORMCostTracker) TrackOperation(ctx context.Context, operation string, fn func() error) error {
	startTime := time.Now()

	// Get initial consumed capacity from existing tracker
	initialReads := ct.dynamoReads.Load()
	initialWrites := ct.dynamoWrites.Load()

	// Execute operation
	err := fn()

	// Calculate consumed capacity difference
	finalReads := ct.dynamoReads.Load()
	finalWrites := ct.dynamoWrites.Load()

	consumedReads := finalReads - initialReads
	consumedWrites := finalWrites - initialWrites

	// Log operation details if logger is available
	if ct.logger != nil {
		ct.logger.Debug("dynamodb_operation_tracked",
			zap.String("operation", operation),
			zap.Int64("consumed_reads", consumedReads),
			zap.Int64("consumed_writes", consumedWrites),
			zap.Duration("duration", time.Since(startTime)),
			zap.Error(err),
		)
	}

	// Store cost in context using existing method if context has tracker
	if contextTracker := FromContext(ctx); contextTracker != nil {
		contextTracker.TrackDynamoRead(int(consumedReads))
		contextTracker.TrackDynamoWrite(int(consumedWrites))
	}

	return err
}

// TrackQuery wraps a DynamORM query operation with cost tracking
func (ct *DynamORMCostTracker) TrackQuery(ctx context.Context, tableName string, fn func() error) error {
	return ct.TrackOperation(ctx, fmt.Sprintf("query_%s", tableName), fn)
}

// TrackPut wraps a DynamORM put operation with cost tracking
func (ct *DynamORMCostTracker) TrackPut(ctx context.Context, tableName string, fn func() error) error {
	return ct.TrackOperation(ctx, fmt.Sprintf("put_%s", tableName), func() error {
		err := fn()
		if err == nil {
			ct.TrackDynamoWrite(1) // One write unit for put
		}
		return err
	})
}

// TrackUpdate wraps a DynamORM update operation with cost tracking
func (ct *DynamORMCostTracker) TrackUpdate(ctx context.Context, tableName string, fn func() error) error {
	return ct.TrackOperation(ctx, fmt.Sprintf("update_%s", tableName), func() error {
		err := fn()
		if err == nil {
			ct.TrackDynamoWrite(1) // One write unit for update
		}
		return err
	})
}

// TrackDelete wraps a DynamORM delete operation with cost tracking
func (ct *DynamORMCostTracker) TrackDelete(ctx context.Context, tableName string, fn func() error) error {
	return ct.TrackOperation(ctx, fmt.Sprintf("delete_%s", tableName), func() error {
		err := fn()
		if err == nil {
			ct.TrackDynamoWrite(1) // One write unit for delete
		}
		return err
	})
}

// TrackBatchWrite wraps a DynamORM batch write operation with cost tracking
func (ct *DynamORMCostTracker) TrackBatchWrite(ctx context.Context, tableName string, itemCount int, fn func() error) error {
	return ct.TrackOperation(ctx, fmt.Sprintf("batch_write_%s", tableName), func() error {
		err := fn()
		if err == nil {
			ct.TrackDynamoWrite(itemCount) // Track actual item count
		}
		return err
	})
}

// TrackTransaction wraps a DynamORM transaction with cost tracking
func (ct *DynamORMCostTracker) TrackTransaction(ctx context.Context, operationCount int, fn func() error) error {
	return ct.TrackOperation(ctx, "transaction", func() error {
		err := fn()
		if err == nil {
			// Transactions consume write capacity for each operation
			ct.TrackDynamoWrite(operationCount)
		}
		return err
	})
}

// GetClient returns the underlying DynamORM client
func (ct *DynamORMCostTracker) GetClient() core.DB {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	return ct.client
}

// GetCostSummary returns a summary of tracked costs
func (ct *DynamORMCostTracker) GetCostSummary() *OperationCost {
	return ct.CalculateCost()
}

// Reset resets the cost tracking counters
func (ct *DynamORMCostTracker) Reset() {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	ct.Tracker.Reset()
}

// WrapWithCostTracking wraps a DynamORM client with cost tracking
func WrapWithCostTracking(client core.DB, logger *zap.Logger) *DynamORMCostTracker {
	return NewDynamORMCostTracker(client, logger)
}

// WrapWithCostTrackingAndRequest wraps a DynamORM client with cost tracking and request context
func WrapWithCostTrackingAndRequest(client core.DB, requestID, operationType string, logger *zap.Logger) *DynamORMCostTracker {
	return NewDynamORMCostTrackerWithRequest(client, requestID, operationType, logger)
}

// TrackingDB implements the core.DB interface with cost tracking
type TrackingDB struct {
	core.DB
	tracker *Tracker
	logger  *zap.Logger
}

// NewTrackingDB creates a new cost tracking DB wrapper
func NewTrackingDB(db core.DB, tracker *Tracker, logger *zap.Logger) *TrackingDB {
	return &TrackingDB{
		DB:      db,
		tracker: tracker,
		logger:  logger,
	}
}

// Model wraps the Model method with cost tracking context
func (ctdb *TrackingDB) Model(model any) core.Query {
	// Return a cost tracking query wrapper
	return &TrackingQuery{
		query:   ctdb.DB.Model(model),
		tracker: ctdb.tracker,
		logger:  ctdb.logger,
	}
}

// Transaction wraps the Transaction method with cost tracking
func (ctdb *TrackingDB) Transaction(fn func(*core.Tx) error) error {
	operationCount := 1 // Estimate - could be enhanced to count actual operations

	err := ctdb.DB.Transaction(fn)
	if err == nil && ctdb.tracker != nil {
		ctdb.tracker.TrackDynamoWrite(operationCount * 2) // Transactions use 2x WCU
	}

	if ctdb.logger != nil {
		ctdb.logger.Debug("dynamodb_transaction_tracked",
			zap.Int("operation_count", operationCount),
			zap.Error(err),
		)
	}

	return err
}

// GetTracker returns the cost tracker
func (ctdb *TrackingDB) GetTracker() *Tracker {
	return ctdb.tracker
}

// TrackingQuery wraps core.Query with cost tracking
type TrackingQuery struct {
	query   core.Query
	tracker *Tracker
	logger  *zap.Logger
}

// Implement all Query interface methods for proper chaining

// Where wraps the Where method
func (ctq *TrackingQuery) Where(field string, op string, value any) core.Query {
	ctq.query = ctq.query.Where(field, op, value)
	return ctq
}

// Index wraps the Index method
func (ctq *TrackingQuery) Index(indexName string) core.Query {
	ctq.query = ctq.query.Index(indexName)
	return ctq
}

// Filter wraps the Filter method
func (ctq *TrackingQuery) Filter(field string, op string, value any) core.Query {
	ctq.query = ctq.query.Filter(field, op, value)
	return ctq
}

// OrFilter wraps the OrFilter method
func (ctq *TrackingQuery) OrFilter(field string, op string, value any) core.Query {
	ctq.query = ctq.query.OrFilter(field, op, value)
	return ctq
}

// FilterGroup wraps the FilterGroup method
func (ctq *TrackingQuery) FilterGroup(fn func(core.Query)) core.Query {
	ctq.query = ctq.query.FilterGroup(fn)
	return ctq
}

// OrFilterGroup wraps the OrFilterGroup method
func (ctq *TrackingQuery) OrFilterGroup(fn func(core.Query)) core.Query {
	ctq.query = ctq.query.OrFilterGroup(fn)
	return ctq
}

// OrderBy wraps the OrderBy method
func (ctq *TrackingQuery) OrderBy(field string, order string) core.Query {
	ctq.query = ctq.query.OrderBy(field, order)
	return ctq
}

// Limit wraps the Limit method
func (ctq *TrackingQuery) Limit(limit int) core.Query {
	ctq.query = ctq.query.Limit(limit)
	return ctq
}

// Offset wraps the Offset method
func (ctq *TrackingQuery) Offset(offset int) core.Query {
	ctq.query = ctq.query.Offset(offset)
	return ctq
}

// Select wraps the Select method
func (ctq *TrackingQuery) Select(fields ...string) core.Query {
	ctq.query = ctq.query.Select(fields...)
	return ctq
}

// ConsistentRead wraps the ConsistentRead method
func (ctq *TrackingQuery) ConsistentRead() core.Query {
	ctq.query = ctq.query.ConsistentRead()
	return ctq
}

// WithRetry wraps the WithRetry method
func (ctq *TrackingQuery) WithRetry(maxRetries int, initialDelay time.Duration) core.Query {
	ctq.query = ctq.query.WithRetry(maxRetries, initialDelay)
	return ctq
}

// Terminal methods with cost tracking

// First wraps the First method with cost tracking
func (ctq *TrackingQuery) First(dest any) error {
	err := ctq.query.First(dest)
	if err == nil && ctq.tracker != nil {
		ctq.tracker.TrackDynamoRead(1) // One read unit for first
	}

	if ctq.logger != nil {
		ctq.logger.Debug("dynamodb_first_tracked",
			zap.Int("read_units", 1),
			zap.Error(err),
		)
	}

	return err
}

// All wraps the All method with cost tracking
func (ctq *TrackingQuery) All(dest any) error {
	err := ctq.query.All(dest)
	if err == nil && ctq.tracker != nil {
		// Estimate read units - in practice, this could be enhanced
		// by counting items in the result or using query metadata
		ctq.tracker.TrackDynamoRead(10) // Conservative estimate
	}

	if ctq.logger != nil {
		ctq.logger.Debug("dynamodb_all_tracked",
			zap.Int("read_units", 10),
			zap.Error(err),
		)
	}

	return err
}

// AllPaginated wraps the AllPaginated method with cost tracking
func (ctq *TrackingQuery) AllPaginated(dest any) (*core.PaginatedResult, error) {
	result, err := ctq.query.AllPaginated(dest)
	if err == nil && ctq.tracker != nil {
		// Track based on actual items returned if possible
		readUnits := 10 // Conservative estimate
		if result != nil && result.Count > 0 {
			readUnits = result.Count
		}
		ctq.tracker.TrackDynamoRead(readUnits)
	}

	if ctq.logger != nil {
		readUnits := 10
		if result != nil {
			readUnits = result.Count
		}
		ctq.logger.Debug("dynamodb_all_paginated_tracked",
			zap.Int("read_units", readUnits),
			zap.Error(err),
		)
	}

	return result, err
}

// Count wraps the Count method with cost tracking
func (ctq *TrackingQuery) Count() (int64, error) {
	count, err := ctq.query.Count()
	if err == nil && ctq.tracker != nil {
		ctq.tracker.TrackDynamoRead(1) // Count operations typically use 1 RCU minimum
	}

	if ctq.logger != nil {
		ctq.logger.Debug("dynamodb_count_tracked",
			zap.Int("read_units", 1),
			zap.Int64("count", count),
			zap.Error(err),
		)
	}

	return count, err
}

// Create wraps the Create method with cost tracking
func (ctq *TrackingQuery) Create() error {
	err := ctq.query.Create()
	if err == nil && ctq.tracker != nil {
		ctq.tracker.TrackDynamoWrite(1) // One write unit for create
	}

	if ctq.logger != nil {
		ctq.logger.Debug("dynamodb_create_tracked",
			zap.Int("write_units", 1),
			zap.Error(err),
		)
	}

	return err
}

// CreateOrUpdate wraps the CreateOrUpdate method with cost tracking
func (ctq *TrackingQuery) CreateOrUpdate() error {
	err := ctq.query.CreateOrUpdate()
	if err == nil && ctq.tracker != nil {
		ctq.tracker.TrackDynamoWrite(1) // One write unit for create or update
	}

	if ctq.logger != nil {
		ctq.logger.Debug("dynamodb_create_or_update_tracked",
			zap.Int("write_units", 1),
			zap.Error(err),
		)
	}

	return err
}

// Update wraps the Update method with cost tracking
func (ctq *TrackingQuery) Update(fields ...string) error {
	err := ctq.query.Update(fields...)
	if err == nil && ctq.tracker != nil {
		ctq.tracker.TrackDynamoWrite(1) // One write unit for update
	}

	if ctq.logger != nil {
		ctq.logger.Debug("dynamodb_update_tracked",
			zap.Int("write_units", 1),
			zap.Strings("fields", fields),
			zap.Error(err),
		)
	}

	return err
}

// UpdateBuilder wraps the UpdateBuilder method
func (ctq *TrackingQuery) UpdateBuilder() core.UpdateBuilder {
	// Return the underlying update builder - cost tracking happens at execution
	return ctq.query.UpdateBuilder()
}

// Delete wraps the Delete method with cost tracking
func (ctq *TrackingQuery) Delete() error {
	err := ctq.query.Delete()
	if err == nil && ctq.tracker != nil {
		ctq.tracker.TrackDynamoWrite(1) // One write unit for delete
	}

	if ctq.logger != nil {
		ctq.logger.Debug("dynamodb_delete_tracked",
			zap.Int("write_units", 1),
			zap.Error(err),
		)
	}

	return err
}

// Helper functions for easy integration

// TrackDynamORMOperation is a convenience function for tracking DynamORM operations
func TrackDynamORMOperation(ctx context.Context, operation string, fn func() error) error {
	tracker := FromContext(ctx)
	if tracker == nil {
		// No tracker in context, execute without tracking
		return fn()
	}

	err := fn()

	// Track operation cost if successful
	if err == nil {
		// This is a basic implementation - in practice, you'd want to extract
		// actual consumed capacity from DynamoDB response metadata
		switch operation {
		case "put", "update", "delete", "create":
			tracker.TrackDynamoWrite(1)
		case "query", "scan", "get", "first", "all", "count":
			tracker.TrackDynamoRead(1)
		}
	}

	return err
}

// WithDynamORMCostTracking adds cost tracking to a context
func WithDynamORMCostTracking(ctx context.Context, requestID, operationType string) context.Context {
	tracker := NewWithRequest(requestID, operationType)
	return WithTracker(ctx, tracker)
}

// Pagination methods

// Cursor wraps the Cursor method
func (ctq *TrackingQuery) Cursor(cursor string) core.Query {
	ctq.query = ctq.query.Cursor(cursor)
	return ctq
}

// SetCursor wraps the SetCursor method
func (ctq *TrackingQuery) SetCursor(cursor string) error {
	return ctq.query.SetCursor(cursor)
}

// Context method

// WithContext wraps the WithContext method
func (ctq *TrackingQuery) WithContext(ctx context.Context) core.Query {
	ctq.query = ctq.query.WithContext(ctx)
	return ctq
}

// Scan operations

// Scan wraps the Scan method with cost tracking
func (ctq *TrackingQuery) Scan(dest any) error {
	err := ctq.query.Scan(dest)
	if err == nil && ctq.tracker != nil {
		// Scans are expensive - estimate high RCU usage
		ctq.tracker.TrackDynamoRead(100) // Conservative estimate for scan
	}

	if ctq.logger != nil {
		ctq.logger.Debug("dynamodb_scan_tracked",
			zap.Int("read_units", 100),
			zap.Error(err),
		)
	}

	return err
}

// ParallelScan wraps the ParallelScan method
func (ctq *TrackingQuery) ParallelScan(segment int32, totalSegments int32) core.Query {
	ctq.query = ctq.query.ParallelScan(segment, totalSegments)
	return ctq
}

// ScanAllSegments wraps the ScanAllSegments method with cost tracking
func (ctq *TrackingQuery) ScanAllSegments(dest any, totalSegments int32) error {
	err := ctq.query.ScanAllSegments(dest, totalSegments)
	if err == nil && ctq.tracker != nil {
		// Parallel scan across segments - estimate high usage
		estimatedReads := int(totalSegments) * 100
		ctq.tracker.TrackDynamoRead(estimatedReads)
	}

	if ctq.logger != nil {
		estimatedReads := int(totalSegments) * 100
		ctq.logger.Debug("dynamodb_scan_all_segments_tracked",
			zap.Int("read_units", estimatedReads),
			zap.Int32("total_segments", totalSegments),
			zap.Error(err),
		)
	}

	return err
}

// Batch operations

// BatchGet wraps the BatchGet method with cost tracking
func (ctq *TrackingQuery) BatchGet(keys []any, dest any) error {
	err := ctq.query.BatchGet(keys, dest)
	if err == nil && ctq.tracker != nil {
		ctq.tracker.TrackDynamoRead(len(keys)) // One read unit per key
	}

	if ctq.logger != nil {
		ctq.logger.Debug("dynamodb_batch_get_tracked",
			zap.Int("read_units", len(keys)),
			zap.Int("key_count", len(keys)),
			zap.Error(err),
		)
	}

	return err
}

// BatchCreate wraps the BatchCreate method with cost tracking
func (ctq *TrackingQuery) BatchCreate(items any) error {
	// Count items for cost tracking
	itemCount := 1 // Default
	// Use reflection to count if items is a slice
	if reflect.TypeOf(items).Kind() == reflect.Slice {
		itemCount = reflect.ValueOf(items).Len()
	}

	err := ctq.query.BatchCreate(items)
	if err == nil && ctq.tracker != nil {
		ctq.tracker.TrackDynamoWrite(itemCount)
	}

	if ctq.logger != nil {
		ctq.logger.Debug("dynamodb_batch_create_tracked",
			zap.Int("write_units", itemCount),
			zap.Int("item_count", itemCount),
			zap.Error(err),
		)
	}

	return err
}

// BatchDelete wraps the BatchDelete method with cost tracking
func (ctq *TrackingQuery) BatchDelete(keys []any) error {
	err := ctq.query.BatchDelete(keys)
	if err == nil && ctq.tracker != nil {
		ctq.tracker.TrackDynamoWrite(len(keys)) // One write unit per key
	}

	if ctq.logger != nil {
		ctq.logger.Debug("dynamodb_batch_delete_tracked",
			zap.Int("write_units", len(keys)),
			zap.Int("key_count", len(keys)),
			zap.Error(err),
		)
	}

	return err
}

// BatchWrite wraps the BatchWrite method with cost tracking
func (ctq *TrackingQuery) BatchWrite(putItems []any, deleteKeys []any) error {
	err := ctq.query.BatchWrite(putItems, deleteKeys)
	if err == nil && ctq.tracker != nil {
		totalWrites := len(putItems) + len(deleteKeys)
		ctq.tracker.TrackDynamoWrite(totalWrites)
	}

	if ctq.logger != nil {
		totalWrites := len(putItems) + len(deleteKeys)
		ctq.logger.Debug("dynamodb_batch_write_tracked",
			zap.Int("write_units", totalWrites),
			zap.Int("put_items", len(putItems)),
			zap.Int("delete_keys", len(deleteKeys)),
			zap.Error(err),
		)
	}

	return err
}

// BatchUpdateWithOptions wraps the BatchUpdateWithOptions method with cost tracking
func (ctq *TrackingQuery) BatchUpdateWithOptions(items []any, fields []string, options ...any) error {
	err := ctq.query.BatchUpdateWithOptions(items, fields, options...)
	if err == nil && ctq.tracker != nil {
		ctq.tracker.TrackDynamoWrite(len(items)) // One write unit per item
	}

	if ctq.logger != nil {
		ctq.logger.Debug("dynamodb_batch_update_tracked",
			zap.Int("write_units", len(items)),
			zap.Int("item_count", len(items)),
			zap.Strings("fields", fields),
			zap.Error(err),
		)
	}

	return err
}
