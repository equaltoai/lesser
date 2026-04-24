package circuit

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/federation/types"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

func TestNewServerlessCircuitBreaker_DefaultConfig(t *testing.T) {
	repo := NewMockCircuitBreakerRepository()
	cb := NewServerlessCircuitBreaker(repo, nil, zaptest.NewLogger(t))
	require.NotNil(t, cb)
	require.NotNil(t, cb.config)
	assert.Equal(t, models.DefaultCircuitBreakerConfig().FailureThreshold, cb.config.FailureThreshold)
}

func TestServerlessCircuitBreaker_FailOpenOnRepoErrors(t *testing.T) {
	repo := NewMockCircuitBreakerRepository()
	repo.getErr = errors.New("boom")

	cb := NewServerlessCircuitBreaker(repo, models.DefaultCircuitBreakerConfig(), zaptest.NewLogger(t))
	ctx := context.Background()
	instanceID := "err.instance"

	assert.False(t, cb.IsOpen(ctx, instanceID))
	assert.True(t, cb.CanAttempt(ctx, instanceID))
	assert.Equal(t, types.CircuitClosed, cb.GetStatus(ctx, instanceID))

	metrics := cb.GetMetrics(ctx, instanceID)
	assert.Equal(t, "boom", metrics["error"])
}

func TestServerlessCircuitBreaker_IsOpen_EvaluatesStates(t *testing.T) {
	ctx := context.Background()
	logger := zaptest.NewLogger(t)

	t.Run("open_state_future_retry_is_open", func(t *testing.T) {
		repo := NewMockCircuitBreakerRepository()
		cb := NewServerlessCircuitBreaker(repo, models.DefaultCircuitBreakerConfig(), logger)
		instanceID := "open.future"

		require.NoError(t, repo.SaveCircuitState(ctx, &models.CircuitBreakerState{
			InstanceID:      instanceID,
			Status:          stateOpen,
			NextRetry:       time.Now().Add(time.Minute),
			LastStateChange: time.Now(),
		}))

		assert.True(t, cb.IsOpen(ctx, instanceID))
	})

	t.Run("open_state_past_retry_is_attemptable", func(t *testing.T) {
		repo := NewMockCircuitBreakerRepository()
		cb := NewServerlessCircuitBreaker(repo, models.DefaultCircuitBreakerConfig(), logger)
		instanceID := "open.past"

		require.NoError(t, repo.SaveCircuitState(ctx, &models.CircuitBreakerState{
			InstanceID:      instanceID,
			Status:          stateOpen,
			NextRetry:       time.Now().Add(-time.Second),
			LastStateChange: time.Now(),
		}))

		assert.False(t, cb.IsOpen(ctx, instanceID))
	})

	t.Run("half_open_future_retry_is_attemptable", func(t *testing.T) {
		repo := NewMockCircuitBreakerRepository()
		cb := NewServerlessCircuitBreaker(repo, models.DefaultCircuitBreakerConfig(), logger)
		instanceID := "half.future"

		require.NoError(t, repo.SaveCircuitState(ctx, &models.CircuitBreakerState{
			InstanceID:      instanceID,
			Status:          stateHalfOpen,
			NextRetry:       time.Now().Add(time.Minute),
			LastStateChange: time.Now(),
		}))

		assert.False(t, cb.IsOpen(ctx, instanceID))
	})

	t.Run("half_open_timeout_transitions_back_to_open_async", func(t *testing.T) {
		repo := NewMockCircuitBreakerRepository()
		cb := NewServerlessCircuitBreaker(repo, models.DefaultCircuitBreakerConfig(), logger)
		instanceID := "half.timeout"

		require.NoError(t, repo.SaveCircuitState(ctx, &models.CircuitBreakerState{
			InstanceID:      instanceID,
			Status:          stateHalfOpen,
			NextRetry:       time.Now().Add(-time.Second),
			LastStateChange: time.Now(),
		}))

		assert.True(t, cb.IsOpen(ctx, instanceID))
		repo.waitForCall(t, repo.updateCalls)

		updated, err := repo.GetCircuitState(ctx, instanceID)
		require.NoError(t, err)
		assert.Equal(t, stateOpen, updated.Status)
		assert.Equal(t, "half-open timeout", updated.Reason)
	})

	t.Run("half_open_timeout_update_error_is_logged_async", func(t *testing.T) {
		repo := NewMockCircuitBreakerRepository()
		repo.updateErr = errors.New("update failed")

		cb := NewServerlessCircuitBreaker(repo, models.DefaultCircuitBreakerConfig(), zap.NewNop())
		instanceID := "half.timeout.err"

		require.NoError(t, repo.SaveCircuitState(ctx, &models.CircuitBreakerState{
			InstanceID:      instanceID,
			Status:          stateHalfOpen,
			NextRetry:       time.Now().Add(-time.Second),
			LastStateChange: time.Now(),
		}))

		assert.True(t, cb.IsOpen(ctx, instanceID))
		repo.waitForCall(t, repo.updateCalls)
	})
}

func TestServerlessCircuitBreaker_CanAttempt_Branches(t *testing.T) {
	ctx := context.Background()
	logger := zaptest.NewLogger(t)
	config := models.DefaultCircuitBreakerConfig()

	t.Run("open_state_future_retry_denied", func(t *testing.T) {
		repo := NewMockCircuitBreakerRepository()
		cb := NewServerlessCircuitBreaker(repo, config, logger)
		instanceID := "canattempt.open.future"

		require.NoError(t, repo.SaveCircuitState(ctx, &models.CircuitBreakerState{
			InstanceID:      instanceID,
			Status:          stateOpen,
			NextRetry:       time.Now().Add(time.Minute),
			LastStateChange: time.Now(),
		}))

		assert.False(t, cb.CanAttempt(ctx, instanceID))
	})

	t.Run("open_state_past_retry_transitions_to_half_open", func(t *testing.T) {
		repo := NewMockCircuitBreakerRepository()
		cb := NewServerlessCircuitBreaker(repo, config, logger)
		instanceID := "canattempt.open.past"

		require.NoError(t, repo.SaveCircuitState(ctx, &models.CircuitBreakerState{
			InstanceID:      instanceID,
			Status:          stateOpen,
			NextRetry:       time.Now().Add(-time.Second),
			LastStateChange: time.Now(),
		}))

		assert.True(t, cb.CanAttempt(ctx, instanceID))
		updated, err := repo.GetCircuitState(ctx, instanceID)
		require.NoError(t, err)
		assert.Equal(t, stateHalfOpen, updated.Status)
		assert.Equal(t, "testing recovery", updated.Reason)
	})

	t.Run("open_state_transition_error_denies_attempt", func(t *testing.T) {
		repo := NewMockCircuitBreakerRepository()
		repo.updateErr = errors.New("boom")
		cb := NewServerlessCircuitBreaker(repo, config, logger)
		instanceID := "canattempt.open.err"

		require.NoError(t, repo.SaveCircuitState(ctx, &models.CircuitBreakerState{
			InstanceID:      instanceID,
			Status:          stateOpen,
			NextRetry:       time.Now().Add(-time.Second),
			LastStateChange: time.Now(),
		}))

		assert.False(t, cb.CanAttempt(ctx, instanceID))
	})

	t.Run("half_open_allows_attempts", func(t *testing.T) {
		repo := NewMockCircuitBreakerRepository()
		cb := NewServerlessCircuitBreaker(repo, config, logger)
		instanceID := "canattempt.half"

		require.NoError(t, repo.SaveCircuitState(ctx, &models.CircuitBreakerState{
			InstanceID:      instanceID,
			Status:          stateHalfOpen,
			NextRetry:       time.Now().Add(time.Minute),
			LastStateChange: time.Now(),
		}))

		assert.True(t, cb.CanAttempt(ctx, instanceID))
	})

	t.Run("unknown_status_fails_open", func(t *testing.T) {
		repo := NewMockCircuitBreakerRepository()
		cb := NewServerlessCircuitBreaker(repo, config, logger)
		instanceID := "canattempt.unknown"

		require.NoError(t, repo.SaveCircuitState(ctx, &models.CircuitBreakerState{
			InstanceID:      instanceID,
			Status:          "mystery",
			LastStateChange: time.Now(),
		}))

		assert.True(t, cb.CanAttempt(ctx, instanceID))
	})
}

func TestServerlessCircuitBreaker_TransitionToHalfOpen_InvalidTransition(t *testing.T) {
	ctx := context.Background()
	logger := zaptest.NewLogger(t)

	repo := NewMockCircuitBreakerRepository()
	cb := NewServerlessCircuitBreaker(repo, models.DefaultCircuitBreakerConfig(), logger)
	instanceID := "transition.invalid"

	require.NoError(t, repo.SaveCircuitState(ctx, &models.CircuitBreakerState{
		InstanceID:      instanceID,
		Status:          stateClosed,
		LastStateChange: time.Now(),
	}))

	err := cb.transitionToHalfOpen(ctx, &models.CircuitBreakerState{InstanceID: instanceID})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidStateTransition)
}

func TestServerlessCircuitBreaker_RecordSuccess_TransitionsAndAsyncMetrics(t *testing.T) {
	ctx := context.Background()
	logger := zaptest.NewLogger(t)
	config := models.DefaultCircuitBreakerConfig()

	t.Run("half_open_recovery_closes_circuit", func(t *testing.T) {
		repo := NewMockCircuitBreakerRepository()
		repo.recordStateErr = errors.New("state change failed")
		repo.recordMetricErr = errors.New("metric failed")

		cb := NewServerlessCircuitBreaker(repo, config, zap.NewNop())
		instanceID := "success.half.close"

		require.NoError(t, repo.SaveCircuitState(ctx, &models.CircuitBreakerState{
			InstanceID:       instanceID,
			Status:           stateHalfOpen,
			SuccessCount:     config.SuccessThreshold - 1,
			FailureCount:     10,
			ConsecutiveFails: 5,
			LastStateChange:  time.Now(),
		}))

		require.NoError(t, cb.RecordSuccess(ctx, instanceID))
		repo.waitForCall(t, repo.stateChangeCalls)
		repo.waitForCall(t, repo.metricCalls)

		updated, err := repo.GetCircuitState(ctx, instanceID)
		require.NoError(t, err)
		assert.Equal(t, stateClosed, updated.Status)
		assert.Equal(t, "recovery successful", updated.Reason)
		assert.Equal(t, 0, updated.FailureCount)
		assert.Equal(t, 0, updated.SuccessCount)
		assert.Equal(t, int64(0), updated.BackoffDuration)
	})

	t.Run("open_state_success_transitions_to_half_open", func(t *testing.T) {
		repo := NewMockCircuitBreakerRepository()
		cb := NewServerlessCircuitBreaker(repo, config, logger)
		instanceID := "success.open.half"

		require.NoError(t, repo.SaveCircuitState(ctx, &models.CircuitBreakerState{
			InstanceID:      instanceID,
			Status:          stateOpen,
			NextRetry:       time.Now().Add(time.Hour),
			LastStateChange: time.Now(),
		}))

		require.NoError(t, cb.RecordSuccess(ctx, instanceID))
		repo.waitForCall(t, repo.stateChangeCalls)
		repo.waitForCall(t, repo.metricCalls)

		updated, err := repo.GetCircuitState(ctx, instanceID)
		require.NoError(t, err)
		assert.Equal(t, stateHalfOpen, updated.Status)
		assert.Equal(t, 1, updated.SuccessCount)
		assert.Equal(t, "unexpected success during open state", updated.Reason)
	})

	t.Run("update_error_is_returned", func(t *testing.T) {
		repo := NewMockCircuitBreakerRepository()
		repo.updateErr = errors.New("boom")
		cb := NewServerlessCircuitBreaker(repo, config, logger)

		require.Error(t, cb.RecordSuccess(ctx, "success.update.err"))
	})
}

func TestServerlessCircuitBreaker_RecordFailure_TransitionsAndBackoff(t *testing.T) {
	ctx := context.Background()
	logger := zaptest.NewLogger(t)
	config := models.DefaultCircuitBreakerConfig()

	t.Run("closed_state_below_threshold_stays_closed", func(t *testing.T) {
		repo := NewMockCircuitBreakerRepository()
		cb := NewServerlessCircuitBreaker(repo, config, logger)
		instanceID := "failure.closed.single"

		require.NoError(t, repo.SaveCircuitState(ctx, &models.CircuitBreakerState{
			InstanceID:      instanceID,
			Status:          stateClosed,
			LastStateChange: time.Now(),
		}))

		require.NoError(t, cb.RecordFailure(ctx, instanceID, errors.New("timeout")))
		repo.waitForCall(t, repo.metricCalls)

		updated, err := repo.GetCircuitState(ctx, instanceID)
		require.NoError(t, err)
		assert.Equal(t, stateClosed, updated.Status)
		assert.Equal(t, 1, updated.ConsecutiveFails)
	})

	t.Run("closed_state_reaching_threshold_opens_circuit", func(t *testing.T) {
		repo := NewMockCircuitBreakerRepository()
		cb := NewServerlessCircuitBreaker(repo, config, logger)
		instanceID := "failure.closed.open"

		require.NoError(t, repo.SaveCircuitState(ctx, &models.CircuitBreakerState{
			InstanceID:       instanceID,
			Status:           stateClosed,
			ConsecutiveFails: config.FailureThreshold - 1,
			LastStateChange:  time.Now(),
			BackoffDuration:  0,
			SuccessCount:     0,
		}))

		require.NoError(t, cb.RecordFailure(ctx, instanceID, errors.New("timeout")))
		repo.waitForCall(t, repo.stateChangeCalls)
		repo.waitForCall(t, repo.metricCalls)

		updated, err := repo.GetCircuitState(ctx, instanceID)
		require.NoError(t, err)
		assert.Equal(t, stateOpen, updated.Status)
		assert.Contains(t, updated.Reason, "consecutive failures")
		assert.Equal(t, int64(config.OpenTimeout), updated.BackoffDuration)
		assert.True(t, updated.NextRetry.After(time.Now().Add(-time.Second)))
	})

	t.Run("half_open_failure_reopens_circuit", func(t *testing.T) {
		repo := NewMockCircuitBreakerRepository()
		cb := NewServerlessCircuitBreaker(repo, config, logger)
		instanceID := "failure.half.open"

		require.NoError(t, repo.SaveCircuitState(ctx, &models.CircuitBreakerState{
			InstanceID:      instanceID,
			Status:          stateHalfOpen,
			SuccessCount:    2,
			LastStateChange: time.Now(),
		}))

		require.NoError(t, cb.RecordFailure(ctx, instanceID, errors.New("500 server error")))
		repo.waitForCall(t, repo.stateChangeCalls)
		repo.waitForCall(t, repo.metricCalls)

		updated, err := repo.GetCircuitState(ctx, instanceID)
		require.NoError(t, err)
		assert.Equal(t, stateOpen, updated.Status)
		assert.Equal(t, 0, updated.SuccessCount)
		assert.Contains(t, updated.Reason, "half-open test failed")
	})

	t.Run("open_state_failure_extends_backoff_under_max", func(t *testing.T) {
		repo := NewMockCircuitBreakerRepository()
		cb := NewServerlessCircuitBreaker(repo, config, logger)
		instanceID := "failure.open.backoff"

		require.NoError(t, repo.SaveCircuitState(ctx, &models.CircuitBreakerState{
			InstanceID:      instanceID,
			Status:          stateOpen,
			BackoffDuration: int64(config.OpenTimeout),
			NextRetry:       time.Now().Add(time.Hour),
			LastStateChange: time.Now(),
		}))

		require.NoError(t, cb.RecordFailure(ctx, instanceID, errors.New("network glitch")))
		repo.waitForCall(t, repo.metricCalls)

		updated, err := repo.GetCircuitState(ctx, instanceID)
		require.NoError(t, err)
		assert.Equal(t, stateOpen, updated.Status)
		assert.Equal(t, int64(2*config.OpenTimeout), updated.BackoffDuration)
	})

	t.Run("open_state_failure_backoff_caps_at_max", func(t *testing.T) {
		repo := NewMockCircuitBreakerRepository()
		cb := NewServerlessCircuitBreaker(repo, config, logger)
		instanceID := "failure.open.max"

		require.NoError(t, repo.SaveCircuitState(ctx, &models.CircuitBreakerState{
			InstanceID:      instanceID,
			Status:          stateOpen,
			BackoffDuration: int64(4 * time.Minute),
			NextRetry:       time.Now().Add(time.Hour),
			LastStateChange: time.Now(),
		}))

		require.NoError(t, cb.RecordFailure(ctx, instanceID, errors.New("network glitch")))
		repo.waitForCall(t, repo.metricCalls)

		updated, err := repo.GetCircuitState(ctx, instanceID)
		require.NoError(t, err)
		assert.Equal(t, int64(config.MaxBackoff), updated.BackoffDuration)
	})

	t.Run("update_error_is_returned", func(t *testing.T) {
		repo := NewMockCircuitBreakerRepository()
		repo.updateErr = errors.New("boom")
		cb := NewServerlessCircuitBreaker(repo, config, logger)

		require.Error(t, cb.RecordFailure(ctx, "failure.update.err", errors.New("timeout")))
	})
}

func TestServerlessCircuitBreaker_GetStatusAndMetrics(t *testing.T) {
	ctx := context.Background()
	logger := zaptest.NewLogger(t)

	repo := NewMockCircuitBreakerRepository()
	cb := NewServerlessCircuitBreaker(repo, models.DefaultCircuitBreakerConfig(), logger)

	require.NoError(t, repo.SaveCircuitState(ctx, &models.CircuitBreakerState{
		InstanceID:      "status.open",
		Status:          stateOpen,
		NextRetry:       time.Now().Add(time.Minute),
		TotalRequests:   10,
		TotalSuccesses:  4,
		TotalFailures:   6,
		LastStateChange: time.Now(),
	}))

	require.NoError(t, repo.SaveCircuitState(ctx, &models.CircuitBreakerState{
		InstanceID:      "status.half",
		Status:          stateHalfOpen,
		NextRetry:       time.Now().Add(time.Minute),
		LastStateChange: time.Now(),
	}))

	require.NoError(t, repo.SaveCircuitState(ctx, &models.CircuitBreakerState{
		InstanceID:      "status.unknown",
		Status:          "wat",
		LastStateChange: time.Now(),
	}))

	assert.Equal(t, types.CircuitOpen, cb.GetStatus(ctx, "status.open"))
	assert.Equal(t, types.CircuitHalfOpen, cb.GetStatus(ctx, "status.half"))
	assert.Equal(t, types.CircuitClosed, cb.GetStatus(ctx, "status.unknown"))

	metrics := cb.GetMetrics(ctx, "status.open")
	assert.Equal(t, stateOpen, metrics["status"])
	assert.Equal(t, float64(0.4), metrics["successRate"])
}

func TestServerlessCircuitBreaker_ClassifyError(t *testing.T) {
	cb := NewServerlessCircuitBreaker(NewMockCircuitBreakerRepository(), models.DefaultCircuitBreakerConfig(), zaptest.NewLogger(t))

	tests := []struct {
		name string
		err  error
		want string
	}{
		{"nil", nil, "none"},
		{"timeout", errors.New("request timeout"), "timeout"},
		{"connection_refused", errors.New("connection refused"), "connection_refused"},
		{"dns_failure", errors.New("no such host"), "dns_failure"},
		{"server_error_500", errors.New("500"), "server_error"},
		{"server_error_503", errors.New("503 service unavailable"), "server_error"},
		{"rate_limit", errors.New("429 too many requests"), "rate_limit"},
		{"network_error", errors.New("network unreachable"), "network_error"},
		{"unknown", errors.New("something else"), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, cb.classifyError(tt.err))
		})
	}
}
