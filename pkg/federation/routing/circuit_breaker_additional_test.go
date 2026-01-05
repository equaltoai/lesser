package routing

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	fedTypes "github.com/equaltoai/lesser/pkg/federation/types"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type fakeCircuitBreakerRepo struct {
	mu sync.Mutex

	states map[string]*models.CircuitBreakerState

	getErr          error
	saveErr         error
	updateErr       error
	recordStateErr  error
	recordMetricErr error

	saveCalls int

	stateChangeCh chan struct{}
	metricCh      chan struct{}
}

func newFakeCircuitBreakerRepo() *fakeCircuitBreakerRepo {
	return &fakeCircuitBreakerRepo{
		states:        make(map[string]*models.CircuitBreakerState),
		stateChangeCh: make(chan struct{}, 100),
		metricCh:      make(chan struct{}, 100),
	}
}

func cloneCircuitState(s *models.CircuitBreakerState) *models.CircuitBreakerState {
	if s == nil {
		return nil
	}
	c := *s
	return &c
}

func (r *fakeCircuitBreakerRepo) GetCircuitState(_ context.Context, instanceID string) (*models.CircuitBreakerState, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.getErr != nil {
		return nil, r.getErr
	}

	state := r.states[instanceID]
	if state == nil {
		state = &models.CircuitBreakerState{
			InstanceID:      instanceID,
			Status:          string(fedTypes.CircuitClosed),
			LastStateChange: time.Now(),
		}
		r.states[instanceID] = state
	}

	return cloneCircuitState(state), nil
}

func (r *fakeCircuitBreakerRepo) SaveCircuitState(_ context.Context, state *models.CircuitBreakerState) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.saveCalls++
	if r.saveErr != nil {
		return r.saveErr
	}
	r.states[state.InstanceID] = cloneCircuitState(state)
	return nil
}

func (r *fakeCircuitBreakerRepo) UpdateCircuitState(_ context.Context, instanceID string, updateFn func(*models.CircuitBreakerState) error) (*models.CircuitBreakerState, error) {
	r.mu.Lock()
	state := r.states[instanceID]
	if state == nil {
		state = &models.CircuitBreakerState{
			InstanceID:      instanceID,
			Status:          string(fedTypes.CircuitClosed),
			LastStateChange: time.Now(),
		}
		r.states[instanceID] = state
	}
	working := cloneCircuitState(state)
	r.mu.Unlock()

	if r.updateErr != nil {
		return nil, r.updateErr
	}
	if err := updateFn(working); err != nil {
		return nil, err
	}
	if err := r.SaveCircuitState(context.Background(), working); err != nil {
		return nil, err
	}
	return cloneCircuitState(working), nil
}

func (r *fakeCircuitBreakerRepo) RecordStateChange(_ context.Context, _ string, _ string, _ string, _ string) error {
	select {
	case r.stateChangeCh <- struct{}{}:
	default:
	}
	return r.recordStateErr
}

func (r *fakeCircuitBreakerRepo) RecordMetric(_ context.Context, _ string, _ bool, _ error, _ string) error {
	select {
	case r.metricCh <- struct{}{}:
	default:
	}
	return r.recordMetricErr
}

func waitForSignals(t *testing.T, ch <-chan struct{}, n int) {
	t.Helper()
	deadline := time.After(200 * time.Millisecond)
	for i := 0; i < n; i++ {
		select {
		case <-ch:
		case <-deadline:
			t.Fatalf("timed out waiting for %d signals (got %d)", n, i)
		}
	}
}

func TestDistributedCircuitBreaker_Open_Close_HalfOpen_CanAttempt(t *testing.T) {
	repo := newFakeCircuitBreakerRepo()
	logger := zap.NewNop()
	cfg := &models.CircuitBreakerConfig{
		FailureThreshold:  2,
		SuccessThreshold:  2,
		OpenTimeout:       25 * time.Millisecond,
		HalfOpenTimeout:   10 * time.Millisecond,
		BackoffMultiplier: 2.0,
		MaxBackoff:        50 * time.Millisecond,
	}

	dcb := newDistributedCircuitBreaker(repo, nil, logger, cfg)

	// Open sets initial backoff to OpenTimeout.
	assert.NoError(t, dcb.Open("a", "test open"))
	state := repo.states["a"]
	require.NotNil(t, state)
	assert.Equal(t, string(fedTypes.CircuitOpen), state.Status)
	assert.Equal(t, cfg.OpenTimeout, state.GetBackoffDuration())
	assert.Greater(t, state.NextRetry.UnixNano(), int64(0))
	assert.GreaterOrEqual(t, len(repo.stateChangeCh), 1)

	// Close is a no-op when already closed.
	repo.states["closed"] = &models.CircuitBreakerState{InstanceID: "closed", Status: string(fedTypes.CircuitClosed)}
	saveCalls := repo.saveCalls
	assert.NoError(t, dcb.Close("closed"))
	assert.Equal(t, saveCalls, repo.saveCalls)

	// HalfOpen invalid transition.
	assert.ErrorIs(t, dcb.HalfOpen("closed"), ErrInvalidCircuitTransition)

	// CanAttempt when open and not yet ready.
	repo.states["b"] = &models.CircuitBreakerState{
		InstanceID: "b",
		Status:     string(fedTypes.CircuitOpen),
		NextRetry:  time.Now().Add(10 * time.Second),
	}
	assert.False(t, dcb.CanAttempt("b"))

	// CanAttempt transitions to half-open when ready.
	repo.states["c"] = &models.CircuitBreakerState{
		InstanceID: "c",
		Status:     string(fedTypes.CircuitOpen),
		NextRetry:  time.Now().Add(-1 * time.Second),
	}
	assert.True(t, dcb.CanAttempt("c"))
	assert.Equal(t, string(fedTypes.CircuitHalfOpen), repo.states["c"].Status)

	// CanAttempt returns false if HalfOpen fails.
	repo.states["d"] = &models.CircuitBreakerState{
		InstanceID: "d",
		Status:     string(fedTypes.CircuitOpen),
		NextRetry:  time.Now().Add(-1 * time.Second),
	}
	repo.saveErr = errors.New("save failed")
	assert.False(t, dcb.CanAttempt("d"))
	repo.saveErr = nil
}

func TestDistributedCircuitBreaker_RecordFailureAndSuccess_TransitionsAndMetrics(t *testing.T) {
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

	// Two failures from closed should open the circuit.
	assert.NoError(t, dcb.RecordFailure("x", errors.New("timeout exceeded")))
	assert.NoError(t, dcb.RecordFailure("x", errors.New("timeout exceeded")))
	assert.Equal(t, string(fedTypes.CircuitOpen), repo.states["x"].Status)

	// RecordFailure triggers metric recording asynchronously.
	waitForSignals(t, repo.metricCh, 2)
	// Second failure triggers a state change event (async) when circuit opens.
	waitForSignals(t, repo.stateChangeCh, 1)

	// Successes in half-open close the circuit at threshold.
	repo.states["y"] = &models.CircuitBreakerState{
		InstanceID:      "y",
		Status:          string(fedTypes.CircuitHalfOpen),
		SuccessCount:    1,
		FailureCount:    2,
		BackoffDuration: int64(cfg.OpenTimeout),
	}
	assert.NoError(t, dcb.RecordSuccess("y"))
	waitForSignals(t, repo.metricCh, 1)
	waitForSignals(t, repo.stateChangeCh, 1)
	assert.Equal(t, string(fedTypes.CircuitClosed), repo.states["y"].Status)
	assert.Equal(t, 0, repo.states["y"].SuccessCount)
	assert.Equal(t, 0, repo.states["y"].FailureCount)

	// Unexpected success while open transitions to half-open.
	repo.states["z"] = &models.CircuitBreakerState{
		InstanceID: "z",
		Status:     string(fedTypes.CircuitOpen),
	}
	assert.NoError(t, dcb.RecordSuccess("z"))
	waitForSignals(t, repo.metricCh, 1)
	waitForSignals(t, repo.stateChangeCh, 1)
	assert.Equal(t, string(fedTypes.CircuitHalfOpen), repo.states["z"].Status)
}

func TestDistributedCircuitBreaker_GetMetricsAndClassifyError(t *testing.T) {
	repo := newFakeCircuitBreakerRepo()
	logger := zap.NewNop()
	dcb := newDistributedCircuitBreaker(repo, nil, logger, nil)

	repo.getErr = errors.New("boom")
	metrics := dcb.GetMetrics("missing")
	assert.Equal(t, string(fedTypes.CircuitClosed), metrics["status"])

	repo.getErr = nil
	repo.states["a"] = &models.CircuitBreakerState{
		InstanceID:     "a",
		Status:         string(fedTypes.CircuitClosed),
		TotalRequests:  10,
		TotalSuccesses: 7,
		TotalFailures:  3,
	}
	metrics = dcb.GetMetrics("a")
	assert.Equal(t, string(fedTypes.CircuitClosed), metrics["status"])
	assert.InDelta(t, 0.7, metrics["successRate"].(float64), 0.0001)

	assert.Equal(t, "none", dcb.classifyError(nil))
	assert.Equal(t, "timeout", dcb.classifyError(errors.New("request timeout")))
	assert.Equal(t, "connection_refused", dcb.classifyError(errors.New("dial tcp: connection refused")))
	assert.Equal(t, "dns_failure", dcb.classifyError(errors.New("lookup: no such host")))
	assert.Equal(t, "server_error", dcb.classifyError(errors.New("http 503")))
	assert.Equal(t, "rate_limit", dcb.classifyError(errors.New("http 429")))
	assert.Equal(t, "unknown", dcb.classifyError(errors.New("other")))
}

func TestDistributedCircuitBreaker_AssessRouteHealthAndEmergencyMode(t *testing.T) {
	repo := newFakeCircuitBreakerRepo()
	logger := zap.NewNop()
	thresholdManager := NewRouteThresholdManager(logger, DefaultThresholdConfig())
	dcb := newDistributedCircuitBreaker(repo, thresholdManager, logger, nil)

	// No threshold manager configured => no-op.
	noThreshold := newDistributedCircuitBreaker(repo, nil, logger, nil)
	assert.NoError(t, noThreshold.AssessRouteHealthAndAdjustCircuit(context.Background(), "r1", &fedTypes.RouteMetrics{}))
	assert.False(t, noThreshold.ShouldEnterEmergencyMode(0, 10))
	assert.NotNil(t, noThreshold.GetBackpressureRules())

	// Critical route should open circuit when not already open.
	crit := &fedTypes.RouteMetrics{
		TotalMessages:   10,
		SuccessfulCount: 0,
		FailedCount:     10,
		LastUpdated:     time.Now(),
		AvgLatency:      100 * time.Millisecond,
		P95Latency:      100 * time.Millisecond,
		P99Latency:      100 * time.Millisecond,
	}
	assert.NoError(t, dcb.AssessRouteHealthAndAdjustCircuit(context.Background(), "r1", crit))
	assert.Equal(t, string(fedTypes.CircuitOpen), repo.states["r1"].Status)

	// Healthy route should close an open circuit.
	repo.states["r2"] = &models.CircuitBreakerState{InstanceID: "r2", Status: string(fedTypes.CircuitOpen)}
	healthy := &fedTypes.RouteMetrics{
		TotalMessages:   10,
		SuccessfulCount: 10,
		FailedCount:     0,
		LastUpdated:     time.Now(),
		AvgLatency:      100 * time.Millisecond,
		P95Latency:      100 * time.Millisecond,
		P99Latency:      100 * time.Millisecond,
	}
	assert.NoError(t, dcb.AssessRouteHealthAndAdjustCircuit(context.Background(), "r2", healthy))
	assert.Equal(t, string(fedTypes.CircuitClosed), repo.states["r2"].Status)

	assert.True(t, dcb.ShouldEnterEmergencyMode(1, 10))
	assert.NotEmpty(t, dcb.GetBackpressureRules())
}
