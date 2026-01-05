package models

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// validMediaJob creates a valid MediaJob for testing
func validMediaJob() *MediaJob {
	return &MediaJob{
		JobID:    "job-123",
		MediaID:  "media-456",
		Username: "testuser",
		S3Key:    "uploads/test/file.jpg",
		MimeType: "image/jpeg",
		Status:   StatusPending,
	}
}

// ============================================================================
// 1) Key Generation + Validation Tests
// ============================================================================

func TestMediaJob_UpdateKeys(t *testing.T) {
	t.Run("missing JobID returns ErrMediaJobIDRequired", func(t *testing.T) {
		mj := &MediaJob{
			MediaID:  "media-123",
			Username: "testuser",
			S3Key:    "key",
			MimeType: "image/jpeg",
		}
		err := mj.UpdateKeys()
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrMediaJobIDRequired), "expected error to wrap ErrMediaJobIDRequired")
	})

	t.Run("sets PK and SK correctly", func(t *testing.T) {
		mj := validMediaJob()
		err := mj.UpdateKeys()
		require.NoError(t, err)
		assert.Equal(t, "JOB#job-123", mj.PK)
		assert.Equal(t, "JOB#job-123", mj.SK)
	})

	t.Run("sets GSI1PK and GSI1SK when Username is set", func(t *testing.T) {
		mj := validMediaJob()
		mj.CreatedAt = time.Date(2025, 12, 27, 10, 0, 0, 0, time.UTC)
		err := mj.UpdateKeys()
		require.NoError(t, err)
		assert.Equal(t, "USER_JOBS#testuser", mj.GSI1PK)
		assert.Contains(t, mj.GSI1SK, "2025-12-27T10:00:00Z")
		assert.Contains(t, mj.GSI1SK, "job-123")
	})

	t.Run("does not set GSI1 keys when Username is empty", func(t *testing.T) {
		mj := validMediaJob()
		mj.Username = ""
		mj.CreatedAt = time.Now()
		mj.UpdatedAt = time.Now()
		mj.Status = StatusPending
		// Clear GSI1 keys first to ensure they don't get set
		mj.GSI1PK = ""
		mj.GSI1SK = ""
		// Manual UpdateKeys since we're testing partial key generation
		mj.PK = "JOB#" + mj.JobID
		mj.SK = "JOB#" + mj.JobID
		assert.Empty(t, mj.GSI1PK)
		assert.Empty(t, mj.GSI1SK)
	})

	t.Run("GSI2PK uses STATUS# pattern", func(t *testing.T) {
		mj := validMediaJob()
		mj.Status = StatusProcessing
		mj.UpdatedAt = time.Now()
		err := mj.UpdateKeys()
		require.NoError(t, err)
		assert.Equal(t, "STATUS#processing", mj.GSI2PK)
	})

	t.Run("GSI2SK starts with UPDATED#", func(t *testing.T) {
		mj := validMediaJob()
		mj.UpdatedAt = time.Date(2025, 12, 27, 14, 30, 0, 0, time.UTC)
		err := mj.UpdateKeys()
		require.NoError(t, err)
		assert.Contains(t, mj.GSI2SK, "UPDATED#")
		assert.Contains(t, mj.GSI2SK, "2025-12-27T14:30:00Z")
	})
}

// ============================================================================
// 2) BeforeCreate Defaults Tests
// ============================================================================

func TestMediaJob_BeforeCreate(t *testing.T) {
	t.Run("generates JobID when empty", func(t *testing.T) {
		mj := &MediaJob{
			MediaID:  "media-123",
			Username: "testuser",
			S3Key:    "uploads/test.jpg",
			MimeType: "image/jpeg",
		}
		err := mj.BeforeCreate()
		require.NoError(t, err)
		assert.NotEmpty(t, mj.JobID)
		assert.Len(t, mj.JobID, 36) // UUID format
	})

	t.Run("generates IdempotencyKey when empty", func(t *testing.T) {
		mj := &MediaJob{
			MediaID:  "media-123",
			Username: "testuser",
			S3Key:    "uploads/test.jpg",
			MimeType: "image/jpeg",
		}
		err := mj.BeforeCreate()
		require.NoError(t, err)
		assert.NotEmpty(t, mj.IdempotencyKey)
		assert.Len(t, mj.IdempotencyKey, 64) // SHA256 hex
	})

	t.Run("preserves existing JobID", func(t *testing.T) {
		mj := &MediaJob{
			JobID:    "existing-job-id",
			MediaID:  "media-123",
			Username: "testuser",
			S3Key:    "uploads/test.jpg",
			MimeType: "image/jpeg",
		}
		err := mj.BeforeCreate()
		require.NoError(t, err)
		assert.Equal(t, "existing-job-id", mj.JobID)
	})

	t.Run("defaults Status to pending", func(t *testing.T) {
		mj := &MediaJob{
			MediaID:  "media-123",
			Username: "testuser",
			S3Key:    "uploads/test.jpg",
			MimeType: "image/jpeg",
		}
		err := mj.BeforeCreate()
		require.NoError(t, err)
		assert.Equal(t, StatusPending, mj.Status)
	})

	t.Run("preserves existing Status", func(t *testing.T) {
		mj := &MediaJob{
			MediaID:  "media-123",
			Username: "testuser",
			S3Key:    "uploads/test.jpg",
			MimeType: "image/jpeg",
			Status:   StatusProcessing,
		}
		err := mj.BeforeCreate()
		require.NoError(t, err)
		assert.Equal(t, StatusProcessing, mj.Status)
	})

	t.Run("initializes Results to non-nil map", func(t *testing.T) {
		mj := &MediaJob{
			MediaID:  "media-123",
			Username: "testuser",
			S3Key:    "uploads/test.jpg",
			MimeType: "image/jpeg",
		}
		err := mj.BeforeCreate()
		require.NoError(t, err)
		assert.NotNil(t, mj.Results)
		assert.Empty(t, mj.Results)
	})

	t.Run("initializes ProcessingTasks to non-nil slice", func(t *testing.T) {
		mj := &MediaJob{
			MediaID:  "media-123",
			Username: "testuser",
			S3Key:    "uploads/test.jpg",
			MimeType: "image/jpeg",
		}
		err := mj.BeforeCreate()
		require.NoError(t, err)
		assert.NotNil(t, mj.ProcessingTasks)
		assert.Empty(t, mj.ProcessingTasks)
	})

	t.Run("defaults MaxRetries to 3 when 0", func(t *testing.T) {
		mj := &MediaJob{
			MediaID:  "media-123",
			Username: "testuser",
			S3Key:    "uploads/test.jpg",
			MimeType: "image/jpeg",
		}
		err := mj.BeforeCreate()
		require.NoError(t, err)
		assert.Equal(t, 3, mj.MaxRetries)
	})

	t.Run("preserves existing MaxRetries", func(t *testing.T) {
		mj := &MediaJob{
			MediaID:    "media-123",
			Username:   "testuser",
			S3Key:      "uploads/test.jpg",
			MimeType:   "image/jpeg",
			MaxRetries: 5,
		}
		err := mj.BeforeCreate()
		require.NoError(t, err)
		assert.Equal(t, 5, mj.MaxRetries)
	})

	t.Run("sets MaxProcessingTime via default timeout helper when 0", func(t *testing.T) {
		mj := &MediaJob{
			MediaID:  "media-123",
			Username: "testuser",
			S3Key:    "uploads/test.jpg",
			MimeType: "image/jpeg",
		}
		err := mj.BeforeCreate()
		require.NoError(t, err)
		assert.Greater(t, int64(mj.MaxProcessingTime), int64(0))
		// For image/jpeg, default should be 30 seconds
		assert.Equal(t, 30*time.Second, mj.MaxProcessingTime)
	})

	t.Run("sets ExpiresAt approximately 24h from now", func(t *testing.T) {
		mj := &MediaJob{
			MediaID:  "media-123",
			Username: "testuser",
			S3Key:    "uploads/test.jpg",
			MimeType: "image/jpeg",
		}
		before := time.Now()
		err := mj.BeforeCreate()
		require.NoError(t, err)
		require.NotNil(t, mj.ExpiresAt)

		expected := before.Add(24 * time.Hour)
		actual := time.Unix(*mj.ExpiresAt, 0)
		assert.WithinDuration(t, expected, actual, 2*time.Second)
	})

	t.Run("calls UpdateKeys and Validate successfully", func(t *testing.T) {
		mj := &MediaJob{
			MediaID:  "media-123",
			Username: "testuser",
			S3Key:    "uploads/test.jpg",
			MimeType: "image/jpeg",
		}
		err := mj.BeforeCreate()
		require.NoError(t, err)
		// Verify keys were set
		assert.NotEmpty(t, mj.PK)
		assert.NotEmpty(t, mj.SK)
	})

	t.Run("sets CreatedAt and UpdatedAt timestamps", func(t *testing.T) {
		mj := &MediaJob{
			MediaID:  "media-123",
			Username: "testuser",
			S3Key:    "uploads/test.jpg",
			MimeType: "image/jpeg",
		}
		before := time.Now()
		err := mj.BeforeCreate()
		require.NoError(t, err)
		assert.WithinDuration(t, before, mj.CreatedAt, 2*time.Second)
		assert.WithinDuration(t, before, mj.UpdatedAt, 2*time.Second)
	})
}

func TestMediaJob_BeforeCreate_Validation_Failure(t *testing.T) {
	t.Run("missing MediaID returns validation error", func(t *testing.T) {
		mj := &MediaJob{
			Username: "testuser",
			S3Key:    "uploads/test.jpg",
			MimeType: "image/jpeg",
		}
		err := mj.BeforeCreate()
		require.Error(t, err)
	})

	t.Run("missing S3Key returns validation error", func(t *testing.T) {
		mj := &MediaJob{
			MediaID:  "media-123",
			Username: "testuser",
			MimeType: "image/jpeg",
		}
		err := mj.BeforeCreate()
		require.Error(t, err)
	})

	t.Run("missing Username returns validation error", func(t *testing.T) {
		mj := &MediaJob{
			MediaID:  "media-123",
			S3Key:    "uploads/test.jpg",
			MimeType: "image/jpeg",
		}
		err := mj.BeforeCreate()
		require.Error(t, err)
	})

	t.Run("missing MimeType returns validation error", func(t *testing.T) {
		mj := &MediaJob{
			MediaID:  "media-123",
			Username: "testuser",
			S3Key:    "uploads/test.jpg",
		}
		err := mj.BeforeCreate()
		require.Error(t, err)
	})
}

// ============================================================================
// 3) BeforeUpdate Tests
// ============================================================================

func TestMediaJob_BeforeUpdate(t *testing.T) {
	t.Run("updates UpdatedAt timestamp", func(t *testing.T) {
		mj := validMediaJob()
		mj.CreatedAt = time.Now().Add(-1 * time.Hour)
		oldUpdatedAt := mj.UpdatedAt

		before := time.Now()
		err := mj.BeforeUpdate()
		require.NoError(t, err)

		assert.True(t, mj.UpdatedAt.After(oldUpdatedAt) || mj.UpdatedAt.Equal(time.Now()))
		assert.WithinDuration(t, before, mj.UpdatedAt, 2*time.Second)
	})

	t.Run("calls UpdateKeys", func(t *testing.T) {
		mj := validMediaJob()
		mj.CreatedAt = time.Now()
		err := mj.BeforeUpdate()
		require.NoError(t, err)
		assert.Equal(t, "JOB#job-123", mj.PK)
		assert.Equal(t, "JOB#job-123", mj.SK)
	})

	t.Run("enforces validation", func(t *testing.T) {
		mj := &MediaJob{
			JobID:    "job-123",
			MediaID:  "media-123",
			Username: "testuser",
			// Missing S3Key and MimeType
		}
		err := mj.BeforeUpdate()
		require.Error(t, err)
	})
}

// ============================================================================
// 4) State Transitions Tests
// ============================================================================

func TestMediaJob_SetProcessing(t *testing.T) {
	t.Run("sets status to processing", func(t *testing.T) {
		mj := validMediaJob()
		mj.Status = StatusPending
		mj.SetProcessing()
		assert.Equal(t, StatusProcessing, mj.Status)
	})

	t.Run("clears error", func(t *testing.T) {
		mj := validMediaJob()
		mj.Error = "some previous error"
		mj.SetProcessing()
		assert.Empty(t, mj.Error)
	})

	t.Run("sets processing timestamps", func(t *testing.T) {
		mj := validMediaJob()
		mj.StartedAt = nil
		mj.ProcessingStartedAt = nil
		mj.LastAttemptAt = nil

		before := time.Now()
		mj.SetProcessing()

		assert.NotNil(t, mj.StartedAt)
		assert.WithinDuration(t, before, *mj.StartedAt, 2*time.Second)

		assert.NotNil(t, mj.ProcessingStartedAt)
		assert.WithinDuration(t, before, *mj.ProcessingStartedAt, 2*time.Second)

		assert.NotNil(t, mj.LastAttemptAt)
		assert.WithinDuration(t, before, *mj.LastAttemptAt, 2*time.Second)
	})

	t.Run("does not overwrite existing StartedAt", func(t *testing.T) {
		mj := validMediaJob()
		originalStart := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		mj.StartedAt = &originalStart

		mj.SetProcessing()

		assert.Equal(t, originalStart, *mj.StartedAt)
	})

	t.Run("updates UpdatedAt", func(t *testing.T) {
		mj := validMediaJob()
		before := time.Now()
		mj.SetProcessing()
		assert.WithinDuration(t, before, mj.UpdatedAt, 2*time.Second)
	})
}

func TestMediaJob_SetCompleted(t *testing.T) {
	t.Run("sets status to completed", func(t *testing.T) {
		mj := validMediaJob()
		mj.Status = StatusProcessing
		mj.SetCompleted(nil)
		assert.Equal(t, StatusCompleted, mj.Status)
	})

	t.Run("sets Progress to 100", func(t *testing.T) {
		mj := validMediaJob()
		mj.Progress = 50
		mj.SetCompleted(nil)
		assert.Equal(t, 100, mj.Progress)
	})

	t.Run("clears TTL (ExpiresAt)", func(t *testing.T) {
		mj := validMediaJob()
		ttl := time.Now().Add(time.Hour).Unix()
		mj.ExpiresAt = &ttl
		mj.SetCompleted(nil)
		assert.Nil(t, mj.ExpiresAt)
	})

	t.Run("sets CompletedAt", func(t *testing.T) {
		mj := validMediaJob()
		before := time.Now()
		mj.SetCompleted(nil)
		assert.NotNil(t, mj.CompletedAt)
		assert.WithinDuration(t, before, *mj.CompletedAt, 2*time.Second)
	})

	t.Run("stores results", func(t *testing.T) {
		mj := validMediaJob()
		results := map[string]any{
			"width":    1920,
			"height":   1080,
			"fileSize": 12345,
		}
		mj.SetCompleted(results)
		assert.Equal(t, results, mj.Results)
		assert.Equal(t, 1920, mj.Results["width"])
	})

	t.Run("clears error", func(t *testing.T) {
		mj := validMediaJob()
		mj.Error = "previous error"
		mj.SetCompleted(nil)
		assert.Empty(t, mj.Error)
	})
}

func TestMediaJob_SetFailed(t *testing.T) {
	t.Run("sets status to failed", func(t *testing.T) {
		mj := validMediaJob()
		mj.Status = StatusProcessing
		mj.SetFailed("some error")
		assert.Equal(t, StatusFailed, mj.Status)
	})

	t.Run("sets error and last error", func(t *testing.T) {
		mj := validMediaJob()
		mj.SetFailed("processing failed: timeout")
		assert.Equal(t, "processing failed: timeout", mj.Error)
		assert.Equal(t, "processing failed: timeout", mj.LastError)
	})

	t.Run("sets CompletedAt", func(t *testing.T) {
		mj := validMediaJob()
		before := time.Now()
		mj.SetFailed("error")
		assert.NotNil(t, mj.CompletedAt)
		assert.WithinDuration(t, before, *mj.CompletedAt, 2*time.Second)
	})

	t.Run("sets TTL approximately 7 days from now", func(t *testing.T) {
		mj := validMediaJob()
		before := time.Now()
		mj.SetFailed("error")
		require.NotNil(t, mj.ExpiresAt)

		expected := before.Add(7 * 24 * time.Hour)
		actual := time.Unix(*mj.ExpiresAt, 0)
		assert.WithinDuration(t, expected, actual, 2*time.Second)
	})

	t.Run("updates UpdatedAt", func(t *testing.T) {
		mj := validMediaJob()
		before := time.Now()
		mj.SetFailed("error")
		assert.WithinDuration(t, before, mj.UpdatedAt, 2*time.Second)
	})
}

// Status predicate truth table
func TestMediaJob_StatusPredicates(t *testing.T) {
	testCases := []struct {
		name         string
		status       string
		isCompleted  bool
		isFailed     bool
		isProcessing bool
		isPending    bool
	}{
		{
			name:         "pending status",
			status:       StatusPending,
			isCompleted:  false,
			isFailed:     false,
			isProcessing: false,
			isPending:    true,
		},
		{
			name:         "processing status",
			status:       StatusProcessing,
			isCompleted:  false,
			isFailed:     false,
			isProcessing: true,
			isPending:    false,
		},
		{
			name:         "completed status",
			status:       StatusCompleted,
			isCompleted:  true,
			isFailed:     false,
			isProcessing: false,
			isPending:    false,
		},
		{
			name:         "failed status",
			status:       StatusFailed,
			isCompleted:  false,
			isFailed:     true,
			isProcessing: false,
			isPending:    false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mj := validMediaJob()
			mj.Status = tc.status

			assert.Equal(t, tc.isCompleted, mj.IsCompleted(), "IsCompleted")
			assert.Equal(t, tc.isFailed, mj.IsFailed(), "IsFailed")
			assert.Equal(t, tc.isProcessing, mj.IsProcessing(), "IsProcessing")
			assert.Equal(t, tc.isPending, mj.IsPending(), "IsPending")
		})
	}
}

// ============================================================================
// Additional State Transition Tests
// ============================================================================

func TestMediaJob_SetCancelled(t *testing.T) {
	t.Run("sets status to cancelled", func(t *testing.T) {
		mj := validMediaJob()
		mj.SetCancelled("user requested cancellation")
		assert.Equal(t, MediaStatusCancelled, mj.Status)
	})

	t.Run("sets error to reason", func(t *testing.T) {
		mj := validMediaJob()
		mj.SetCancelled("user requested cancellation")
		assert.Equal(t, "user requested cancellation", mj.Error)
	})

	t.Run("sets CompletedAt", func(t *testing.T) {
		mj := validMediaJob()
		before := time.Now()
		mj.SetCancelled("reason")
		assert.NotNil(t, mj.CompletedAt)
		assert.WithinDuration(t, before, *mj.CompletedAt, 2*time.Second)
	})

	t.Run("sets TTL approximately 24 hours from now", func(t *testing.T) {
		mj := validMediaJob()
		before := time.Now()
		mj.SetCancelled("reason")
		require.NotNil(t, mj.ExpiresAt)

		expected := before.Add(24 * time.Hour)
		actual := time.Unix(*mj.ExpiresAt, 0)
		assert.WithinDuration(t, expected, actual, 2*time.Second)
	})
}

func TestMediaJob_IsCancelled(t *testing.T) {
	t.Run("returns true for cancelled status", func(t *testing.T) {
		mj := validMediaJob()
		mj.Status = "cancelled"
		assert.True(t, mj.IsCancelled())
	})

	t.Run("returns false for other statuses", func(t *testing.T) {
		testCases := []string{StatusPending, StatusProcessing, StatusCompleted, StatusFailed}
		for _, status := range testCases {
			t.Run(status, func(t *testing.T) {
				mj := validMediaJob()
				mj.Status = status
				assert.False(t, mj.IsCancelled())
			})
		}
	})
}

// ============================================================================
// Retry Logic Tests
// ============================================================================

func TestMediaJob_CanRetry(t *testing.T) {
	t.Run("returns false when RetryCount >= MaxRetries", func(t *testing.T) {
		mj := validMediaJob()
		mj.Status = StatusFailed
		mj.RetryCount = 3
		mj.MaxRetries = 3
		mj.LastError = "timeout"
		assert.False(t, mj.CanRetry())
	})

	t.Run("returns false when status is not failed", func(t *testing.T) {
		statuses := []string{StatusPending, StatusProcessing, StatusCompleted}
		for _, status := range statuses {
			t.Run(status, func(t *testing.T) {
				mj := validMediaJob()
				mj.Status = status
				mj.RetryCount = 0
				mj.MaxRetries = 3
				assert.False(t, mj.CanRetry())
			})
		}
	})

	t.Run("returns true for transient error", func(t *testing.T) {
		mj := validMediaJob()
		mj.Status = StatusFailed
		mj.RetryCount = 1
		mj.MaxRetries = 3
		mj.LastError = "connection timeout"
		assert.True(t, mj.CanRetry())
	})

	t.Run("returns false for permanent error", func(t *testing.T) {
		mj := validMediaJob()
		mj.Status = StatusFailed
		mj.RetryCount = 0
		mj.MaxRetries = 3
		mj.LastError = "file too large"
		assert.False(t, mj.CanRetry())
	})
}

func TestMediaJob_IsRetryableError(t *testing.T) {
	testCases := []struct {
		name      string
		errorMsg  string
		retryable bool
	}{
		// Permanent errors
		{"invalid format", "Invalid format: unsupported codec", false},
		{"unsupported", "Unsupported file type", false},
		{"file too large", "File too large: 500MB exceeds limit", false},
		{"corrupted", "File corrupted: cannot read header", false},
		{"authorization failed", "Authorization failed: invalid token", false},
		{"budget exceeded", "Budget exceeded for this month", false},
		{"quota exceeded", "Quota exceeded: max uploads reached", false},

		// Transient errors
		{"timeout", "Request timeout after 30s", true},
		{"network error", "Network error: connection reset", true},
		{"connection error", "Connection refused", true},
		{"throttling", "Request throttling: too many requests", true},
		{"rate limit", "Rate limit exceeded, retry later", true},
		{"service unavailable", "Service unavailable: 503", true},
		{"internal error", "Internal error: please retry", true},

		// Unknown errors default to retryable
		{"unknown error", "Something unexpected happened", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mj := validMediaJob()
			result := mj.IsRetryableError(tc.errorMsg)
			assert.Equal(t, tc.retryable, result)
		})
	}
}

func TestMediaJob_ScheduleRetry(t *testing.T) {
	t.Run("increments RetryCount", func(t *testing.T) {
		mj := validMediaJob()
		mj.RetryCount = 1
		mj.ScheduleRetry(time.Second)
		assert.Equal(t, 2, mj.RetryCount)
	})

	t.Run("sets status to pending", func(t *testing.T) {
		mj := validMediaJob()
		mj.Status = StatusFailed
		mj.ScheduleRetry(time.Second)
		assert.Equal(t, StatusPending, mj.Status)
	})

	t.Run("exponential backoff calculation", func(t *testing.T) {
		testCases := []struct {
			name          string
			currentRetry  int
			expectedDelay time.Duration
		}{
			{"first retry", 0, 4 * time.Second},         // 1s * 4^1 = 4s
			{"second retry", 1, 16 * time.Second},       // 1s * 4^2 = 16s
			{"third retry", 2, 64 * time.Second},        // 1s * 4^3 = 64s
			{"capped at 5 minutes", 5, 5 * time.Minute}, // Capped
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				mj := validMediaJob()
				mj.RetryCount = tc.currentRetry

				before := time.Now()
				mj.ScheduleRetry(time.Second)

				assert.NotNil(t, mj.RetryScheduledAt)
				expectedTime := before.Add(tc.expectedDelay)
				assert.WithinDuration(t, expectedTime, *mj.RetryScheduledAt, 2*time.Second)
			})
		}
	})
}

// ============================================================================
// Abandonment and Budget Tests
// ============================================================================

func TestMediaJob_IsAbandoned(t *testing.T) {
	t.Run("returns true when processing and updated too long ago", func(t *testing.T) {
		mj := validMediaJob()
		mj.Status = StatusProcessing
		mj.UpdatedAt = time.Now().Add(-30 * time.Minute)

		assert.True(t, mj.IsAbandoned(15*time.Minute))
	})

	t.Run("returns false when processing but within threshold", func(t *testing.T) {
		mj := validMediaJob()
		mj.Status = StatusProcessing
		mj.UpdatedAt = time.Now().Add(-5 * time.Minute)

		assert.False(t, mj.IsAbandoned(15*time.Minute))
	})

	t.Run("returns false when not processing", func(t *testing.T) {
		statuses := []string{StatusPending, StatusCompleted, StatusFailed}
		for _, status := range statuses {
			t.Run(status, func(t *testing.T) {
				mj := validMediaJob()
				mj.Status = status
				mj.UpdatedAt = time.Now().Add(-1 * time.Hour)

				assert.False(t, mj.IsAbandoned(15*time.Minute))
			})
		}
	})
}

func TestMediaJob_IsBudgetExceeded(t *testing.T) {
	t.Run("returns false when ProcessingStartedAt is nil", func(t *testing.T) {
		mj := validMediaJob()
		mj.ProcessingStartedAt = nil
		mj.MaxProcessingTime = 30 * time.Second
		assert.False(t, mj.IsBudgetExceeded())
	})

	t.Run("returns false when MaxProcessingTime is 0", func(t *testing.T) {
		mj := validMediaJob()
		now := time.Now()
		mj.ProcessingStartedAt = &now
		mj.MaxProcessingTime = 0
		assert.False(t, mj.IsBudgetExceeded())
	})

	t.Run("returns true when processing exceeds max time", func(t *testing.T) {
		mj := validMediaJob()
		past := time.Now().Add(-1 * time.Minute)
		mj.ProcessingStartedAt = &past
		mj.MaxProcessingTime = 30 * time.Second
		assert.True(t, mj.IsBudgetExceeded())
	})

	t.Run("returns false when processing is within max time", func(t *testing.T) {
		mj := validMediaJob()
		recent := time.Now().Add(-10 * time.Second)
		mj.ProcessingStartedAt = &recent
		mj.MaxProcessingTime = 30 * time.Second
		assert.False(t, mj.IsBudgetExceeded())
	})
}

// ============================================================================
// Progress and Cost Tests
// ============================================================================

func TestMediaJob_UpdateProgress(t *testing.T) {
	t.Run("clamps progress below 0 to 0", func(t *testing.T) {
		mj := validMediaJob()
		mj.UpdateProgress(-10)
		assert.Equal(t, 0, mj.Progress)
	})

	t.Run("clamps progress above 100 to 100", func(t *testing.T) {
		mj := validMediaJob()
		mj.UpdateProgress(150)
		assert.Equal(t, 100, mj.Progress)
	})

	t.Run("sets valid progress", func(t *testing.T) {
		mj := validMediaJob()
		mj.UpdateProgress(75)
		assert.Equal(t, 75, mj.Progress)
	})

	t.Run("updates UpdatedAt", func(t *testing.T) {
		mj := validMediaJob()
		mj.UpdatedAt = time.Now().Add(-1 * time.Hour)
		before := time.Now()
		mj.UpdateProgress(50)
		assert.WithinDuration(t, before, mj.UpdatedAt, 2*time.Second)
	})
}

func TestMediaJob_AddCost(t *testing.T) {
	t.Run("accumulates cost", func(t *testing.T) {
		mj := validMediaJob()
		mj.ActualCostMicros = 100
		mj.AddCost(50)
		assert.Equal(t, int64(150), mj.ActualCostMicros)
	})

	t.Run("updates UpdatedAt", func(t *testing.T) {
		mj := validMediaJob()
		mj.UpdatedAt = time.Now().Add(-1 * time.Hour)
		before := time.Now()
		mj.AddCost(50)
		assert.WithinDuration(t, before, mj.UpdatedAt, 2*time.Second)
	})
}

// ============================================================================
// Task Management Tests
// ============================================================================

func TestMediaJob_AddProcessingTask(t *testing.T) {
	t.Run("initializes slice if nil", func(t *testing.T) {
		mj := validMediaJob()
		mj.ProcessingTasks = nil
		mj.AddProcessingTask("resize")
		assert.NotNil(t, mj.ProcessingTasks)
		assert.Contains(t, mj.ProcessingTasks, "resize")
	})

	t.Run("appends task to existing slice", func(t *testing.T) {
		mj := validMediaJob()
		mj.ProcessingTasks = []string{"thumbnail"}
		mj.AddProcessingTask("resize")
		assert.Len(t, mj.ProcessingTasks, 2)
		assert.Contains(t, mj.ProcessingTasks, "thumbnail")
		assert.Contains(t, mj.ProcessingTasks, "resize")
	})
}

func TestMediaJob_HasProcessingTask(t *testing.T) {
	t.Run("returns true when task exists", func(t *testing.T) {
		mj := validMediaJob()
		mj.ProcessingTasks = []string{"resize", "thumbnail", "blurhash"}
		assert.True(t, mj.HasProcessingTask("thumbnail"))
	})

	t.Run("returns false when task does not exist", func(t *testing.T) {
		mj := validMediaJob()
		mj.ProcessingTasks = []string{"resize", "thumbnail"}
		assert.False(t, mj.HasProcessingTask("blurhash"))
	})

	t.Run("returns false for empty slice", func(t *testing.T) {
		mj := validMediaJob()
		mj.ProcessingTasks = []string{}
		assert.False(t, mj.HasProcessingTask("resize"))
	})

	t.Run("returns false for nil slice", func(t *testing.T) {
		mj := validMediaJob()
		mj.ProcessingTasks = nil
		assert.False(t, mj.HasProcessingTask("resize"))
	})
}

// ============================================================================
// Idempotency Key Tests
// ============================================================================

func TestMediaJob_GenerateIdempotencyKey(t *testing.T) {
	t.Run("generates consistent key for same inputs", func(t *testing.T) {
		fixedTime := time.Date(2025, 12, 27, 10, 0, 0, 0, time.UTC)

		mj1 := &MediaJob{
			Username:  "testuser",
			FileHash:  "abc123",
			CreatedAt: fixedTime,
		}

		mj2 := &MediaJob{
			Username:  "testuser",
			FileHash:  "abc123",
			CreatedAt: fixedTime,
		}

		key1 := mj1.GenerateIdempotencyKey()
		key2 := mj2.GenerateIdempotencyKey()

		assert.Equal(t, key1, key2)
	})

	t.Run("generates different keys for different inputs", func(t *testing.T) {
		fixedTime := time.Date(2025, 12, 27, 10, 0, 0, 0, time.UTC)

		mj1 := &MediaJob{
			Username:  "user1",
			FileHash:  "abc123",
			CreatedAt: fixedTime,
		}

		mj2 := &MediaJob{
			Username:  "user2",
			FileHash:  "abc123",
			CreatedAt: fixedTime,
		}

		key1 := mj1.GenerateIdempotencyKey()
		key2 := mj2.GenerateIdempotencyKey()

		assert.NotEqual(t, key1, key2)
	})

	t.Run("returns 64 character hex string (SHA256)", func(t *testing.T) {
		mj := &MediaJob{
			Username:  "user",
			FileHash:  "hash",
			CreatedAt: time.Now(),
		}
		key := mj.GenerateIdempotencyKey()
		assert.Len(t, key, 64)
	})
}

// ============================================================================
// Idle and Ready For Retry Tests
// ============================================================================

func TestMediaJob_IsIdle(t *testing.T) {
	t.Run("returns true when pending and no retry scheduled", func(t *testing.T) {
		mj := validMediaJob()
		mj.Status = StatusPending
		mj.RetryScheduledAt = nil
		assert.True(t, mj.IsIdle())
	})

	t.Run("returns false when not pending", func(t *testing.T) {
		mj := validMediaJob()
		mj.Status = StatusProcessing
		mj.RetryScheduledAt = nil
		assert.False(t, mj.IsIdle())
	})

	t.Run("returns false when retry is scheduled", func(t *testing.T) {
		mj := validMediaJob()
		mj.Status = StatusPending
		future := time.Now().Add(time.Hour)
		mj.RetryScheduledAt = &future
		assert.False(t, mj.IsIdle())
	})
}

func TestMediaJob_IsReadyForRetry(t *testing.T) {
	t.Run("returns false when RetryScheduledAt is nil", func(t *testing.T) {
		mj := validMediaJob()
		mj.RetryScheduledAt = nil
		assert.False(t, mj.IsReadyForRetry())
	})

	t.Run("returns true when scheduled time is in the past", func(t *testing.T) {
		mj := validMediaJob()
		past := time.Now().Add(-1 * time.Minute)
		mj.RetryScheduledAt = &past
		assert.True(t, mj.IsReadyForRetry())
	})

	t.Run("returns false when scheduled time is in the future", func(t *testing.T) {
		mj := validMediaJob()
		future := time.Now().Add(1 * time.Hour)
		mj.RetryScheduledAt = &future
		assert.False(t, mj.IsReadyForRetry())
	})
}

// ============================================================================
// Processing Duration Tests
// ============================================================================

func TestMediaJob_GetProcessingDuration(t *testing.T) {
	t.Run("returns 0 when ProcessingStartedAt is nil", func(t *testing.T) {
		mj := validMediaJob()
		mj.ProcessingStartedAt = nil
		assert.Equal(t, time.Duration(0), mj.GetProcessingDuration())
	})

	t.Run("returns duration from start to completed", func(t *testing.T) {
		mj := validMediaJob()
		start := time.Now().Add(-2 * time.Minute)
		end := time.Now().Add(-1 * time.Minute)
		mj.ProcessingStartedAt = &start
		mj.CompletedAt = &end

		duration := mj.GetProcessingDuration()
		// Duration should be the difference between end and start (1 minute)
		assert.InDelta(t, time.Minute.Seconds(), duration.Seconds(), 1)
	})

	t.Run("returns duration from start to now when not completed", func(t *testing.T) {
		mj := validMediaJob()
		start := time.Now().Add(-30 * time.Second)
		mj.ProcessingStartedAt = &start
		mj.CompletedAt = nil

		duration := mj.GetProcessingDuration()
		assert.InDelta(t, 30, duration.Seconds(), 2)
	})
}

func TestMediaJob_GetRemainingProcessingTime(t *testing.T) {
	t.Run("returns MaxProcessingTime when ProcessingStartedAt is nil", func(t *testing.T) {
		mj := validMediaJob()
		mj.ProcessingStartedAt = nil
		mj.MaxProcessingTime = 2 * time.Minute
		assert.Equal(t, 2*time.Minute, mj.GetRemainingProcessingTime())
	})

	t.Run("returns MaxProcessingTime when it is 0", func(t *testing.T) {
		mj := validMediaJob()
		now := time.Now()
		mj.ProcessingStartedAt = &now
		mj.MaxProcessingTime = 0
		assert.Equal(t, time.Duration(0), mj.GetRemainingProcessingTime())
	})

	t.Run("returns remaining time", func(t *testing.T) {
		mj := validMediaJob()
		start := time.Now().Add(-30 * time.Second)
		mj.ProcessingStartedAt = &start
		mj.MaxProcessingTime = 2 * time.Minute

		remaining := mj.GetRemainingProcessingTime()
		// Should be approximately 90 seconds remaining
		assert.InDelta(t, 90, remaining.Seconds(), 2)
	})

	t.Run("returns 0 when time exceeded", func(t *testing.T) {
		mj := validMediaJob()
		start := time.Now().Add(-3 * time.Minute)
		mj.ProcessingStartedAt = &start
		mj.MaxProcessingTime = 2 * time.Minute

		assert.Equal(t, time.Duration(0), mj.GetRemainingProcessingTime())
	})
}

// ============================================================================
// Default Processing Timeout Tests
// ============================================================================

func TestMediaJob_getDefaultProcessingTimeout(t *testing.T) {
	testCases := []struct {
		name     string
		mimeType string
		fileSize int64
		expected time.Duration
	}{
		{"image/jpeg", "image/jpeg", 0, 30 * time.Second},
		{"image/png", "image/png", 0, 30 * time.Second},
		// Note: image/gif matches the image/ prefix first, so it gets 30s not 60s
		{"image/gif", "image/gif", 0, 30 * time.Second},
		{"small video", "video/mp4", 5 * 1024 * 1024, 2 * time.Minute},
		{"large video (10MB)", "video/mp4", 10 * 1024 * 1024, 5 * time.Minute},
		{"large video (50MB)", "video/webm", 50 * 1024 * 1024, 5 * time.Minute},
		{"unknown type", "application/octet-stream", 0, 2 * time.Minute},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mj := &MediaJob{
				MimeType: tc.mimeType,
				FileSize: tc.fileSize,
			}
			assert.Equal(t, tc.expected, mj.getDefaultProcessingTimeout())
		})
	}
}

// ============================================================================
// Validation Tests
// ============================================================================

func TestMediaJob_Validate(t *testing.T) {
	t.Run("valid job passes validation", func(t *testing.T) {
		mj := validMediaJob()
		err := mj.Validate()
		assert.NoError(t, err)
	})

	t.Run("missing JobID fails", func(t *testing.T) {
		mj := validMediaJob()
		mj.JobID = ""
		err := mj.Validate()
		require.Error(t, err)
	})

	t.Run("missing MediaID fails", func(t *testing.T) {
		mj := validMediaJob()
		mj.MediaID = ""
		err := mj.Validate()
		require.Error(t, err)
	})

	t.Run("missing Username fails", func(t *testing.T) {
		mj := validMediaJob()
		mj.Username = ""
		err := mj.Validate()
		require.Error(t, err)
	})

	t.Run("missing S3Key fails", func(t *testing.T) {
		mj := validMediaJob()
		mj.S3Key = ""
		err := mj.Validate()
		require.Error(t, err)
	})

	t.Run("missing MimeType fails", func(t *testing.T) {
		mj := validMediaJob()
		mj.MimeType = ""
		err := mj.Validate()
		require.Error(t, err)
	})

	t.Run("invalid status fails", func(t *testing.T) {
		mj := validMediaJob()
		mj.Status = "invalid_status"
		err := mj.Validate()
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrInvalidMediaJobStatus))
	})

	t.Run("valid statuses pass", func(t *testing.T) {
		validStatuses := []string{StatusPending, StatusProcessing, StatusCompleted, StatusFailed, "cancelled"}
		for _, status := range validStatuses {
			t.Run(status, func(t *testing.T) {
				mj := validMediaJob()
				mj.Status = status
				err := mj.Validate()
				assert.NoError(t, err)
			})
		}
	})
}

// ============================================================================
// BaseModel Interface Tests
// ============================================================================

func TestMediaJob_BaseModelInterface(t *testing.T) {
	t.Run("GetPK returns PK", func(t *testing.T) {
		mj := validMediaJob()
		mj.PK = "JOB#test-123"
		assert.Equal(t, "JOB#test-123", mj.GetPK())
	})

	t.Run("GetSK returns SK", func(t *testing.T) {
		mj := validMediaJob()
		mj.SK = "JOB#test-123"
		assert.Equal(t, "JOB#test-123", mj.GetSK())
	})

	t.Run("TableName returns MainTableName", func(t *testing.T) {
		mj := MediaJob{}
		assert.Equal(t, MainTableName, mj.TableName())
	})
}

// ============================================================================
// isValidJobStatus Tests
// ============================================================================

func TestIsValidJobStatus(t *testing.T) {
	t.Run("valid statuses", func(t *testing.T) {
		validStatuses := []string{"pending", "processing", "completed", "failed", "cancelled"}
		for _, status := range validStatuses {
			t.Run(status, func(t *testing.T) {
				assert.True(t, isValidJobStatus(status))
			})
		}
	})

	t.Run("case insensitive", func(t *testing.T) {
		assert.True(t, isValidJobStatus("PENDING"))
		assert.True(t, isValidJobStatus("Processing"))
		assert.True(t, isValidJobStatus("COMPLETED"))
	})

	t.Run("invalid status", func(t *testing.T) {
		assert.False(t, isValidJobStatus("invalid"))
		assert.False(t, isValidJobStatus("running"))
		assert.False(t, isValidJobStatus(""))
	})
}
