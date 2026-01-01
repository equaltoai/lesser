package cost

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCostCircuitBreaker(t *testing.T) {
	config := CostCircuitBreakerConfig{
		MaxCostPerHour:    1.0,
		MaxCostPerRequest: 0.01,
		WindowSize:        time.Hour,
		FailureThreshold:  5,
		RecoveryTimeout:   5 * time.Minute,
	}

	cb := NewCostCircuitBreaker(config)
	require.NotNil(t, cb)
	assert.Equal(t, StateClosed, cb.state)
	assert.NotNil(t, cb.costWindow)
}

func TestCheckCost_WithinLimits(t *testing.T) {
	config := CostCircuitBreakerConfig{
		MaxCostPerHour:    1.0,
		MaxCostPerRequest: 0.01,
		WindowSize:        time.Hour,
		FailureThreshold:  5,
		RecoveryTimeout:   5 * time.Minute,
	}

	cb := NewCostCircuitBreaker(config)
	ctx := context.Background()

	err := cb.CheckCost(ctx, 0.005)
	assert.NoError(t, err)
}

func TestCheckCost_ExceedsRequestLimit(t *testing.T) {
	config := CostCircuitBreakerConfig{
		MaxCostPerHour:    1.0,
		MaxCostPerRequest: 0.01,
		WindowSize:        time.Hour,
		FailureThreshold:  5,
		RecoveryTimeout:   5 * time.Minute,
	}

	cb := NewCostCircuitBreaker(config)
	ctx := context.Background()

	err := cb.CheckCost(ctx, 0.02) // Exceeds per-request limit
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds limit")
}

func TestCheckCost_ExceedsHourlyLimit(t *testing.T) {
	config := CostCircuitBreakerConfig{
		MaxCostPerHour:    0.001, // Very low limit
		MaxCostPerRequest: 0.01,
		WindowSize:        time.Hour,
		FailureThreshold:  5,
		RecoveryTimeout:   5 * time.Minute,
	}

	cb := NewCostCircuitBreaker(config)
	ctx := context.Background()

	// Add some cost to the window
	cb.RecordCost(0.0005)

	err := cb.CheckCost(ctx, 0.001) // Would exceed hourly limit
	assert.Error(t, err)
	assert.Equal(t, ErrHourlyCostLimitExceeded, err)
}

func TestCheckCost_CircuitOpen(t *testing.T) {
	config := CostCircuitBreakerConfig{
		MaxCostPerHour:    1.0,
		MaxCostPerRequest: 0.001, // Very low limit
		WindowSize:        time.Hour,
		FailureThreshold:  2,
		RecoveryTimeout:   1 * time.Hour, // Long recovery
	}

	cb := NewCostCircuitBreaker(config)
	ctx := context.Background()

	// Trigger failures to open circuit
	for i := 0; i < 3; i++ {
		_ = cb.CheckCost(ctx, 0.01) // Exceeds per-request limit
	}

	// Circuit should be open
	assert.Equal(t, StateOpen, cb.state)

	// Further requests should fail with circuit open error
	err := cb.CheckCost(ctx, 0.0001)
	assert.Equal(t, ErrCircuitBreakerOpen, err)
}

func TestCheckCost_CircuitRecovery(t *testing.T) {
	config := CostCircuitBreakerConfig{
		MaxCostPerHour:    1.0,
		MaxCostPerRequest: 0.001,
		WindowSize:        time.Hour,
		FailureThreshold:  2,
		RecoveryTimeout:   1 * time.Millisecond, // Very short recovery
	}

	cb := NewCostCircuitBreaker(config)
	ctx := context.Background()

	// Trigger failures to open circuit
	for i := 0; i < 3; i++ {
		_ = cb.CheckCost(ctx, 0.01)
	}

	assert.Equal(t, StateOpen, cb.state)

	// Wait for recovery timeout
	time.Sleep(5 * time.Millisecond)

	// Should transition to half-open
	err := cb.CheckCost(ctx, 0.0001)
	assert.NoError(t, err)
	assert.Equal(t, StateHalfOpen, cb.state)
}

func TestRecordCost(t *testing.T) {
	config := CostCircuitBreakerConfig{
		MaxCostPerHour:    1.0,
		MaxCostPerRequest: 0.01,
		WindowSize:        time.Hour,
		FailureThreshold:  5,
		RecoveryTimeout:   5 * time.Minute,
	}

	cb := NewCostCircuitBreaker(config)

	cb.RecordCost(0.005)
	cb.RecordCost(0.003)

	// Verify cost was recorded
	totalCost := cb.costWindow.getCurrentHourCost()
	assert.InDelta(t, 0.008, totalCost, 0.0001)
}

func TestRecordCost_ResetsFailuresInHalfOpen(t *testing.T) {
	config := CostCircuitBreakerConfig{
		MaxCostPerHour:    1.0,
		MaxCostPerRequest: 0.001,
		WindowSize:        time.Hour,
		FailureThreshold:  2,
		RecoveryTimeout:   1 * time.Millisecond,
	}

	cb := NewCostCircuitBreaker(config)
	ctx := context.Background()

	// Open the circuit
	for i := 0; i < 3; i++ {
		_ = cb.CheckCost(ctx, 0.01)
	}

	// Wait for recovery
	time.Sleep(5 * time.Millisecond)

	// Transition to half-open
	_ = cb.CheckCost(ctx, 0.0001)
	assert.Equal(t, StateHalfOpen, cb.state)

	// Record successful cost - should close circuit
	cb.RecordCost(0.0001)
	assert.Equal(t, StateClosed, cb.state)
	assert.Equal(t, 0, cb.failures)
}

func TestTransitionToHalfOpen(t *testing.T) {
	config := CostCircuitBreakerConfig{
		MaxCostPerHour:    1.0,
		MaxCostPerRequest: 0.01,
		WindowSize:        time.Hour,
		FailureThreshold:  5,
		RecoveryTimeout:   5 * time.Minute,
	}

	cb := NewCostCircuitBreaker(config)
	cb.state = StateOpen

	cb.transitionToHalfOpen()
	assert.Equal(t, StateHalfOpen, cb.state)
}

func TestTransitionToOpen(t *testing.T) {
	config := CostCircuitBreakerConfig{
		MaxCostPerHour:    1.0,
		MaxCostPerRequest: 0.01,
		WindowSize:        time.Hour,
		FailureThreshold:  5,
		RecoveryTimeout:   5 * time.Minute,
	}

	cb := NewCostCircuitBreaker(config)

	cb.transitionToOpen()
	assert.Equal(t, StateOpen, cb.state)
	assert.False(t, cb.lastFailTime.IsZero())
}

func TestCostWindow_GetCurrentHourCost(t *testing.T) {
	cw := &CostWindow{
		costs:     make([]CostSample, 0),
		startTime: time.Now(),
	}

	// Add samples
	cw.addSample(CostSample{cost: 0.01, timestamp: time.Now()})
	cw.addSample(CostSample{cost: 0.02, timestamp: time.Now()})
	cw.addSample(CostSample{cost: 0.03, timestamp: time.Now()})

	total := cw.getCurrentHourCost()
	assert.InDelta(t, 0.06, total, 0.0001)
}

func TestCostWindow_ExcludesOldSamples(t *testing.T) {
	cw := &CostWindow{
		costs:     make([]CostSample, 0),
		startTime: time.Now(),
	}

	// Add old sample (more than 1 hour ago)
	cw.costs = append(cw.costs, CostSample{
		cost:      0.05,
		timestamp: time.Now().Add(-2 * time.Hour),
	})

	// Add recent sample
	cw.addSample(CostSample{cost: 0.01, timestamp: time.Now()})

	total := cw.getCurrentHourCost()
	assert.InDelta(t, 0.01, total, 0.0001)
}

func TestCostWindow_AddSample_CleansOldSamples(t *testing.T) {
	cw := &CostWindow{
		costs:     make([]CostSample, 0),
		startTime: time.Now(),
	}

	// Add old sample directly
	cw.costs = append(cw.costs, CostSample{
		cost:      0.05,
		timestamp: time.Now().Add(-2 * time.Hour),
	})

	// Add new sample - should clean old ones
	cw.addSample(CostSample{cost: 0.01, timestamp: time.Now()})

	// Old sample should be removed
	assert.Len(t, cw.costs, 1)
	assert.InDelta(t, 0.01, cw.costs[0].cost, 0.0001)
}

func TestCheckCost_HalfOpenExceedsLimit(t *testing.T) {
	config := CostCircuitBreakerConfig{
		MaxCostPerHour:    0.001, // Very low limit
		MaxCostPerRequest: 0.01,
		WindowSize:        time.Hour,
		FailureThreshold:  2,
		RecoveryTimeout:   1 * time.Millisecond,
	}

	cb := NewCostCircuitBreaker(config)
	ctx := context.Background()

	// Open the circuit
	for i := 0; i < 3; i++ {
		_ = cb.CheckCost(ctx, 0.02)
	}

	// Wait for recovery
	time.Sleep(5 * time.Millisecond)

	// Add cost to window to exceed hourly limit
	cb.RecordCost(0.002)

	// Transition to half-open
	cb.state = StateHalfOpen

	// Check cost should reopen circuit
	err := cb.CheckCost(ctx, 0.0001)
	assert.Equal(t, ErrCircuitBreakerReopened, err)
	assert.Equal(t, StateOpen, cb.state)
}
