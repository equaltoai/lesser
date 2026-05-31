package circuit

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// MockCircuitBreakerRepository for testing
type MockCircuitBreakerRepository struct {
	mu sync.Mutex

	states map[string]*models.CircuitBreakerState
	events []*models.CircuitBreakerEvent

	getErr          error
	updateErr       error
	recordStateErr  error
	recordMetricErr error

	updateCalls      chan struct{}
	stateChangeCalls chan struct{}
	metricCalls      chan struct{}
}

func NewMockCircuitBreakerRepository() *MockCircuitBreakerRepository {
	return &MockCircuitBreakerRepository{
		states:           make(map[string]*models.CircuitBreakerState),
		events:           make([]*models.CircuitBreakerEvent, 0),
		updateCalls:      make(chan struct{}, 128),
		stateChangeCalls: make(chan struct{}, 128),
		metricCalls:      make(chan struct{}, 128),
	}
}

func (m *MockCircuitBreakerRepository) GetCircuitState(_ context.Context, instanceID string) (*models.CircuitBreakerState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.getErr != nil {
		return nil, m.getErr
	}

	if state, exists := m.states[instanceID]; exists {
		stateCopy := *state
		return &stateCopy, nil
	}
	// Return a new closed circuit state if not found
	return &models.CircuitBreakerState{
		InstanceID:      instanceID,
		Status:          stateClosed,
		LastStateChange: time.Now(),
	}, nil
}

func (m *MockCircuitBreakerRepository) SaveCircuitState(_ context.Context, state *models.CircuitBreakerState) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	_ = state.UpdateKeys()
	m.states[state.InstanceID] = state
	return nil
}

func (m *MockCircuitBreakerRepository) UpdateCircuitState(ctx context.Context, instanceID string, updateFn func(*models.CircuitBreakerState) error) (*models.CircuitBreakerState, error) {
	select {
	case m.updateCalls <- struct{}{}:
	default:
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.updateErr != nil {
		return nil, m.updateErr
	}

	state, exists := m.states[instanceID]
	if !exists {
		state = &models.CircuitBreakerState{
			InstanceID:      instanceID,
			Status:          stateClosed,
			LastStateChange: time.Now(),
		}
		m.states[instanceID] = state
	}

	if err := updateFn(state); err != nil {
		return nil, err
	}

	_ = state.UpdateKeys()

	stateCopy := *state
	return &stateCopy, nil
}

func (m *MockCircuitBreakerRepository) RecordEvent(_ context.Context, event *models.CircuitBreakerEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	_ = event.UpdateKeys()
	m.events = append(m.events, event)
	return nil
}

func (m *MockCircuitBreakerRepository) RecordStateChange(ctx context.Context, instanceID, oldStatus, newStatus, reason string) error {
	var recordErr error
	if m.recordStateErr != nil {
		recordErr = m.recordStateErr
	} else {
		event := &models.CircuitBreakerEvent{
			InstanceID: instanceID,
			EventType:  "state_change",
			OldStatus:  oldStatus,
			NewStatus:  newStatus,
			Reason:     reason,
			Timestamp:  time.Now(),
		}
		recordErr = m.RecordEvent(ctx, event)
	}

	select {
	case m.stateChangeCalls <- struct{}{}:
	default:
	}

	return recordErr
}

func (m *MockCircuitBreakerRepository) RecordMetric(ctx context.Context, instanceID string, success bool, err error, errorType string) error {
	var recordErr error
	if m.recordMetricErr != nil {
		recordErr = m.recordMetricErr
	} else {
		event := &models.CircuitBreakerEvent{
			InstanceID: instanceID,
			EventType:  "metric",
			Success:    &success,
			Timestamp:  time.Now(),
		}
		if err != nil {
			event.Error = err.Error()
			event.ErrorType = errorType
		}
		recordErr = m.RecordEvent(ctx, event)
	}

	select {
	case m.metricCalls <- struct{}{}:
	default:
	}

	return recordErr
}

func (m *MockCircuitBreakerRepository) GetRecentEvents(_ context.Context, instanceID string, limit int) ([]*models.CircuitBreakerEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var result []*models.CircuitBreakerEvent
	for _, event := range m.events {
		if event.InstanceID == instanceID {
			eventCopy := *event
			result = append(result, &eventCopy)
			if len(result) >= limit {
				break
			}
		}
	}
	return result, nil
}

func (m *MockCircuitBreakerRepository) DeleteCircuitState(_ context.Context, instanceID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.states, instanceID)
	return nil
}

func (m *MockCircuitBreakerRepository) GetAllCircuitStates(_ context.Context) ([]*models.CircuitBreakerState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	result := make([]*models.CircuitBreakerState, 0, len(m.states))
	for _, state := range m.states {
		stateCopy := *state
		result = append(result, &stateCopy)
	}
	return result, nil
}

func (m *MockCircuitBreakerRepository) waitForCall(t *testing.T, ch <-chan struct{}) {
	t.Helper()

	require.Eventually(t, func() bool {
		select {
		case <-ch:
			return true
		default:
			return false
		}
	}, 5*time.Second, 10*time.Millisecond, "timed out waiting for async call")
}

func (m *MockCircuitBreakerRepository) waitForCalls(t *testing.T, ch <-chan struct{}, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		m.waitForCall(t, ch)
	}
}

func TestServerlessCircuitBreaker_BasicFlow(t *testing.T) {
	// Setup
	mockRepo := NewMockCircuitBreakerRepository()
	logger := zap.NewNop()
	config := models.DefaultCircuitBreakerConfig()

	cb := NewServerlessCircuitBreaker(mockRepo, config, logger)
	ctx := context.Background()
	instanceID := "test.instance.com"

	// Test 1: Circuit should start closed
	if cb.IsOpen(ctx, instanceID) {
		t.Error("Circuit should start closed")
	}

	if !cb.CanAttempt(ctx, instanceID) {
		t.Error("Should be able to attempt when circuit is closed")
	}

	// Test 2: Record some successes
	for i := 0; i < 3; i++ {
		if err := cb.RecordSuccess(ctx, instanceID); err != nil {
			t.Errorf("Failed to record success: %v", err)
		}
	}

	// Circuit should still be closed
	if cb.IsOpen(ctx, instanceID) {
		t.Error("Circuit should still be closed after successes")
	}

	// Test 3: Record failures to open circuit
	testError := errors.New("test connection timeout")
	for i := 0; i < config.FailureThreshold; i++ {
		if err := cb.RecordFailure(ctx, instanceID, testError); err != nil {
			t.Errorf("Failed to record failure: %v", err)
		}
	}

	// Circuit should now be open
	if !cb.IsOpen(ctx, instanceID) {
		t.Error("Circuit should be open after failures")
	}

	// Test 4: Check metrics
	metrics := cb.GetMetrics(ctx, instanceID)
	if metrics["status"] != "open" {
		t.Errorf("Expected status 'open', got %v", metrics["status"])
	}

	if metrics["totalFailures"].(int64) != int64(config.FailureThreshold) {
		t.Errorf("Expected %d failures, got %v", config.FailureThreshold, metrics["totalFailures"])
	}

	if metrics["totalSuccesses"].(int64) != 3 {
		t.Errorf("Expected 3 successes, got %v", metrics["totalSuccesses"])
	}

	// Test 5: Verify events were recorded (wait for async operations)
	mockRepo.waitForCall(t, mockRepo.metricCalls)
	mockRepo.waitForCall(t, mockRepo.stateChangeCalls)
	events, err := mockRepo.GetRecentEvents(ctx, instanceID, 10)
	if err != nil {
		t.Errorf("Failed to get events: %v", err)
	}

	if len(events) == 0 {
		t.Error("Expected events to be recorded")
	}

	// Should have state change events and metric events
	hasStateChange := false
	hasMetric := false
	for _, event := range events {
		if event.EventType == "state_change" {
			hasStateChange = true
		}
		if event.EventType == "metric" {
			hasMetric = true
		}
	}

	if !hasStateChange {
		t.Error("Expected state change events to be recorded")
	}

	if !hasMetric {
		t.Error("Expected metric events to be recorded")
	}
}

func TestCircuitBreakerConfig_Defaults(t *testing.T) {
	config := models.DefaultCircuitBreakerConfig()

	if config.FailureThreshold != 5 {
		t.Errorf("Expected default failure threshold 5, got %d", config.FailureThreshold)
	}

	if config.SuccessThreshold != 3 {
		t.Errorf("Expected default success threshold 3, got %d", config.SuccessThreshold)
	}

	if config.OpenTimeout != 30*time.Second {
		t.Errorf("Expected default open timeout 30s, got %v", config.OpenTimeout)
	}
}

func TestCircuitBreakerState_KeyGeneration(t *testing.T) {
	state := &models.CircuitBreakerState{
		InstanceID: "test.example.com",
	}

	state.UpdateKeys()

	expectedPK := "CIRCUIT#test.example.com"
	if state.PK != expectedPK {
		t.Errorf("Expected PK %s, got %s", expectedPK, state.PK)
	}

	if state.SK != "STATE" {
		t.Errorf("Expected SK 'STATE', got %s", state.SK)
	}

	if state.TTL == 0 {
		t.Error("Expected TTL to be set")
	}
}
