package monitoring

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"go.uber.org/zap"
)

// DynamORMMetricsWrapper wraps DynamoDB operations with automatic metrics collection
type DynamORMMetricsWrapper struct {
	client  DynamoDBClientInterface
	metrics *CloudWatchMetrics
	logger  *zap.Logger
}

// DynamoDBClientInterface defines the interface for DynamoDB operations we want to monitor
type DynamoDBClientInterface interface {
	GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	PutItem(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
	UpdateItem(ctx context.Context, params *dynamodb.UpdateItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error)
	DeleteItem(ctx context.Context, params *dynamodb.DeleteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error)
	Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
	Scan(ctx context.Context, params *dynamodb.ScanInput, optFns ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error)
	BatchGetItem(ctx context.Context, params *dynamodb.BatchGetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.BatchGetItemOutput, error)
	BatchWriteItem(ctx context.Context, params *dynamodb.BatchWriteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.BatchWriteItemOutput, error)
	TransactWriteItems(ctx context.Context, params *dynamodb.TransactWriteItemsInput, optFns ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error)
	TransactGetItems(ctx context.Context, params *dynamodb.TransactGetItemsInput, optFns ...func(*dynamodb.Options)) (*dynamodb.TransactGetItemsOutput, error)
}

// NewDynamORMMetricsWrapper creates a new metrics wrapper for DynamoDB operations
func NewDynamORMMetricsWrapper(client DynamoDBClientInterface, metrics *CloudWatchMetrics, logger *zap.Logger) *DynamORMMetricsWrapper {
	return &DynamORMMetricsWrapper{
		client:  client,
		metrics: metrics,
		logger:  logger,
	}
}

// GetItem wraps the GetItem operation with metrics
func (dmw *DynamORMMetricsWrapper) GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	start := time.Now()
	result, err := dmw.client.GetItem(ctx, params, optFns...)
	duration := time.Since(start)

	// Record metrics
	metrics := DynamORMMetrics{
		Operation: "GetItem",
		TableName: getTableName(params.TableName),
		Duration:  duration,
		Error:     err,
		ItemCount: getItemCount(result, err),
	}

	if result != nil && result.ConsumedCapacity != nil {
		metrics.ConsumedCapacity = ConsumedCapacity{
			ReadUnits:  getCapacityUnits(result.ConsumedCapacity.ReadCapacityUnits),
			WriteUnits: getCapacityUnits(result.ConsumedCapacity.WriteCapacityUnits),
		}
	}

	dmw.metrics.RecordDynamORMMetrics(ctx, metrics)

	if err != nil {
		dmw.logger.Error("DynamoDB GetItem error",
			zap.Error(err),
			zap.String("table", metrics.TableName),
			zap.Duration("duration", duration))
	}

	return result, err
}

// PutItem wraps the PutItem operation with metrics
func (dmw *DynamORMMetricsWrapper) PutItem(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	result, err := dmw.wrapSingleItemOperation(ctx, "PutItem", getTableName(params.TableName), func() (interface{}, error) {
		return dmw.client.PutItem(ctx, params, optFns...)
	})
	if result == nil {
		return nil, err
	}
	return result.(*dynamodb.PutItemOutput), err
}

// UpdateItem wraps the UpdateItem operation with metrics
func (dmw *DynamORMMetricsWrapper) UpdateItem(ctx context.Context, params *dynamodb.UpdateItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
	result, err := dmw.wrapSingleItemOperation(ctx, "UpdateItem", getTableName(params.TableName), func() (interface{}, error) {
		return dmw.client.UpdateItem(ctx, params, optFns...)
	})
	if result == nil {
		return nil, err
	}
	return result.(*dynamodb.UpdateItemOutput), err
}

// DeleteItem wraps the DeleteItem operation with metrics
func (dmw *DynamORMMetricsWrapper) DeleteItem(ctx context.Context, params *dynamodb.DeleteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error) {
	result, err := dmw.wrapSingleItemOperation(ctx, "DeleteItem", getTableName(params.TableName), func() (interface{}, error) {
		return dmw.client.DeleteItem(ctx, params, optFns...)
	})
	if result == nil {
		return nil, err
	}
	return result.(*dynamodb.DeleteItemOutput), err
}

// Query wraps the Query operation with metrics
func (dmw *DynamORMMetricsWrapper) Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	result, err := dmw.wrapQueryOperation(ctx, "Query", getTableName(params.TableName), 100*time.Millisecond, func() (interface{}, error) {
		return dmw.client.Query(ctx, params, optFns...)
	})
	if result == nil {
		return nil, err
	}
	return result.(*dynamodb.QueryOutput), err
}

// Scan wraps the Scan operation with metrics
func (dmw *DynamORMMetricsWrapper) Scan(ctx context.Context, params *dynamodb.ScanInput, optFns ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error) {
	result, err := dmw.wrapQueryOperation(ctx, "Scan", getTableName(params.TableName), 500*time.Millisecond, func() (interface{}, error) {
		return dmw.client.Scan(ctx, params, optFns...)
	})
	if result == nil {
		return nil, err
	}
	return result.(*dynamodb.ScanOutput), err
}

// BatchGetItem wraps the BatchGetItem operation with metrics
func (dmw *DynamORMMetricsWrapper) BatchGetItem(ctx context.Context, params *dynamodb.BatchGetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.BatchGetItemOutput, error) {
	start := time.Now()
	result, err := dmw.client.BatchGetItem(ctx, params, optFns...)
	duration := time.Since(start)

	// Count total items across all tables
	itemCount := int64(0)
	tableNames := make([]string, 0, len(params.RequestItems))
	for tableName, request := range params.RequestItems {
		tableNames = append(tableNames, tableName)
		itemCount += int64(len(request.Keys))
	}

	// Use first table name or "multiple" if multiple tables
	tableName := "multiple"
	if len(tableNames) == 1 {
		tableName = tableNames[0]
	}

	metrics := DynamORMMetrics{
		Operation: "BatchGetItem",
		TableName: tableName,
		Duration:  duration,
		Error:     err,
		ItemCount: itemCount,
	}

	// Sum consumed capacity from all tables
	if result != nil && len(result.ConsumedCapacity) > 0 {
		var totalRead, totalWrite float64
		for _, capacity := range result.ConsumedCapacity {
			totalRead += getCapacityUnits(capacity.ReadCapacityUnits)
			totalWrite += getCapacityUnits(capacity.WriteCapacityUnits)
		}
		metrics.ConsumedCapacity = ConsumedCapacity{
			ReadUnits:  totalRead,
			WriteUnits: totalWrite,
		}
	}

	dmw.metrics.RecordDynamORMMetrics(ctx, metrics)

	if err != nil {
		dmw.logger.Error("DynamoDB BatchGetItem error",
			zap.Error(err),
			zap.String("table", tableName),
			zap.Duration("duration", duration),
			zap.Int64("item_count", itemCount))
	}

	return result, err
}

// BatchWriteItem wraps the BatchWriteItem operation with metrics
func (dmw *DynamORMMetricsWrapper) BatchWriteItem(ctx context.Context, params *dynamodb.BatchWriteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.BatchWriteItemOutput, error) {
	start := time.Now()
	result, err := dmw.client.BatchWriteItem(ctx, params, optFns...)
	duration := time.Since(start)

	// Count total items across all tables
	itemCount := int64(0)
	tableNames := make([]string, 0, len(params.RequestItems))
	for tableName, requests := range params.RequestItems {
		tableNames = append(tableNames, tableName)
		itemCount += int64(len(requests))
	}

	// Use first table name or "multiple" if multiple tables
	tableName := "multiple"
	if len(tableNames) == 1 {
		tableName = tableNames[0]
	}

	metrics := DynamORMMetrics{
		Operation: "BatchWriteItem",
		TableName: tableName,
		Duration:  duration,
		Error:     err,
		ItemCount: itemCount,
	}

	// Sum consumed capacity from all tables
	if result != nil && len(result.ConsumedCapacity) > 0 {
		var totalRead, totalWrite float64
		for _, capacity := range result.ConsumedCapacity {
			totalRead += getCapacityUnits(capacity.ReadCapacityUnits)
			totalWrite += getCapacityUnits(capacity.WriteCapacityUnits)
		}
		metrics.ConsumedCapacity = ConsumedCapacity{
			ReadUnits:  totalRead,
			WriteUnits: totalWrite,
		}
	}

	dmw.metrics.RecordDynamORMMetrics(ctx, metrics)

	if err != nil {
		dmw.logger.Error("DynamoDB BatchWriteItem error",
			zap.Error(err),
			zap.String("table", tableName),
			zap.Duration("duration", duration),
			zap.Int64("item_count", itemCount))
	}

	return result, err
}

// TransactWriteItems wraps the TransactWriteItems operation with metrics
func (dmw *DynamORMMetricsWrapper) TransactWriteItems(ctx context.Context, params *dynamodb.TransactWriteItemsInput, optFns ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error) {
	result, err := dmw.wrapTransactionOperation(ctx, "TransactWriteItems", int64(len(params.TransactItems)), func() (interface{}, error) {
		return dmw.client.TransactWriteItems(ctx, params, optFns...)
	})
	if result == nil {
		return nil, err
	}
	return result.(*dynamodb.TransactWriteItemsOutput), err
}

// TransactGetItems wraps the TransactGetItems operation with metrics
func (dmw *DynamORMMetricsWrapper) TransactGetItems(ctx context.Context, params *dynamodb.TransactGetItemsInput, optFns ...func(*dynamodb.Options)) (*dynamodb.TransactGetItemsOutput, error) {
	result, err := dmw.wrapTransactionOperation(ctx, "TransactGetItems", int64(len(params.TransactItems)), func() (interface{}, error) {
		return dmw.client.TransactGetItems(ctx, params, optFns...)
	})
	if result == nil {
		return nil, err
	}
	return result.(*dynamodb.TransactGetItemsOutput), err
}

// Helper methods for consolidating common patterns

// wrapSingleItemOperation handles the common pattern for single-item operations (Put, Update, Delete)
func (dmw *DynamORMMetricsWrapper) wrapSingleItemOperation(ctx context.Context, operation, tableName string, clientCall func() (interface{}, error)) (interface{}, error) {
	start := time.Now()
	result, err := clientCall()
	duration := time.Since(start)

	metrics := DynamORMMetrics{
		Operation: operation,
		TableName: tableName,
		Duration:  duration,
		Error:     err,
		ItemCount: 1, // Always 1 for single-item operations
	}

	// Extract consumed capacity based on result type
	switch r := result.(type) {
	case *dynamodb.PutItemOutput:
		if r != nil && r.ConsumedCapacity != nil {
			metrics.ConsumedCapacity = ConsumedCapacity{
				ReadUnits:  getCapacityUnits(r.ConsumedCapacity.ReadCapacityUnits),
				WriteUnits: getCapacityUnits(r.ConsumedCapacity.WriteCapacityUnits),
			}
		}
	case *dynamodb.UpdateItemOutput:
		if r != nil && r.ConsumedCapacity != nil {
			metrics.ConsumedCapacity = ConsumedCapacity{
				ReadUnits:  getCapacityUnits(r.ConsumedCapacity.ReadCapacityUnits),
				WriteUnits: getCapacityUnits(r.ConsumedCapacity.WriteCapacityUnits),
			}
		}
	case *dynamodb.DeleteItemOutput:
		if r != nil && r.ConsumedCapacity != nil {
			metrics.ConsumedCapacity = ConsumedCapacity{
				ReadUnits:  getCapacityUnits(r.ConsumedCapacity.ReadCapacityUnits),
				WriteUnits: getCapacityUnits(r.ConsumedCapacity.WriteCapacityUnits),
			}
		}
	}

	dmw.metrics.RecordDynamORMMetrics(ctx, metrics)

	if err != nil {
		dmw.logger.Error("DynamoDB "+operation+" error",
			zap.Error(err),
			zap.String("table", tableName),
			zap.Duration("duration", duration))
	}

	return result, err
}

// wrapQueryOperation handles the common pattern for query operations (Query, Scan)
func (dmw *DynamORMMetricsWrapper) wrapQueryOperation(ctx context.Context, operation, tableName string, slowThreshold time.Duration, clientCall func() (interface{}, error)) (interface{}, error) {
	start := time.Now()
	result, err := clientCall()
	duration := time.Since(start)

	itemCount := int64(0)
	var consumedCapacity ConsumedCapacity

	// Extract data based on result type
	switch r := result.(type) {
	case *dynamodb.QueryOutput:
		if r != nil {
			itemCount = int64(r.Count)
			if r.ConsumedCapacity != nil {
				consumedCapacity = ConsumedCapacity{
					ReadUnits:  getCapacityUnits(r.ConsumedCapacity.ReadCapacityUnits),
					WriteUnits: getCapacityUnits(r.ConsumedCapacity.WriteCapacityUnits),
				}
			}
		}
	case *dynamodb.ScanOutput:
		if r != nil {
			itemCount = int64(r.Count)
			if r.ConsumedCapacity != nil {
				consumedCapacity = ConsumedCapacity{
					ReadUnits:  getCapacityUnits(r.ConsumedCapacity.ReadCapacityUnits),
					WriteUnits: getCapacityUnits(r.ConsumedCapacity.WriteCapacityUnits),
				}
			}
		}
	}

	metrics := DynamORMMetrics{
		Operation:        operation,
		TableName:        tableName,
		Duration:         duration,
		Error:            err,
		ItemCount:        itemCount,
		ConsumedCapacity: consumedCapacity,
	}

	dmw.metrics.RecordDynamORMMetrics(ctx, metrics)

	// Log slow operations
	if duration > slowThreshold {
		dmw.logger.Warn("Slow DynamoDB "+operation,
			zap.String("table", tableName),
			zap.Duration("duration", duration),
			zap.Int64("item_count", itemCount),
			zap.Bool("has_error", err != nil))
	}

	if err != nil {
		dmw.logger.Error("DynamoDB "+operation+" error",
			zap.Error(err),
			zap.String("table", tableName),
			zap.Duration("duration", duration))
	}

	return result, err
}

// wrapTransactionOperation handles the common pattern for transaction operations
func (dmw *DynamORMMetricsWrapper) wrapTransactionOperation(ctx context.Context, operation string, itemCount int64, clientCall func() (interface{}, error)) (interface{}, error) {
	start := time.Now()
	result, err := clientCall()
	duration := time.Since(start)

	metrics := DynamORMMetrics{
		Operation: operation,
		TableName: "transaction", // Transactions can span multiple tables
		Duration:  duration,
		Error:     err,
		ItemCount: itemCount,
	}

	// Sum consumed capacity from all tables
	switch r := result.(type) {
	case *dynamodb.TransactWriteItemsOutput:
		if r != nil && len(r.ConsumedCapacity) > 0 {
			var totalRead, totalWrite float64
			for _, capacity := range r.ConsumedCapacity {
				totalRead += getCapacityUnits(capacity.ReadCapacityUnits)
				totalWrite += getCapacityUnits(capacity.WriteCapacityUnits)
			}
			metrics.ConsumedCapacity = ConsumedCapacity{
				ReadUnits:  totalRead,
				WriteUnits: totalWrite,
			}
		}
	case *dynamodb.TransactGetItemsOutput:
		if r != nil && len(r.ConsumedCapacity) > 0 {
			var totalRead, totalWrite float64
			for _, capacity := range r.ConsumedCapacity {
				totalRead += getCapacityUnits(capacity.ReadCapacityUnits)
				totalWrite += getCapacityUnits(capacity.WriteCapacityUnits)
			}
			metrics.ConsumedCapacity = ConsumedCapacity{
				ReadUnits:  totalRead,
				WriteUnits: totalWrite,
			}
		}
	}

	dmw.metrics.RecordDynamORMMetrics(ctx, metrics)

	if err != nil {
		dmw.logger.Error("DynamoDB "+operation+" error",
			zap.Error(err),
			zap.Duration("duration", duration),
			zap.Int("transaction_items", int(itemCount)))
	}

	return result, err
}

// Helper functions

// getTableName safely extracts table name from pointer
func getTableName(tableName *string) string {
	if tableName == nil {
		return "unknown"
	}
	return *tableName
}

// getCapacityUnits safely extracts capacity units
func getCapacityUnits(units *float64) float64 {
	if units == nil {
		return 0
	}
	return *units
}

// getItemCount determines item count from various result types
func getItemCount(result interface{}, err error) int64 {
	if err != nil {
		return 0
	}

	switch r := result.(type) {
	case *dynamodb.GetItemOutput:
		if r != nil && r.Item != nil {
			return 1
		}
		return 0
	case *dynamodb.QueryOutput:
		if r != nil {
			return int64(r.Count)
		}
		return 0
	case *dynamodb.ScanOutput:
		if r != nil {
			return int64(r.Count)
		}
		return 0
	default:
		return 0
	}
}

// WrapperFactory creates DynamORM metrics wrappers for common use cases
type WrapperFactory struct {
	metrics *CloudWatchMetrics
	logger  *zap.Logger
}

// NewWrapperFactory creates a new wrapper factory
func NewWrapperFactory(metrics *CloudWatchMetrics, logger *zap.Logger) *WrapperFactory {
	return &WrapperFactory{
		metrics: metrics,
		logger:  logger,
	}
}

// WrapClient wraps a DynamoDB client with metrics collection
func (wf *WrapperFactory) WrapClient(client DynamoDBClientInterface) *DynamORMMetricsWrapper {
	return NewDynamORMMetricsWrapper(client, wf.metrics, wf.logger)
}
