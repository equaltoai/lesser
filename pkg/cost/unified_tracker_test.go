package cost

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNewUnifiedTracker(t *testing.T) {
	logger := zap.NewNop()

	tracker := NewUnifiedTracker(nil, logger, "user-123", "req-456")
	require.NotNil(t, tracker)

	assert.Equal(t, "user-123", tracker.userID)
	assert.Equal(t, "req-456", tracker.requestID)
	assert.NotNil(t, tracker.service)
	assert.NotNil(t, tracker.serviceCosts)
	assert.NotNil(t, tracker.operationCounts)

	// Clean up
	_ = tracker.Close(context.Background())
}

func TestNewUnifiedTrackerWithService(t *testing.T) {
	logger := zap.NewNop()
	service := NewCostTrackingService(nil, logger)
	defer service.Close(context.Background())

	tracker := NewUnifiedTrackerWithService(service, logger, "user-123", "req-456")
	require.NotNil(t, tracker)

	assert.Equal(t, service, tracker.service)
}

func TestUnifiedTracker_TrackDynamoRead(t *testing.T) {
	logger := zap.NewNop()
	tracker := NewUnifiedTracker(nil, logger, "user-123", "req-456")
	defer tracker.Close(context.Background())

	ctx := context.Background()
	err := tracker.TrackDynamoRead(ctx, "users", 10)
	assert.NoError(t, err)

	assert.True(t, tracker.GetCurrentCostMicroCents() > 0)
	breakdown := tracker.GetCostBreakdown()
	assert.True(t, breakdown["DynamoDB"] > 0)
}

func TestUnifiedTracker_TrackDynamoWrite(t *testing.T) {
	logger := zap.NewNop()
	tracker := NewUnifiedTracker(nil, logger, "user-123", "req-456")
	defer tracker.Close(context.Background())

	ctx := context.Background()
	err := tracker.TrackDynamoWrite(ctx, "users", 5)
	assert.NoError(t, err)

	assert.True(t, tracker.GetCurrentCostMicroCents() > 0)
}

func TestUnifiedTracker_TrackDynamoQuery(t *testing.T) {
	logger := zap.NewNop()
	tracker := NewUnifiedTracker(nil, logger, "user-123", "req-456")
	defer tracker.Close(context.Background())

	ctx := context.Background()
	err := tracker.TrackDynamoQuery(ctx, "users", 20)
	assert.NoError(t, err)

	counts := tracker.GetOperationCounts()
	assert.Equal(t, int64(1), counts["Query"])
}

func TestUnifiedTracker_TrackDynamoScan(t *testing.T) {
	logger := zap.NewNop()
	tracker := NewUnifiedTracker(nil, logger, "user-123", "req-456")
	defer tracker.Close(context.Background())

	ctx := context.Background()
	err := tracker.TrackDynamoScan(ctx, "users", 100)
	assert.NoError(t, err)

	counts := tracker.GetOperationCounts()
	assert.Equal(t, int64(1), counts["Scan"])
}

func TestUnifiedTracker_TrackS3Get(t *testing.T) {
	logger := zap.NewNop()
	tracker := NewUnifiedTracker(nil, logger, "user-123", "req-456")
	defer tracker.Close(context.Background())

	ctx := context.Background()
	err := tracker.TrackS3Get(ctx, "my-bucket", 1024)
	assert.NoError(t, err)

	breakdown := tracker.GetCostBreakdown()
	assert.True(t, breakdown["S3"] >= 0)
}

func TestUnifiedTracker_TrackS3Put(t *testing.T) {
	logger := zap.NewNop()
	tracker := NewUnifiedTracker(nil, logger, "user-123", "req-456")
	defer tracker.Close(context.Background())

	ctx := context.Background()
	err := tracker.TrackS3Put(ctx, "my-bucket", 2048)
	assert.NoError(t, err)

	counts := tracker.GetOperationCounts()
	assert.Equal(t, int64(1), counts["Put"])
}

func TestUnifiedTracker_TrackS3Delete(t *testing.T) {
	logger := zap.NewNop()
	tracker := NewUnifiedTracker(nil, logger, "user-123", "req-456")
	defer tracker.Close(context.Background())

	ctx := context.Background()
	err := tracker.TrackS3Delete(ctx, "my-bucket")
	assert.NoError(t, err)

	counts := tracker.GetOperationCounts()
	assert.Equal(t, int64(1), counts["Delete"])
}

func TestUnifiedTracker_TrackLambdaInvocation(t *testing.T) {
	logger := zap.NewNop()
	tracker := NewUnifiedTracker(nil, logger, "user-123", "req-456")
	defer tracker.Close(context.Background())

	ctx := context.Background()
	err := tracker.TrackLambdaInvocation(ctx, "my-function", 100*time.Millisecond, 256)
	assert.NoError(t, err)

	breakdown := tracker.GetCostBreakdown()
	assert.True(t, breakdown["Lambda"] > 0)
}

func TestUnifiedTracker_GetCurrentCostMicroCents(t *testing.T) {
	logger := zap.NewNop()
	tracker := NewUnifiedTracker(nil, logger, "user-123", "req-456")
	defer tracker.Close(context.Background())

	assert.Equal(t, int64(0), tracker.GetCurrentCostMicroCents())

	ctx := context.Background()
	_ = tracker.TrackDynamoRead(ctx, "users", 100)

	assert.True(t, tracker.GetCurrentCostMicroCents() > 0)
}

func TestUnifiedTracker_GetCostBreakdown(t *testing.T) {
	logger := zap.NewNop()
	tracker := NewUnifiedTracker(nil, logger, "user-123", "req-456")
	defer tracker.Close(context.Background())

	ctx := context.Background()
	_ = tracker.TrackDynamoRead(ctx, "users", 10)
	_ = tracker.TrackS3Get(ctx, "bucket", 1024)
	_ = tracker.TrackLambdaInvocation(ctx, "func", 100*time.Millisecond, 128)

	breakdown := tracker.GetCostBreakdown()
	assert.NotNil(t, breakdown)
	assert.True(t, breakdown["DynamoDB"] > 0)
	assert.True(t, breakdown["Lambda"] > 0)
}

func TestUnifiedTracker_Reset(t *testing.T) {
	logger := zap.NewNop()
	tracker := NewUnifiedTracker(nil, logger, "user-123", "req-456")
	defer tracker.Close(context.Background())

	ctx := context.Background()
	_ = tracker.TrackDynamoRead(ctx, "users", 10)
	_ = tracker.TrackS3Get(ctx, "bucket", 1024)

	assert.True(t, tracker.GetCurrentCostMicroCents() > 0)

	tracker.Reset()

	assert.Equal(t, int64(0), tracker.GetCurrentCostMicroCents())
	assert.Empty(t, tracker.GetCostBreakdown())
	assert.Empty(t, tracker.GetOperationCounts())
}

func TestUnifiedTracker_GetOperationCounts(t *testing.T) {
	logger := zap.NewNop()
	tracker := NewUnifiedTracker(nil, logger, "user-123", "req-456")
	defer tracker.Close(context.Background())

	ctx := context.Background()
	_ = tracker.TrackDynamoRead(ctx, "users", 10)
	_ = tracker.TrackDynamoRead(ctx, "users", 20)
	_ = tracker.TrackDynamoWrite(ctx, "users", 5)

	counts := tracker.GetOperationCounts()
	assert.Equal(t, int64(2), counts["Read"])
	assert.Equal(t, int64(1), counts["Write"])
}

func TestUnifiedTracker_GetCostDollars(t *testing.T) {
	logger := zap.NewNop()
	tracker := NewUnifiedTracker(nil, logger, "user-123", "req-456")
	defer tracker.Close(context.Background())

	assert.Equal(t, 0.0, tracker.GetCostDollars())

	ctx := context.Background()
	_ = tracker.TrackDynamoRead(ctx, "users", 1000000) // Large read

	assert.True(t, tracker.GetCostDollars() > 0)
}

func TestUnifiedTracker_Close(t *testing.T) {
	logger := zap.NewNop()
	tracker := NewUnifiedTracker(nil, logger, "user-123", "req-456")

	err := tracker.Close(context.Background())
	assert.NoError(t, err)
}

func TestUnifiedTracker_Close_NilService(t *testing.T) {
	tracker := &UnifiedTracker{
		service:         nil,
		serviceCosts:    make(map[string]int64),
		operationCounts: make(map[string]int64),
	}

	err := tracker.Close(context.Background())
	assert.NoError(t, err)
}

func TestNewRepositoryTracker(t *testing.T) {
	logger := zap.NewNop()

	tracker := NewRepositoryTracker(nil, logger, "users", "user-123", "req-456")
	require.NotNil(t, tracker)
	defer tracker.Close(context.Background())

	assert.Contains(t, tracker.service.config.CloudWatchNamespace, "Repository")
}

func TestNewLambdaUnifiedTracker(t *testing.T) {
	logger := zap.NewNop()

	tracker := NewLambdaUnifiedTracker(nil, logger, "my-function", "user-123", "req-456")
	require.NotNil(t, tracker)
	defer tracker.Close(context.Background())

	assert.Contains(t, tracker.service.config.CloudWatchNamespace, "Lambda")
}

func TestNewRequestTracker(t *testing.T) {
	logger := zap.NewNop()

	tracker := NewRequestTracker(nil, logger, "/api/users", "user-123", "req-456")
	require.NotNil(t, tracker)
	defer tracker.Close(context.Background())

	assert.Contains(t, tracker.service.config.CloudWatchNamespace, "API")
}

func TestNewBatchTracker(t *testing.T) {
	logger := zap.NewNop()

	tracker := NewBatchTracker(nil, logger, "data-import", "user-123", "req-456")
	require.NotNil(t, tracker)
	defer tracker.Close(context.Background())

	assert.Contains(t, tracker.service.config.CloudWatchNamespace, "Batch")
	assert.Equal(t, 50, tracker.service.config.MetricsBatchSize)
	assert.Equal(t, 60*time.Second, tracker.service.config.MetricsFlushInterval)
}

func TestUnifiedTracker_TrackDynamoOperationWithConsumedCapacity(t *testing.T) {
	logger := zap.NewNop()
	tracker := NewUnifiedTracker(nil, logger, "user-123", "req-456")
	defer tracker.Close(context.Background())

	ctx := context.Background()
	capacity := &ConsumedCapacity{
		TableName:          "users",
		ReadCapacityUnits:  10.5,
		WriteCapacityUnits: 5.5,
	}

	err := tracker.TrackDynamoOperationWithConsumedCapacity(ctx, "users", "Query", capacity)
	assert.NoError(t, err)

	assert.True(t, tracker.GetCurrentCostMicroCents() > 0)
}

func TestUnifiedTracker_TrackMultipleOperations(t *testing.T) {
	logger := zap.NewNop()
	tracker := NewUnifiedTracker(nil, logger, "user-123", "req-456")
	defer tracker.Close(context.Background())

	ctx := context.Background()
	operations := []OperationInfo{
		{Type: "DynamoDB.Read", ResourceName: "users", Units: 10},
		{Type: "DynamoDB.Write", ResourceName: "users", Units: 5},
		{Type: "S3.Get", ResourceName: "bucket", Bytes: 1024},
		{Type: "S3.Put", ResourceName: "bucket", Bytes: 2048},
		{Type: "Lambda.Invoke", ResourceName: "func", Duration: 100 * time.Millisecond, MemoryMB: 128},
	}

	err := tracker.TrackMultipleOperations(ctx, operations)
	assert.NoError(t, err)

	counts := tracker.GetOperationCounts()
	assert.True(t, len(counts) > 0)
}

func TestUnifiedTracker_TrackMultipleOperations_UnknownType(t *testing.T) {
	logger := zap.NewNop()
	tracker := NewUnifiedTracker(nil, logger, "user-123", "req-456")
	defer tracker.Close(context.Background())

	ctx := context.Background()
	operations := []OperationInfo{
		{Type: "Unknown.Operation", ResourceName: "resource"},
	}

	// Unknown operations should be silently ignored
	err := tracker.TrackMultipleOperations(ctx, operations)
	assert.NoError(t, err)
}

func TestConsumedCapacity_Fields(t *testing.T) {
	capacity := ConsumedCapacity{
		TableName:          "users",
		ReadCapacityUnits:  10.5,
		WriteCapacityUnits: 5.5,
		GlobalSecondaryIndexes: map[string]ConsumedCapacity{
			"gsi1": {ReadCapacityUnits: 2.0},
		},
		LocalSecondaryIndexes: map[string]ConsumedCapacity{
			"lsi1": {ReadCapacityUnits: 1.0},
		},
	}

	assert.Equal(t, "users", capacity.TableName)
	assert.Equal(t, 10.5, capacity.ReadCapacityUnits)
	assert.Equal(t, 5.5, capacity.WriteCapacityUnits)
	assert.Len(t, capacity.GlobalSecondaryIndexes, 1)
	assert.Len(t, capacity.LocalSecondaryIndexes, 1)
}

func TestOperationInfo_Fields(t *testing.T) {
	info := OperationInfo{
		Type:         "DynamoDB.Read",
		ResourceName: "users",
		Units:        10,
		Bytes:        1024,
		Duration:     100 * time.Millisecond,
		MemoryMB:     256,
	}

	assert.Equal(t, "DynamoDB.Read", info.Type)
	assert.Equal(t, "users", info.ResourceName)
	assert.Equal(t, int64(10), info.Units)
	assert.Equal(t, int64(1024), info.Bytes)
	assert.Equal(t, 100*time.Millisecond, info.Duration)
	assert.Equal(t, int64(256), info.MemoryMB)
}

func TestUnifiedTracker_TrackMultipleOperations_AllTypes(t *testing.T) {
	logger := zap.NewNop()
	tracker := NewUnifiedTracker(nil, logger, "user-123", "req-456")

	operations := []OperationInfo{
		{Type: "DynamoDB.Read", ResourceName: "users", Units: 5},
		{Type: "DynamoDB.Write", ResourceName: "users", Units: 3},
		{Type: "S3.Get", ResourceName: "my-bucket", Bytes: 1024},
		{Type: "S3.Put", ResourceName: "my-bucket", Bytes: 2048},
		{Type: "Lambda.Invoke", ResourceName: "my-function", Duration: 100 * time.Millisecond, MemoryMB: 256},
		{Type: "Unknown", ResourceName: "test"}, // Should be ignored
	}

	ctx := context.Background()
	err := tracker.TrackMultipleOperations(ctx, operations)
	assert.NoError(t, err)

	// Verify that costs were tracked (total should be > 0)
	totalCost := tracker.GetCurrentCostMicroCents()
	assert.True(t, totalCost > 0, "Expected total cost to be greater than 0")
}
