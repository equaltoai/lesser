package cost

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// CostCircuitBreakerConfig defines circuit breaker thresholds
//
//nolint:revive // Cost prefix clarifies this is cost-specific circuit breaker config
type CostCircuitBreakerConfig struct {
	MaxCostPerHour    float64       // Maximum cost per hour in USD
	MaxCostPerRequest float64       // Maximum cost per request in USD
	WindowSize        time.Duration // Cost calculation window
	FailureThreshold  int           // Number of failures to open circuit
	RecoveryTimeout   time.Duration // Time before attempting recovery
}

// CostCircuitBreaker implements cost-aware circuit breaking
//
//nolint:revive // Cost prefix clarifies this is cost-specific circuit breaker
type CostCircuitBreaker struct {
	config       CostCircuitBreakerConfig
	state        CircuitState
	failures     int
	lastFailTime time.Time
	costWindow   *CostWindow
	mu           sync.RWMutex
}

// CircuitState represents the state of a circuit breaker
type CircuitState int

const (
	// StateClosed represents normal operation
	StateClosed   CircuitState = iota
	// StateOpen represents circuit open - rejecting requests
	StateOpen
	// StateHalfOpen represents testing recovery
	StateHalfOpen
)

// CostWindow tracks costs within a time window
//
//nolint:revive // Cost prefix clarifies this is cost-specific window
type CostWindow struct {
	costs     []CostSample
	startTime time.Time
	mu        sync.RWMutex
}

// CostSample represents a single cost measurement
//
//nolint:revive // Cost prefix clarifies this is cost-specific sample
type CostSample struct {
	cost      float64
	timestamp time.Time
}

// NewCostCircuitBreaker creates a new cost-aware circuit breaker
func NewCostCircuitBreaker(config CostCircuitBreakerConfig) *CostCircuitBreaker {
	return &CostCircuitBreaker{
		config: config,
		state:  StateClosed,
		costWindow: &CostWindow{
			costs:     make([]CostSample, 0, 1000),
			startTime: time.Now(),
		},
	}
}

// CheckCost validates if operation is within cost limits
func (cb *CostCircuitBreaker) CheckCost(_ context.Context, estimatedCost float64) error {
	// First, check current state without holding locks for too long
	cb.mu.RLock()
	state := cb.state
	lastFailTime := cb.lastFailTime
	cb.mu.RUnlock()

	switch state {
	case StateOpen:
		// Check if recovery timeout has passed
		if time.Since(lastFailTime) > cb.config.RecoveryTimeout {
			cb.transitionToHalfOpen()
		} else {
			return fmt.Errorf("circuit breaker open: cost limit exceeded")
		}
	case StateHalfOpen:
		// Allow limited requests to test recovery
		if cb.costWindow.getCurrentHourCost() > cb.config.MaxCostPerHour {
			cb.transitionToOpen()
			return fmt.Errorf("circuit breaker reopened: cost still too high")
		}
	}

	// Check request cost limit
	if estimatedCost > cb.config.MaxCostPerRequest {
		cb.recordFailure()
		return fmt.Errorf("request cost %.6f exceeds limit %.6f",
			estimatedCost, cb.config.MaxCostPerRequest)
	}

	// Check hourly cost limit
	if cb.costWindow.getCurrentHourCost()+estimatedCost > cb.config.MaxCostPerHour {
		cb.recordFailure()
		return fmt.Errorf("hourly cost limit would be exceeded")
	}

	return nil
}

// RecordCost records actual operation cost
func (cb *CostCircuitBreaker) RecordCost(cost float64) {
	cb.costWindow.addSample(CostSample{
		cost:      cost,
		timestamp: time.Now(),
	})

	// Reset failure count on successful operation
	if cb.state == StateHalfOpen {
		cb.mu.Lock()
		cb.failures = 0
		cb.state = StateClosed
		cb.mu.Unlock()
	}
}

// recordFailure increments failure count and may open circuit
func (cb *CostCircuitBreaker) recordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures++
	cb.lastFailTime = time.Now()

	if cb.failures >= cb.config.FailureThreshold {
		cb.state = StateOpen
	}
}

// transitionToHalfOpen moves circuit to half-open state
func (cb *CostCircuitBreaker) transitionToHalfOpen() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.state = StateHalfOpen
}

// transitionToOpen moves circuit to open state
func (cb *CostCircuitBreaker) transitionToOpen() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.state = StateOpen
	cb.lastFailTime = time.Now()
}

// getCurrentHourCost calculates cost in the current hour
func (cw *CostWindow) getCurrentHourCost() float64 {
	cw.mu.RLock()
	defer cw.mu.RUnlock()

	cutoff := time.Now().Add(-time.Hour)
	total := 0.0

	for _, sample := range cw.costs {
		if sample.timestamp.After(cutoff) {
			total += sample.cost
		}
	}

	return total
}

// addSample adds a cost sample and cleans old ones
func (cw *CostWindow) addSample(sample CostSample) {
	cw.mu.Lock()
	defer cw.mu.Unlock()

	// Add new sample
	cw.costs = append(cw.costs, sample)

	// Clean samples older than window
	cutoff := time.Now().Add(-time.Hour)
	var kept []CostSample
	for _, s := range cw.costs {
		if s.timestamp.After(cutoff) {
			kept = append(kept, s)
		}
	}
	cw.costs = kept
}
