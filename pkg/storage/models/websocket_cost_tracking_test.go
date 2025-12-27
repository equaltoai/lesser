package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWebSocketCostRecord_UpdateKeys tests key generation for WebSocketCostRecord
func TestWebSocketCostRecord_UpdateKeys(t *testing.T) {
	ts := time.Date(2024, 6, 15, 10, 30, 45, 0, time.UTC)

	t.Run("PK format", func(t *testing.T) {
		w := &WebSocketCostRecord{
			OperationType: "connect",
			ConnectionID:  "conn-123",
			Timestamp:     ts,
		}
		err := w.UpdateKeys()
		require.NoError(t, err)
		assert.Equal(t, "WS_COST#connect", w.PK)
	})

	t.Run("SK format", func(t *testing.T) {
		w := &WebSocketCostRecord{
			ID:            "record-id",
			OperationType: "connect",
			ConnectionID:  "conn-123",
			Timestamp:     ts,
		}
		err := w.UpdateKeys()
		require.NoError(t, err)
		assert.Equal(t, "ts#20240615103045#record-id", w.SK)
	})

	t.Run("generates ID when empty", func(t *testing.T) {
		w := &WebSocketCostRecord{
			OperationType: "connect",
			ConnectionID:  "conn-123",
			Timestamp:     ts,
		}
		err := w.UpdateKeys()
		require.NoError(t, err)
		assert.NotEmpty(t, w.ID)
		assert.Len(t, w.ID, 36) // UUID length
	})

	t.Run("sets timestamp when empty", func(t *testing.T) {
		w := &WebSocketCostRecord{
			OperationType: "connect",
			ConnectionID:  "conn-123",
		}
		before := time.Now()
		err := w.UpdateKeys()
		require.NoError(t, err)
		assert.WithinDuration(t, before, w.Timestamp, time.Second)
	})
}

// TestWebSocketCostRecord_setupGSIKeys tests GSI key generation
func TestWebSocketCostRecord_setupGSIKeys(t *testing.T) {
	ts := time.Date(2024, 6, 15, 10, 30, 45, 0, time.UTC)

	t.Run("GSI1 connection key", func(t *testing.T) {
		w := &WebSocketCostRecord{
			ID:            "record-id",
			OperationType: "message_in",
			ConnectionID:  "conn-abc",
			UserID:        "user-123",
			Timestamp:     ts,
		}
		w.setupGSIKeys()
		assert.Equal(t, "WS_CONN#conn-abc", w.GSI1PK)
		assert.Contains(t, w.GSI1SK, "message_in")
		assert.Contains(t, w.GSI1SK, "record-id")
	})

	t.Run("GSI2 user key", func(t *testing.T) {
		w := &WebSocketCostRecord{
			ID:            "record-id",
			OperationType: "message_out",
			ConnectionID:  "conn-abc",
			UserID:        "user-123",
			Timestamp:     ts,
		}
		w.setupGSIKeys()
		assert.Equal(t, "WS_USER#user-123", w.GSI2PK)
		assert.Contains(t, w.GSI2SK, "message_out")
	})

	t.Run("GSI1 empty when no connection ID", func(t *testing.T) {
		w := &WebSocketCostRecord{
			ID:            "record-id",
			OperationType: "connect",
			ConnectionID:  "",
			UserID:        "user-123",
			Timestamp:     ts,
		}
		w.setupGSIKeys()
		assert.Empty(t, w.GSI1PK)
	})

	t.Run("GSI2 empty when no user ID", func(t *testing.T) {
		w := &WebSocketCostRecord{
			ID:            "record-id",
			OperationType: "connect",
			ConnectionID:  "conn-abc",
			UserID:        "",
			Timestamp:     ts,
		}
		w.setupGSIKeys()
		assert.Empty(t, w.GSI2PK)
	})
}

// TestWebSocketCostRecord_Validate tests validation
func TestWebSocketCostRecord_Validate(t *testing.T) {
	validOperationTypes := []string{
		"connect", "disconnect", "message_in", "message_out",
		"subscribe", "unsubscribe", "idle_time", "ping", "error",
	}

	for _, opType := range validOperationTypes {
		t.Run("valid operation type: "+opType, func(t *testing.T) {
			w := &WebSocketCostRecord{
				ID:            "123",
				OperationType: opType,
				ConnectionID:  "conn-abc",
			}
			err := w.Validate()
			assert.NoError(t, err)
		})
	}

	t.Run("invalid operation type", func(t *testing.T) {
		w := &WebSocketCostRecord{
			ID:            "123",
			OperationType: "invalid_op",
			ConnectionID:  "conn-abc",
		}
		err := w.Validate()
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidWebSocketOperationType)
	})

	t.Run("missing connection ID", func(t *testing.T) {
		w := &WebSocketCostRecord{
			ID:            "123",
			OperationType: "connect",
		}
		err := w.Validate()
		assert.Error(t, err)
	})

	t.Run("missing ID", func(t *testing.T) {
		w := &WebSocketCostRecord{
			OperationType: "connect",
			ConnectionID:  "conn-abc",
		}
		err := w.Validate()
		assert.Error(t, err)
	})
}

// TestWebSocketCostBudget_updateStatus tests budget status transitions
func TestWebSocketCostBudget_updateStatus(t *testing.T) {
	testCases := []struct {
		name           string
		usagePercent   float64
		suspendAt      int
		expectedStatus string
	}{
		{"100% exceeded", 100, 0, "exceeded"},
		{"150% exceeded", 150, 0, "exceeded"},
		{"90% warning", 90, 0, "warning"},
		{"95% warning", 95, 0, "warning"},
		{"89% active", 89, 0, "active"},
		{"50% active", 50, 0, "active"},
		{"0% active", 0, 0, "active"},
		{"80% suspended (suspendAt=80)", 80, 80, "suspended"},
		{"85% suspended (suspendAt=80)", 85, 80, "suspended"},
		{"70% active (suspendAt=80)", 70, 80, "active"},
		{"100% exceeded takes precedence over suspended", 100, 80, "exceeded"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			w := &WebSocketCostBudget{
				UsagePercent: tc.usagePercent,
				SuspendAt:    tc.suspendAt,
			}
			w.updateStatus()
			assert.Equal(t, tc.expectedStatus, w.Status)
		})
	}
}

// TestWebSocketCostBudget_Validate tests budget validation
func TestWebSocketCostBudget_Validate(t *testing.T) {
	now := time.Now()
	earlier := now.Add(-time.Hour)

	t.Run("valid budget", func(t *testing.T) {
		w := &WebSocketCostBudget{
			UserID:           "user-123",
			Period:           "daily",
			BudgetMicroCents: 1000000,
			WindowStart:      earlier,
			WindowEnd:        now,
		}
		err := w.Validate()
		assert.NoError(t, err)
	})

	t.Run("missing UserID", func(t *testing.T) {
		w := &WebSocketCostBudget{
			Period:           "daily",
			BudgetMicroCents: 1000000,
			WindowStart:      earlier,
			WindowEnd:        now,
		}
		err := w.Validate()
		assert.Error(t, err)
	})

	t.Run("missing Period", func(t *testing.T) {
		w := &WebSocketCostBudget{
			UserID:           "user-123",
			BudgetMicroCents: 1000000,
			WindowStart:      earlier,
			WindowEnd:        now,
		}
		err := w.Validate()
		assert.Error(t, err)
	})

	t.Run("invalid period", func(t *testing.T) {
		w := &WebSocketCostBudget{
			UserID:           "user-123",
			Period:           "yearly",
			BudgetMicroCents: 1000000,
			WindowStart:      earlier,
			WindowEnd:        now,
		}
		err := w.Validate()
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidWebSocketPeriod)
	})

	t.Run("negative budget", func(t *testing.T) {
		w := &WebSocketCostBudget{
			UserID:           "user-123",
			Period:           "daily",
			BudgetMicroCents: -100,
			WindowStart:      earlier,
			WindowEnd:        now,
		}
		err := w.Validate()
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrBudgetMicroCentsNegative)
	})

	t.Run("missing WindowStart", func(t *testing.T) {
		w := &WebSocketCostBudget{
			UserID:           "user-123",
			Period:           "daily",
			BudgetMicroCents: 1000000,
			WindowEnd:        now,
		}
		err := w.Validate()
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrWebSocketWindowStartRequired)
	})

	t.Run("missing WindowEnd", func(t *testing.T) {
		w := &WebSocketCostBudget{
			UserID:           "user-123",
			Period:           "daily",
			BudgetMicroCents: 1000000,
			WindowStart:      earlier,
		}
		err := w.Validate()
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrWebSocketWindowEndRequired)
	})

	t.Run("WindowEnd before WindowStart", func(t *testing.T) {
		w := &WebSocketCostBudget{
			UserID:           "user-123",
			Period:           "daily",
			BudgetMicroCents: 1000000,
			WindowStart:      now,
			WindowEnd:        earlier,
		}
		err := w.Validate()
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrWebSocketWindowEndBeforeStart)
	})
}

// TestWebSocketCostBudget_ValidPeriods tests valid budget periods
func TestWebSocketCostBudget_ValidPeriods(t *testing.T) {
	now := time.Now()
	earlier := now.Add(-time.Hour)
	validPeriods := []string{"daily", "weekly", "monthly"}

	for _, period := range validPeriods {
		t.Run(period, func(t *testing.T) {
			w := &WebSocketCostBudget{
				UserID:           "user-123",
				Period:           period,
				BudgetMicroCents: 1000000,
				WindowStart:      earlier,
				WindowEnd:        now,
			}
			err := w.Validate()
			assert.NoError(t, err)
		})
	}
}

// TestCalculateWebSocketCosts tests cost calculation
func TestCalculateWebSocketCosts(t *testing.T) {
	t.Run("connection cost", func(t *testing.T) {
		// APIGatewayConnectionCostPerMinute = 25 microcents per 100 minutes
		// Formula: (connectionMinutes * 25) / 100
		breakdown := CalculateWebSocketCosts("connect", 100, 0, 0, 0)
		assert.Equal(t, int64(25), breakdown.APIGatewayConnectionCost)
	})

	t.Run("connection cost scaling", func(t *testing.T) {
		breakdown := CalculateWebSocketCosts("connect", 200, 0, 0, 0)
		assert.Equal(t, int64(50), breakdown.APIGatewayConnectionCost)
	})

	t.Run("message cost", func(t *testing.T) {
		// APIGatewayMessageCostPerMessage = 1 microcent per message
		breakdown := CalculateWebSocketCosts("message_out", 0, 100, 0, 0)
		assert.Equal(t, int64(100), breakdown.APIGatewayMessageCost)
	})

	t.Run("lambda execution cost", func(t *testing.T) {
		// LambdaCostPerSecond512MB = 8334 microcents per 1000 seconds
		// Formula: (durationSeconds * 8334) / 1000
		// 1000ms = 1 second -> 8.334 microcents
		breakdown := CalculateWebSocketCosts("message_in", 0, 0, 0, 1000)
		assert.Equal(t, int64(8), breakdown.LambdaExecutionCost)
	})

	t.Run("data transfer cost", func(t *testing.T) {
		// DataTransferCostPerMB = 90 microcents per MB
		breakdown := CalculateWebSocketCosts("message_out", 0, 0, 2.0, 0)
		assert.Equal(t, int64(180), breakdown.DataTransferCost)
	})

	t.Run("total cost consistency", func(t *testing.T) {
		breakdown := CalculateWebSocketCosts("message_out", 100, 50, 1.0, 500)

		expectedTotal := breakdown.APIGatewayConnectionCost +
			breakdown.APIGatewayMessageCost +
			breakdown.LambdaExecutionCost +
			breakdown.DataTransferCost

		assert.Equal(t, expectedTotal, breakdown.TotalCostMicroCents)
	})

	t.Run("zero values produce zero costs", func(t *testing.T) {
		breakdown := CalculateWebSocketCosts("connect", 0, 0, 0, 0)
		assert.Equal(t, int64(0), breakdown.APIGatewayConnectionCost)
		assert.Equal(t, int64(0), breakdown.APIGatewayMessageCost)
		assert.Equal(t, int64(0), breakdown.LambdaExecutionCost)
		assert.Equal(t, int64(0), breakdown.DataTransferCost)
		assert.Equal(t, int64(0), breakdown.TotalCostMicroCents)
	})

	t.Run("operation type captured", func(t *testing.T) {
		breakdown := CalculateWebSocketCosts("subscribe", 0, 0, 0, 0)
		assert.Equal(t, "subscribe", breakdown.OperationType)
	})
}

// TestWebSocketCostRecord_Helpers tests helper methods
func TestWebSocketCostRecord_Helpers(t *testing.T) {
	t.Run("AddTag", func(t *testing.T) {
		w := &WebSocketCostRecord{}
		w.AddTag("env", "prod")
		assert.Equal(t, "prod", w.Tags["env"])
	})

	t.Run("SetProperty and GetProperty", func(t *testing.T) {
		w := &WebSocketCostRecord{}
		w.SetProperty("custom", "value")
		val, ok := w.GetProperty("custom")
		assert.True(t, ok)
		assert.Equal(t, "value", val)
	})

	t.Run("GetProperty not found", func(t *testing.T) {
		w := &WebSocketCostRecord{}
		val, ok := w.GetProperty("nonexistent")
		assert.False(t, ok)
		assert.Nil(t, val)
	})
}

// TestWebSocketCostBudget_BeforeCreate tests lifecycle hooks
func TestWebSocketCostBudget_BeforeCreate(t *testing.T) {
	now := time.Now()
	earlier := now.Add(-time.Hour)

	t.Run("calculates remaining budget", func(t *testing.T) {
		w := &WebSocketCostBudget{
			UserID:           "user-123",
			Period:           "daily",
			BudgetMicroCents: 1000000,
			UsedMicroCents:   300000,
			WindowStart:      earlier,
			WindowEnd:        now,
		}
		err := w.BeforeCreate()
		require.NoError(t, err)
		assert.Equal(t, int64(700000), w.RemainingMicroCents)
	})

	t.Run("calculates usage percentage", func(t *testing.T) {
		w := &WebSocketCostBudget{
			UserID:           "user-123",
			Period:           "daily",
			BudgetMicroCents: 1000000,
			UsedMicroCents:   500000,
			WindowStart:      earlier,
			WindowEnd:        now,
		}
		err := w.BeforeCreate()
		require.NoError(t, err)
		assert.Equal(t, 50.0, w.UsagePercent)
	})

	t.Run("negative remaining clamped to zero", func(t *testing.T) {
		w := &WebSocketCostBudget{
			UserID:           "user-123",
			Period:           "daily",
			BudgetMicroCents: 1000000,
			UsedMicroCents:   1500000,
			WindowStart:      earlier,
			WindowEnd:        now,
		}
		err := w.BeforeCreate()
		require.NoError(t, err)
		assert.Equal(t, int64(0), w.RemainingMicroCents)
	})

	t.Run("sets keys", func(t *testing.T) {
		w := &WebSocketCostBudget{
			UserID:           "user-123",
			Period:           "daily",
			BudgetMicroCents: 1000000,
			WindowStart:      earlier,
			WindowEnd:        now,
		}
		err := w.BeforeCreate()
		require.NoError(t, err)
		assert.Equal(t, "WS_BUDGET#user-123#daily", w.PK)
		assert.Equal(t, "BUDGET#daily", w.SK)
	})

	t.Run("sets TTL based on period", func(t *testing.T) {
		testCases := []struct {
			period      string
			expectedTTL int // days (approximate)
		}{
			{"daily", 2},
			{"weekly", 14},
			{"monthly", 62},
		}

		for _, tc := range testCases {
			t.Run(tc.period, func(t *testing.T) {
				w := &WebSocketCostBudget{
					UserID:           "user-123",
					Period:           tc.period,
					BudgetMicroCents: 1000000,
					WindowStart:      earlier,
					WindowEnd:        now,
				}
				before := time.Now()
				err := w.BeforeCreate()
				require.NoError(t, err)

				expectedExpiry := before.Add(time.Duration(tc.expectedTTL) * 24 * time.Hour)
				actualExpiry := time.Unix(w.ExpiresAt, 0)
				assert.WithinDuration(t, expectedExpiry, actualExpiry, 2*time.Second)
			})
		}
	})
}

// TestWebSocketCostRecordBuilder tests the builder pattern
func TestWebSocketCostRecordBuilder(t *testing.T) {
	t.Run("builds complete record", func(t *testing.T) {
		breakdown := CalculateWebSocketCosts("message_out", 10, 5, 0.5, 100)

		record := NewWebSocketCostRecordBuilder().
			ForOperation("message_out").
			WithConnection("conn-123", "user-456", "alice").
			WithDuration(5000).
			WithMessages(5, 1024).
			WithCosts(breakdown).
			Build()

		assert.Equal(t, "message_out", record.OperationType)
		assert.Equal(t, "conn-123", record.ConnectionID)
		assert.Equal(t, "user-456", record.UserID)
		assert.Equal(t, "alice", record.Username)
		assert.Equal(t, int64(5000), record.ConnectionDurationMs)
		assert.Equal(t, 5, record.MessageCount)
		assert.Equal(t, int64(1024), record.MessageSizeBytes)
		assert.Equal(t, breakdown.TotalCostMicroCents, record.TotalCostMicroCents)
	})
}
