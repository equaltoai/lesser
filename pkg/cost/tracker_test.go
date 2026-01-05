package cost

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	tracker := New()
	require.NotNil(t, tracker)
	require.NotNil(t, tracker.circuitBreaker)
	assert.Equal(t, int64(0), tracker.dynamoReads.Load())
	assert.Equal(t, int64(0), tracker.dynamoWrites.Load())
}

func TestNewWithRequest(t *testing.T) {
	requestID := "test-request-id"
	operationType := "test-operation"

	tracker := NewWithRequest(requestID, operationType)
	require.NotNil(t, tracker)
	assert.Equal(t, requestID, tracker.requestID)
	assert.Equal(t, operationType, tracker.operationType)
}

func TestTrackDynamoRead(t *testing.T) {
	t.Run("tracks reads successfully", func(t *testing.T) {
		tracker := New()
		tracker.circuitBreaker = nil // Disable circuit breaker for test

		err := tracker.TrackDynamoRead(5)
		require.NoError(t, err)
		assert.Equal(t, int64(5), tracker.dynamoReads.Load())
	})

	t.Run("accumulates multiple reads", func(t *testing.T) {
		tracker := New()
		tracker.circuitBreaker = nil

		_ = tracker.TrackDynamoRead(3)
		_ = tracker.TrackDynamoRead(7)
		assert.Equal(t, int64(10), tracker.dynamoReads.Load())
	})
}

func TestTrackDynamoWrite(t *testing.T) {
	t.Run("tracks writes successfully", func(t *testing.T) {
		tracker := New()
		tracker.circuitBreaker = nil

		err := tracker.TrackDynamoWrite(3)
		require.NoError(t, err)
		assert.Equal(t, int64(3), tracker.dynamoWrites.Load())
	})

	t.Run("accumulates multiple writes", func(t *testing.T) {
		tracker := New()
		tracker.circuitBreaker = nil

		_ = tracker.TrackDynamoWrite(2)
		_ = tracker.TrackDynamoWrite(4)
		assert.Equal(t, int64(6), tracker.dynamoWrites.Load())
	})
}

func TestTrackDynamoStorage(t *testing.T) {
	tracker := New()
	tracker.TrackDynamoStorage(1024)
	assert.Equal(t, int64(1024), tracker.dynamoStorage.Load())

	tracker.TrackDynamoStorage(2048)
	assert.Equal(t, int64(3072), tracker.dynamoStorage.Load())
}

func TestTrackLambdaInvocation(t *testing.T) {
	tracker := New()

	tracker.TrackLambdaInvocation(100, 256)
	assert.Equal(t, int64(1), tracker.lambdaInvocations.Load())
	assert.Equal(t, int64(100), tracker.lambdaDurationMs.Load())
	assert.Equal(t, int64(256), tracker.lambdaMemoryMB.Load())
}

func TestTrackLambdaInvocation_MinDuration(t *testing.T) {
	tracker := New()

	// Duration less than minimum should be rounded up
	tracker.TrackLambdaInvocation(0, 128)
	assert.Equal(t, int64(1), tracker.lambdaDurationMs.Load())
}

func TestTrackS3Get(t *testing.T) {
	tracker := New()
	tracker.TrackS3Get(5)
	assert.Equal(t, int64(5), tracker.s3Gets.Load())
}

func TestTrackS3Put(t *testing.T) {
	tracker := New()
	tracker.TrackS3Put(3)
	assert.Equal(t, int64(3), tracker.s3Puts.Load())
}

func TestTrackS3Storage(t *testing.T) {
	tracker := New()
	tracker.TrackS3Storage(1024 * 1024)
	assert.Equal(t, int64(1024*1024), tracker.s3Storage.Load())
}

func TestTrackDataTransfer(t *testing.T) {
	tracker := New()
	tracker.TrackDataTransfer(5000)
	assert.Equal(t, int64(5000), tracker.dataTransfer.Load())
}

func TestCalculateCost(t *testing.T) {
	t.Run("calculates DynamoDB read costs", func(t *testing.T) {
		tracker := New()
		tracker.circuitBreaker = nil
		_ = tracker.TrackDynamoRead(1000000) // 1 million reads

		cost := tracker.CalculateCost()
		assert.Equal(t, int64(1000000), cost.DynamoDBReads)
		assert.True(t, cost.TotalCostMicroCents > 0)
	})

	t.Run("calculates DynamoDB write costs", func(t *testing.T) {
		tracker := New()
		tracker.circuitBreaker = nil
		_ = tracker.TrackDynamoWrite(1000000) // 1 million writes

		cost := tracker.CalculateCost()
		assert.Equal(t, int64(1000000), cost.DynamoDBWrites)
		assert.True(t, cost.TotalCostMicroCents > 0)
	})

	t.Run("calculates DynamoDB storage costs", func(t *testing.T) {
		tracker := New()
		tracker.TrackDynamoStorage(1024 * 1024 * 1024) // 1 GB

		cost := tracker.CalculateCost()
		assert.Equal(t, int64(1024*1024*1024), cost.DynamoDBStorage)
		assert.True(t, cost.TotalCostMicroCents > 0)
	})

	t.Run("calculates Lambda costs", func(t *testing.T) {
		tracker := New()
		tracker.TrackLambdaInvocation(1000, 1024) // 1 second, 1GB

		cost := tracker.CalculateCost()
		assert.Equal(t, int64(1), cost.LambdaInvocations)
		assert.Equal(t, int64(1000), cost.LambdaDurationMs)
		assert.True(t, cost.TotalCostMicroCents > 0)
	})

	t.Run("calculates S3 costs", func(t *testing.T) {
		tracker := New()
		tracker.TrackS3Get(1000)
		tracker.TrackS3Put(1000)
		tracker.TrackS3Storage(1024 * 1024 * 1024) // 1 GB

		cost := tracker.CalculateCost()
		assert.Equal(t, int64(1000), cost.S3Gets)
		assert.Equal(t, int64(1000), cost.S3Puts)
		assert.True(t, cost.TotalCostMicroCents > 0)
	})

	t.Run("calculates data transfer costs", func(t *testing.T) {
		tracker := New()
		tracker.TrackDataTransfer(1024 * 1024 * 1024) // 1 GB

		cost := tracker.CalculateCost()
		assert.Equal(t, int64(1024*1024*1024), cost.DataTransferBytes)
		assert.True(t, cost.TotalCostMicroCents > 0)
	})

	t.Run("includes request metadata", func(t *testing.T) {
		tracker := NewWithRequest("req-123", "GET /api/users")
		cost := tracker.CalculateCost()

		assert.Equal(t, "req-123", cost.RequestID)
		assert.Equal(t, "GET /api/users", cost.OperationType)
		assert.False(t, cost.Timestamp.IsZero())
	})
}

func TestTrackerReset(t *testing.T) {
	tracker := New()
	tracker.circuitBreaker = nil

	// Add some data
	_ = tracker.TrackDynamoRead(10)
	_ = tracker.TrackDynamoWrite(5)
	tracker.TrackDynamoStorage(1024)
	tracker.TrackLambdaInvocation(100, 256)
	tracker.TrackS3Get(3)
	tracker.TrackS3Put(2)
	tracker.TrackS3Storage(2048)
	tracker.TrackDataTransfer(5000)

	// Reset
	tracker.Reset()

	// Verify all counters are zero
	assert.Equal(t, int64(0), tracker.dynamoReads.Load())
	assert.Equal(t, int64(0), tracker.dynamoWrites.Load())
	assert.Equal(t, int64(0), tracker.dynamoStorage.Load())
	assert.Equal(t, int64(0), tracker.lambdaInvocations.Load())
	assert.Equal(t, int64(0), tracker.lambdaDurationMs.Load())
	assert.Equal(t, int64(0), tracker.lambdaMemoryMB.Load())
	assert.Equal(t, int64(0), tracker.s3Gets.Load())
	assert.Equal(t, int64(0), tracker.s3Puts.Load())
	assert.Equal(t, int64(0), tracker.s3Storage.Load())
	assert.Equal(t, int64(0), tracker.dataTransfer.Load())
}

func TestMerge(t *testing.T) {
	t.Run("merges two trackers", func(t *testing.T) {
		tracker1 := New()
		tracker1.circuitBreaker = nil
		_ = tracker1.TrackDynamoRead(10)
		_ = tracker1.TrackDynamoWrite(5)

		tracker2 := New()
		tracker2.circuitBreaker = nil
		_ = tracker2.TrackDynamoRead(20)
		_ = tracker2.TrackDynamoWrite(15)

		tracker1.Merge(tracker2)

		assert.Equal(t, int64(30), tracker1.dynamoReads.Load())
		assert.Equal(t, int64(20), tracker1.dynamoWrites.Load())
	})

	t.Run("handles nil tracker", func(t *testing.T) {
		tracker := New()
		tracker.circuitBreaker = nil
		_ = tracker.TrackDynamoRead(10)

		tracker.Merge(nil)

		assert.Equal(t, int64(10), tracker.dynamoReads.Load())
	})

	t.Run("merges all fields", func(t *testing.T) {
		tracker1 := New()
		tracker1.circuitBreaker = nil
		tracker1.TrackDynamoStorage(100)
		tracker1.TrackLambdaInvocation(50, 128)
		tracker1.TrackS3Get(5)
		tracker1.TrackS3Put(3)
		tracker1.TrackS3Storage(200)
		tracker1.TrackDataTransfer(1000)

		tracker2 := New()
		tracker2.circuitBreaker = nil
		tracker2.TrackDynamoStorage(200)
		tracker2.TrackLambdaInvocation(100, 256)
		tracker2.TrackS3Get(10)
		tracker2.TrackS3Put(7)
		tracker2.TrackS3Storage(300)
		tracker2.TrackDataTransfer(2000)

		tracker1.Merge(tracker2)

		assert.Equal(t, int64(300), tracker1.dynamoStorage.Load())
		assert.Equal(t, int64(2), tracker1.lambdaInvocations.Load())
		assert.Equal(t, int64(150), tracker1.lambdaDurationMs.Load())
		assert.Equal(t, int64(384), tracker1.lambdaMemoryMB.Load())
		assert.Equal(t, int64(15), tracker1.s3Gets.Load())
		assert.Equal(t, int64(10), tracker1.s3Puts.Load())
		assert.Equal(t, int64(500), tracker1.s3Storage.Load())
		assert.Equal(t, int64(3000), tracker1.dataTransfer.Load())
	})
}

func TestClone(t *testing.T) {
	original := NewWithRequest("req-123", "test-op")
	original.circuitBreaker = nil
	_ = original.TrackDynamoRead(10)
	_ = original.TrackDynamoWrite(5)
	original.TrackDynamoStorage(100)
	original.TrackLambdaInvocation(50, 128)
	original.TrackS3Get(3)
	original.TrackS3Put(2)
	original.TrackS3Storage(200)
	original.TrackDataTransfer(1000)

	clone := original.Clone()

	// Verify clone has same values
	assert.Equal(t, original.requestID, clone.requestID)
	assert.Equal(t, original.operationType, clone.operationType)
	assert.Equal(t, original.dynamoReads.Load(), clone.dynamoReads.Load())
	assert.Equal(t, original.dynamoWrites.Load(), clone.dynamoWrites.Load())
	assert.Equal(t, original.dynamoStorage.Load(), clone.dynamoStorage.Load())
	assert.Equal(t, original.lambdaInvocations.Load(), clone.lambdaInvocations.Load())
	assert.Equal(t, original.lambdaDurationMs.Load(), clone.lambdaDurationMs.Load())
	assert.Equal(t, original.lambdaMemoryMB.Load(), clone.lambdaMemoryMB.Load())
	assert.Equal(t, original.s3Gets.Load(), clone.s3Gets.Load())
	assert.Equal(t, original.s3Puts.Load(), clone.s3Puts.Load())
	assert.Equal(t, original.s3Storage.Load(), clone.s3Storage.Load())
	assert.Equal(t, original.dataTransfer.Load(), clone.dataTransfer.Load())

	// Verify clone is independent
	_ = original.TrackDynamoRead(100)
	assert.NotEqual(t, original.dynamoReads.Load(), clone.dynamoReads.Load())
}

func TestTrackWrite(t *testing.T) {
	t.Run("tracks write with tracker", func(t *testing.T) {
		tracker := New()
		tracker.circuitBreaker = nil
		ctx := context.Background()

		TrackWrite(ctx, tracker, "test-table", 5)
		assert.Equal(t, int64(5), tracker.dynamoWrites.Load())
	})

	t.Run("handles nil tracker", func(t *testing.T) {
		ctx := context.Background()
		// Should not panic
		TrackWrite(ctx, nil, "test-table", 5)
	})
}

func TestTrackRead(t *testing.T) {
	t.Run("tracks read with tracker", func(t *testing.T) {
		tracker := New()
		tracker.circuitBreaker = nil
		ctx := context.Background()

		TrackRead(ctx, tracker, "test-table", 10)
		assert.Equal(t, int64(10), tracker.dynamoReads.Load())
	})

	t.Run("handles nil tracker", func(t *testing.T) {
		ctx := context.Background()
		// Should not panic
		TrackRead(ctx, nil, "test-table", 10)
	})
}
