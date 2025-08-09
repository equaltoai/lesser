package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// CircuitBreakerRepository handles circuit breaker state persistence
type CircuitBreakerRepository struct {
	db        core.DB
	tableName string
	logger    *zap.Logger
}

// NewCircuitBreakerRepository creates a new circuit breaker repository
func NewCircuitBreakerRepository(db core.DB, tableName string, logger *zap.Logger) *CircuitBreakerRepository {
	return &CircuitBreakerRepository{
		db:        db,
		tableName: tableName,
		logger:    logger,
	}
}

// GetCircuitState retrieves the current state of a circuit breaker for an instance
func (r *CircuitBreakerRepository) GetCircuitState(ctx context.Context, instanceID string) (*models.CircuitBreakerState, error) {
	var state models.CircuitBreakerState
	state.InstanceID = instanceID
	state.UpdateKeys()

	err := r.db.WithContext(ctx).Model(&models.CircuitBreakerState{}).
		Where("PK", "=", state.PK).
		Where("SK", "=", state.SK).
		First(&state)

	if err != nil {
		if errors.IsNotFound(err) {
			// Return a new closed circuit state if not found
			return &models.CircuitBreakerState{
				InstanceID:      instanceID,
				Status:          "closed",
				LastStateChange: time.Now(),
			}, nil
		}
		return nil, fmt.Errorf("get circuit state: %w", err)
	}

	return &state, nil
}

// SaveCircuitState saves the circuit breaker state atomically
func (r *CircuitBreakerRepository) SaveCircuitState(ctx context.Context, state *models.CircuitBreakerState) error {
	// Ensure keys are updated
	state.UpdateKeys()

	// Use conditional update to prevent race conditions
	err := r.db.WithContext(ctx).Model(state).Create()

	if err != nil {
		// If item already exists, update it
		if errors.IsConditionFailed(err) {
			// Get the existing state first
			var existing models.CircuitBreakerState
			getErr := r.db.WithContext(ctx).Model(&models.CircuitBreakerState{}).
				Where("PK", "=", state.PK).
				Where("SK", "=", state.SK).
				First(&existing)
			if getErr != nil {
				return fmt.Errorf("get existing state for update: %w", getErr)
			}
			// Copy the new data over
			existing = *state
			err = r.db.WithContext(ctx).Model(&existing).Update()
		}
		if err != nil {
			return fmt.Errorf("save circuit state: %w", err)
		}
	}

	return nil
}

// UpdateCircuitState updates an existing circuit state atomically
func (r *CircuitBreakerRepository) UpdateCircuitState(ctx context.Context, instanceID string, updateFn func(*models.CircuitBreakerState) error) (*models.CircuitBreakerState, error) {
	// Get current state
	state, err := r.GetCircuitState(ctx, instanceID)
	if err != nil {
		return nil, err
	}

	// Apply the update function
	if err := updateFn(state); err != nil {
		return nil, fmt.Errorf("update function failed: %w", err)
	}

	// Save the updated state
	if err := r.SaveCircuitState(ctx, state); err != nil {
		return nil, err
	}

	return state, nil
}

// RecordEvent records a circuit breaker event for debugging and monitoring
func (r *CircuitBreakerRepository) RecordEvent(ctx context.Context, event *models.CircuitBreakerEvent) error {
	// Ensure keys are updated
	event.UpdateKeys()

	err := r.db.WithContext(ctx).Model(event).Create()

	if err != nil {
		r.logger.Warn("failed to record circuit breaker event",
			zap.String("instanceID", event.InstanceID),
			zap.String("eventType", event.EventType),
			zap.Error(err))
		// Don't fail the main operation if event recording fails
		return nil
	}

	return nil
}

// RecordStateChange is a convenience method to record state change events
func (r *CircuitBreakerRepository) RecordStateChange(ctx context.Context, instanceID, oldStatus, newStatus, reason string) error {
	event := &models.CircuitBreakerEvent{
		InstanceID: instanceID,
		EventType:  "state_change",
		OldStatus:  oldStatus,
		NewStatus:  newStatus,
		Reason:     reason,
		Timestamp:  time.Now(),
	}

	return r.RecordEvent(ctx, event)
}

// RecordMetric is a convenience method to record success/failure metrics
func (r *CircuitBreakerRepository) RecordMetric(ctx context.Context, instanceID string, success bool, err error, errorType string) error {
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

	return r.RecordEvent(ctx, event)
}

// GetRecentEvents retrieves recent events for an instance (for debugging)
func (r *CircuitBreakerRepository) GetRecentEvents(ctx context.Context, instanceID string, limit int) ([]*models.CircuitBreakerEvent, error) {
	var events []*models.CircuitBreakerEvent

	pk := fmt.Sprintf("CIRCUIT#%s", instanceID)

	// Use scan to get all events for this instance
	err := r.db.WithContext(ctx).Model(&models.CircuitBreakerEvent{}).
		Where("PK", "=", pk).
		Where("SK", "begins_with", "EVENT#").
		Limit(limit).
		Scan(&events)

	if err != nil {
		return nil, fmt.Errorf("get recent events: %w", err)
	}

	return events, nil
}

// DeleteCircuitState removes circuit state for an instance (for cleanup)
func (r *CircuitBreakerRepository) DeleteCircuitState(ctx context.Context, instanceID string) error {
	state := &models.CircuitBreakerState{
		InstanceID: instanceID,
	}
	state.UpdateKeys()

	err := r.db.WithContext(ctx).Model(&models.CircuitBreakerState{}).
		Where("PK", "=", state.PK).
		Where("SK", "=", state.SK).
		Delete()

	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("delete circuit state: %w", err)
	}

	return nil
}

// GetAllCircuitStates retrieves all circuit states (for monitoring/debugging)
func (r *CircuitBreakerRepository) GetAllCircuitStates(ctx context.Context) ([]*models.CircuitBreakerState, error) {
	var states []*models.CircuitBreakerState

	err := r.db.WithContext(ctx).Model(&models.CircuitBreakerState{}).
		Where("PK", "begins_with", "CIRCUIT#").
		Where("SK", "=", "STATE").
		Scan(&states)

	if err != nil {
		return nil, fmt.Errorf("get all circuit states: %w", err)
	}

	return states, nil
}
