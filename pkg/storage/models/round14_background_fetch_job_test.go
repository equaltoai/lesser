package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBackgroundFetchJob_LifecycleAndRetryLogic(t *testing.T) {
	t.Run("NewBackgroundFetchJob sets defaults and keys", func(t *testing.T) {
		before := time.Now()
		job := NewBackgroundFetchJob("status-1", "thread_sync")
		require.NotNil(t, job)

		assert.NotEmpty(t, job.JobID)
		assert.Equal(t, "status-1", job.StatusID)
		assert.Equal(t, "thread_sync", job.FetchType)
		assert.Equal(t, "normal", job.Priority)
		assert.Equal(t, 3, job.MaxRetries)
		assert.Equal(t, StatusPending, job.Status)
		assert.Equal(t, 0, job.Attempts)
		assert.NotNil(t, job.FetchMetadata)
		assert.True(t, job.TTL > 0)
		assert.True(t, time.Unix(job.TTL, 0).After(before.Add(6*24*time.Hour)))

		assert.Contains(t, job.PK, "FETCH_JOB#")
		assert.Contains(t, job.SK, "JOB#")
		assert.Contains(t, job.GSI1PK, "STATUS#status-1")
		assert.Contains(t, job.GSI1SK, "FETCH#")
		assert.Equal(t, MainTableName, job.TableName())
	})

	t.Run("BeforeCreate/BeforeUpdate update timestamps and keys", func(t *testing.T) {
		job := &BackgroundFetchJob{JobID: "j1", StatusID: "s1"}
		require.NoError(t, job.BeforeCreate())
		assert.False(t, job.CreatedAt.IsZero())
		assert.True(t, job.CreatedAt.Equal(job.UpdatedAt))
		assert.Equal(t, "FETCH_JOB#j1", job.PK)

		prev := job.UpdatedAt
		require.NoError(t, job.BeforeUpdate())
		assert.True(t, job.UpdatedAt.After(prev))
	})

	t.Run("MarkRunning/MarkCompleted update status and fields", func(t *testing.T) {
		job := &BackgroundFetchJob{MaxRetries: 3, Status: StatusPending}
		job.MarkRunning()
		assert.Equal(t, StatusProcessing, job.Status)
		assert.Equal(t, 1, job.Attempts)
		require.NotNil(t, job.LastAttempt)

		job.LastError = "oops"
		job.ErrorDetails = "details"
		job.MarkCompleted()
		assert.Equal(t, StatusCompleted, job.Status)
		require.NotNil(t, job.CompletedAt)
		assert.Empty(t, job.LastError)
		assert.Empty(t, job.ErrorDetails)
	})

	t.Run("MarkFailed schedules retries with exponential-ish backoff", func(t *testing.T) {
		now := time.Now()

		job := &BackgroundFetchJob{MaxRetries: 3, Attempts: 1}
		job.MarkFailed("err", "details")
		assert.Equal(t, StatusPending, job.Status)
		require.NotNil(t, job.NextAttempt)
		assert.True(t, job.NextAttempt.After(now.Add(4*time.Minute)))

		job = &BackgroundFetchJob{MaxRetries: 3, Attempts: 2}
		job.MarkFailed("err", "details")
		assert.Equal(t, StatusPending, job.Status)
		require.NotNil(t, job.NextAttempt)
		assert.True(t, job.NextAttempt.After(now.Add(14*time.Minute)))

		// Cover case 3 and default by allowing more retries.
		job = &BackgroundFetchJob{MaxRetries: 5, Attempts: 3}
		job.MarkFailed("err", "details")
		assert.Equal(t, StatusPending, job.Status)
		require.NotNil(t, job.NextAttempt)
		assert.True(t, job.NextAttempt.After(now.Add(44*time.Minute)))

		job = &BackgroundFetchJob{MaxRetries: 5, Attempts: 4}
		job.MarkFailed("err", "details")
		assert.Equal(t, StatusPending, job.Status)
		require.NotNil(t, job.NextAttempt)
		assert.True(t, job.NextAttempt.After(now.Add(59*time.Minute)))

		// No retry when attempts >= maxRetries.
		job = &BackgroundFetchJob{MaxRetries: 3, Attempts: 3}
		job.MarkFailed("err", "details")
		assert.Equal(t, StatusFailed, job.Status)
		assert.Nil(t, job.NextAttempt)
	})

	t.Run("IsRetryable and IsReady", func(t *testing.T) {
		job := &BackgroundFetchJob{MaxRetries: 3, Attempts: 1, Status: StatusFailed}
		assert.True(t, job.IsRetryable())

		job.Status = StatusCompleted
		assert.False(t, job.IsRetryable())

		job = &BackgroundFetchJob{Status: StatusPending}
		assert.True(t, job.IsReady())

		future := time.Now().Add(time.Hour)
		job.NextAttempt = &future
		assert.False(t, job.IsReady())

		past := time.Now().Add(-time.Minute)
		job.NextAttempt = &past
		assert.True(t, job.IsReady())
	})

	t.Run("Metadata helpers handle nil map", func(t *testing.T) {
		job := &BackgroundFetchJob{}
		assert.Equal(t, "", job.GetMetadata("k"))

		job.AddMetadata("k", "v")
		assert.Equal(t, "v", job.GetMetadata("k"))

		job.FetchMetadata = nil
		job.AddMetadata("k2", "v2")
		assert.Equal(t, "v2", job.FetchMetadata["k2"])
	})
}
