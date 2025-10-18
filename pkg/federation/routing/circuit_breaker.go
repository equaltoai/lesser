// Package routing provides distributed circuit breaker implementation for federation request routing.
package routing

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/equaltoai/lesser/pkg/federation/types"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"go.uber.org/zap"
)

// DistributedCircuitBreaker implements circuit breaker pattern with DynamORM persistence
type DistributedCircuitBreaker struct {
	repo             *repositories.CircuitBreakerRepository
	thresholdManager *RouteThresholdManager
	logger           *zap.Logger
	config           *models.CircuitBreakerConfig
}

// NewDistributedCircuitBreaker creates a new circuit breaker
func NewDistributedCircuitBreaker(repo *repositories.CircuitBreakerRepository, thresholdManager *RouteThresholdManager, logger *zap.Logger, config *models.CircuitBreakerConfig) *DistributedCircuitBreaker {
	if config == nil {
		config = models.DefaultCircuitBreakerConfig()
	}

	return &DistributedCircuitBreaker{
		repo:             repo,
		thresholdManager: thresholdManager,
		logger:           logger,
		config:           config,
	}
}

// Open opens the circuit for an instance
func (dcb *DistributedCircuitBreaker) Open(instanceID string, reason string) error {
	ctx := context.Background()

	// Get current state
	state, err := dcb.repo.GetCircuitState(ctx, instanceID)
	if err != nil {
		dcb.logger.Error("failed to retrieve circuit state for opening",
			zap.String("instance_id", instanceID),
			zap.String("operation", "open"),
			zap.String("reason", reason),
			zap.Error(err))
		return errors.Join(ErrCircuitStateRetrieveFailed, err)
	}

	previousStatus := state.Status
	state.Status = string(types.CircuitOpen)
	state.LastStateChange = time.Now()
	state.NextRetry = time.Now().Add(dcb.config.OpenTimeout)
	state.Reason = reason

	// Apply exponential backoff
	if state.GetBackoffDuration() == 0 {
		state.SetBackoffDuration(dcb.config.OpenTimeout)
	} else {
		backoffDuration := time.Duration(float64(state.GetBackoffDuration()) * dcb.config.BackoffMultiplier)
		if backoffDuration > dcb.config.MaxBackoff {
			backoffDuration = dcb.config.MaxBackoff
		}
		state.SetBackoffDuration(backoffDuration)
	}

	// Save state
	if err := dcb.repo.SaveCircuitState(ctx, state); err != nil {
		dcb.logger.Error("failed to save circuit state during opening",
			zap.String("instance_id", instanceID),
			zap.String("operation", "open"),
			zap.String("new_status", state.Status),
			zap.String("reason", reason),
			zap.Duration("backoff", state.GetBackoffDuration()),
			zap.Error(err))
		return errors.Join(ErrCircuitStateSaveFailed, err)
	}

	// Record state change event
	_ = dcb.repo.RecordStateChange(ctx, instanceID, previousStatus, state.Status, reason)

	dcb.logger.Warn("circuit opened",
		zap.String("instanceID", instanceID),
		zap.String("previousStatus", previousStatus),
		zap.String("reason", reason),
		zap.Duration("backoff", state.GetBackoffDuration()))

	return nil
}

// Close closes the circuit for an instance
func (dcb *DistributedCircuitBreaker) Close(instanceID string) error {
	ctx := context.Background()

	// Get current state
	state, err := dcb.repo.GetCircuitState(ctx, instanceID)
	if err != nil {
		dcb.logger.Error("failed to retrieve circuit state for closing",
			zap.String("instance_id", instanceID),
			zap.String("operation", "close"),
			zap.Error(err))
		return errors.Join(ErrCircuitStateRetrieveFailed, err)
	}

	if state.Status == string(types.CircuitClosed) {
		return nil // Already closed
	}

	previousStatus := state.Status
	state.Status = string(types.CircuitClosed)
	state.LastStateChange = time.Now()
	state.FailureCount = 0
	state.SuccessCount = 0
	state.ConsecutiveFails = 0
	state.SetBackoffDuration(0)
	state.Reason = "circuit closed"

	// Save state
	if err := dcb.repo.SaveCircuitState(ctx, state); err != nil {
		dcb.logger.Error("failed to save circuit state during closing",
			zap.String("instance_id", instanceID),
			zap.String("operation", "close"),
			zap.String("previous_status", previousStatus),
			zap.String("new_status", state.Status),
			zap.Error(err))
		return errors.Join(ErrCircuitStateSaveFailed, err)
	}

	// Record state change event
	_ = dcb.repo.RecordStateChange(ctx, instanceID, previousStatus, state.Status, "circuit closed")

	dcb.logger.Info("circuit closed",
		zap.String("instanceID", instanceID))

	return nil
}

// HalfOpen puts the circuit in half-open state for testing
func (dcb *DistributedCircuitBreaker) HalfOpen(instanceID string) error {
	ctx := context.Background()

	// Get current state
	state, err := dcb.repo.GetCircuitState(ctx, instanceID)
	if err != nil {
		dcb.logger.Error("failed to retrieve circuit state for half-open transition",
			zap.String("instance_id", instanceID),
			zap.String("operation", "half_open"),
			zap.Error(err))
		return errors.Join(ErrCircuitStateRetrieveFailed, err)
	}

	if state.Status != string(types.CircuitOpen) {
		return ErrInvalidCircuitTransition
	}

	previousStatus := state.Status
	state.Status = string(types.CircuitHalfOpen)
	state.LastStateChange = time.Now()
	state.NextRetry = time.Now().Add(dcb.config.HalfOpenTimeout)
	state.Reason = "testing recovery"

	// Save state
	if err := dcb.repo.SaveCircuitState(ctx, state); err != nil {
		dcb.logger.Error("failed to save circuit state during half-open transition",
			zap.String("instance_id", instanceID),
			zap.String("operation", "half_open"),
			zap.String("previous_status", previousStatus),
			zap.String("new_status", state.Status),
			zap.Time("next_retry", state.NextRetry),
			zap.Error(err))
		return errors.Join(ErrCircuitStateSaveFailed, err)
	}

	// Record state change event
	_ = dcb.repo.RecordStateChange(ctx, instanceID, previousStatus, state.Status, "testing recovery")

	dcb.logger.Info("circuit half-open",
		zap.String("instanceID", instanceID))

	return nil
}

// IsOpen checks if the circuit is open
func (dcb *DistributedCircuitBreaker) IsOpen(instanceID string) bool {
	ctx := context.Background()

	state, err := dcb.repo.GetCircuitState(ctx, instanceID)
	if err != nil {
		dcb.logger.Debug("failed to get circuit state",
			zap.String("instanceID", instanceID),
			zap.Error(err))
		return false // Default to closed on error
	}

	return state.Status == string(types.CircuitOpen)
}

// CanAttempt checks if a request can be attempted
func (dcb *DistributedCircuitBreaker) CanAttempt(instanceID string) bool {
	ctx := context.Background()

	state, err := dcb.repo.GetCircuitState(ctx, instanceID)
	if err != nil {
		dcb.logger.Debug("failed to get circuit state",
			zap.String("instanceID", instanceID),
			zap.Error(err))
		return true // Default to allow on error
	}

	switch types.CircuitStatus(state.Status) {
	case types.CircuitClosed:
		return true

	case types.CircuitOpen:
		// Check if it's time to test
		if time.Now().After(state.NextRetry) {
			// Transition to half-open synchronously
			if err := dcb.HalfOpen(instanceID); err != nil {
				dcb.logger.Error("Failed to transition to half-open state",
					zap.String("instanceID", instanceID),
					zap.Error(err))
				return false
			}
			return true
		}
		return false

	case types.CircuitHalfOpen:
		// Allow limited attempts
		return true

	default:
		return true
	}
}

// RecordSuccess records a successful request
func (dcb *DistributedCircuitBreaker) RecordSuccess(instanceID string) error {
	ctx := context.Background()

	// Update state atomically
	_, err := dcb.repo.UpdateCircuitState(ctx, instanceID, func(state *models.CircuitBreakerState) error {
		state.SuccessCount++
		state.TotalSuccesses++
		state.TotalRequests++
		state.LastSuccess = time.Now()
		state.ConsecutiveFails = 0

		// Check state transitions
		switch types.CircuitStatus(state.Status) {
		case types.CircuitHalfOpen:
			if state.SuccessCount >= dcb.config.SuccessThreshold {
				// Close the circuit
				oldStatus := state.Status
				state.Status = string(types.CircuitClosed)
				state.LastStateChange = time.Now()
				state.FailureCount = 0
				state.SuccessCount = 0
				state.SetBackoffDuration(0)
				state.Reason = "circuit recovered"

				dcb.logger.Info("circuit recovered",
					zap.String("instanceID", instanceID))

				// Record state change (non-blocking)
				go func() {
					_ = dcb.repo.RecordStateChange(context.Background(), instanceID, oldStatus, state.Status, "circuit recovered")
				}()
			}

		case types.CircuitOpen:
			// Shouldn't happen, but handle gracefully
			oldStatus := state.Status
			state.Status = string(types.CircuitHalfOpen)
			state.LastStateChange = time.Now()
			state.Reason = "unexpected success during open state"

			// Record state change (non-blocking)
			go func() {
				_ = dcb.repo.RecordStateChange(context.Background(), instanceID, oldStatus, state.Status, "unexpected success during open state")
			}()
		}

		return nil
	})

	if err != nil {
		dcb.logger.Error("failed to update circuit state during success recording",
			zap.String("instance_id", instanceID),
			zap.String("operation", "record_success"),
			zap.Error(err))
		return errors.Join(ErrCircuitStateUpdateFailed, err)
	}

	// Record metric (non-blocking)
	go func() {
		if err := dcb.repo.RecordMetric(context.Background(), instanceID, true, nil, ""); err != nil {
			dcb.logger.Warn("failed to record success metric",
				zap.String("instance_id", instanceID),
				zap.Error(err))
		}
	}()

	return nil
}

// RecordFailure records a failed request
func (dcb *DistributedCircuitBreaker) RecordFailure(instanceID string, err error) error {
	ctx := context.Background()

	// Determine error type for better decision making
	errorType := dcb.classifyError(err)

	// Update state atomically
	_, updateErr := dcb.repo.UpdateCircuitState(ctx, instanceID, func(state *models.CircuitBreakerState) error {
		state.FailureCount++
		state.TotalFailures++
		state.TotalRequests++
		state.ConsecutiveFails++
		state.LastFailure = time.Now()

		// Check state transitions
		switch types.CircuitStatus(state.Status) {
		case types.CircuitClosed:
			// Check if we should open the circuit
			if state.ConsecutiveFails >= dcb.config.FailureThreshold {
				oldStatus := state.Status
				state.Status = string(types.CircuitOpen)
				state.LastStateChange = time.Now()
				state.NextRetry = time.Now().Add(dcb.config.OpenTimeout)
				state.Reason = "consecutive failures: " + strconv.Itoa(state.ConsecutiveFails) + ", error: " + errorType

				dcb.logger.Warn("circuit opened due to failures",
					zap.String("instanceID", instanceID),
					zap.Int("failures", state.ConsecutiveFails),
					zap.String("errorType", errorType))

				// Record state change (non-blocking)
				go func() {
					_ = dcb.repo.RecordStateChange(context.Background(), instanceID, oldStatus, state.Status, state.Reason)
				}()
			}

		case types.CircuitHalfOpen:
			// Single failure returns to open
			oldStatus := state.Status
			state.Status = string(types.CircuitOpen)
			state.LastStateChange = time.Now()
			state.NextRetry = time.Now().Add(state.GetBackoffDuration())
			state.SuccessCount = 0
			state.Reason = "half-open test failed: " + errorType

			dcb.logger.Warn("circuit reopened",
				zap.String("instanceID", instanceID),
				zap.String("errorType", errorType))

			// Record state change (non-blocking)
			go func() {
				_ = dcb.repo.RecordStateChange(context.Background(), instanceID, oldStatus, state.Status, state.Reason)
			}()
		}

		return nil
	})

	if updateErr != nil {
		dcb.logger.Error("failed to update circuit state during failure recording",
			zap.String("instance_id", instanceID),
			zap.String("operation", "record_failure"),
			zap.String("error_type", errorType),
			zap.Error(updateErr))
		return errors.Join(ErrCircuitStateUpdateFailed, updateErr)
	}

	// Record metric (non-blocking)
	go func() {
		if metricErr := dcb.repo.RecordMetric(context.Background(), instanceID, false, err, errorType); metricErr != nil {
			dcb.logger.Warn("failed to record failure metric",
				zap.String("instance_id", instanceID),
				zap.String("error_type", errorType),
				zap.Error(metricErr))
		}
	}()

	return nil
}

// GetStatus returns the current circuit status
func (dcb *DistributedCircuitBreaker) GetStatus(instanceID string) types.CircuitStatus {
	ctx := context.Background()

	state, err := dcb.repo.GetCircuitState(ctx, instanceID)
	if err != nil {
		dcb.logger.Debug("failed to get circuit state",
			zap.String("instanceID", instanceID),
			zap.Error(err))
		return types.CircuitClosed // Default to closed on error
	}

	return types.CircuitStatus(state.Status)
}

// GetMetrics returns circuit breaker metrics
func (dcb *DistributedCircuitBreaker) GetMetrics(instanceID string) map[string]any {
	ctx := context.Background()

	state, err := dcb.repo.GetCircuitState(ctx, instanceID)
	if err != nil {
		dcb.logger.Debug("failed to get circuit state",
			zap.String("instanceID", instanceID),
			zap.Error(err))
		// Return empty metrics on error
		return map[string]any{
			"status":           string(types.CircuitClosed),
			"totalRequests":    int64(0),
			"totalFailures":    int64(0),
			"totalSuccesses":   int64(0),
			"successRate":      float64(0),
			"consecutiveFails": 0,
			"lastFailure":      time.Time{},
			"lastSuccess":      time.Time{},
			"nextRetry":        time.Time{},
			"backoffDuration":  time.Duration(0),
		}
	}

	successRate := float64(0)
	if state.TotalRequests > 0 {
		successRate = float64(state.TotalSuccesses) / float64(state.TotalRequests)
	}

	return map[string]any{
		"status":           state.Status,
		"totalRequests":    state.TotalRequests,
		"totalFailures":    state.TotalFailures,
		"totalSuccesses":   state.TotalSuccesses,
		"successRate":      successRate,
		"consecutiveFails": state.ConsecutiveFails,
		"lastFailure":      state.LastFailure,
		"lastSuccess":      state.LastSuccess,
		"nextRetry":        state.NextRetry,
		"backoffDuration":  state.GetBackoffDuration(),
	}
}

// Helper methods

func (dcb *DistributedCircuitBreaker) classifyError(err error) string {
	if err == nil {
		return "none"
	}

	errStr := err.Error()
	switch {
	case contains(errStr, "timeout"):
		return "timeout"
	case contains(errStr, "connection refused"):
		return "connection_refused"
	case contains(errStr, "no such host"):
		return "dns_failure"
	case contains(errStr, "500") || contains(errStr, "503"):
		return "server_error"
	case contains(errStr, "429"):
		return "rate_limit"
	default:
		return "unknown"
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[0:len(substr)] == substr || len(s) > len(substr) && contains(s[1:], substr)
}

// AssessRouteHealthAndAdjustCircuit uses threshold manager to assess route health and adjust circuit state
func (dcb *DistributedCircuitBreaker) AssessRouteHealthAndAdjustCircuit(ctx context.Context, routeID string, metrics *types.RouteMetrics) error {
	if dcb.thresholdManager == nil {
		return nil // Skip if no threshold manager configured
	}

	// Assess route health using threshold manager
	assessment := dcb.thresholdManager.AssessRouteHealth(ctx, routeID, metrics)

	dcb.logger.Debug("Route health assessment",
		zap.String("routeID", routeID),
		zap.String("status", assessment.Status.String()),
		zap.Float64("successRate", assessment.SuccessRate),
		zap.String("action", assessment.RecommendedAction))

	// Take action based on assessment
	switch assessment.Status {
	case RouteHealthCritical:
		// Open circuit immediately for critical health status
		if dcb.GetStatus(routeID) != types.CircuitOpen {
			reason := "critical health: " + assessment.DegradationReason
			return dcb.Open(routeID, reason)
		}

	case RouteHealthDegraded:
		// For degraded routes, consider opening circuit if it's not already managed
		currentStatus := dcb.GetStatus(routeID)
		if currentStatus == types.CircuitClosed && assessment.SuccessRate < dcb.thresholdManager.config.DegradedSuccessRate {
			dcb.logger.Warn("Route degraded, consider traffic reduction",
				zap.String("routeID", routeID),
				zap.String("reason", assessment.DegradationReason))
		}

	case RouteHealthPreferred, RouteHealthHealthy:
		// Close circuit if it's currently open and the route is healthy
		if dcb.GetStatus(routeID) == types.CircuitOpen {
			dcb.logger.Info("Route recovered, closing circuit",
				zap.String("routeID", routeID))
			return dcb.Close(routeID)
		}
	}

	return nil
}

// ShouldEnterEmergencyMode checks if the system should enter emergency mode
func (dcb *DistributedCircuitBreaker) ShouldEnterEmergencyMode(healthyRoutes, totalRoutes int) bool {
	if dcb.thresholdManager == nil {
		return false
	}
	return dcb.thresholdManager.ShouldEnterEmergencyMode(healthyRoutes, totalRoutes)
}

// GetBackpressureRules returns backpressure rules for emergency mode
func (dcb *DistributedCircuitBreaker) GetBackpressureRules() map[MessagePriority]BackpressureRule {
	if dcb.thresholdManager == nil {
		return make(map[MessagePriority]BackpressureRule)
	}
	return dcb.thresholdManager.GetEmergencyBackpressureRules()
}
