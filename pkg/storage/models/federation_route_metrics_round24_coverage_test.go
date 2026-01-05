package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFederationRouteMetrics_AddRetryAttempt_Round24(t *testing.T) {
	frm := &FederationRouteMetrics{}

	frm.AddRetryAttempt(100)
	require.Equal(t, int64(1), frm.TotalRetries)
	require.Equal(t, int64(100), frm.AvgRetryDelayMs)

	frm.AddRetryAttempt(300)
	require.Equal(t, int64(2), frm.TotalRetries)
	require.Equal(t, int64(200), frm.AvgRetryDelayMs)
}

func TestFederationRouteMetrics_updateCircuitBreakerState_Round24(t *testing.T) {
	frm := &FederationRouteMetrics{
		CircuitBreakerState: "",
		ConsecutiveFailures: 5,
	}

	frm.updateCircuitBreakerState()
	require.Equal(t, CircuitBreakerStateOpen, frm.CircuitBreakerState)
	require.NotNil(t, frm.StateChangeTime)
	require.NotNil(t, frm.NextRetryTime)
	require.True(t, frm.NextRetryTime.After(*frm.StateChangeTime))
	require.WithinDuration(t, frm.StateChangeTime.Add(1*time.Minute), *frm.NextRetryTime, 2*time.Second)

	past := time.Now().Add(-time.Second)
	frm.NextRetryTime = &past
	frm.updateCircuitBreakerState()
	require.Equal(t, "half_open", frm.CircuitBreakerState)
	require.NotNil(t, frm.StateChangeTime)

	frm.CircuitBreakerState = "half_open"
	frm.updateCircuitBreakerState()
	require.Equal(t, "half_open", frm.CircuitBreakerState)
}

func TestFederationRouteMetrics_AddDeliveryAttempt_Round24(t *testing.T) {
	now := time.Now()

	frm := &FederationRouteMetrics{
		CircuitBreakerState: CircuitBreakerStateClosed,
		ConsecutiveFailures: 4,
		ErrorBreakdown:      map[string]int64{},
	}

	frm.AddDeliveryAttempt(false, 100, "500", "boom", 10, 1000)
	require.Equal(t, int64(1), frm.TotalAttempts)
	require.Equal(t, int64(1), frm.FailedAttempts)
	require.Equal(t, int64(5), frm.ConsecutiveFailures)
	require.Equal(t, CircuitBreakerStateOpen, frm.CircuitBreakerState)
	require.NotNil(t, frm.NextRetryTime)
	require.Equal(t, int64(10), frm.TotalCostMicroCents)
	require.Equal(t, int64(1000), frm.DataTransferBytes)
	require.Equal(t, int64(1000), frm.AvgPayloadSize)
	require.Equal(t, int64(1), frm.ErrorBreakdown["500"])
	require.Equal(t, "500", frm.LastErrorCode)
	require.Equal(t, "boom", frm.LastErrorMessage)
	require.NotNil(t, frm.LastErrorTime)
	require.True(t, frm.LastUsed.After(now.Add(-1*time.Second)))

	// Second attempt exercises the EMA branch in latency stats.
	frm.AddDeliveryAttempt(false, 200, "", "", 0, 0)
	require.Equal(t, int64(2), frm.TotalAttempts)
	require.Equal(t, int64(2), frm.FailedAttempts)
	require.GreaterOrEqual(t, frm.MaxLatencyMs, frm.MinLatencyMs)

	// Success while open closes the circuit and records recovery.
	frm.CircuitBreakerState = CircuitBreakerStateOpen
	frm.AddDeliveryAttempt(true, 50, "", "", 0, 0)
	require.Equal(t, int64(3), frm.TotalAttempts)
	require.Equal(t, int64(1), frm.SuccessfulAttempts)
	require.Equal(t, int64(0), frm.ConsecutiveFailures)
	require.Equal(t, CircuitBreakerStateClosed, frm.CircuitBreakerState)
	require.NotNil(t, frm.RecoveryTime)
	require.NotNil(t, frm.StateChangeTime)
}
