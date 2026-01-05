package cost

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithTracker(t *testing.T) {
	tracker := New()
	ctx := WithTracker(context.Background(), tracker)

	retrieved := FromContext(ctx)
	require.NotNil(t, retrieved)
	assert.Equal(t, tracker, retrieved)
}

func TestFromContext_NoTracker(t *testing.T) {
	ctx := context.Background()
	tracker := FromContext(ctx)
	assert.Nil(t, tracker)
}

func TestFromContext_WrongType(t *testing.T) {
	ctx := context.WithValue(context.Background(), trackerKey, "not a tracker")
	tracker := FromContext(ctx)
	assert.Nil(t, tracker)
}

func TestTrack(t *testing.T) {
	t.Run("executes function when tracker exists", func(t *testing.T) {
		tracker := New()
		tracker.circuitBreaker = nil
		ctx := WithTracker(context.Background(), tracker)

		executed := false
		Track(ctx, func(t *Tracker) {
			executed = true
			_ = t.TrackDynamoRead(5)
		})

		assert.True(t, executed)
		assert.Equal(t, int64(5), tracker.dynamoReads.Load())
	})

	t.Run("does nothing when no tracker", func(t *testing.T) {
		ctx := context.Background()

		executed := false
		Track(ctx, func(t *Tracker) {
			executed = true
		})

		assert.False(t, executed)
	})
}

func TestTrackDynamoReadContext(t *testing.T) {
	t.Run("tracks reads when tracker exists", func(t *testing.T) {
		tracker := New()
		tracker.circuitBreaker = nil
		ctx := WithTracker(context.Background(), tracker)

		TrackDynamoReadContext(ctx, 10)
		assert.Equal(t, int64(10), tracker.dynamoReads.Load())
	})

	t.Run("does nothing when no tracker", func(t *testing.T) {
		ctx := context.Background()
		// Should not panic
		TrackDynamoReadContext(ctx, 10)
	})
}

func TestTrackDynamoWriteContext(t *testing.T) {
	t.Run("tracks writes when tracker exists", func(t *testing.T) {
		tracker := New()
		tracker.circuitBreaker = nil
		ctx := WithTracker(context.Background(), tracker)

		TrackDynamoWriteContext(ctx, 5)
		assert.Equal(t, int64(5), tracker.dynamoWrites.Load())
	})

	t.Run("does nothing when no tracker", func(t *testing.T) {
		ctx := context.Background()
		// Should not panic
		TrackDynamoWriteContext(ctx, 5)
	})
}

func TestTrackS3GetContext(t *testing.T) {
	t.Run("tracks S3 gets when tracker exists", func(t *testing.T) {
		tracker := New()
		ctx := WithTracker(context.Background(), tracker)

		TrackS3GetContext(ctx, 3)
		assert.Equal(t, int64(3), tracker.s3Gets.Load())
	})

	t.Run("does nothing when no tracker", func(t *testing.T) {
		ctx := context.Background()
		// Should not panic
		TrackS3GetContext(ctx, 3)
	})
}

func TestTrackS3PutContext(t *testing.T) {
	t.Run("tracks S3 puts when tracker exists", func(t *testing.T) {
		tracker := New()
		ctx := WithTracker(context.Background(), tracker)

		TrackS3PutContext(ctx, 2)
		assert.Equal(t, int64(2), tracker.s3Puts.Load())
	})

	t.Run("does nothing when no tracker", func(t *testing.T) {
		ctx := context.Background()
		// Should not panic
		TrackS3PutContext(ctx, 2)
	})
}

func TestTrackDataTransferContext(t *testing.T) {
	t.Run("tracks data transfer when tracker exists", func(t *testing.T) {
		tracker := New()
		ctx := WithTracker(context.Background(), tracker)

		TrackDataTransferContext(ctx, 1024)
		assert.Equal(t, int64(1024), tracker.dataTransfer.Load())
	})

	t.Run("does nothing when no tracker", func(t *testing.T) {
		ctx := context.Background()
		// Should not panic
		TrackDataTransferContext(ctx, 1024)
	})
}
