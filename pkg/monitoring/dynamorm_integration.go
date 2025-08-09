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
	start := time.Now()
	result, err := dmw.client.PutItem(ctx, params, optFns...)
	duration := time.Since(start)

	metrics := DynamORMMetrics{
		Operation: "PutItem",
		TableName: getTableName(params.TableName),
		Duration:  duration,
		Error:     err,
		ItemCount: 1, // Always 1 for PutItem
	}

	if result != nil && result.ConsumedCapacity != nil {
		metrics.ConsumedCapacity = ConsumedCapacity{
			ReadUnits:  getCapacityUnits(result.ConsumedCapacity.ReadCapacityUnits),
			WriteUnits: getCapacityUnits(result.ConsumedCapacity.WriteCapacityUnits),
		}
	}

	dmw.metrics.RecordDynamORMMetrics(ctx, metrics)

	if err != nil {
		dmw.logger.Error("DynamoDB PutItem error",
			zap.Error(err),
			zap.String("table", metrics.TableName),
			zap.Duration("duration", duration))
	}

	return result, err
}

// UpdateItem wraps the UpdateItem operation with metrics
func (dmw *DynamORMMetricsWrapper) UpdateItem(ctx context.Context, params *dynamodb.UpdateItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
	start := time.Now()
	result, err := dmw.client.UpdateItem(ctx, params, optFns...)
	duration := time.Since(start)

	metrics := DynamORMMetrics{
		Operation: "UpdateItem",
		TableName: getTableName(params.TableName),
		Duration:  duration,
		Error:     err,
		ItemCount: 1, // Always 1 for UpdateItem
	}

	if result != nil && result.ConsumedCapacity != nil {
		metrics.ConsumedCapacity = ConsumedCapacity{
			ReadUnits:  getCapacityUnits(result.ConsumedCapacity.ReadCapacityUnits),
			WriteUnits: getCapacityUnits(result.ConsumedCapacity.WriteCapacityUnits),
		}
	}

	dmw.metrics.RecordDynamORMMetrics(ctx, metrics)

	if err != nil {
		dmw.logger.Error("DynamoDB UpdateItem error",
			zap.Error(err),
			zap.String("table", metrics.TableName),
			zap.Duration("duration", duration))
	}

	return result, err
}

// DeleteItem wraps the DeleteItem operation with metrics
func (dmw *DynamORMMetricsWrapper) DeleteItem(ctx context.Context, params *dynamodb.DeleteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error) {
	start := time.Now()
	result, err := dmw.client.DeleteItem(ctx, params, optFns...)
	duration := time.Since(start)

	metrics := DynamORMMetrics{
		Operation: "DeleteItem",
		TableName: getTableName(params.TableName),
		Duration:  duration,
		Error:     err,
		ItemCount: 1, // Always 1 for DeleteItem
	}

	if result != nil && result.ConsumedCapacity != nil {
		metrics.ConsumedCapacity = ConsumedCapacity{
			ReadUnits:  getCapacityUnits(result.ConsumedCapacity.ReadCapacityUnits),
			WriteUnits: getCapacityUnits(result.ConsumedCapacity.WriteCapacityUnits),
		}
	}

	dmw.metrics.RecordDynamORMMetrics(ctx, metrics)

	if err != nil {
		dmw.logger.Error("DynamoDB DeleteItem error",
			zap.Error(err),
			zap.String("table", metrics.TableName),
			zap.Duration("duration", duration))
	}

	return result, err
}

// Query wraps the Query operation with metrics
func (dmw *DynamORMMetricsWrapper) Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	start := time.Now()
	result, err := dmw.client.Query(ctx, params, optFns...)
	duration := time.Since(start)

	itemCount := int64(0)
	if result != nil {
		itemCount = int64(result.Count)
	}

	metrics := DynamORMMetrics{
		Operation: "Query",
		TableName: getTableName(params.TableName),
		Duration:  duration,
		Error:     err,
		ItemCount: itemCount,
	}

	if result != nil && result.ConsumedCapacity != nil {
		metrics.ConsumedCapacity = ConsumedCapacity{
			ReadUnits:  getCapacityUnits(result.ConsumedCapacity.ReadCapacityUnits),
			WriteUnits: getCapacityUnits(result.ConsumedCapacity.WriteCapacityUnits),
		}
	}

	dmw.metrics.RecordDynamORMMetrics(ctx, metrics)

	// Log slow queries
	if duration > 100*time.Millisecond {
		dmw.logger.Warn("Slow DynamoDB Query",
			zap.String("table", metrics.TableName),
			zap.Duration("duration", duration),
			zap.Int64("item_count", itemCount),
			zap.Bool("has_error", err != nil))
	}

	if err != nil {
		dmw.logger.Error("DynamoDB Query error",
			zap.Error(err),
			zap.String("table", metrics.TableName),
			zap.Duration("duration", duration))
	}

	return result, err
}

// Scan wraps the Scan operation with metrics
func (dmw *DynamORMMetricsWrapper) Scan(ctx context.Context, params *dynamodb.ScanInput, optFns ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error) {
	start := time.Now()
	result, err := dmw.client.Scan(ctx, params, optFns...)
	duration := time.Since(start)

	itemCount := int64(0)
	if result != nil {
		itemCount = int64(result.Count)
	}

	metrics := DynamORMMetrics{
		Operation: "Scan",
		TableName: getTableName(params.TableName),
		Duration:  duration,
		Error:     err,
		ItemCount: itemCount,
	}

	if result != nil && result.ConsumedCapacity != nil {
		metrics.ConsumedCapacity = ConsumedCapacity{
			ReadUnits:  getCapacityUnits(result.ConsumedCapacity.ReadCapacityUnits),
			WriteUnits: getCapacityUnits(result.ConsumedCapacity.WriteCapacityUnits),
		}
	}

	dmw.metrics.RecordDynamORMMetrics(ctx, metrics)

	// Log slow scans (scans are typically slow, so higher threshold)
	if duration > 500*time.Millisecond {
		dmw.logger.Warn("Slow DynamoDB Scan",
			zap.String("table", metrics.TableName),
			zap.Duration("duration", duration),
			zap.Int64("item_count", itemCount),
			zap.Bool("has_error", err != nil))
	}

	if err != nil {
		dmw.logger.Error("DynamoDB Scan error",
			zap.Error(err),
			zap.String("table", metrics.TableName),
			zap.Duration("duration", duration))
	}

	return result, err
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
	start := time.Now()
	result, err := dmw.client.TransactWriteItems(ctx, params, optFns...)
	duration := time.Since(start)

	metrics := DynamORMMetrics{
		Operation: "TransactWriteItems",
		TableName: "transaction", // Transactions can span multiple tables
		Duration:  duration,
		Error:     err,
		ItemCount: int64(len(params.TransactItems)),
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
		dmw.logger.Error("DynamoDB TransactWriteItems error",
			zap.Error(err),
			zap.Duration("duration", duration),
			zap.Int("transaction_items", len(params.TransactItems)))
	}

	return result, err
}

// TransactGetItems wraps the TransactGetItems operation with metrics
func (dmw *DynamORMMetricsWrapper) TransactGetItems(ctx context.Context, params *dynamodb.TransactGetItemsInput, optFns ...func(*dynamodb.Options)) (*dynamodb.TransactGetItemsOutput, error) {
	start := time.Now()
	result, err := dmw.client.TransactGetItems(ctx, params, optFns...)
	duration := time.Since(start)

	metrics := DynamORMMetrics{
		Operation: "TransactGetItems",
		TableName: "transaction", // Transactions can span multiple tables
		Duration:  duration,
		Error:     err,
		ItemCount: int64(len(params.TransactItems)),
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
		dmw.logger.Error("DynamoDB TransactGetItems error",
			zap.Error(err),
			zap.Duration("duration", duration),
			zap.Int("transaction_items", len(params.TransactItems)))
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
