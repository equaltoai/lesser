package cost

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

// TrackingServiceConfig contains configuration for the centralized cost tracking service
type TrackingServiceConfig struct {
	CloudWatchNamespace   string
	MetricsBatchSize      int
	MetricsFlushInterval  time.Duration
	EnableDetailedMetrics bool
	CostThresholds        Thresholds
}

// Thresholds defines cost alert thresholds for various AWS services
type Thresholds struct {
	DynamoDBReadWarning     float64 // dollars
	DynamoDBWriteWarning    float64 // dollars
	S3OperationWarning      float64 // dollars
	LambdaInvocationWarning float64 // dollars
	DailyBudgetLimit        float64 // dollars
}

// DefaultTrackingServiceConfig returns sensible defaults for cost tracking
func DefaultTrackingServiceConfig() TrackingServiceConfig {
	return TrackingServiceConfig{
		CloudWatchNamespace:   "Lesser/CostTracking",
		MetricsBatchSize:      20,
		MetricsFlushInterval:  30 * time.Second,
		EnableDetailedMetrics: true,
		CostThresholds: Thresholds{
			DynamoDBReadWarning:     10.0,  // $10/day
			DynamoDBWriteWarning:    50.0,  // $50/day
			S3OperationWarning:      5.0,   // $5/day
			LambdaInvocationWarning: 25.0,  // $25/day
			DailyBudgetLimit:        100.0, // $100/day total
		},
	}
}

// TrackingService provides centralized cost tracking for all AWS operations
type TrackingService struct {
	config     TrackingServiceConfig
	cloudWatch *cloudwatch.Client
	logger     *zap.Logger

	// Core trackers
	dynamoTracker *DynamoDBTracker
	s3Tracker     *S3Tracker
	lambdaTracker *LambdaTracker

	// Metrics batching
	metricsBatch []types.MetricDatum
	batchMu      sync.Mutex
	flushTicker  *time.Ticker
	stopChan     chan struct{}
	wg           sync.WaitGroup
}

// NewTrackingService creates a new centralized cost tracking service
func NewTrackingService(cloudWatch *cloudwatch.Client, logger *zap.Logger, config TrackingServiceConfig) *TrackingService {
	ts := &TrackingService{
		config:        config,
		cloudWatch:    cloudWatch,
		logger:        logger,
		dynamoTracker: NewDynamoDBTracker(),
		s3Tracker:     NewS3Tracker(),
		lambdaTracker: NewLambdaTracker(),
		metricsBatch:  make([]types.MetricDatum, 0, config.MetricsBatchSize),
		stopChan:      make(chan struct{}),
	}

	// Start metrics flushing goroutine
	ts.startMetricsFlusher()

	return ts
}

// DynamoDB Operation Tracking

// TrackDynamoOperation tracks a DynamoDB operation with comprehensive cost calculation
func (ts *TrackingService) TrackDynamoOperation(ctx context.Context, operation DynamoOperation) error {
	cost := ts.dynamoTracker.CalculateCost(operation)

	ts.logger.Debug("tracking DynamoDB operation",
		zap.String("operation", operation.Type),
		zap.String("table", operation.TableName),
		zap.Float64("cost_dollars", cost.TotalDollars()),
		zap.Int64("read_units", operation.ConsumedReadUnits),
		zap.Int64("write_units", operation.ConsumedWriteUnits),
	)

	// Record metrics
	if err := ts.recordDynamoMetrics(ctx, operation, cost); err != nil {
		ts.logger.Error("failed to record DynamoDB metrics", zap.Error(err))
		return fmt.Errorf("failed to record DynamoDB metrics: %w", err)
	}

	// Check cost thresholds
	ts.checkCostThresholds(operation.Type, cost.TotalDollars())

	return nil
}

// S3 Operation Tracking

// TrackS3Operation tracks an S3 operation with cost calculation
func (ts *TrackingService) TrackS3Operation(ctx context.Context, operation S3Operation) error {
	cost := ts.s3Tracker.CalculateCost(operation)

	ts.logger.Debug("tracking S3 operation",
		zap.String("operation", operation.Type),
		zap.String("bucket", operation.BucketName),
		zap.Float64("cost_dollars", cost.TotalDollars()),
		zap.Int64("requests", operation.RequestCount),
		zap.Int64("bytes", operation.BytesTransferred),
	)

	// Record metrics
	if err := ts.recordS3Metrics(ctx, operation, cost); err != nil {
		ts.logger.Error("failed to record S3 metrics", zap.Error(err))
		return fmt.Errorf("failed to record S3 metrics: %w", err)
	}

	// Check cost thresholds
	ts.checkCostThresholds("S3", cost.TotalDollars())

	return nil
}

// Lambda Operation Tracking

// TrackLambdaInvocation tracks a Lambda invocation with cost calculation
func (ts *TrackingService) TrackLambdaInvocation(ctx context.Context, operation LambdaOperation) error {
	cost := ts.lambdaTracker.CalculateCost(operation)

	ts.logger.Debug("tracking Lambda invocation",
		zap.String("function", operation.FunctionName),
		zap.Duration("duration", operation.Duration),
		zap.Int64("memory_mb", operation.MemoryMB),
		zap.Float64("cost_dollars", cost.TotalDollars()),
	)

	// Record metrics
	if err := ts.recordLambdaMetrics(ctx, operation, cost); err != nil {
		ts.logger.Error("failed to record Lambda metrics", zap.Error(err))
		return fmt.Errorf("failed to record Lambda metrics: %w", err)
	}

	// Check cost thresholds
	ts.checkCostThresholds("Lambda", cost.TotalDollars())

	return nil
}

// Bulk Operations

// RecordMetrics records a batch of custom metrics to CloudWatch
func (ts *TrackingService) RecordMetrics(ctx context.Context, metrics []MetricData) error {
	if err := common.ValidateSliceNotEmpty("metrics", metrics); err != nil {
		return nil
	}

	// Convert to CloudWatch format
	cwMetrics := make([]types.MetricDatum, 0, len(metrics))
	for _, metric := range metrics {
		cwMetrics = append(cwMetrics, types.MetricDatum{
			MetricName: aws.String(metric.Name),
			Value:      aws.Float64(metric.Value),
			Unit:       metric.Unit,
			Dimensions: metric.Dimensions,
			Timestamp:  aws.Time(metric.Timestamp),
		})
	}

	// Add to batch for efficient sending
	ts.batchMu.Lock()
	ts.metricsBatch = append(ts.metricsBatch, cwMetrics...)
	shouldFlush := len(ts.metricsBatch) >= ts.config.MetricsBatchSize
	ts.batchMu.Unlock()

	// Flush immediately if batch is full
	if shouldFlush {
		return ts.flushMetrics(ctx)
	}

	return nil
}

// Operation Cost Calculation Utilities

// CalculateDynamoDBCost calculates the cost of DynamoDB operations
func CalculateDynamoDBCost(readUnits, writeUnits float64) Cost {
	readCostMicroCents := int64(readUnits * float64(DynamoDBReadRequestUnit))
	writeCostMicroCents := int64(writeUnits * float64(DynamoDBWriteRequestUnit))

	return Cost{
		Service:             "DynamoDB",
		ReadCostMicroCents:  readCostMicroCents,
		WriteCostMicroCents: writeCostMicroCents,
		TotalMicroCents:     readCostMicroCents + writeCostMicroCents,
		Timestamp:           time.Now(),
	}
}

// CalculateS3Cost calculates the cost of S3 operations
func CalculateS3Cost(requests int64, storage float64) Cost {
	requestCostMicroCents := int64(float64(requests) * float64(S3PutRequestCost) / 1000)
	storageCostMicroCents := int64(storage * float64(S3StorageStandardGB) / (1024 * 1024 * 1024))

	return Cost{
		Service:               "S3",
		RequestCostMicroCents: requestCostMicroCents,
		StorageCostMicroCents: storageCostMicroCents,
		TotalMicroCents:       requestCostMicroCents + storageCostMicroCents,
		Timestamp:             time.Now(),
	}
}

// CalculateLambdaCost calculates the cost of Lambda invocations
func CalculateLambdaCost(duration time.Duration, memory int64) Cost {
	durationMs := duration.Milliseconds()
	if durationMs < LambdaDurationMinMS {
		durationMs = LambdaDurationMinMS
	}

	gbSeconds := float64(memory) / 1024.0 * float64(durationMs) / 1000.0
	invocationCostMicroCents := int64(LambdaRequestCost)
	durationCostMicroCents := int64(gbSeconds * float64(LambdaGBSecondCost))

	return Cost{
		Service:                  "Lambda",
		InvocationCostMicroCents: invocationCostMicroCents,
		DurationCostMicroCents:   durationCostMicroCents,
		TotalMicroCents:          invocationCostMicroCents + durationCostMicroCents,
		Timestamp:                time.Now(),
	}
}

// Factory Functions

// NewCostTrackingService creates a tracking service with default configuration
func NewCostTrackingService(cloudWatch *cloudwatch.Client, logger *zap.Logger) *TrackingService {
	return NewTrackingService(cloudWatch, logger, DefaultTrackingServiceConfig())
}

// NewCostTrackingServiceForLambda creates a tracking service optimized for Lambda environments
func NewCostTrackingServiceForLambda(cloudWatch *cloudwatch.Client, logger *zap.Logger, functionName string) *TrackingService {
	config := DefaultTrackingServiceConfig()
	config.CloudWatchNamespace = fmt.Sprintf("Lesser/Lambda/%s", functionName)
	config.MetricsFlushInterval = 10 * time.Second // More frequent flushing for Lambda
	return NewTrackingService(cloudWatch, logger, config)
}

// NewCostTrackingServiceForRepository creates a tracking service optimized for repository operations
func NewCostTrackingServiceForRepository(cloudWatch *cloudwatch.Client, logger *zap.Logger, repositoryName string) *TrackingService {
	config := DefaultTrackingServiceConfig()
	config.CloudWatchNamespace = fmt.Sprintf("Lesser/Repository/%s", repositoryName)
	config.EnableDetailedMetrics = true
	return NewTrackingService(cloudWatch, logger, config)
}

// Cleanup

// Close gracefully shuts down the tracking service
func (ts *TrackingService) Close(ctx context.Context) error {
	// Stop the metrics flusher
	close(ts.stopChan)
	ts.wg.Wait()

	// Flush any remaining metrics
	return ts.flushMetrics(ctx)
}

// Private methods for internal operations

func (ts *TrackingService) startMetricsFlusher() {
	ts.flushTicker = time.NewTicker(ts.config.MetricsFlushInterval)
	ts.wg.Add(1)

	go func() {
		defer ts.wg.Done()
		defer ts.flushTicker.Stop()

		for {
			select {
			case <-ts.flushTicker.C:
				if err := ts.flushMetrics(context.Background()); err != nil {
					ts.logger.Error("failed to flush metrics", zap.Error(err))
				}
			case <-ts.stopChan:
				return
			}
		}
	}()
}

func (ts *TrackingService) flushMetrics(ctx context.Context) error {
	ts.batchMu.Lock()
	if err := common.ValidateSliceNotEmpty("ts.metricsBatch", ts.metricsBatch); err != nil {
		ts.batchMu.Unlock()
		return nil
	}

	// Copy batch and reset
	batch := make([]types.MetricDatum, len(ts.metricsBatch))
	copy(batch, ts.metricsBatch)
	ts.metricsBatch = ts.metricsBatch[:0]
	ts.batchMu.Unlock()

	// Send to CloudWatch
	input := &cloudwatch.PutMetricDataInput{
		Namespace:  aws.String(ts.config.CloudWatchNamespace),
		MetricData: batch,
	}

	if _, err := ts.cloudWatch.PutMetricData(ctx, input); err != nil {
		return fmt.Errorf("failed to put metric data: %w", err)
	}

	ts.logger.Debug("flushed metrics to CloudWatch",
		zap.Int("metric_count", len(batch)),
		zap.String("namespace", ts.config.CloudWatchNamespace),
	)

	return nil
}

// ServiceMetric represents a metric specific to a service
type ServiceMetric struct {
	Name  string
	Value float64
	Unit  types.StandardUnit
}

// recordServiceMetrics provides a generic method for recording service-specific metrics
func (ts *TrackingService) recordServiceMetrics(ctx context.Context, serviceName string, cost Cost, serviceMetrics []ServiceMetric, dimensions []types.Dimension) error {
	metrics := make([]MetricData, 0, len(serviceMetrics)+1)

	// Add service-specific metrics
	for _, metric := range serviceMetrics {
		metrics = append(metrics, MetricData{
			Name:       fmt.Sprintf("%s.%s", serviceName, metric.Name),
			Value:      metric.Value,
			Unit:       metric.Unit,
			Timestamp:  time.Now(),
			Dimensions: dimensions,
		})
	}

	// Add cost metric
	metrics = append(metrics, MetricData{
		Name:       fmt.Sprintf("%s.Cost", serviceName),
		Value:      cost.TotalDollars(),
		Unit:       types.StandardUnitNone,
		Timestamp:  time.Now(),
		Dimensions: dimensions,
	})

	return ts.RecordMetrics(ctx, metrics)
}

func (ts *TrackingService) recordDynamoMetrics(ctx context.Context, operation DynamoOperation, cost Cost) error {
	return ts.recordServiceMetrics(ctx, "DynamoDB", cost, []ServiceMetric{
		{Name: "ReadUnits", Value: float64(operation.ConsumedReadUnits), Unit: types.StandardUnitCount},
		{Name: "WriteUnits", Value: float64(operation.ConsumedWriteUnits), Unit: types.StandardUnitCount},
	}, []types.Dimension{
		{Name: aws.String("Operation"), Value: aws.String(operation.Type)},
		{Name: aws.String("Table"), Value: aws.String(operation.TableName)},
	})
}

func (ts *TrackingService) recordS3Metrics(ctx context.Context, operation S3Operation, cost Cost) error {
	return ts.recordServiceMetrics(ctx, "S3", cost, []ServiceMetric{
		{Name: "Requests", Value: float64(operation.RequestCount), Unit: types.StandardUnitCount},
		{Name: "BytesTransferred", Value: float64(operation.BytesTransferred), Unit: types.StandardUnitBytes},
	}, []types.Dimension{
		{Name: aws.String("Operation"), Value: aws.String(operation.Type)},
		{Name: aws.String("Bucket"), Value: aws.String(operation.BucketName)},
	})
}

func (ts *TrackingService) recordLambdaMetrics(ctx context.Context, operation LambdaOperation, cost Cost) error {
	return ts.recordServiceMetrics(ctx, "Lambda", cost, []ServiceMetric{
		{Name: "Duration", Value: float64(operation.Duration.Milliseconds()), Unit: types.StandardUnitMilliseconds},
		{Name: "Memory", Value: float64(operation.MemoryMB), Unit: types.StandardUnitBytes},
	}, []types.Dimension{
		{Name: aws.String("Function"), Value: aws.String(operation.FunctionName)},
	})
}

func (ts *TrackingService) checkCostThresholds(operationType string, costDollars float64) {
	var threshold float64
	switch operationType {
	case "DynamoDB.Read":
		threshold = ts.config.CostThresholds.DynamoDBReadWarning
	case "DynamoDB.Write":
		threshold = ts.config.CostThresholds.DynamoDBWriteWarning
	case "S3":
		threshold = ts.config.CostThresholds.S3OperationWarning
	case "Lambda":
		threshold = ts.config.CostThresholds.LambdaInvocationWarning
	default:
		return
	}

	if costDollars > threshold {
		ts.logger.Warn("cost threshold exceeded",
			zap.String("operation", operationType),
			zap.Float64("cost_dollars", costDollars),
			zap.Float64("threshold_dollars", threshold),
		)
	}
}
