package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Fixed timestamp for deterministic key generation
var alertFixedTime = time.Date(2024, 6, 15, 10, 30, 45, 0, time.UTC)

// TestAlert_Resolve tests the Resolve method
func TestAlert_Resolve(t *testing.T) {
	t.Run("sets status to resolved", func(t *testing.T) {
		a := &Alert{Status: "firing"}
		a.Resolve()
		assert.Equal(t, "resolved", a.Status)
	})

	t.Run("sets ResolvedAt timestamp", func(t *testing.T) {
		a := &Alert{Status: "firing"}
		before := time.Now()
		a.Resolve()
		assert.NotNil(t, a.ResolvedAt)
		assert.WithinDuration(t, before, *a.ResolvedAt, time.Second)
	})

	t.Run("updates UpdatedAt", func(t *testing.T) {
		a := &Alert{Status: "firing"}
		before := time.Now()
		a.Resolve()
		assert.WithinDuration(t, before, a.UpdatedAt, time.Second)
	})
}

// TestAlert_Acknowledge tests the Acknowledge method
func TestAlert_Acknowledge(t *testing.T) {
	a := &Alert{Status: "firing"}
	before := time.Now()
	a.Acknowledge()

	assert.Equal(t, "acknowledged", a.Status)
	assert.WithinDuration(t, before, a.UpdatedAt, time.Second)
}

// TestAlert_Suppress tests the Suppress method
func TestAlert_Suppress(t *testing.T) {
	until := time.Now().Add(2 * time.Hour)
	a := &Alert{Status: "firing"}
	before := time.Now()
	a.Suppress(until)

	assert.Equal(t, "suppressed", a.Status)
	assert.NotNil(t, a.SuppressionUntil)
	assert.Equal(t, until, *a.SuppressionUntil)
	assert.WithinDuration(t, before, a.UpdatedAt, time.Second)
}

// TestAlert_Escalate tests the Escalate method
func TestAlert_Escalate(t *testing.T) {
	t.Run("increments escalation level", func(t *testing.T) {
		a := &Alert{EscalationLevel: 0}
		a.Escalate()
		assert.Equal(t, 1, a.EscalationLevel)
		a.Escalate()
		assert.Equal(t, 2, a.EscalationLevel)
	})

	t.Run("updates timestamp", func(t *testing.T) {
		a := &Alert{}
		before := time.Now()
		a.Escalate()
		assert.WithinDuration(t, before, a.UpdatedAt, time.Second)
	})
}

// TestAlert_RecordDeliveryAttempt tests delivery attempt recording
func TestAlert_RecordDeliveryAttempt(t *testing.T) {
	t.Run("increments delivery attempts", func(t *testing.T) {
		a := &Alert{DeliveryAttempts: 0}
		a.RecordDeliveryAttempt(true)
		assert.Equal(t, 1, a.DeliveryAttempts)
		a.RecordDeliveryAttempt(false)
		assert.Equal(t, 2, a.DeliveryAttempts)
	})

	t.Run("sets LastDeliveryAt", func(t *testing.T) {
		a := &Alert{}
		before := time.Now()
		a.RecordDeliveryAttempt(true)
		assert.NotNil(t, a.LastDeliveryAt)
		assert.WithinDuration(t, before, *a.LastDeliveryAt, time.Second)
	})

	t.Run("success clears NextRetryAt", func(t *testing.T) {
		future := time.Now().Add(time.Hour)
		a := &Alert{NextRetryAt: &future}
		a.RecordDeliveryAttempt(true)
		assert.Nil(t, a.NextRetryAt)
	})

	t.Run("failure sets NextRetryAt", func(t *testing.T) {
		a := &Alert{DeliveryAttempts: 0}
		before := time.Now()
		a.RecordDeliveryAttempt(false)
		assert.NotNil(t, a.NextRetryAt)
		// After increment: DeliveryAttempts=1, so delay = 1<<1 = 2 minutes
		expectedRetry := before.Add(2 * time.Minute)
		assert.WithinDuration(t, expectedRetry, *a.NextRetryAt, 2*time.Second)
	})
}

// TestAlert_CalculateNextRetry tests exponential backoff calculation
// Note: CalculateNextRetry uses 1<<DeliveryAttempts, and RecordDeliveryAttempt
// increments DeliveryAttempts BEFORE calling CalculateNextRetry.
func TestAlert_CalculateNextRetry(t *testing.T) {
	testCases := []struct {
		name             string
		deliveryAttempts int
		expectedDelay    time.Duration
	}{
		{"attempts=1 (2^1)", 1, 2 * time.Minute},
		{"attempts=2 (2^2)", 2, 4 * time.Minute},
		{"attempts=3 (2^3)", 3, 8 * time.Minute},
		{"attempts=4 (2^4 capped)", 4, 16 * time.Minute},
		{"attempts=5 (capped)", 5, 16 * time.Minute},
		{"attempts=10 (capped)", 10, 16 * time.Minute},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			a := &Alert{DeliveryAttempts: tc.deliveryAttempts}
			before := time.Now()
			nextRetry := a.CalculateNextRetry()

			expectedRetry := before.Add(tc.expectedDelay)
			assert.WithinDuration(t, expectedRetry, nextRetry, 2*time.Second)
		})
	}
}

// TestAlert_ShouldRetry tests retry eligibility
func TestAlert_ShouldRetry(t *testing.T) {
	now := time.Now()
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)

	testCases := []struct {
		name     string
		alert    *Alert
		expected bool
	}{
		{
			name:     "nil NextRetryAt returns false",
			alert:    &Alert{DeliveryAttempts: 1},
			expected: false,
		},
		{
			name:     "NextRetryAt in future returns false",
			alert:    &Alert{DeliveryAttempts: 1, NextRetryAt: &future},
			expected: false,
		},
		{
			name:     "NextRetryAt in past with < 5 attempts returns true",
			alert:    &Alert{DeliveryAttempts: 3, NextRetryAt: &past},
			expected: true,
		},
		{
			name:     "5 attempts returns false",
			alert:    &Alert{DeliveryAttempts: 5, NextRetryAt: &past},
			expected: false,
		},
		{
			name:     "more than 5 attempts returns false",
			alert:    &Alert{DeliveryAttempts: 6, NextRetryAt: &past},
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.alert.ShouldRetry()
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestAlert_UpdateKeys tests key generation
func TestAlert_UpdateKeys(t *testing.T) {
	t.Run("missing AlertID returns error", func(t *testing.T) {
		a := &Alert{}
		err := a.UpdateKeys()
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrAlertIDRequired)
	})

	t.Run("PK format", func(t *testing.T) {
		a := &Alert{
			AlertID: "alert-123",
			Type:    "error_rate",
			Service: "api",
			Status:  "firing",
			FiredAt: alertFixedTime,
		}
		err := a.UpdateKeys()
		require.NoError(t, err)
		assert.Equal(t, "ALERT#alert-123", a.PK)
		assert.Equal(t, "METADATA", a.SK)
	})

	t.Run("GSI1 type index", func(t *testing.T) {
		a := &Alert{
			AlertID: "alert-123",
			Type:    "latency",
			Service: "api",
			Status:  "firing",
			FiredAt: alertFixedTime,
		}
		err := a.UpdateKeys()
		require.NoError(t, err)
		assert.Equal(t, "ALERT_TYPE#latency", a.GSI1PK)
		assert.Contains(t, a.GSI1SK, "TIMESTAMP#")
	})

	t.Run("GSI2 service index", func(t *testing.T) {
		a := &Alert{
			AlertID:  "alert-123",
			Type:     "error_rate",
			Service:  "federation",
			Severity: "critical",
			Status:   "firing",
			FiredAt:  alertFixedTime,
		}
		err := a.UpdateKeys()
		require.NoError(t, err)
		assert.Equal(t, "SERVICE#federation", a.GSI2PK)
		assert.Contains(t, a.GSI2SK, "SEVERITY#critical")
	})

	t.Run("GSI3 status index", func(t *testing.T) {
		a := &Alert{
			AlertID:  "alert-123",
			Type:     "error_rate",
			Service:  "api",
			Status:   "firing",
			Priority: "P0",
			FiredAt:  alertFixedTime,
		}
		err := a.UpdateKeys()
		require.NoError(t, err)
		assert.Equal(t, "STATUS#firing", a.GSI3PK)
		assert.Contains(t, a.GSI3SK, "PRIORITY#P0")
	})

	t.Run("GSI3 uses default priority when empty", func(t *testing.T) {
		a := &Alert{
			AlertID: "alert-123",
			Type:    "error_rate",
			Service: "api",
			Status:  "firing",
			FiredAt: alertFixedTime,
		}
		err := a.UpdateKeys()
		require.NoError(t, err)
		assert.Contains(t, a.GSI3SK, "PRIORITY#P2")
	})

	t.Run("sets TTL 30 days", func(t *testing.T) {
		a := &Alert{
			AlertID: "alert-123",
			Type:    "error_rate",
			Service: "api",
			Status:  "firing",
			FiredAt: alertFixedTime,
		}
		before := time.Now()
		err := a.UpdateKeys()
		require.NoError(t, err)

		expectedExpiry := before.Add(30 * 24 * time.Hour)
		actualExpiry := time.Unix(a.TTL, 0)
		assert.WithinDuration(t, expectedExpiry, actualExpiry, 2*time.Second)
	})
}

// TestAlert_BooleanHelpers tests boolean helper methods
func TestAlert_BooleanHelpers(t *testing.T) {
	t.Run("IsActive", func(t *testing.T) {
		assert.True(t, (&Alert{Status: "firing"}).IsActive())
		assert.False(t, (&Alert{Status: "resolved"}).IsActive())
		assert.False(t, (&Alert{Status: "acknowledged"}).IsActive())
	})

	t.Run("IsResolved", func(t *testing.T) {
		assert.True(t, (&Alert{Status: "resolved"}).IsResolved())
		assert.False(t, (&Alert{Status: "firing"}).IsResolved())
	})

	t.Run("IsCritical", func(t *testing.T) {
		assert.True(t, (&Alert{Severity: "critical"}).IsCritical())
		assert.False(t, (&Alert{Severity: "warning"}).IsCritical())
		assert.False(t, (&Alert{Severity: "info"}).IsCritical())
	})

	t.Run("IsHighPriority", func(t *testing.T) {
		assert.True(t, (&Alert{Priority: "P0"}).IsHighPriority())
		assert.True(t, (&Alert{Priority: "P1"}).IsHighPriority())
		assert.False(t, (&Alert{Priority: "P2"}).IsHighPriority())
		assert.False(t, (&Alert{Priority: ""}).IsHighPriority())
	})
}

// TestAlert_Helpers tests helper methods
func TestAlert_Helpers(t *testing.T) {
	t.Run("AddDimension", func(t *testing.T) {
		a := &Alert{}
		a.AddDimension("region", "us-east-1")
		assert.Equal(t, "us-east-1", a.Dimensions["region"])
	})

	t.Run("AddMetadata", func(t *testing.T) {
		a := &Alert{}
		a.AddMetadata("custom", "value")
		assert.Equal(t, "value", a.Metadata["custom"])
	})

	t.Run("AddValue", func(t *testing.T) {
		a := &Alert{}
		a.AddValue("error_rate", 0.05)
		assert.Equal(t, 0.05, a.Values["error_rate"])
	})

	t.Run("AddThreshold", func(t *testing.T) {
		a := &Alert{}
		a.AddThreshold("max_errors", 100.0)
		assert.Equal(t, 100.0, a.Thresholds["max_errors"])
	})
}

// TestWebhookDelivery_UpdateKeys tests WebhookDelivery key generation
func TestWebhookDelivery_UpdateKeys(t *testing.T) {
	t.Run("missing DeliveryID returns error", func(t *testing.T) {
		w := &WebhookDelivery{WebhookID: "webhook-123"}
		err := w.UpdateKeys()
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrDeliveryIDRequired)
	})

	t.Run("missing WebhookID returns error", func(t *testing.T) {
		w := &WebhookDelivery{DeliveryID: "del-123"}
		err := w.UpdateKeys()
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrWebhookIDRequired)
	})

	t.Run("PK and SK format", func(t *testing.T) {
		w := &WebhookDelivery{
			DeliveryID:  "del-123",
			WebhookID:   "webhook-456",
			AlertID:     "alert-789",
			Status:      "pending",
			ScheduledAt: alertFixedTime,
		}
		err := w.UpdateKeys()
		require.NoError(t, err)

		assert.Equal(t, "WEBHOOK#webhook-456", w.PK)
		assert.Equal(t, "DELIVERY#del-123", w.SK)
	})

	t.Run("GSI1 alert index", func(t *testing.T) {
		w := &WebhookDelivery{
			DeliveryID:  "del-123",
			WebhookID:   "webhook-456",
			AlertID:     "alert-789",
			Status:      "success",
			ScheduledAt: alertFixedTime,
		}
		err := w.UpdateKeys()
		require.NoError(t, err)

		assert.Equal(t, "ALERT#alert-789", w.GSI1PK)
		assert.Contains(t, w.GSI1SK, "STATUS#success")
	})

	t.Run("GSI2 status index", func(t *testing.T) {
		w := &WebhookDelivery{
			DeliveryID:  "del-123",
			WebhookID:   "webhook-456",
			Status:      "failed",
			ScheduledAt: alertFixedTime,
		}
		err := w.UpdateKeys()
		require.NoError(t, err)

		assert.Equal(t, "STATUS#failed", w.GSI2PK)
		assert.Contains(t, w.GSI2SK, "TIMESTAMP#")
	})

	t.Run("TTL 7 days", func(t *testing.T) {
		w := &WebhookDelivery{
			DeliveryID:  "del-123",
			WebhookID:   "webhook-456",
			Status:      "pending",
			ScheduledAt: alertFixedTime,
		}
		before := time.Now()
		err := w.UpdateKeys()
		require.NoError(t, err)

		expectedExpiry := before.Add(7 * 24 * time.Hour)
		actualExpiry := time.Unix(w.TTL, 0)
		assert.WithinDuration(t, expectedExpiry, actualExpiry, 2*time.Second)
	})
}

// TestWebhookDelivery_StateHelpers tests state helper methods
func TestWebhookDelivery_StateHelpers(t *testing.T) {
	t.Run("IsComplete", func(t *testing.T) {
		assert.True(t, (&WebhookDelivery{Status: "success"}).IsComplete())
		assert.True(t, (&WebhookDelivery{Status: "failed", AttemptNumber: 3, MaxAttempts: 3}).IsComplete())
		assert.False(t, (&WebhookDelivery{Status: "failed", AttemptNumber: 2, MaxAttempts: 3}).IsComplete())
		assert.False(t, (&WebhookDelivery{Status: "pending"}).IsComplete())
	})

	t.Run("CanRetry", func(t *testing.T) {
		assert.True(t, (&WebhookDelivery{Status: "failed", AttemptNumber: 2, MaxAttempts: 3}).CanRetry())
		assert.False(t, (&WebhookDelivery{Status: "failed", AttemptNumber: 3, MaxAttempts: 3}).CanRetry())
		assert.False(t, (&WebhookDelivery{Status: "success"}).CanRetry())
	})

	t.Run("ShouldRetry", func(t *testing.T) {
		past := time.Now().Add(-time.Hour)
		future := time.Now().Add(time.Hour)

		// Can retry and no NextRetryAt -> true
		assert.True(t, (&WebhookDelivery{Status: "failed", AttemptNumber: 1, MaxAttempts: 3}).ShouldRetry())

		// Can retry and NextRetryAt in past -> true
		assert.True(t, (&WebhookDelivery{Status: "failed", AttemptNumber: 1, MaxAttempts: 3, NextRetryAt: &past}).ShouldRetry())

		// Can retry but NextRetryAt in future -> false
		assert.False(t, (&WebhookDelivery{Status: "failed", AttemptNumber: 1, MaxAttempts: 3, NextRetryAt: &future}).ShouldRetry())

		// Cannot retry -> false
		assert.False(t, (&WebhookDelivery{Status: "success"}).ShouldRetry())
	})
}

// TestWebhookDelivery_StateMethods tests state transition methods
func TestWebhookDelivery_StateMethods(t *testing.T) {
	t.Run("MarkStarted", func(t *testing.T) {
		w := &WebhookDelivery{}
		before := time.Now()
		w.MarkStarted()
		assert.NotNil(t, w.StartedAt)
		assert.WithinDuration(t, before, *w.StartedAt, time.Second)
	})

	t.Run("MarkSuccess", func(t *testing.T) {
		w := &WebhookDelivery{Status: "pending"}
		headers := map[string]string{"X-Test": "value"}
		w.MarkSuccess(200, "OK", headers, 150*time.Millisecond)

		assert.Equal(t, "success", w.Status)
		assert.Equal(t, 200, w.ResponseCode)
		assert.Equal(t, "OK", w.ResponseBody)
		assert.Equal(t, headers, w.ResponseHeaders)
		assert.Equal(t, int64(150), w.Duration)
		assert.NotNil(t, w.CompletedAt)
		assert.Nil(t, w.NextRetryAt)
	})

	t.Run("MarkFailed with retries remaining", func(t *testing.T) {
		w := &WebhookDelivery{
			Status:        "pending",
			AttemptNumber: 1,
			MaxAttempts:   3,
		}
		w.MarkFailed("connection timeout", "timeout", 0, "", 100*time.Millisecond)

		assert.Equal(t, "retrying", w.Status)
		assert.Equal(t, "connection timeout", w.ErrorMessage)
		assert.Equal(t, "timeout", w.ErrorType)
		assert.NotNil(t, w.NextRetryAt)
	})

	t.Run("MarkFailed with no retries remaining", func(t *testing.T) {
		w := &WebhookDelivery{
			Status:        "pending",
			AttemptNumber: 3,
			MaxAttempts:   3,
		}
		w.MarkFailed("server error", "server_error", 500, "Internal Server Error", 100*time.Millisecond)

		assert.Equal(t, "failed", w.Status)
		assert.Equal(t, 500, w.ResponseCode)
	})
}

// TestDeadLetterMessage_UpdateKeys tests DeadLetterMessage key generation
func TestDeadLetterMessage_UpdateKeys(t *testing.T) {
	t.Run("missing MessageID returns error", func(t *testing.T) {
		d := &DeadLetterMessage{OriginalType: "alert"}
		err := d.UpdateKeys()
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrMessageIDRequired)
	})

	t.Run("PK and SK format", func(t *testing.T) {
		d := &DeadLetterMessage{
			MessageID:    "msg-123",
			OriginalType: "notification",
		}
		err := d.UpdateKeys()
		require.NoError(t, err)

		assert.Equal(t, "DLQ#notification", d.PK)
		assert.Equal(t, "MESSAGE#msg-123", d.SK)
	})

	t.Run("TTL 30 days", func(t *testing.T) {
		d := &DeadLetterMessage{
			MessageID:    "msg-123",
			OriginalType: "notification",
		}
		before := time.Now()
		err := d.UpdateKeys()
		require.NoError(t, err)

		expectedExpiry := before.Add(30 * 24 * time.Hour)
		actualExpiry := time.Unix(d.TTL, 0)
		assert.WithinDuration(t, expectedExpiry, actualExpiry, 2*time.Second)
	})
}
