package cost

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestDefaultTrackingServiceConfig(t *testing.T) {
	config := DefaultTrackingServiceConfig()

	assert.Equal(t, "Lesser/CostTracking", config.CloudWatchNamespace)
	assert.Equal(t, 20, config.MetricsBatchSize)
	assert.Equal(t, 30*time.Second, config.MetricsFlushInterval)
	assert.True(t, config.EnableDetailedMetrics)
	assert.Equal(t, 10.0, config.CostThresholds.DynamoDBReadWarning)
	assert.Equal(t, 50.0, config.CostThresholds.DynamoDBWriteWarning)
	assert.Equal(t, 5.0, config.CostThresholds.S3OperationWarning)
	assert.Equal(t, 25.0, config.CostThresholds.LambdaInvocationWarning)
	assert.Equal(t, 100.0, config.CostThresholds.DailyBudgetLimit)
}

func TestNewTrackingService(t *testing.T) {
	logger := zap.NewNop()
	config := DefaultTrackingServiceConfig()

	// Create without CloudWatch client (nil is acceptable for testing)
	ts := NewTrackingService(nil, logger, config)
	require.NotNil(t, ts)

	assert.NotNil(t, ts.dynamoTracker)
	assert.NotNil(t, ts.s3Tracker)
	assert.NotNil(t, ts.lambdaTracker)
	assert.NotNil(t, ts.stopChan)

	// Clean up
	err := ts.Close(context.Background())
	assert.NoError(t, err)
}

func TestNewCostTrackingService(t *testing.T) {
	logger := zap.NewNop()

	ts := NewCostTrackingService(nil, logger)
	require.NotNil(t, ts)

	err := ts.Close(context.Background())
	assert.NoError(t, err)
}

func TestNewCostTrackingServiceForLambda(t *testing.T) {
	logger := zap.NewNop()

	ts := NewCostTrackingServiceForLambda(nil, logger, "my-function")
	require.NotNil(t, ts)

	assert.Contains(t, ts.config.CloudWatchNamespace, "Lambda")
	assert.Contains(t, ts.config.CloudWatchNamespace, "my-function")
	assert.Equal(t, 10*time.Second, ts.config.MetricsFlushInterval)

	err := ts.Close(context.Background())
	assert.NoError(t, err)
}

func TestNewCostTrackingServiceForRepository(t *testing.T) {
	logger := zap.NewNop()

	ts := NewCostTrackingServiceForRepository(nil, logger, "users")
	require.NotNil(t, ts)

	assert.Contains(t, ts.config.CloudWatchNamespace, "Repository")
	assert.Contains(t, ts.config.CloudWatchNamespace, "users")
	assert.True(t, ts.config.EnableDetailedMetrics)

	err := ts.Close(context.Background())
	assert.NoError(t, err)
}

func TestTrackingService_TrackDynamoOperation(t *testing.T) {
	logger := zap.NewNop()
	ts := NewCostTrackingService(nil, logger)
	defer ts.Close(context.Background())

	ctx := context.Background()
	operation := DynamoOperation{
		Type:               "Query",
		TableName:          "users",
		ConsumedReadUnits:  10,
		ConsumedWriteUnits: 0,
		ItemCount:          5,
		Timestamp:          time.Now(),
	}

	err := ts.TrackDynamoOperation(ctx, operation)
	assert.NoError(t, err)
}

func TestTrackingService_TrackS3Operation(t *testing.T) {
	logger := zap.NewNop()
	ts := NewCostTrackingService(nil, logger)
	defer ts.Close(context.Background())

	ctx := context.Background()
	operation := S3Operation{
		Type:             "PutObject",
		BucketName:       "my-bucket",
		RequestCount:     1,
		BytesTransferred: 1024,
		Timestamp:        time.Now(),
	}

	err := ts.TrackS3Operation(ctx, operation)
	assert.NoError(t, err)
}

func TestTrackingService_TrackLambdaInvocation(t *testing.T) {
	logger := zap.NewNop()
	ts := NewCostTrackingService(nil, logger)
	defer ts.Close(context.Background())

	ctx := context.Background()
	operation := LambdaOperation{
		FunctionName: "my-function",
		Duration:     100 * time.Millisecond,
		MemoryMB:     256,
		ColdStart:    false,
		Timestamp:    time.Now(),
	}

	err := ts.TrackLambdaInvocation(ctx, operation)
	assert.NoError(t, err)
}

func TestTrackingService_RecordMetrics_Empty(t *testing.T) {
	logger := zap.NewNop()
	ts := NewCostTrackingService(nil, logger)
	defer ts.Close(context.Background())

	ctx := context.Background()

	// Empty metrics should not error
	err := ts.RecordMetrics(ctx, []MetricData{})
	assert.NoError(t, err)
}

func TestTrackingService_RecordMetrics(t *testing.T) {
	logger := zap.NewNop()
	ts := NewCostTrackingService(nil, logger)
	defer ts.Close(context.Background())

	ctx := context.Background()
	metrics := []MetricData{
		{
			Name:      "TestMetric",
			Value:     100.0,
			Timestamp: time.Now(),
		},
	}

	err := ts.RecordMetrics(ctx, metrics)
	assert.NoError(t, err)
}

func TestCalculateDynamoDBCost(t *testing.T) {
	cost := CalculateDynamoDBCost(100, 50)

	assert.Equal(t, "DynamoDB", cost.Service)
	assert.True(t, cost.ReadCostMicroCents > 0)
	assert.True(t, cost.WriteCostMicroCents > 0)
	assert.Equal(t, cost.ReadCostMicroCents+cost.WriteCostMicroCents, cost.TotalMicroCents)
}

func TestCalculateS3Cost(t *testing.T) {
	cost := CalculateS3Cost(1000, 1024*1024*1024) // 1000 requests, 1 GB

	assert.Equal(t, "S3", cost.Service)
	assert.True(t, cost.RequestCostMicroCents >= 0)
	assert.True(t, cost.StorageCostMicroCents >= 0)
}

func TestCalculateLambdaCost(t *testing.T) {
	cost := CalculateLambdaCost(time.Second, 1024) // 1 second, 1 GB

	assert.Equal(t, "Lambda", cost.Service)
	assert.True(t, cost.InvocationCostMicroCents > 0)
	assert.True(t, cost.DurationCostMicroCents > 0)
}

func TestCalculateLambdaCost_MinDuration(t *testing.T) {
	cost := CalculateLambdaCost(0, 128) // 0 duration

	assert.Equal(t, "Lambda", cost.Service)
	// Should still have cost due to minimum duration
	assert.True(t, cost.TotalMicroCents > 0)
}

func TestTrackingService_Close(t *testing.T) {
	logger := zap.NewNop()
	ts := NewCostTrackingService(nil, logger)

	// Add some metrics
	ctx := context.Background()
	_ = ts.RecordMetrics(ctx, []MetricData{
		{Name: "Test", Value: 1.0, Timestamp: time.Now()},
	})

	err := ts.Close(ctx)
	assert.NoError(t, err)
}

func TestTrackingService_FlushMetrics_NilCloudWatch(t *testing.T) {
	logger := zap.NewNop()
	ts := NewTrackingService(nil, logger, DefaultTrackingServiceConfig())
	defer ts.Close(context.Background())

	// Add metrics to batch using the proper CloudWatch type
	ts.batchMu.Lock()
	ts.metricsBatch = append(ts.metricsBatch, types.MetricDatum{
		MetricName: aws.String("Test"),
		Value:      aws.Float64(1.0),
		Timestamp:  aws.Time(time.Now()),
	})
	ts.batchMu.Unlock()

	// Flush should succeed even without CloudWatch client
	err := ts.flushMetrics(context.Background())
	assert.NoError(t, err)
}

func TestTrackingService_CheckCostThresholds(t *testing.T) {
	logger := zap.NewNop()
	ts := NewCostTrackingService(nil, logger)
	defer ts.Close(context.Background())

	// These should not panic
	ts.checkCostThresholds("DynamoDB.Read", 5.0)
	ts.checkCostThresholds("DynamoDB.Write", 25.0)
	ts.checkCostThresholds("S3", 2.0)
	ts.checkCostThresholds("Lambda", 10.0)
	ts.checkCostThresholds("Unknown", 1.0)
}

func TestTrackingService_CheckCostThresholds_ExceedsThreshold(t *testing.T) {
	logger := zap.NewNop()
	ts := NewCostTrackingService(nil, logger)
	defer ts.Close(context.Background())

	// Should log warning but not error
	ts.checkCostThresholds("DynamoDB.Read", 100.0) // Exceeds 10.0 threshold
	ts.checkCostThresholds("DynamoDB.Write", 100.0)
	ts.checkCostThresholds("S3", 100.0)
	ts.checkCostThresholds("Lambda", 100.0)
}

func TestTrackingService_TrackDynamoOperation_WithError(t *testing.T) {
	logger := zap.NewNop()
	ts := NewCostTrackingService(nil, logger)
	defer ts.Close(context.Background())

	ctx := context.Background()
	operation := DynamoOperation{
		Type:               "Query",
		TableName:          "users",
		ConsumedReadUnits:  10,
		ConsumedWriteUnits: 0,
		ItemCount:          5,
		Timestamp:          time.Now(),
	}

	// Should succeed even without CloudWatch client
	err := ts.TrackDynamoOperation(ctx, operation)
	assert.NoError(t, err)
}

func TestTrackingService_TrackS3Operation_WithError(t *testing.T) {
	logger := zap.NewNop()
	ts := NewCostTrackingService(nil, logger)
	defer ts.Close(context.Background())

	ctx := context.Background()
	operation := S3Operation{
		Type:             "PutObject",
		BucketName:       "my-bucket",
		RequestCount:     1,
		BytesTransferred: 1024,
		Timestamp:        time.Now(),
	}

	err := ts.TrackS3Operation(ctx, operation)
	assert.NoError(t, err)
}

func TestTrackingService_TrackLambdaInvocation_WithError(t *testing.T) {
	logger := zap.NewNop()
	ts := NewCostTrackingService(nil, logger)
	defer ts.Close(context.Background())

	ctx := context.Background()
	operation := LambdaOperation{
		FunctionName: "my-function",
		Duration:     100 * time.Millisecond,
		MemoryMB:     256,
		ColdStart:    true,
		Timestamp:    time.Now(),
	}

	err := ts.TrackLambdaInvocation(ctx, operation)
	assert.NoError(t, err)
}

func TestTrackingService_RecordMetrics_BatchFull(t *testing.T) {
	logger := zap.NewNop()
	config := DefaultTrackingServiceConfig()
	config.MetricsBatchSize = 2 // Small batch size to trigger flush
	ts := NewTrackingService(nil, logger, config)
	defer ts.Close(context.Background())

	ctx := context.Background()

	// Add metrics to fill the batch
	for i := 0; i < 3; i++ {
		metrics := []MetricData{
			{
				Name:      "TestMetric",
				Value:     float64(i),
				Timestamp: time.Now(),
			},
		}
		err := ts.RecordMetrics(ctx, metrics)
		assert.NoError(t, err)
	}
}
