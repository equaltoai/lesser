package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDLQMessage_BeforeCreate tests the BeforeCreate lifecycle hook
func TestDLQMessage_BeforeCreate(t *testing.T) {
	t.Run("generates ID when empty", func(t *testing.T) {
		d := &DLQMessage{
			OriginalMessageID: "msg-123",
			Service:           "notification-processor",
			MessageBody:       `{"test": "data"}`,
			ErrorType:         "validation_error",
			ErrorMessage:      "Invalid message format",
		}
		err := d.BeforeCreate()
		require.NoError(t, err)
		assert.NotEmpty(t, d.ID)
		assert.Len(t, d.ID, 36)
	})

	t.Run("sets default status to new", func(t *testing.T) {
		d := &DLQMessage{
			OriginalMessageID: "msg-123",
			Service:           "notification-processor",
			MessageBody:       `{"test": "data"}`,
			ErrorType:         "validation_error",
			ErrorMessage:      "Error",
		}
		err := d.BeforeCreate()
		require.NoError(t, err)
		assert.Equal(t, "new", d.Status)
	})

	t.Run("sets default priority to medium", func(t *testing.T) {
		d := &DLQMessage{
			OriginalMessageID: "msg-123",
			Service:           "notification-processor",
			MessageBody:       `{"test": "data"}`,
			ErrorType:         "validation_error",
			ErrorMessage:      "Error",
		}
		err := d.BeforeCreate()
		require.NoError(t, err)
		assert.Equal(t, "medium", d.Priority)
	})

	t.Run("sets default MaxReprocessAttempts to 3", func(t *testing.T) {
		d := &DLQMessage{
			OriginalMessageID: "msg-123",
			Service:           "notification-processor",
			MessageBody:       `{"test": "data"}`,
			ErrorType:         "validation_error",
			ErrorMessage:      "Error",
		}
		err := d.BeforeCreate()
		require.NoError(t, err)
		assert.Equal(t, 3, d.MaxReprocessAttempts)
	})

	t.Run("TTL 90 days", func(t *testing.T) {
		d := &DLQMessage{
			OriginalMessageID: "msg-123",
			Service:           "notification-processor",
			MessageBody:       `{"test": "data"}`,
			ErrorType:         "validation_error",
			ErrorMessage:      "Error",
		}
		before := time.Now()
		err := d.BeforeCreate()
		require.NoError(t, err)

		expectedExpiry := before.Add(90 * 24 * time.Hour)
		actualExpiry := time.Unix(d.ExpiresAt, 0)
		assert.WithinDuration(t, expectedExpiry, actualExpiry, 2*time.Second)
	})

	t.Run("generates SimilarityHash", func(t *testing.T) {
		d := &DLQMessage{
			OriginalMessageID: "msg-123",
			Service:           "notification-processor",
			MessageBody:       `{"test": "data"}`,
			ErrorType:         "validation_error",
			ErrorMessage:      "Error",
		}
		err := d.BeforeCreate()
		require.NoError(t, err)
		assert.NotEmpty(t, d.SimilarityHash)
	})

	t.Run("PK format", func(t *testing.T) {
		d := &DLQMessage{
			OriginalMessageID: "msg-123",
			Service:           "notification-processor",
			MessageBody:       `{"test": "data"}`,
			ErrorType:         "validation_error",
			ErrorMessage:      "Error",
		}
		err := d.BeforeCreate()
		require.NoError(t, err)
		// Format: DLQ#{service}#{date}
		assert.Contains(t, d.PK, "DLQ#notification-processor#")
	})

	t.Run("SK format", func(t *testing.T) {
		d := &DLQMessage{
			OriginalMessageID: "msg-123",
			Service:           "notification-processor",
			MessageBody:       `{"test": "data"}`,
			ErrorType:         "validation_error",
			ErrorMessage:      "Error",
		}
		err := d.BeforeCreate()
		require.NoError(t, err)
		// Format: MSG#{timestamp}#{messageId}
		assert.Contains(t, d.SK, "MSG#")
		assert.Contains(t, d.SK, "#msg-123")
	})
}

// TestDLQMessage_setupGSIKeys tests GSI key generation
func TestDLQMessage_setupGSIKeys(t *testing.T) {
	now := time.Now()

	t.Run("GSI1 error type", func(t *testing.T) {
		d := &DLQMessage{
			ID:          "dlq-123",
			Service:     "api",
			ErrorType:   "timeout_error",
			Status:      "new",
			FirstSeenAt: now,
		}
		d.setupGSIKeys()
		assert.Equal(t, "DLQ_ERROR#timeout_error", d.GSI1PK)
		assert.Contains(t, d.GSI1SK, "api")
		assert.Contains(t, d.GSI1SK, "dlq-123")
	})

	t.Run("GSI2 retry status", func(t *testing.T) {
		d := &DLQMessage{
			ID:          "dlq-123",
			Service:     "api",
			ErrorType:   "timeout_error",
			Status:      "reprocessing",
			FirstSeenAt: now,
		}
		d.setupGSIKeys()
		assert.Equal(t, "DLQ_RETRY#api#reprocessing", d.GSI2PK)
		assert.Contains(t, d.GSI2SK, "dlq-123")
	})

	t.Run("GSI3 service", func(t *testing.T) {
		d := &DLQMessage{
			ID:          "dlq-123",
			Service:     "federation",
			ErrorType:   "network_error",
			Status:      "new",
			FirstSeenAt: now,
		}
		d.setupGSIKeys()
		assert.Equal(t, "DLQ_SERVICE#federation", d.GSI3PK)
		assert.Contains(t, d.GSI3SK, "network_error")
		assert.Contains(t, d.GSI3SK, "dlq-123")
	})
}

// TestDLQMessage_Validate tests validation
func TestDLQMessage_Validate(t *testing.T) {
	validDLQ := func() *DLQMessage {
		return &DLQMessage{
			ID:                "dlq-123",
			OriginalMessageID: "msg-456",
			Service:           "test-service",
			MessageBody:       "test body",
			ErrorType:         "validation_error",
			ErrorMessage:      "test error",
			Status:            "new",
			Priority:          "medium",
		}
	}

	t.Run("valid DLQ message", func(t *testing.T) {
		d := validDLQ()
		err := d.Validate()
		assert.NoError(t, err)
	})

	t.Run("missing ID", func(t *testing.T) {
		d := validDLQ()
		d.ID = ""
		err := d.Validate()
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrDLQIDRequired)
	})

	t.Run("missing OriginalMessageID", func(t *testing.T) {
		d := validDLQ()
		d.OriginalMessageID = ""
		err := d.Validate()
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrDLQOriginalMessageIDRequired)
	})

	t.Run("missing Service", func(t *testing.T) {
		d := validDLQ()
		d.Service = ""
		err := d.Validate()
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrDLQServiceRequired)
	})

	t.Run("missing MessageBody", func(t *testing.T) {
		d := validDLQ()
		d.MessageBody = ""
		err := d.Validate()
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrDLQMessageBodyRequired)
	})

	t.Run("missing ErrorType", func(t *testing.T) {
		d := validDLQ()
		d.ErrorType = ""
		err := d.Validate()
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrDLQErrorTypeRequired)
	})

	t.Run("missing ErrorMessage", func(t *testing.T) {
		d := validDLQ()
		d.ErrorMessage = ""
		err := d.Validate()
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrDLQErrorMessageRequired)
	})

	t.Run("invalid status", func(t *testing.T) {
		d := validDLQ()
		d.Status = "invalid_status"
		err := d.Validate()
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrDLQInvalidStatus)
	})

	t.Run("invalid priority", func(t *testing.T) {
		d := validDLQ()
		d.Priority = "invalid_priority"
		err := d.Validate()
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrDLQInvalidPriority)
	})

	// Test valid statuses
	validStatuses := []string{"new", "reprocessing", "failed", "resolved", "abandoned"}
	for _, status := range validStatuses {
		t.Run("valid status: "+status, func(t *testing.T) {
			d := validDLQ()
			d.Status = status
			err := d.Validate()
			assert.NoError(t, err)
		})
	}

	// Test valid priorities
	validPriorities := []string{"low", "medium", "high", "critical"}
	for _, priority := range validPriorities {
		t.Run("valid priority: "+priority, func(t *testing.T) {
			d := validDLQ()
			d.Priority = priority
			err := d.Validate()
			assert.NoError(t, err)
		})
	}
}

// TestDLQMessage_MarkForReprocessing tests exponential backoff
func TestDLQMessage_MarkForReprocessing(t *testing.T) {
	testCases := []struct {
		name              string
		reprocessingCount int
		expectedBackoff   int // minutes
	}{
		{"first attempt", 0, 2},           // 2^1 = 2 minutes
		{"second attempt", 1, 4},          // 2^2 = 4 minutes
		{"third attempt", 2, 8},           // 2^3 = 8 minutes
		{"fourth attempt", 3, 16},         // 2^4 = 16 minutes
		{"fifth attempt", 4, 32},          // 2^5 = 32 minutes
		{"sixth attempt (capped)", 5, 60}, // capped at 60 minutes
		{"high count (capped)", 10, 60},   // capped at 60 minutes
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			d := &DLQMessage{
				Status:            "new",
				ReprocessingCount: tc.reprocessingCount,
			}
			before := time.Now()
			d.MarkForReprocessing()

			assert.Equal(t, "reprocessing", d.Status)
			assert.Equal(t, tc.reprocessingCount+1, d.ReprocessingCount)
			assert.NotNil(t, d.NextRetryAt)

			expectedRetry := before.Add(time.Duration(tc.expectedBackoff) * time.Minute)
			assert.WithinDuration(t, expectedRetry, *d.NextRetryAt, 2*time.Second)
		})
	}
}

// TestDLQMessage_CanReprocess tests reprocessing eligibility
func TestDLQMessage_CanReprocess(t *testing.T) {
	now := time.Now()
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)

	testCases := []struct {
		name     string
		setup    func() *DLQMessage
		expected bool
	}{
		{
			name: "resolved returns false",
			setup: func() *DLQMessage {
				return &DLQMessage{
					Status:               "resolved",
					MaxReprocessAttempts: 3,
				}
			},
			expected: false,
		},
		{
			name: "abandoned returns false",
			setup: func() *DLQMessage {
				return &DLQMessage{
					Status:               "abandoned",
					MaxReprocessAttempts: 3,
				}
			},
			expected: false,
		},
		{
			name: "permanent failure returns false",
			setup: func() *DLQMessage {
				return &DLQMessage{
					Status:               "new",
					IsPermanent:          true,
					MaxReprocessAttempts: 3,
				}
			},
			expected: false,
		},
		{
			name: "max attempts reached returns false",
			setup: func() *DLQMessage {
				return &DLQMessage{
					Status:               "failed",
					ReprocessingCount:    3,
					MaxReprocessAttempts: 3,
				}
			},
			expected: false,
		},
		{
			name: "NextRetryAt in future returns false",
			setup: func() *DLQMessage {
				return &DLQMessage{
					Status:               "reprocessing",
					ReprocessingCount:    1,
					MaxReprocessAttempts: 3,
					NextRetryAt:          &future,
				}
			},
			expected: false,
		},
		{
			name: "NextRetryAt in past returns true",
			setup: func() *DLQMessage {
				return &DLQMessage{
					Status:               "failed",
					ReprocessingCount:    1,
					MaxReprocessAttempts: 3,
					NextRetryAt:          &past,
				}
			},
			expected: true,
		},
		{
			name: "new status with no NextRetryAt returns true",
			setup: func() *DLQMessage {
				return &DLQMessage{
					Status:               "new",
					ReprocessingCount:    0,
					MaxReprocessAttempts: 3,
				}
			},
			expected: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			d := tc.setup()
			result := d.CanReprocess()
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestDLQMessage_ShouldAbandon tests abandonment conditions
func TestDLQMessage_ShouldAbandon(t *testing.T) {
	testCases := []struct {
		name     string
		dlq      *DLQMessage
		expected bool
	}{
		{
			name: "at max attempts",
			dlq: &DLQMessage{
				ReprocessingCount:    3,
				MaxReprocessAttempts: 3,
			},
			expected: true,
		},
		{
			name: "over max attempts",
			dlq: &DLQMessage{
				ReprocessingCount:    5,
				MaxReprocessAttempts: 3,
			},
			expected: true,
		},
		{
			name: "permanent failure",
			dlq: &DLQMessage{
				ReprocessingCount:    0,
				MaxReprocessAttempts: 3,
				IsPermanent:          true,
			},
			expected: true,
		},
		{
			name: "under max attempts, not permanent",
			dlq: &DLQMessage{
				ReprocessingCount:    1,
				MaxReprocessAttempts: 3,
				IsPermanent:          false,
			},
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.dlq.ShouldAbandon()
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestDLQMessage_StatusMethods tests status transition methods
func TestDLQMessage_StatusMethods(t *testing.T) {
	t.Run("MarkResolved", func(t *testing.T) {
		d := &DLQMessage{Status: "reprocessing"}
		d.MarkResolved()
		assert.Equal(t, "resolved", d.Status)
		assert.NotNil(t, d.ResolvedAt)
		assert.Nil(t, d.NextRetryAt)
	})

	t.Run("MarkFailed", func(t *testing.T) {
		d := &DLQMessage{Status: "reprocessing"}
		d.MarkFailed("new error message")
		assert.Equal(t, "failed", d.Status)
		assert.Equal(t, "new error message", d.ErrorMessage)
		assert.NotNil(t, d.LastProcessedAt)
	})

	t.Run("MarkAbandoned", func(t *testing.T) {
		future := time.Now().Add(time.Hour)
		d := &DLQMessage{
			Status:      "failed",
			NextRetryAt: &future,
		}
		d.MarkAbandoned()
		assert.Equal(t, "abandoned", d.Status)
		assert.Nil(t, d.NextRetryAt)
	})
}

// TestDLQMessage_Helpers tests helper methods
func TestDLQMessage_Helpers(t *testing.T) {
	t.Run("AddTag deduplicates", func(t *testing.T) {
		d := &DLQMessage{}
		d.AddTag("important")
		d.AddTag("urgent")
		d.AddTag("important") // duplicate
		assert.Len(t, d.Tags, 2)
		assert.Contains(t, d.Tags, "important")
		assert.Contains(t, d.Tags, "urgent")
	})

	t.Run("SetMetadata and GetMetadata", func(t *testing.T) {
		d := &DLQMessage{}
		d.SetMetadata("key", "value")
		val, ok := d.GetMetadata("key")
		assert.True(t, ok)
		assert.Equal(t, "value", val)
	})

	t.Run("GetMetadata not found", func(t *testing.T) {
		d := &DLQMessage{}
		val, ok := d.GetMetadata("nonexistent")
		assert.False(t, ok)
		assert.Nil(t, val)
	})

	t.Run("UpdateCosts and GetTotalCost", func(t *testing.T) {
		d := &DLQMessage{}
		d.UpdateCosts(100, 50)
		d.UpdateCosts(200, 100)
		assert.Equal(t, int64(300), d.ProcessingCostMicroCents)
		assert.Equal(t, int64(150), d.ReprocessingCostMicroCents)
		assert.Equal(t, int64(450), d.GetTotalCost())
	})
}

// TestDLQMessageBuilder tests the builder pattern
func TestDLQMessageBuilder(t *testing.T) {
	t.Run("builds complete message", func(t *testing.T) {
		d := NewDLQMessageBuilder().
			ForService("notification-processor").
			WithOriginalMessage("msg-123", `{"test": "data"}`).
			WithQueue("dlq-queue", "source-queue").
			WithError("validation_error", "Invalid format", "stack trace").
			WithFailureReason("Message validation failed").
			WithPriority("high").
			WithContext("my-lambda", "log-group", "log-stream", "req-123").
			WithRetryInfo(2, 5).
			WithBusinessImpact("High - affects user notifications").
			WithTags("important", "customer-facing").
			Build()

		assert.Equal(t, "notification-processor", d.Service)
		assert.Equal(t, "msg-123", d.OriginalMessageID)
		assert.Equal(t, `{"test": "data"}`, d.MessageBody)
		assert.Equal(t, "dlq-queue", d.QueueName)
		assert.Equal(t, "source-queue", d.SourceQueue)
		assert.Equal(t, "validation_error", d.ErrorType)
		assert.Equal(t, "Invalid format", d.ErrorMessage)
		assert.Equal(t, "stack trace", d.ErrorStack)
		assert.Equal(t, "Message validation failed", d.FailureReason)
		assert.Equal(t, "high", d.Priority)
		assert.Equal(t, "my-lambda", d.FunctionName)
		assert.Equal(t, 2, d.OriginalRetryCount)
		assert.Equal(t, 5, d.MaxReprocessAttempts)
		assert.Equal(t, "High - affects user notifications", d.BusinessImpact)
		assert.Len(t, d.Tags, 2)
	})

	t.Run("MarkAsPermanent", func(t *testing.T) {
		d := NewDLQMessageBuilder().
			ForService("test").
			MarkAsPermanent().
			Build()
		assert.True(t, d.IsPermanent)
	})
}

// TestNewValidationErrorDLQ tests convenience constructor
func TestNewValidationErrorDLQ(t *testing.T) {
	d := NewValidationErrorDLQ("api", "msg-123", `{"bad": "data"}`, "Field X is required")
	assert.Equal(t, "api", d.Service)
	assert.Equal(t, "msg-123", d.OriginalMessageID)
	assert.Equal(t, `{"bad": "data"}`, d.MessageBody)
	assert.Equal(t, "validation_error", d.ErrorType)
	assert.Equal(t, "Field X is required", d.ErrorMessage)
	assert.Equal(t, "medium", d.Priority)
	assert.True(t, d.IsPermanent)
}

// TestNewTransientErrorDLQ tests convenience constructor
func TestNewTransientErrorDLQ(t *testing.T) {
	d := NewTransientErrorDLQ("federation", "msg-456", `{"data": "test"}`, "Connection timeout")
	assert.Equal(t, "federation", d.Service)
	assert.Equal(t, "transient_error", d.ErrorType)
	assert.Equal(t, "high", d.Priority)
	assert.Equal(t, 5, d.MaxReprocessAttempts) // More retries for transient
	assert.False(t, d.IsPermanent)
}

func TestDLQMessage_UpdateKeys_SuccessBuildsKeys(t *testing.T) {
	firstSeen := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	d := &DLQMessage{
		ID:                "dlq-1",
		Service:           "notification-processor",
		OriginalMessageID: "msg-123",
		ErrorType:         "validation_error",
		Status:            "new",
		FirstSeenAt:       firstSeen,
	}

	err := d.UpdateKeys()
	require.NoError(t, err)

	// PK/SK construction from Service + FirstSeenAt + OriginalMessageID.
	assert.Equal(t, "DLQ#notification-processor#20250102", d.PK)
	assert.Equal(t, "MSG#20250102030405#msg-123", d.SK)

	// GSI keys derived from the same fields.
	assert.Equal(t, "DLQ_ERROR#validation_error", d.GSI1PK)
	assert.Equal(t, "2025-01-02T03:04:05Z#notification-processor#dlq-1", d.GSI1SK)
	assert.Equal(t, "DLQ_RETRY#notification-processor#new", d.GSI2PK)
	assert.Equal(t, "2025-01-02T03:04:05Z#dlq-1", d.GSI2SK)
	assert.Equal(t, "DLQ_SERVICE#notification-processor", d.GSI3PK)
	assert.Equal(t, "2025-01-02T03:04:05Z#validation_error#dlq-1", d.GSI3SK)
}
