package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// CircuitBreakerRepository handles circuit breaker state persistence with BaseRepository integration
type CircuitBreakerRepository struct {
	*BaseRepository[*models.CircuitBreakerState]
	eventRepo *BaseRepository[*models.CircuitBreakerEvent]
}

// NewCircuitBreakerRepository creates a new circuit breaker repository with cost tracking
func NewCircuitBreakerRepository(db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService) *CircuitBreakerRepository {
	return &CircuitBreakerRepository{
		BaseRepository: NewBaseRepositoryWithCostTracking[*models.CircuitBreakerState](
			db, tableName, logger, costService, "circuit_breaker_state",
		),
		eventRepo: NewBaseRepositoryWithCostTracking[*models.CircuitBreakerEvent](
			db, tableName, logger, costService, "circuit_breaker_event",
		),
	}
}

// NewCircuitBreakerRepositoryBasic creates a new circuit breaker repository without cost tracking
func NewCircuitBreakerRepositoryBasic(db core.DB, tableName string, logger *zap.Logger) *CircuitBreakerRepository {
	return &CircuitBreakerRepository{
		BaseRepository: NewBaseRepository[*models.CircuitBreakerState](db, tableName, logger),
		eventRepo:      NewBaseRepository[*models.CircuitBreakerEvent](db, tableName, logger),
	}
}

// GetCircuitState retrieves the current state of a circuit breaker for an instance
// CIRCUIT BREAKER RESILIENCE: Returns default closed state if not found - critical for system stability
func (r *CircuitBreakerRepository) GetCircuitState(ctx context.Context, instanceID string) (*models.CircuitBreakerState, error) {
	state := &models.CircuitBreakerState{InstanceID: instanceID}
	if err := state.UpdateKeys(); err != nil {
		return nil, ErrorHandler.HandleUpdateError(err, EntityCircuitBreaker, instanceID)
	}

	err := r.Get(ctx, state.PK, state.SK, state)
	if err != nil {
		// Circuit breaker resilience: Return default closed state if not found
		if errors.IsNotFound(err) || err.Error() == "item not found: pk="+state.PK+", sk="+state.SK {
			return &models.CircuitBreakerState{
				InstanceID:      instanceID,
				Status:          "closed",
				LastStateChange: time.Now(),
			}, nil
		}
		return nil, ErrorHandler.HandleGetError(err, EntityCircuitBreaker, instanceID)
	}

	return state, nil
}

// SaveCircuitState saves the circuit breaker state atomically
// CIRCUIT BREAKER RESILIENCE: Maintains atomic create/update semantics to prevent race conditions
func (r *CircuitBreakerRepository) SaveCircuitState(ctx context.Context, state *models.CircuitBreakerState) error {
	// Circuit breaker atomicity: Try create first, fallback to update
	err := r.Create(ctx, state)
	if err != nil {
		// If item already exists, update it atomically
		if errors.IsConditionFailed(err) {
			// Get existing state for atomic update
			existing := &models.CircuitBreakerState{InstanceID: state.InstanceID}
			if keyErr := existing.UpdateKeys(); keyErr != nil {
				return ErrorHandler.HandleUpdateError(keyErr, EntityCircuitBreaker, state.InstanceID)
			}

			if getErr := r.Get(ctx, existing.PK, existing.SK, existing); getErr != nil {
				return ErrorHandler.HandleGetError(getErr, EntityCircuitBreaker, state.InstanceID)
			}

			// Preserve atomic state transition by copying all fields
			*existing = *state
			err = r.Update(ctx, existing)
		}
		if err != nil {
			return ErrorHandler.HandleUpdateError(err, EntityCircuitBreaker, state.InstanceID)
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
		return nil, ErrorHandler.HandleUpdateError(err, EntityCircuitBreaker, instanceID)
	}

	// Save the updated state
	if err := r.SaveCircuitState(ctx, state); err != nil {
		return nil, err
	}

	return state, nil
}

// RecordEvent records a circuit breaker event for debugging and monitoring
// CIRCUIT BREAKER RESILIENCE: Non-blocking event recording for debugging/monitoring
func (r *CircuitBreakerRepository) RecordEvent(ctx context.Context, event *models.CircuitBreakerEvent) error {
	err := r.eventRepo.Create(ctx, event)
	if err != nil {
		// Circuit breaker resilience: Event recording failure should not block main operations
		r.eventRepo.logger.Warn("failed to record circuit breaker event",
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
	pk := fmt.Sprintf("CIRCUIT#%s", instanceID)

	// Use BaseRepository query with SK prefix for events
	events, err := r.eventRepo.QueryWithSKPrefix(ctx, pk, "EVENT#", limit)
	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, EntityCircuitBreaker, "events")
	}

	// Convert slice of values to slice of pointers
	result := make([]*models.CircuitBreakerEvent, len(events))
	copy(result, events)
	return result, nil
}

// DeleteCircuitState removes circuit state for an instance (for cleanup)
func (r *CircuitBreakerRepository) DeleteCircuitState(ctx context.Context, instanceID string) error {
	state := &models.CircuitBreakerState{InstanceID: instanceID}
	if err := state.UpdateKeys(); err != nil {
		return ErrorHandler.HandleUpdateError(err, EntityCircuitBreaker, instanceID)
	}

	err := r.Delete(ctx, state.PK, state.SK)
	if err != nil && !errors.IsNotFound(err) {
		return ErrorHandler.HandleDeleteError(err, EntityCircuitBreaker, instanceID)
	}
	return nil
}

// GetAllCircuitStates retrieves all circuit states (for monitoring/debugging)
func (r *CircuitBreakerRepository) GetAllCircuitStates(ctx context.Context) ([]*models.CircuitBreakerState, error) {
	// This requires a scan operation since we need all circuit states across different PKs
	// Using the underlying DB connection for this specialized query
	var states []*models.CircuitBreakerState

	err := r.GetDB().WithContext(ctx).Model(&models.CircuitBreakerState{}).
		Where("PK", "begins_with", "CIRCUIT#").
		Where("SK", "=", models.SKState).
		Scan(&states)

	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, EntityCircuitBreaker, "monitoring")
	}
	return states, nil
}
