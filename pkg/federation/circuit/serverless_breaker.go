// Package circuit provides serverless circuit breaker implementation for federation fault tolerance.
package circuit

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/federation/types"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"go.uber.org/zap"
)

// Circuit breaker state constants
const (
	stateClosed   = "closed"
	stateOpen     = "open"
	stateHalfOpen = "half_open"
)

// CircuitBreakerRepository interface for dependency injection and testing
//
//nolint:revive // CircuitBreaker prefix clarifies this is for circuit breaker pattern
type CircuitBreakerRepository interface {
	GetCircuitState(ctx context.Context, instanceID string) (*models.CircuitBreakerState, error)
	SaveCircuitState(ctx context.Context, state *models.CircuitBreakerState) error
	UpdateCircuitState(ctx context.Context, instanceID string, updateFn func(*models.CircuitBreakerState) error) (*models.CircuitBreakerState, error)
	RecordEvent(ctx context.Context, event *models.CircuitBreakerEvent) error
	RecordStateChange(ctx context.Context, instanceID, oldStatus, newStatus, reason string) error
	RecordMetric(ctx context.Context, instanceID string, success bool, err error, errorType string) error
	GetRecentEvents(ctx context.Context, instanceID string, limit int) ([]*models.CircuitBreakerEvent, error)
	DeleteCircuitState(ctx context.Context, instanceID string) error
	GetAllCircuitStates(ctx context.Context) ([]*models.CircuitBreakerState, error)
}

// ServerlessCircuitBreaker implements circuit breaker pattern for serverless environments
// No in-memory state, no background goroutines, purely event-driven
type ServerlessCircuitBreaker struct {
	repo   CircuitBreakerRepository
	config *models.CircuitBreakerConfig
	logger *zap.Logger
}

// NewServerlessCircuitBreaker creates a new serverless circuit breaker
func NewServerlessCircuitBreaker(repo CircuitBreakerRepository, config *models.CircuitBreakerConfig, logger *zap.Logger) *ServerlessCircuitBreaker {
	if config == nil {
		config = models.DefaultCircuitBreakerConfig()
	}

	return &ServerlessCircuitBreaker{
		repo:   repo,
		config: config,
		logger: logger,
	}
}

// IsOpen checks if the circuit is open for the given instance
// This is the main entry point - evaluates state on-demand
func (cb *ServerlessCircuitBreaker) IsOpen(ctx context.Context, instanceID string) bool {
	state, err := cb.repo.GetCircuitState(ctx, instanceID)
	if err != nil {
		cb.logger.Error("failed to get circuit state",
			zap.String("instanceID", instanceID),
			zap.Error(err))
		return false // Fail open - allow requests if we can't determine state
	}

	return cb.evaluateCircuitState(ctx, state)
}

// CanAttempt checks if a request can be attempted
// Handles automatic state transitions from open -> half-open
func (cb *ServerlessCircuitBreaker) CanAttempt(ctx context.Context, instanceID string) bool {
	state, err := cb.repo.GetCircuitState(ctx, instanceID)
	if err != nil {
		cb.logger.Error("failed to get circuit state",
			zap.String("instanceID", instanceID),
			zap.Error(err))
		return true // Fail open
	}

	now := time.Now()

	switch state.Status {
	case stateClosed:
		return true

	case stateOpen:
		// Check if it's time to test recovery
		if now.After(state.NextRetry) {
			// Attempt to transition to half-open
			if err := cb.transitionToHalfOpen(ctx, state); err != nil {
				cb.logger.Error("failed to transition to half-open",
					zap.String("instanceID", instanceID),
					zap.Error(err))
				return false
			}
			return true
		}
		return false

	case stateHalfOpen:
		// Allow limited attempts in half-open state
		return true

	default:
		cb.logger.Warn("unknown circuit status",
			zap.String("instanceID", instanceID),
			zap.String("status", state.Status))
		return true // Fail open for unknown states
	}
}

// RecordSuccess records a successful request and handles state transitions
func (cb *ServerlessCircuitBreaker) RecordSuccess(ctx context.Context, instanceID string) error {
	_, err := cb.repo.UpdateCircuitState(ctx, instanceID, func(state *models.CircuitBreakerState) error {
		oldStatus := state.Status
		now := time.Now()

		// Update counters
		state.SuccessCount++
		state.TotalSuccesses++
		state.TotalRequests++
		state.LastSuccess = now
		state.ConsecutiveFails = 0

		// Handle state transitions
		switch state.Status {
		case stateHalfOpen:
			if state.SuccessCount >= cb.config.SuccessThreshold {
				// Close the circuit - recovery successful
				state.Status = stateClosed
				state.LastStateChange = now
				state.FailureCount = 0
				state.SuccessCount = 0
				state.SetBackoffDuration(0)
				state.Reason = "recovery successful"

				cb.logger.Info("circuit recovered",
					zap.String("instanceID", instanceID))
			}

		case stateOpen:
			// Shouldn't happen, but handle gracefully by transitioning to half-open
			state.Status = stateHalfOpen
			state.LastStateChange = now
			state.SuccessCount = 1 // This success counts
			state.NextRetry = now.Add(cb.config.HalfOpenTimeout)
			state.Reason = "unexpected success during open state"
		}

		// Record state change if status changed
		if oldStatus != state.Status {
			go func() {
				ctx := context.Background() // Use background context for async logging
				if err := cb.repo.RecordStateChange(ctx, instanceID, oldStatus, state.Status, state.Reason); err != nil {
					cb.logger.Warn("failed to record state change", zap.Error(err))
				}
			}()
		}

		// Record success metric
		go func() {
			ctx := context.Background()
			if err := cb.repo.RecordMetric(ctx, instanceID, true, nil, ""); err != nil {
				cb.logger.Warn("failed to record success metric", zap.Error(err))
			}
		}()

		return nil
	})
	return err
}

// RecordFailure records a failed request and handles state transitions
func (cb *ServerlessCircuitBreaker) RecordFailure(ctx context.Context, instanceID string, err error) error {
	errorType := cb.classifyError(err)

	_, updateErr := cb.repo.UpdateCircuitState(ctx, instanceID, func(state *models.CircuitBreakerState) error {
		oldStatus := state.Status
		now := time.Now()

		// Update counters
		state.FailureCount++
		state.TotalFailures++
		state.TotalRequests++
		state.ConsecutiveFails++
		state.LastFailure = now

		// Handle state transitions
		switch state.Status {
		case stateClosed:
			// Check if we should open the circuit
			if state.ConsecutiveFails >= cb.config.FailureThreshold {
				cb.openCircuit(state, now, fmt.Sprintf("consecutive failures: %d, error: %s", state.ConsecutiveFails, errorType))
			}

		case stateHalfOpen:
			// Single failure in half-open returns to open with backoff
			cb.openCircuit(state, now, fmt.Sprintf("half-open test failed: %s", errorType))
			state.SuccessCount = 0

		case stateOpen:
			// Already open, just update counters and potentially extend backoff
			// Don't change state but update next retry time
			backoffDuration := cb.calculateBackoff(state.GetBackoffDuration())
			state.SetBackoffDuration(backoffDuration)
			state.NextRetry = now.Add(backoffDuration)
		}

		// Record state change if status changed
		if oldStatus != state.Status {
			go func() {
				ctx := context.Background()
				if err := cb.repo.RecordStateChange(ctx, instanceID, oldStatus, state.Status, state.Reason); err != nil {
					cb.logger.Warn("failed to record state change", zap.Error(err))
				}
			}()
		}

		// Record failure metric
		go func() {
			ctx := context.Background()
			if err := cb.repo.RecordMetric(ctx, instanceID, false, err, errorType); err != nil {
				cb.logger.Warn("failed to record failure metric", zap.Error(err))
			}
		}()

		return nil
	})
	return updateErr
}

// GetStatus returns the current circuit status
func (cb *ServerlessCircuitBreaker) GetStatus(ctx context.Context, instanceID string) types.CircuitStatus {
	state, err := cb.repo.GetCircuitState(ctx, instanceID)
	if err != nil {
		cb.logger.Error("failed to get circuit state",
			zap.String("instanceID", instanceID),
			zap.Error(err))
		return types.CircuitClosed // Default to closed if we can't determine
	}

	// Evaluate current state before returning
	cb.evaluateCircuitState(ctx, state)

	switch state.Status {
	case stateOpen:
		return types.CircuitOpen
	case stateHalfOpen:
		return types.CircuitHalfOpen
	default:
		return types.CircuitClosed
	}
}

// GetMetrics returns circuit breaker metrics
func (cb *ServerlessCircuitBreaker) GetMetrics(ctx context.Context, instanceID string) map[string]any {
	state, err := cb.repo.GetCircuitState(ctx, instanceID)
	if err != nil {
		cb.logger.Error("failed to get circuit state",
			zap.String("instanceID", instanceID),
			zap.Error(err))
		return map[string]any{"error": err.Error()}
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

// evaluateCircuitState checks if the current state is still valid and updates if needed
func (cb *ServerlessCircuitBreaker) evaluateCircuitState(_ context.Context, state *models.CircuitBreakerState) bool {
	now := time.Now()

	switch state.Status {
	case stateOpen:
		// Check if it's time to attempt recovery
		if now.After(state.NextRetry) {
			// Don't automatically transition here, let CanAttempt handle it
			return false // Circuit can be attempted
		}
		return true // Circuit is still open

	case stateHalfOpen:
		// Check for timeout in half-open state
		if now.After(state.NextRetry) {
			// Half-open timed out, return to open
			go func() {
				ctx := context.Background()
				_, err := cb.repo.UpdateCircuitState(ctx, state.InstanceID, func(s *models.CircuitBreakerState) error {
					s.Status = stateOpen
					s.LastStateChange = now
					s.NextRetry = now.Add(s.GetBackoffDuration())
					s.Reason = "half-open timeout"
					return nil
				})
				if err != nil {
					cb.logger.Error("failed to transition from half-open timeout", zap.Error(err))
				}
			}()
			return true // Treat as open
		}
		return false // Half-open, allow attempts

	default:
		return false // Closed or unknown, allow attempts
	}
}

// transitionToHalfOpen transitions circuit from open to half-open
func (cb *ServerlessCircuitBreaker) transitionToHalfOpen(ctx context.Context, state *models.CircuitBreakerState) error {
	_, err := cb.repo.UpdateCircuitState(ctx, state.InstanceID, func(s *models.CircuitBreakerState) error {
		if s.Status != stateOpen {
			return fmt.Errorf("cannot transition to half-open from %s state", s.Status)
		}

		now := time.Now()
		s.Status = stateHalfOpen
		s.LastStateChange = now
		s.NextRetry = now.Add(cb.config.HalfOpenTimeout)
		s.SuccessCount = 0
		s.Reason = "testing recovery"

		cb.logger.Info("circuit half-open",
			zap.String("instanceID", s.InstanceID))

		return nil
	})

	return err
}

// openCircuit transitions the circuit to open state with backoff
func (cb *ServerlessCircuitBreaker) openCircuit(state *models.CircuitBreakerState, now time.Time, reason string) {
	state.Status = stateOpen
	state.LastStateChange = now
	state.Reason = reason

	// Calculate backoff duration
	backoffDuration := cb.calculateBackoff(state.GetBackoffDuration())
	state.SetBackoffDuration(backoffDuration)
	state.NextRetry = now.Add(backoffDuration)

	cb.logger.Warn("circuit opened",
		zap.String("instanceID", state.InstanceID),
		zap.String("reason", reason),
		zap.Duration("backoff", backoffDuration))
}

// calculateBackoff calculates the next backoff duration using exponential backoff
func (cb *ServerlessCircuitBreaker) calculateBackoff(currentBackoff time.Duration) time.Duration {
	if currentBackoff == 0 {
		return cb.config.OpenTimeout
	}

	newBackoff := time.Duration(float64(currentBackoff) * cb.config.BackoffMultiplier)
	if newBackoff > cb.config.MaxBackoff {
		return cb.config.MaxBackoff
	}
	return newBackoff
}

// classifyError classifies errors for better circuit breaker decisions
func (cb *ServerlessCircuitBreaker) classifyError(err error) string {
	if err == nil {
		return "none"
	}

	errStr := strings.ToLower(err.Error())
	switch {
	case strings.Contains(errStr, "timeout"):
		return "timeout"
	case strings.Contains(errStr, "connection refused"):
		return "connection_refused"
	case strings.Contains(errStr, "no such host"):
		return "dns_failure"
	case strings.Contains(errStr, "500"), strings.Contains(errStr, "503"):
		return "server_error"
	case strings.Contains(errStr, "429"):
		return "rate_limit"
	case strings.Contains(errStr, "network"):
		return "network_error"
	default:
		return "unknown"
	}
}
