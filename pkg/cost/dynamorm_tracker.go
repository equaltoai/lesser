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

// DynamoDB operation type constants
const (
	OperationTypeScan = "Scan"
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
		if err := contextTracker.TrackDynamoRead(int(consumedReads)); err != nil {
			// Log tracking failure but don't fail the DB operation
			zap.L().Warn("failed to track DynamoDB read cost", zap.Error(err))
		}
		if err := contextTracker.TrackDynamoWrite(int(consumedWrites)); err != nil {
			// Log tracking failure but don't fail the DB operation
			zap.L().Warn("failed to track DynamoDB write cost", zap.Error(err))
		}
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
			_ = ct.TrackDynamoWrite(1) // One write unit for put
		}
		return err
	})
}

// TrackUpdate wraps a DynamORM update operation with cost tracking
func (ct *DynamORMCostTracker) TrackUpdate(ctx context.Context, tableName string, fn func() error) error {
	return ct.TrackOperation(ctx, fmt.Sprintf("update_%s", tableName), func() error {
		err := fn()
		if err == nil {
			_ = ct.TrackDynamoWrite(1) // One write unit for update
		}
		return err
	})
}

// TrackDelete wraps a DynamORM delete operation with cost tracking
func (ct *DynamORMCostTracker) TrackDelete(ctx context.Context, tableName string, fn func() error) error {
	return ct.TrackOperation(ctx, fmt.Sprintf("delete_%s", tableName), func() error {
		err := fn()
		if err == nil {
			_ = ct.TrackDynamoWrite(1) // One write unit for delete
		}
		return err
	})
}

// TrackBatchWrite wraps a DynamORM batch write operation with cost tracking
func (ct *DynamORMCostTracker) TrackBatchWrite(ctx context.Context, tableName string, itemCount int, fn func() error) error {
	return ct.TrackOperation(ctx, fmt.Sprintf("batch_write_%s", tableName), func() error {
		err := fn()
		if err == nil {
			_ = ct.TrackDynamoWrite(itemCount) // Track actual item count
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
			_ = ct.TrackDynamoWrite(operationCount)
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

// Model wraps the Model method with enhanced cost tracking context
func (ctdb *TrackingDB) Model(model any) core.Query {
	// Initialize enhanced tracking
	enhancedTracker := NewEnhancedOperationTracker(ctdb.logger)

	// Initialize operation metadata
	metadata := &OperationMetadata{
		OperationType:     "unknown", // Will be set when operation executes
		TableName:         extractTableName(model),
		ItemCount:         0,
		FilterExpressions: make([]string, 0),
		ProjectionFields:  make([]string, 0),
		Conditions:        make([]QueryCondition, 0),
	}

	// Return enhanced cost tracking query wrapper
	return &TrackingQuery{
		query:             ctdb.DB.Model(model),
		tracker:           ctdb.tracker,
		logger:            ctdb.logger,
		enhancedTracker:   enhancedTracker,
		operationMetadata: metadata,
	}
}

// Transaction wraps the Transaction method with enhanced operation estimation
func (ctdb *TrackingDB) Transaction(fn func(*core.Tx) error) error {
	err := ctdb.DB.Transaction(fn)

	// For transactions, we'll use a conservative estimate since precise counting
	// would require implementing the full core.Tx interface wrapper
	// Most transactions contain 2-5 operations on average
	estimatedOperations := 3 // Conservative baseline

	if err == nil && ctdb.tracker != nil {
		// Track estimated transaction cost
		// Transactions typically have both read and write operations
		if trackErr := ctdb.tracker.TrackDynamoRead(1); trackErr != nil {
			ctdb.logger.Warn("failed to track transaction read cost", zap.Error(trackErr))
		}
		if trackErr := ctdb.tracker.TrackDynamoWrite(estimatedOperations); trackErr != nil {
			ctdb.logger.Warn("failed to track transaction write cost", zap.Error(trackErr))
		}
	}

	if ctdb.logger != nil {
		ctdb.logger.Debug("dynamodb_transaction_tracked",
			zap.Int("estimated_operations", estimatedOperations),
			zap.String("note", "Enhanced precision planned for future implementation"),
			zap.Error(err),
		)
	}

	return err
}

// GetTracker returns the cost tracker
func (ctdb *TrackingDB) GetTracker() *Tracker {
	return ctdb.tracker
}

// TrackComprehendRequest tracks AWS Comprehend API requests for cost tracking
func (ct *DynamORMCostTracker) TrackComprehendRequest(operation string, units int) {
	// Track as a generic AWS service request
	// Comprehend pricing is typically per unit (100 characters for text, per request for other operations)
	ct.mu.Lock()
	defer ct.mu.Unlock()

	// Log the Comprehend request for cost tracking
	if ct.logger != nil {
		ct.logger.Debug("comprehend_request_tracked",
			zap.String("operation", operation),
			zap.Int("units", units),
		)
	}

	// You could extend the Tracker to have specific Comprehend metrics
	// For now, we'll track it as a generic operation
	// Comprehend costs approximately $0.0001 per unit for sentiment analysis
	// and $0.001 per unit for entity recognition
}

// TrackTranscribeRequest tracks AWS Transcribe API requests for cost tracking
func (ct *DynamORMCostTracker) TrackTranscribeRequest(jobName string, estimatedMinutes int) {
	// Track as a generic AWS service request
	// Transcribe pricing is typically per minute of audio
	ct.mu.Lock()
	defer ct.mu.Unlock()

	// Log the Transcribe request for cost tracking
	if ct.logger != nil {
		ct.logger.Debug("transcribe_request_tracked",
			zap.String("job_name", jobName),
			zap.Int("estimated_minutes", estimatedMinutes),
		)
	}

	// You could extend the Tracker to have specific Transcribe metrics
	// For now, we'll track it as a generic operation
	// Transcribe costs approximately $0.024 per minute for standard transcription
}

// TrackingQuery wraps core.Query with enhanced cost tracking
type TrackingQuery struct {
	query             core.Query
	tracker           *Tracker
	logger            *zap.Logger
	enhancedTracker   *EnhancedOperationTracker
	operationMetadata *OperationMetadata
}

// Implement all Query interface methods for proper chaining

// Where wraps the Where method with condition tracking
func (ctq *TrackingQuery) Where(field string, op string, value any) core.Query {
	ctq.query = ctq.query.Where(field, op, value)

	// Track query conditions for cost analysis
	if ctq.operationMetadata != nil {
		condition := QueryCondition{
			Field:    field,
			Operator: op,
			Value:    fmt.Sprintf("%v", value), // Simplified for logging
		}
		ctq.operationMetadata.Conditions = append(ctq.operationMetadata.Conditions, condition)
	}

	return ctq
}

// Index wraps the Index method with GSI tracking
func (ctq *TrackingQuery) Index(indexName string) core.Query {
	ctq.query = ctq.query.Index(indexName)

	// Track GSI usage for cost analysis
	if ctq.operationMetadata != nil {
		ctq.operationMetadata.IndexName = indexName
	}

	return ctq
}

// Filter wraps the Filter method with filter expression tracking
func (ctq *TrackingQuery) Filter(field string, op string, value any) core.Query {
	ctq.query = ctq.query.Filter(field, op, value)

	// Track filter expressions for cost impact analysis
	if ctq.operationMetadata != nil {
		filterExpr := fmt.Sprintf("%s %s %v", field, op, value)
		ctq.operationMetadata.FilterExpressions = append(ctq.operationMetadata.FilterExpressions, filterExpr)
	}

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

// Select wraps the Select method with projection tracking
func (ctq *TrackingQuery) Select(fields ...string) core.Query {
	ctq.query = ctq.query.Select(fields...)

	// Track projection fields for cost optimization analysis
	if ctq.operationMetadata != nil {
		ctq.operationMetadata.ProjectionFields = append(ctq.operationMetadata.ProjectionFields, fields...)
	}

	return ctq
}

// ConsistentRead wraps the ConsistentRead method with read type tracking
func (ctq *TrackingQuery) ConsistentRead() core.Query {
	ctq.query = ctq.query.ConsistentRead()

	// Track consistent read usage (affects cost - 2x read capacity units)
	if ctq.operationMetadata != nil {
		ctq.operationMetadata.ConsistentRead = true
	}

	return ctq
}

// WithRetry wraps the WithRetry method
func (ctq *TrackingQuery) WithRetry(maxRetries int, initialDelay time.Duration) core.Query {
	ctq.query = ctq.query.WithRetry(maxRetries, initialDelay)
	return ctq
}

// Terminal methods with cost tracking

// First wraps the First method with enhanced tracking
func (ctq *TrackingQuery) First(dest any) error {
	// Set operation type
	if ctq.operationMetadata != nil {
		ctq.operationMetadata.OperationType = "GetItem"
	}

	err := ctq.query.First(dest)

	readUnits := 1 // Base cost for First operation
	itemCount := 0

	if err == nil {
		itemCount = 1 // First always returns at most 1 item

		// Adjust read units based on operation details
		if ctq.operationMetadata != nil {
			// Consistent read costs 2x
			if ctq.operationMetadata.ConsistentRead {
				readUnits = 2
			}

			// Update metadata
			ctq.operationMetadata.ItemCount = itemCount

			// Track the operation with enhanced metadata
			if ctq.enhancedTracker != nil {
				operationID := fmt.Sprintf("first_%d", time.Now().UnixNano())
				ctq.enhancedTracker.TrackOperation(operationID, ctq.operationMetadata)
			}
		}

		if ctq.tracker != nil {
			_ = ctq.tracker.TrackDynamoRead(readUnits)
		}
	}

	if ctq.logger != nil {
		logFields := []zap.Field{
			zap.Int("read_units", readUnits),
			zap.Int("item_count", itemCount),
			zap.Error(err),
		}

		if ctq.operationMetadata != nil {
			logFields = append(logFields,
				zap.String("table_name", ctq.operationMetadata.TableName),
				zap.String("index_name", ctq.operationMetadata.IndexName),
				zap.Bool("consistent_read", ctq.operationMetadata.ConsistentRead),
				zap.Int("condition_count", len(ctq.operationMetadata.Conditions)),
				zap.Int("filter_count", len(ctq.operationMetadata.FilterExpressions)),
				zap.Int("projection_count", len(ctq.operationMetadata.ProjectionFields)),
			)
		}

		ctq.logger.Debug("dynamodb_first_tracked", logFields...)
	}

	return err
}

// All wraps the All method with enhanced result counting
func (ctq *TrackingQuery) All(dest any) error {
	// Set operation type
	if ctq.operationMetadata != nil {
		if ctq.operationMetadata.IndexName != "" {
			ctq.operationMetadata.OperationType = "Query" // Using GSI
		} else {
			ctq.operationMetadata.OperationType = OperationTypeScan // Table scan
		}
	}

	err := ctq.query.All(dest)

	readUnits := 1 // Default minimum
	itemCount := 0

	if err == nil {
		// Count actual items returned using reflection
		itemCount = countResultItems(dest)

		// Calculate more accurate read units based on item count and operation details
		if itemCount > 0 {
			readUnits = itemCount
		}

		// Adjust for operation specifics
		if ctq.operationMetadata != nil {
			// Consistent read costs 2x
			if ctq.operationMetadata.ConsistentRead {
				readUnits *= 2
			}

			// Scan operations are typically more expensive than query
			if ctq.operationMetadata.OperationType == OperationTypeScan && len(ctq.operationMetadata.FilterExpressions) > 0 {
				// Filter expressions can cause scanning of more items than returned
				// Estimate higher cost for filtered scans
				readUnits = int(float64(readUnits) * 1.5)
			}

			// Update metadata
			ctq.operationMetadata.ItemCount = itemCount

			// Track the operation with enhanced metadata
			if ctq.enhancedTracker != nil {
				operationID := fmt.Sprintf("all_%d", time.Now().UnixNano())
				ctq.enhancedTracker.TrackOperation(operationID, ctq.operationMetadata)
			}
		}

		if ctq.tracker != nil {
			_ = ctq.tracker.TrackDynamoRead(readUnits)
		}
	}

	if ctq.logger != nil {
		logFields := []zap.Field{
			zap.Int("read_units", readUnits),
			zap.Int("item_count", itemCount),
			zap.Error(err),
		}

		if ctq.operationMetadata != nil {
			logFields = append(logFields,
				zap.String("operation_type", ctq.operationMetadata.OperationType),
				zap.String("table_name", ctq.operationMetadata.TableName),
				zap.String("index_name", ctq.operationMetadata.IndexName),
				zap.Bool("consistent_read", ctq.operationMetadata.ConsistentRead),
				zap.Int("condition_count", len(ctq.operationMetadata.Conditions)),
				zap.Int("filter_count", len(ctq.operationMetadata.FilterExpressions)),
				zap.Int("projection_count", len(ctq.operationMetadata.ProjectionFields)),
			)
		}

		ctq.logger.Debug("dynamodb_all_tracked", logFields...)
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
		_ = ctq.tracker.TrackDynamoRead(readUnits)
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
		_ = ctq.tracker.TrackDynamoRead(1) // Count operations typically use 1 RCU minimum
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
		_ = ctq.tracker.TrackDynamoWrite(1) // One write unit for create
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
		_ = ctq.tracker.TrackDynamoWrite(1) // One write unit for create or update
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
		_ = ctq.tracker.TrackDynamoWrite(1) // One write unit for update
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
		_ = ctq.tracker.TrackDynamoWrite(1) // One write unit for delete
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
			_ = tracker.TrackDynamoWrite(1)
		case "query", "scan", "get", "first", "all", "count":
			_ = tracker.TrackDynamoRead(1)
		}
	}

	return err
}

// WithDynamORMCostTracking adds cost tracking to a context
func WithDynamORMCostTracking(ctx context.Context, requestID, operationType string) context.Context {
	tracker := NewWithRequest(requestID, operationType)
	return WithTracker(ctx, tracker)
}

// Note: Complex transaction operation tracking has been simplified for maintainability.
// Future enhancement: Implement full core.Tx interface wrapper for precise transaction tracking.

// countResultItems counts the number of items in a query result using reflection
func countResultItems(dest any) int {
	if dest == nil {
		return 0
	}

	// Get the value and handle pointers
	value := reflect.ValueOf(dest)
	if value.Kind() == reflect.Ptr {
		if value.IsNil() {
			return 0
		}
		value = value.Elem()
	}

	// Count items based on type
	switch value.Kind() {
	case reflect.Slice, reflect.Array:
		return value.Len()
	case reflect.Map:
		return value.Len()
	case reflect.Struct:
		// Single item
		return 1
	case reflect.Interface:
		// Check if it's a slice or array interface
		if value.IsNil() {
			return 0
		}
		return countResultItems(value.Interface())
	default:
		// For basic types, assume single item if not nil/zero
		if value.IsZero() {
			return 0
		}
		return 1
	}
}

// OperationMetadata contains detailed information about a DynamoDB operation
type OperationMetadata struct {
	OperationType     string            `json:"operation_type"`
	TableName         string            `json:"table_name"`
	IndexName         string            `json:"index_name,omitempty"`
	ItemCount         int               `json:"item_count"`
	FilterExpressions []string          `json:"filter_expressions,omitempty"`
	ProjectionFields  []string          `json:"projection_fields,omitempty"`
	ConsistentRead    bool              `json:"consistent_read"`
	BatchSize         int               `json:"batch_size,omitempty"`
	PaginationTokens  map[string]string `json:"pagination_tokens,omitempty"`
	Conditions        []QueryCondition  `json:"conditions,omitempty"`
}

// QueryCondition represents a query condition for cost analysis
type QueryCondition struct {
	Field    string `json:"field"`
	Operator string `json:"operator"`
	Value    string `json:"value"` // Simplified for logging
}

// EnhancedOperationTracker provides detailed operation tracking with metadata
type EnhancedOperationTracker struct {
	mu         sync.RWMutex
	operations map[string]*OperationMetadata
	logger     *zap.Logger
}

// NewEnhancedOperationTracker creates a new enhanced operation tracker
func NewEnhancedOperationTracker(logger *zap.Logger) *EnhancedOperationTracker {
	return &EnhancedOperationTracker{
		operations: make(map[string]*OperationMetadata),
		logger:     logger,
	}
}

// TrackOperation records detailed operation metadata
func (eot *EnhancedOperationTracker) TrackOperation(operationID string, metadata *OperationMetadata) {
	eot.mu.Lock()
	defer eot.mu.Unlock()

	eot.operations[operationID] = metadata

	if eot.logger != nil {
		eot.logger.Debug("enhanced_operation_tracked",
			zap.String("operation_id", operationID),
			zap.String("operation_type", metadata.OperationType),
			zap.String("table_name", metadata.TableName),
			zap.String("index_name", metadata.IndexName),
			zap.Int("item_count", metadata.ItemCount),
			zap.Int("filter_count", len(metadata.FilterExpressions)),
			zap.Int("projection_count", len(metadata.ProjectionFields)),
			zap.Bool("consistent_read", metadata.ConsistentRead),
		)
	}
}

// GetOperationMetadata retrieves metadata for an operation
func (eot *EnhancedOperationTracker) GetOperationMetadata(operationID string) (*OperationMetadata, bool) {
	eot.mu.RLock()
	defer eot.mu.RUnlock()

	metadata, exists := eot.operations[operationID]
	return metadata, exists
}

// GetAllOperations returns all tracked operations
func (eot *EnhancedOperationTracker) GetAllOperations() map[string]*OperationMetadata {
	eot.mu.RLock()
	defer eot.mu.RUnlock()

	// Return a copy to avoid race conditions
	result := make(map[string]*OperationMetadata)
	for id, metadata := range eot.operations {
		result[id] = metadata
	}
	return result
}

// ClearOperations clears all tracked operations
func (eot *EnhancedOperationTracker) ClearOperations() {
	eot.mu.Lock()
	defer eot.mu.Unlock()
	eot.operations = make(map[string]*OperationMetadata)
}

// countBatchItems counts items in a batch operation more accurately
func countBatchItems(items any) int {
	if items == nil {
		return 0
	}

	// Handle various input types
	value := reflect.ValueOf(items)

	// Handle pointers
	if value.Kind() == reflect.Ptr {
		if value.IsNil() {
			return 0
		}
		value = value.Elem()
	}

	// Count based on the actual type
	switch value.Kind() {
	case reflect.Slice, reflect.Array:
		return value.Len()
	case reflect.Map:
		return value.Len()
	case reflect.Struct:
		// Single item
		return 1
	case reflect.Interface:
		if value.IsNil() {
			return 0
		}
		return countBatchItems(value.Interface())
	default:
		// For other types, assume single item
		return 1
	}
}

// extractTableName extracts the table name from a model using reflection
func extractTableName(model any) string {
	if model == nil {
		return "unknown"
	}

	// Get the type, handling pointers
	modelType := reflect.TypeOf(model)
	if modelType.Kind() == reflect.Ptr {
		modelType = modelType.Elem()
	}

	// Try to call TableName() method if it exists
	modelValue := reflect.ValueOf(model)
	if modelValue.Kind() == reflect.Ptr && !modelValue.IsNil() {
		modelValue = modelValue.Elem()
	}

	// Check if model has TableName method
	if modelValue.IsValid() && modelValue.CanAddr() {
		tableNameMethod := modelValue.Addr().MethodByName("TableName")
		if tableNameMethod.IsValid() {
			result := tableNameMethod.Call(nil)
			if len(result) > 0 && result[0].Kind() == reflect.String {
				return result[0].String()
			}
		}
	}

	// Fallback to struct name
	if modelType.Kind() == reflect.Struct {
		return modelType.Name()
	}

	// If slice or array, try to get element type
	if modelType.Kind() == reflect.Slice || modelType.Kind() == reflect.Array {
		elemType := modelType.Elem()
		if elemType.Kind() == reflect.Ptr {
			elemType = elemType.Elem()
		}
		if elemType.Kind() == reflect.Struct {
			return elemType.Name()
		}
	}

	return "unknown"
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
		_ = ctq.tracker.TrackDynamoRead(100) // Conservative estimate for scan
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
		_ = ctq.tracker.TrackDynamoRead(estimatedReads)
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
		_ = ctq.tracker.TrackDynamoRead(len(keys)) // One read unit per key
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

// BatchCreate wraps the BatchCreate method with enhanced batch tracking
func (ctq *TrackingQuery) BatchCreate(items any) error {
	// Count items for precise cost tracking
	itemCount := countBatchItems(items)

	// Calculate write units more accurately
	// DynamoDB batch operations consume WCUs based on item size
	// For now, use item count as baseline (1 WCU per item for baseline)
	writeUnits := itemCount

	// Large batches may be split by DynamoDB - account for this
	// DynamoDB supports max 25 items per batch write
	batchCount := (itemCount + 24) / 25 // Round up division

	err := ctq.query.BatchCreate(items)
	if err == nil && ctq.tracker != nil {
		_ = ctq.tracker.TrackDynamoWrite(writeUnits)
	}

	if ctq.logger != nil {
		ctq.logger.Debug("dynamodb_batch_create_tracked",
			zap.Int("write_units", writeUnits),
			zap.Int("item_count", itemCount),
			zap.Int("batch_count", batchCount),
			zap.Error(err),
		)
	}

	return err
}

// BatchDelete wraps the BatchDelete method with enhanced batch tracking
func (ctq *TrackingQuery) BatchDelete(keys []any) error {
	keyCount := len(keys)

	// Calculate write units and batch count for deletes
	writeUnits := keyCount             // 1 WCU per delete operation
	batchCount := (keyCount + 24) / 25 // Max 25 items per batch

	err := ctq.query.BatchDelete(keys)
	if err == nil && ctq.tracker != nil {
		_ = ctq.tracker.TrackDynamoWrite(writeUnits)
	}

	if ctq.logger != nil {
		ctq.logger.Debug("dynamodb_batch_delete_tracked",
			zap.Int("write_units", writeUnits),
			zap.Int("key_count", keyCount),
			zap.Int("batch_count", batchCount),
			zap.Error(err),
		)
	}

	return err
}

// BatchWrite wraps the BatchWrite method with enhanced batch tracking
func (ctq *TrackingQuery) BatchWrite(putItems []any, deleteKeys []any) error {
	putCount := len(putItems)
	deleteCount := len(deleteKeys)
	totalWrites := putCount + deleteCount

	// Calculate batch count for combined operations
	batchCount := (totalWrites + 24) / 25 // Max 25 items per batch

	err := ctq.query.BatchWrite(putItems, deleteKeys)
	if err == nil && ctq.tracker != nil {
		_ = ctq.tracker.TrackDynamoWrite(totalWrites)
	}

	if ctq.logger != nil {
		ctq.logger.Debug("dynamodb_batch_write_tracked",
			zap.Int("write_units", totalWrites),
			zap.Int("put_items", putCount),
			zap.Int("delete_keys", deleteCount),
			zap.Int("batch_count", batchCount),
			zap.Error(err),
		)
	}

	return err
}

// BatchUpdateWithOptions wraps the BatchUpdateWithOptions method with cost tracking
func (ctq *TrackingQuery) BatchUpdateWithOptions(items []any, fields []string, options ...any) error {
	err := ctq.query.BatchUpdateWithOptions(items, fields, options...)
	if err == nil && ctq.tracker != nil {
		_ = ctq.tracker.TrackDynamoWrite(len(items)) // One write unit per item
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
