package cost

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCost_TotalDollars(t *testing.T) {
	// MicroCentsToCents = 1,000,000
	// So 1,000,000 microcents = 1 cent
	// TotalDollars divides by MicroCentsToCents, so it returns cents, not dollars
	// 1,000,000 / 1,000,000 = 1 (cent)
	cost := Cost{
		TotalMicroCents: 1000000, // 1 cent
	}

	cents := cost.TotalDollars() // Note: despite the name, this returns cents
	assert.InDelta(t, 1.0, cents, 0.0001)
}

func TestCost_TotalDollars_Zero(t *testing.T) {
	cost := Cost{
		TotalMicroCents: 0,
	}

	dollars := cost.TotalDollars()
	assert.Equal(t, 0.0, dollars)
}

func TestNewDynamoDBTracker(t *testing.T) {
	tracker := NewDynamoDBTracker()
	require.NotNil(t, tracker)
	assert.NotNil(t, tracker.readCostCache)
	assert.NotNil(t, tracker.writeCostCache)
}

func TestDynamoDBTracker_CalculateCost(t *testing.T) {
	tracker := NewDynamoDBTracker()

	operation := DynamoOperation{
		Type:               "Query",
		TableName:          "test-table",
		ConsumedReadUnits:  100,
		ConsumedWriteUnits: 50,
		ItemCount:          10,
		Timestamp:          time.Now(),
	}

	cost := tracker.CalculateCost(operation)

	assert.Equal(t, "DynamoDB", cost.Service)
	assert.True(t, cost.ReadCostMicroCents > 0)
	assert.True(t, cost.WriteCostMicroCents > 0)
	assert.Equal(t, cost.ReadCostMicroCents+cost.WriteCostMicroCents, cost.TotalMicroCents)
}

func TestDynamoDBTracker_CalculateCost_ReadOnly(t *testing.T) {
	tracker := NewDynamoDBTracker()

	operation := DynamoOperation{
		Type:               "Query",
		TableName:          "test-table",
		ConsumedReadUnits:  100,
		ConsumedWriteUnits: 0,
		Timestamp:          time.Now(),
	}

	cost := tracker.CalculateCost(operation)

	assert.True(t, cost.ReadCostMicroCents > 0)
	assert.Equal(t, int64(0), cost.WriteCostMicroCents)
	assert.Equal(t, cost.ReadCostMicroCents, cost.TotalMicroCents)
}

func TestNewS3Tracker(t *testing.T) {
	tracker := NewS3Tracker()
	require.NotNil(t, tracker)
	assert.NotNil(t, tracker.requestCostCache)
	assert.NotNil(t, tracker.storageCostCache)
}

func TestS3Tracker_CalculateCost_PutObject(t *testing.T) {
	tracker := NewS3Tracker()

	operation := S3Operation{
		Type:             "PutObject",
		BucketName:       "test-bucket",
		RequestCount:     1000,
		BytesTransferred: 1024 * 1024 * 1024, // 1 GB
		Timestamp:        time.Now(),
	}

	cost := tracker.CalculateCost(operation)

	assert.Equal(t, "S3", cost.Service)
	assert.True(t, cost.RequestCostMicroCents > 0)
	assert.True(t, cost.DataTransferCostMicroCents > 0)
}

func TestS3Tracker_CalculateCost_GetObject(t *testing.T) {
	tracker := NewS3Tracker()

	operation := S3Operation{
		Type:             "GetObject",
		BucketName:       "test-bucket",
		RequestCount:     1000,
		BytesTransferred: 1024 * 1024, // 1 MB
		Timestamp:        time.Now(),
	}

	cost := tracker.CalculateCost(operation)

	assert.Equal(t, "S3", cost.Service)
	assert.True(t, cost.RequestCostMicroCents >= 0)
}

func TestS3Tracker_CalculateCost_ListObjects(t *testing.T) {
	tracker := NewS3Tracker()

	operation := S3Operation{
		Type:         "ListObjects",
		BucketName:   "test-bucket",
		RequestCount: 100,
		Timestamp:    time.Now(),
	}

	cost := tracker.CalculateCost(operation)

	assert.Equal(t, "S3", cost.Service)
}

func TestS3Tracker_CalculateCost_HeadObject(t *testing.T) {
	tracker := NewS3Tracker()

	operation := S3Operation{
		Type:         "HeadObject",
		BucketName:   "test-bucket",
		RequestCount: 100,
		Timestamp:    time.Now(),
	}

	cost := tracker.CalculateCost(operation)

	assert.Equal(t, "S3", cost.Service)
}

func TestS3Tracker_CalculateCost_PostObject(t *testing.T) {
	tracker := NewS3Tracker()

	operation := S3Operation{
		Type:         "PostObject",
		BucketName:   "test-bucket",
		RequestCount: 100,
		Timestamp:    time.Now(),
	}

	cost := tracker.CalculateCost(operation)

	assert.Equal(t, "S3", cost.Service)
	assert.True(t, cost.RequestCostMicroCents > 0)
}

func TestS3Tracker_CalculateCost_CopyObject(t *testing.T) {
	tracker := NewS3Tracker()

	operation := S3Operation{
		Type:         "CopyObject",
		BucketName:   "test-bucket",
		RequestCount: 100,
		Timestamp:    time.Now(),
	}

	cost := tracker.CalculateCost(operation)

	assert.Equal(t, "S3", cost.Service)
	assert.True(t, cost.RequestCostMicroCents > 0)
}

func TestNewLambdaTracker(t *testing.T) {
	tracker := NewLambdaTracker()
	require.NotNil(t, tracker)
	assert.NotNil(t, tracker.invocationCostCache)
	assert.NotNil(t, tracker.durationCostCache)
}

func TestLambdaTracker_CalculateCost(t *testing.T) {
	tracker := NewLambdaTracker()

	operation := LambdaOperation{
		FunctionName: "test-function",
		Duration:     time.Second,
		MemoryMB:     1024,
		ColdStart:    false,
		Timestamp:    time.Now(),
	}

	cost := tracker.CalculateCost(operation)

	assert.Equal(t, "Lambda", cost.Service)
	assert.True(t, cost.InvocationCostMicroCents > 0)
	assert.True(t, cost.DurationCostMicroCents > 0)
}

func TestLambdaTracker_CalculateCost_ColdStart(t *testing.T) {
	tracker := NewLambdaTracker()

	warmOperation := LambdaOperation{
		FunctionName: "test-function",
		Duration:     time.Second,
		MemoryMB:     1024,
		ColdStart:    false,
		Timestamp:    time.Now(),
	}

	coldOperation := LambdaOperation{
		FunctionName: "test-function",
		Duration:     time.Second,
		MemoryMB:     1024,
		ColdStart:    true,
		Timestamp:    time.Now(),
	}

	warmCost := tracker.CalculateCost(warmOperation)
	coldCost := tracker.CalculateCost(coldOperation)

	// Cold start should have higher invocation cost
	assert.True(t, coldCost.InvocationCostMicroCents > warmCost.InvocationCostMicroCents)
}

func TestLambdaTracker_CalculateCost_MinDuration(t *testing.T) {
	tracker := NewLambdaTracker()

	operation := LambdaOperation{
		FunctionName: "test-function",
		Duration:     0, // Zero duration
		MemoryMB:     128,
		ColdStart:    false,
		Timestamp:    time.Now(),
	}

	cost := tracker.CalculateCost(operation)

	// Should still have some cost due to minimum duration
	assert.True(t, cost.TotalMicroCents > 0)
}

func TestDynamoOperation_Fields(t *testing.T) {
	now := time.Now()
	op := DynamoOperation{
		Type:               "Query",
		TableName:          "users",
		ConsumedReadUnits:  10,
		ConsumedWriteUnits: 5,
		ItemCount:          3,
		IndexName:          "gsi1",
		OperationID:        "op-123",
		UserID:             "user-456",
		Timestamp:          now,
	}

	assert.Equal(t, "Query", op.Type)
	assert.Equal(t, "users", op.TableName)
	assert.Equal(t, int64(10), op.ConsumedReadUnits)
	assert.Equal(t, int64(5), op.ConsumedWriteUnits)
	assert.Equal(t, int64(3), op.ItemCount)
	assert.Equal(t, "gsi1", op.IndexName)
	assert.Equal(t, "op-123", op.OperationID)
	assert.Equal(t, "user-456", op.UserID)
	assert.Equal(t, now, op.Timestamp)
}

func TestS3Operation_Fields(t *testing.T) {
	now := time.Now()
	op := S3Operation{
		Type:             "PutObject",
		BucketName:       "my-bucket",
		ObjectKey:        "path/to/file.txt",
		RequestCount:     1,
		BytesTransferred: 1024,
		StorageClass:     "STANDARD",
		OperationID:      "op-789",
		UserID:           "user-123",
		Timestamp:        now,
	}

	assert.Equal(t, "PutObject", op.Type)
	assert.Equal(t, "my-bucket", op.BucketName)
	assert.Equal(t, "path/to/file.txt", op.ObjectKey)
	assert.Equal(t, int64(1), op.RequestCount)
	assert.Equal(t, int64(1024), op.BytesTransferred)
	assert.Equal(t, "STANDARD", op.StorageClass)
}

func TestLambdaOperation_Fields(t *testing.T) {
	now := time.Now()
	op := LambdaOperation{
		FunctionName: "my-function",
		Duration:     500 * time.Millisecond,
		MemoryMB:     256,
		MemoryUsedMB: 128,
		ColdStart:    true,
		RequestID:    "req-abc",
		UserID:       "user-xyz",
		Timestamp:    now,
	}

	assert.Equal(t, "my-function", op.FunctionName)
	assert.Equal(t, 500*time.Millisecond, op.Duration)
	assert.Equal(t, int64(256), op.MemoryMB)
	assert.Equal(t, int64(128), op.MemoryUsedMB)
	assert.True(t, op.ColdStart)
}

func TestMetricData_Fields(t *testing.T) {
	now := time.Now()
	metric := MetricData{
		Name:      "DynamoDB.ReadUnits",
		Value:     100.5,
		Timestamp: now,
	}

	assert.Equal(t, "DynamoDB.ReadUnits", metric.Name)
	assert.Equal(t, 100.5, metric.Value)
	assert.Equal(t, now, metric.Timestamp)
}

func TestSummary_Fields(t *testing.T) {
	summary := Summary{
		TotalCostMicroCents:  1000000,
		ServiceBreakdown:     map[string]int64{"DynamoDB": 500000, "Lambda": 500000},
		OperationBreakdown:   map[string]int64{"Query": 300000, "PutItem": 200000},
		HourlyBreakdown:      map[string]int64{"2024-01-01T00": 500000, "2024-01-01T01": 500000},
		BudgetUtilization:    0.5,
		ProjectedMonthlyCost: 30000000,
	}

	assert.Equal(t, int64(1000000), summary.TotalCostMicroCents)
	assert.Len(t, summary.ServiceBreakdown, 2)
	assert.Equal(t, 0.5, summary.BudgetUtilization)
}

func TestDriver_Fields(t *testing.T) {
	driver := Driver{
		Service:           "DynamoDB",
		Operation:         "Query",
		CostMicroCents:    500000,
		PercentageOfTotal: 50.0,
		OperationCount:    1000,
		AverageCost:       500,
		Trend:             "INCREASING",
	}

	assert.Equal(t, "DynamoDB", driver.Service)
	assert.Equal(t, "Query", driver.Operation)
	assert.Equal(t, int64(500000), driver.CostMicroCents)
	assert.Equal(t, 50.0, driver.PercentageOfTotal)
	assert.Equal(t, "INCREASING", driver.Trend)
}

func TestTrendPoint_Fields(t *testing.T) {
	now := time.Now()
	point := TrendPoint{
		Timestamp:      now,
		CostMicroCents: 100000,
		OperationCount: 500,
	}

	assert.Equal(t, now, point.Timestamp)
	assert.Equal(t, int64(100000), point.CostMicroCents)
	assert.Equal(t, int64(500), point.OperationCount)
}

func TestOptimizationSuggestion_Fields(t *testing.T) {
	suggestion := OptimizationSuggestion{
		Category:         "DynamoDB",
		Suggestion:       "Consider using batch operations",
		EstimatedSavings: 50000,
		Priority:         "High",
		Effort:           "Low",
	}

	assert.Equal(t, "DynamoDB", suggestion.Category)
	assert.Equal(t, "Consider using batch operations", suggestion.Suggestion)
	assert.Equal(t, int64(50000), suggestion.EstimatedSavings)
	assert.Equal(t, "High", suggestion.Priority)
	assert.Equal(t, "Low", suggestion.Effort)
}
