package monitoring

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/equaltoai/lesser/pkg/common"
	"go.uber.org/zap"
)

// CloudWatchMetrics provides enhanced metrics collection with DynamORM integration
type CloudWatchMetrics struct {
	client      cloudWatchAPI
	namespace   string
	environment string
	logger      *zap.Logger
	buffer      *EnhancedMetricBuffer
	dimensions  map[string]string
}

// EnhancedMetricBuffer provides thread-safe buffering with automatic flushing
type EnhancedMetricBuffer struct {
	metrics   []types.MetricDatum
	maxSize   int
	flushSize int
	lastFlush time.Time
	mu        sync.RWMutex
	flushFunc func([]types.MetricDatum) error
}

// MetricConfig configures CloudWatch metrics behavior
type MetricConfig struct {
	Namespace      string
	Environment    string
	BufferSize     int
	FlushSize      int
	FlushInterval  time.Duration
	DefaultDims    map[string]string
	EnableBatching bool
}

// DynamORMMetrics contains DynamoDB operation metrics
type DynamORMMetrics struct {
	Operation        string
	TableName        string
	ConsumedCapacity ConsumedCapacity
	ItemCount        int64
	Duration         time.Duration
	Error            error
}

// ConsumedCapacity represents DynamoDB consumed capacity units
type ConsumedCapacity struct {
	ReadUnits  float64
	WriteUnits float64
}

// DefaultMetricConfig returns sensible defaults
func DefaultMetricConfig() MetricConfig {
	return MetricConfig{
		Namespace:      "Lesser/Monitoring",
		Environment:    "prod",
		BufferSize:     200,
		FlushSize:      20, // CloudWatch limit
		FlushInterval:  30 * time.Second,
		DefaultDims:    make(map[string]string),
		EnableBatching: true,
	}
}

// NewCloudWatchMetrics creates a new CloudWatch metrics collector
func NewCloudWatchMetrics(awsConfig aws.Config, config MetricConfig, logger *zap.Logger) *CloudWatchMetrics {
	cwm := &CloudWatchMetrics{
		client:      cloudwatch.NewFromConfig(awsConfig),
		namespace:   config.Namespace,
		environment: config.Environment,
		logger:      logger,
		dimensions:  config.DefaultDims,
	}

	// Initialize buffer
	cwm.buffer = &EnhancedMetricBuffer{
		metrics:   make([]types.MetricDatum, 0, config.BufferSize),
		maxSize:   config.BufferSize,
		flushSize: config.FlushSize,
		lastFlush: time.Now(),
		flushFunc: cwm.flushToCloudWatch,
	}

	return cwm
}

// RecordDynamORMMetrics records comprehensive DynamoDB operation metrics
func (cwm *CloudWatchMetrics) RecordDynamORMMetrics(_ context.Context, metrics DynamORMMetrics) {
	baseDims := map[string]string{
		"Operation":   metrics.Operation,
		"TableName":   metrics.TableName,
		"Environment": cwm.environment,
	}

	// Record operation latency
	cwm.addMetric("DynamoDB.OperationLatency",
		float64(metrics.Duration.Milliseconds()),
		types.StandardUnitMilliseconds,
		cwm.buildDimensions(baseDims, nil))

	// Record consumed capacity
	if metrics.ConsumedCapacity.ReadUnits > 0 {
		cwm.addMetric("DynamoDB.ConsumedReadCapacity",
			metrics.ConsumedCapacity.ReadUnits,
			types.StandardUnitCount,
			cwm.buildDimensions(baseDims, nil))
	}

	if metrics.ConsumedCapacity.WriteUnits > 0 {
		cwm.addMetric("DynamoDB.ConsumedWriteCapacity",
			metrics.ConsumedCapacity.WriteUnits,
			types.StandardUnitCount,
			cwm.buildDimensions(baseDims, nil))
	}

	// Record item count
	if metrics.ItemCount > 0 {
		cwm.addMetric("DynamoDB.ItemCount",
			float64(metrics.ItemCount),
			types.StandardUnitCount,
			cwm.buildDimensions(baseDims, nil))
	}

	// Record errors
	if metrics.Error != nil {
		errorDims := cwm.buildDimensions(baseDims, map[string]string{
			"ErrorType": classifyDynamoDBError(metrics.Error),
		})
		cwm.addMetric("DynamoDB.Errors", 1, types.StandardUnitCount, errorDims)
	}

	// Record success rate
	successValue := 1.0
	if metrics.Error != nil {
		successValue = 0.0
	}
	cwm.addMetric("DynamoDB.SuccessRate", successValue, types.StandardUnitCount,
		cwm.buildDimensions(baseDims, nil))
}

// RecordCostMetrics records cost-related metrics
func (cwm *CloudWatchMetrics) RecordCostMetrics(operation string, costData CostData) {
	baseDims := map[string]string{
		"Operation":   operation,
		"Environment": cwm.environment,
	}

	// Record total cost in micro-cents
	cwm.addMetric("Cost.TotalMicroCents",
		float64(costData.TotalCostMicroCents),
		types.StandardUnitCount,
		cwm.buildDimensions(baseDims, nil))

	// Record individual service costs
	if costData.DynamoDBCostMicroCents > 0 {
		cwm.addMetric("Cost.DynamoDBMicroCents",
			float64(costData.DynamoDBCostMicroCents),
			types.StandardUnitCount,
			cwm.buildDimensions(baseDims, map[string]string{"Service": "DynamoDB"}))
	}

	if costData.LambdaCostMicroCents > 0 {
		cwm.addMetric("Cost.LambdaMicroCents",
			float64(costData.LambdaCostMicroCents),
			types.StandardUnitCount,
			cwm.buildDimensions(baseDims, map[string]string{"Service": "Lambda"}))
	}

	if costData.S3CostMicroCents > 0 {
		cwm.addMetric("Cost.S3MicroCents",
			float64(costData.S3CostMicroCents),
			types.StandardUnitCount,
			cwm.buildDimensions(baseDims, map[string]string{"Service": "S3"}))
	}
}

// CostData represents cost information for an operation
type CostData struct {
	TotalCostMicroCents    int64
	DynamoDBCostMicroCents int64
	LambdaCostMicroCents   int64
	S3CostMicroCents       int64
}

// RecordBusinessMetrics records application-specific business metrics
func (cwm *CloudWatchMetrics) RecordBusinessMetrics(metricName string, value float64, unit types.StandardUnit, businessDims map[string]string) {
	dimensions := cwm.buildDimensions(map[string]string{
		"Environment": cwm.environment,
		"MetricType":  "Business",
	}, businessDims)

	cwm.addMetric(metricName, value, unit, dimensions)
}

// addMetric adds a metric to the buffer
func (cwm *CloudWatchMetrics) addMetric(name string, value float64, unit types.StandardUnit, dimensions []types.Dimension) {
	metric := types.MetricDatum{
		MetricName: aws.String(name),
		Value:      aws.Float64(value),
		Unit:       unit,
		Dimensions: dimensions,
		Timestamp:  aws.Time(time.Now()),
	}

	cwm.buffer.Add(metric)

	// Synchronous flush if buffer is full to avoid background goroutines
	if cwm.buffer.ShouldFlush() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		// Log any flush errors but don't block the main operation
		if err := cwm.FlushMetrics(ctx); err != nil {
			cwm.logger.Warn("failed to flush metrics buffer", zap.Error(err))
		}
	}
}

// FlushMetrics manually flushes all buffered metrics
func (cwm *CloudWatchMetrics) FlushMetrics(_ context.Context) error {
	return cwm.buffer.Flush()
}

// buildDimensions combines base dimensions with additional dimensions
func (cwm *CloudWatchMetrics) buildDimensions(baseDims, extraDims map[string]string) []types.Dimension {
	allDims := make(map[string]string)

	// Add default dimensions
	for k, v := range cwm.dimensions {
		allDims[k] = v
	}

	// Add base dimensions
	for k, v := range baseDims {
		if v != "" { // Only add non-empty values
			allDims[k] = v
		}
	}

	// Add extra dimensions
	for k, v := range extraDims {
		if v != "" { // Only add non-empty values
			allDims[k] = v
		}
	}

	// Convert to CloudWatch dimensions
	dimensions := make([]types.Dimension, 0, len(allDims))
	for name, value := range allDims {
		dimensions = append(dimensions, types.Dimension{
			Name:  aws.String(name),
			Value: aws.String(value),
		})
	}

	return dimensions
}

// flushToCloudWatch sends metrics to CloudWatch
func (cwm *CloudWatchMetrics) flushToCloudWatch(metrics []types.MetricDatum) error {
	if err := common.ValidateSliceNotEmpty("metrics", metrics); err != nil {
		return nil
	}

	// CloudWatch allows maximum 20 metrics per request
	for i := 0; i < len(metrics); i += 20 {
		end := i + 20
		if end > len(metrics) {
			end = len(metrics)
		}

		input := &cloudwatch.PutMetricDataInput{
			Namespace:  aws.String(cwm.namespace),
			MetricData: metrics[i:end],
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		_, err := cwm.client.PutMetricData(ctx, input)
		cancel()

		if err != nil {
			cwm.logger.Error("failed to put metric data",
				zap.Error(err),
				zap.String("namespace", cwm.namespace),
				zap.Int("metric_count", end-i))
			return fmt.Errorf("failed to put metric data: %w", err)
		}

		cwm.logger.Debug("metrics sent to CloudWatch",
			zap.String("namespace", cwm.namespace),
			zap.Int("metric_count", end-i))
	}

	return nil
}

// Enhanced buffer methods

// Add adds a metric to the buffer (thread-safe)
func (emb *EnhancedMetricBuffer) Add(metric types.MetricDatum) {
	emb.mu.Lock()
	defer emb.mu.Unlock()

	emb.metrics = append(emb.metrics, metric)
}

// ShouldFlush determines if the buffer should be flushed
// In serverless mode, we only flush when buffer reaches capacity to avoid time-based triggers
func (emb *EnhancedMetricBuffer) ShouldFlush() bool {
	emb.mu.RLock()
	defer emb.mu.RUnlock()

	return len(emb.metrics) >= emb.flushSize
}

// Flush sends all buffered metrics using the flush function
func (emb *EnhancedMetricBuffer) Flush() error {
	emb.mu.Lock()
	defer emb.mu.Unlock()

	if err := common.ValidateSliceNotEmpty("emb.metrics", emb.metrics); err != nil {
		return nil
	}

	// Copy metrics to flush
	toFlush := make([]types.MetricDatum, len(emb.metrics))
	copy(toFlush, emb.metrics)

	// Clear buffer
	emb.metrics = emb.metrics[:0]
	emb.lastFlush = time.Now()

	// Flush outside of lock
	return emb.flushFunc(toFlush)
}

// Size returns the current buffer size (thread-safe)
func (emb *EnhancedMetricBuffer) Size() int {
	emb.mu.RLock()
	defer emb.mu.RUnlock()
	return len(emb.metrics)
}

// classifyDynamoDBError classifies DynamoDB errors for metrics
func classifyDynamoDBError(err error) string {
	if err == nil {
		return "none"
	}

	errStr := err.Error()
	switch {
	case contains(errStr, "ProvisionedThroughputExceededException"):
		return "throughput_exceeded"
	case contains(errStr, "ResourceNotFoundException"):
		return "resource_not_found"
	case contains(errStr, "ConditionalCheckFailedException"):
		return "conditional_check_failed"
	case contains(errStr, "ValidationException"):
		return "validation"
	case contains(errStr, "ItemCollectionSizeLimitExceededException"):
		return "item_collection_size_limit"
	case contains(errStr, "TransactionConflictException"):
		return "transaction_conflict"
	case contains(errStr, "RequestLimitExceededException"):
		return "request_limit_exceeded"
	case contains(errStr, "InternalServerError"):
		return "internal_server_error"
	default:
		return StatusUnknown
	}
}

// contains checks if a string contains a substring (case-insensitive)
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		(len(s) > len(substr) &&
			(s[:len(substr)] == substr ||
				s[len(s)-len(substr):] == substr ||
				containsHelper(s, substr))))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
