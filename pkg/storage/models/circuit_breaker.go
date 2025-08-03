package models

import (
	"fmt"
	"time"
)

// CircuitBreakerState represents the state of a circuit breaker for a specific instance
type CircuitBreakerState struct {
	// DynamoDB keys - use exact patterns from legacy
	PK string `dynamorm:"pk" json:"pk"` // CIRCUIT#<instance_id>
	SK string `dynamorm:"sk" json:"sk"` // STATE

	// Core circuit breaker state
	InstanceID       string    `json:"instance_id"`
	Status           string    `json:"status"` // closed, open, half_open
	FailureCount     int       `json:"failure_count"`
	SuccessCount     int       `json:"success_count"`
	ConsecutiveFails int       `json:"consecutive_fails"`
	LastFailure      time.Time `json:"last_failure"`
	LastSuccess      time.Time `json:"last_success"`
	LastStateChange  time.Time `json:"last_state_change"`
	NextRetry        time.Time `json:"next_retry"`
	BackoffDuration  int64     `json:"backoff_duration_nanos"` // Store as nanoseconds for precision

	// Metrics
	TotalRequests  int64 `json:"total_requests"`
	TotalFailures  int64 `json:"total_failures"`
	TotalSuccesses int64 `json:"total_successes"`

	// Metadata
	Reason string `json:"reason"` // Reason for last state change

	// TTL for automatic cleanup (30 days after last change)
	TTL int64 `dynamorm:"ttl" json:"ttl"`
}

// UpdateKeys sets the DynamoDB keys
func (c *CircuitBreakerState) UpdateKeys() {
	c.PK = fmt.Sprintf("CIRCUIT#%s", c.InstanceID)
	c.SK = "STATE"
	
	// Set TTL to 30 days after last state change
	if !c.LastStateChange.IsZero() {
		c.TTL = c.LastStateChange.Add(30 * 24 * time.Hour).Unix()
	} else {
		c.TTL = time.Now().Add(30 * 24 * time.Hour).Unix()
	}
}

// GetBackoffDuration returns the backoff duration as time.Duration
func (c *CircuitBreakerState) GetBackoffDuration() time.Duration {
	return time.Duration(c.BackoffDuration)
}

// SetBackoffDuration sets the backoff duration from time.Duration
func (c *CircuitBreakerState) SetBackoffDuration(d time.Duration) {
	c.BackoffDuration = int64(d)
}

// CircuitBreakerEvent represents a state change event for debugging and monitoring
type CircuitBreakerEvent struct {
	// DynamoDB keys
	PK string `dynamorm:"pk" json:"pk"` // CIRCUIT#<instance_id>
	SK string `dynamorm:"sk" json:"sk"` // EVENT#<timestamp_nanos>

	// Event details
	InstanceID  string    `json:"instance_id"`
	EventType   string    `json:"event_type"` // state_change, metric
	NewStatus   string    `json:"new_status,omitempty"`
	OldStatus   string    `json:"old_status,omitempty"`
	Reason      string    `json:"reason"`
	Timestamp   time.Time `json:"timestamp"`

	// For metric events
	Success   *bool   `json:"success,omitempty"`
	Error     string  `json:"error,omitempty"`
	ErrorType string  `json:"error_type,omitempty"`

	// TTL for cleanup (7 days for events)
	TTL int64 `dynamorm:"ttl" json:"ttl"`
}

// UpdateKeys sets the DynamoDB keys for events
func (e *CircuitBreakerEvent) UpdateKeys() {
	e.PK = fmt.Sprintf("CIRCUIT#%s", e.InstanceID)
	
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now()
	}
	
	e.SK = fmt.Sprintf("EVENT#%d", e.Timestamp.UnixNano())
	
	// Set TTL to 7 days from now
	e.TTL = time.Now().Add(7 * 24 * time.Hour).Unix()
}

// CircuitBreakerConfig represents the configuration for circuit breaker behavior
type CircuitBreakerConfig struct {
	// Failure thresholds
	FailureThreshold int // Failures to open circuit
	SuccessThreshold int // Successes to close circuit

	// Timeouts
	OpenTimeout     time.Duration // How long to stay open
	HalfOpenTimeout time.Duration // How long to test in half-open

	// Sampling
	SampleWindow    time.Duration // Window for counting failures
	MinimumRequests int           // Min requests before evaluating

	// Recovery
	BackoffMultiplier float64       // Exponential backoff multiplier
	MaxBackoff        time.Duration // Maximum backoff duration
}

// DefaultCircuitBreakerConfig returns sensible defaults
func DefaultCircuitBreakerConfig() *CircuitBreakerConfig {
	return &CircuitBreakerConfig{
		FailureThreshold:  5,
		SuccessThreshold:  3,
		OpenTimeout:       30 * time.Second,
		HalfOpenTimeout:   10 * time.Second,
		SampleWindow:      1 * time.Minute,
		MinimumRequests:   10,
		BackoffMultiplier: 2.0,
		MaxBackoff:        5 * time.Minute,
	}
}