package routing

import (
	"errors"
	"testing"
	"time"

	fedTypes "github.com/equaltoai/lesser/pkg/federation/types"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNewDistributedCircuitBreaker_DefaultConfig(t *testing.T) {
	t.Parallel()

	dcb := NewDistributedCircuitBreaker((*repositories.CircuitBreakerRepository)(nil), nil, zap.NewNop(), nil)
	require.NotNil(t, dcb)
	require.NotNil(t, dcb.config)
}

func TestDistributedCircuitBreaker_IsOpen_GetStatus_AndBackoffCapping(t *testing.T) {
	t.Parallel()

	repo := newFakeCircuitBreakerRepo()
	logger := zap.NewNop()
	cfg := &models.CircuitBreakerConfig{
		FailureThreshold:  2,
		SuccessThreshold:  2,
		OpenTimeout:       10 * time.Millisecond,
		HalfOpenTimeout:   10 * time.Millisecond,
		BackoffMultiplier: 2.0,
		MaxBackoff:        15 * time.Millisecond,
	}

	dcb := newDistributedCircuitBreaker(repo, nil, logger, cfg)

	// IsOpen defaults to false on repo error.
	repo.getErr = errors.New("boom")
	assert.False(t, dcb.IsOpen("missing"))
	assert.Equal(t, fedTypes.CircuitClosed, dcb.GetStatus("missing"))
	repo.getErr = nil

	// IsOpen true when circuit is open.
	repo.states["open"] = &models.CircuitBreakerState{InstanceID: "open", Status: string(fedTypes.CircuitOpen)}
	assert.True(t, dcb.IsOpen("open"))

	// Backoff duration caps at MaxBackoff on repeated opens.
	assert.NoError(t, dcb.Open("cap", "first"))
	assert.Equal(t, cfg.OpenTimeout, repo.states["cap"].GetBackoffDuration())

	assert.NoError(t, dcb.Open("cap", "second"))
	assert.Equal(t, cfg.MaxBackoff, repo.states["cap"].GetBackoffDuration())
}

func TestDistributedCircuitBreaker_RecordFailure_FromHalfOpen_Reopens(t *testing.T) {
	t.Parallel()

	repo := newFakeCircuitBreakerRepo()
	logger := zap.NewNop()
	cfg := &models.CircuitBreakerConfig{
		FailureThreshold:  2,
		SuccessThreshold:  2,
		OpenTimeout:       10 * time.Millisecond,
		HalfOpenTimeout:   10 * time.Millisecond,
		BackoffMultiplier: 2.0,
		MaxBackoff:        50 * time.Millisecond,
	}
	dcb := newDistributedCircuitBreaker(repo, nil, logger, cfg)

	repo.states["half"] = &models.CircuitBreakerState{
		InstanceID:      "half",
		Status:          string(fedTypes.CircuitHalfOpen),
		SuccessCount:    1,
		BackoffDuration: int64(10 * time.Millisecond),
	}

	assert.NoError(t, dcb.RecordFailure("half", errors.New("connection refused")))
	waitForSignals(t, repo.metricCh, 1)
	waitForSignals(t, repo.stateChangeCh, 1)

	state := repo.states["half"]
	require.NotNil(t, state)
	assert.Equal(t, string(fedTypes.CircuitOpen), state.Status)
	assert.Equal(t, 0, state.SuccessCount)
	assert.Greater(t, state.NextRetry.UnixNano(), int64(0))
}
