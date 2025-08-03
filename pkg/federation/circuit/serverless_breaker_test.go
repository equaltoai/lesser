package circuit

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"go.uber.org/zap"
)

// MockCircuitBreakerRepository for testing
type MockCircuitBreakerRepository struct {
	states map[string]*models.CircuitBreakerState
	events []*models.CircuitBreakerEvent
}

func NewMockCircuitBreakerRepository() *MockCircuitBreakerRepository {
	return &MockCircuitBreakerRepository{
		states: make(map[string]*models.CircuitBreakerState),
		events: make([]*models.CircuitBreakerEvent, 0),
	}
}

func (m *MockCircuitBreakerRepository) GetCircuitState(ctx context.Context, instanceID string) (*models.CircuitBreakerState, error) {
	if state, exists := m.states[instanceID]; exists {
		return state, nil
	}
	// Return a new closed circuit state if not found
	return &models.CircuitBreakerState{
		InstanceID:      instanceID,
		Status:          "closed",
		LastStateChange: time.Now(),
	}, nil
}

func (m *MockCircuitBreakerRepository) SaveCircuitState(ctx context.Context, state *models.CircuitBreakerState) error {
	state.UpdateKeys()
	m.states[state.InstanceID] = state
	return nil
}

func (m *MockCircuitBreakerRepository) UpdateCircuitState(ctx context.Context, instanceID string, updateFn func(*models.CircuitBreakerState) error) (*models.CircuitBreakerState, error) {
	state, err := m.GetCircuitState(ctx, instanceID)
	if err != nil {
		return nil, err
	}
	
	if err := updateFn(state); err != nil {
		return nil, err
	}
	
	if err := m.SaveCircuitState(ctx, state); err != nil {
		return nil, err
	}
	
	return state, nil
}

func (m *MockCircuitBreakerRepository) RecordEvent(ctx context.Context, event *models.CircuitBreakerEvent) error {
	event.UpdateKeys()
	m.events = append(m.events, event)
	return nil
}

func (m *MockCircuitBreakerRepository) RecordStateChange(ctx context.Context, instanceID, oldStatus, newStatus, reason string) error {
	event := &models.CircuitBreakerEvent{
		InstanceID: instanceID,
		EventType:  "state_change",
		OldStatus:  oldStatus,
		NewStatus:  newStatus,
		Reason:     reason,
		Timestamp:  time.Now(),
	}
	return m.RecordEvent(ctx, event)
}

func (m *MockCircuitBreakerRepository) RecordMetric(ctx context.Context, instanceID string, success bool, err error, errorType string) error {
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
	return m.RecordEvent(ctx, event)
}

func (m *MockCircuitBreakerRepository) GetRecentEvents(ctx context.Context, instanceID string, limit int) ([]*models.CircuitBreakerEvent, error) {
	var result []*models.CircuitBreakerEvent
	for _, event := range m.events {
		if event.InstanceID == instanceID {
			result = append(result, event)
			if len(result) >= limit {
				break
			}
		}
	}
	return result, nil
}

func (m *MockCircuitBreakerRepository) DeleteCircuitState(ctx context.Context, instanceID string) error {
	delete(m.states, instanceID)
	return nil
}

func (m *MockCircuitBreakerRepository) GetAllCircuitStates(ctx context.Context) ([]*models.CircuitBreakerState, error) {
	var result []*models.CircuitBreakerState
	for _, state := range m.states {
		result = append(result, state)
	}
	return result, nil
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
	
	// Test 5: Verify events were recorded (wait a bit for async operations)
	time.Sleep(10 * time.Millisecond)
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