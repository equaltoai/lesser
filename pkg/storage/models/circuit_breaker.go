package models

import (
	"fmt"
	"time"
)

// CircuitBreakerState represents the state of a circuit breaker for a specific instance
type CircuitBreakerState struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// DynamoDB keys - use exact patterns from legacy
	PK string `theorydb:"pk,attr:PK" json:"pk"` // CIRCUIT#<instance_id>
	SK string `theorydb:"sk,attr:SK" json:"sk"` // STATE

	// Core circuit breaker state
	InstanceID       string    `theorydb:"attr:instanceID" json:"instance_id"`
	Status           string    `theorydb:"attr:status" json:"status"` // closed, open, half_open
	FailureCount     int       `theorydb:"attr:failureCount" json:"failure_count"`
	SuccessCount     int       `theorydb:"attr:successCount" json:"success_count"`
	ConsecutiveFails int       `theorydb:"attr:consecutiveFails" json:"consecutive_fails"`
	LastFailure      time.Time `theorydb:"attr:lastFailure" json:"last_failure"`
	LastSuccess      time.Time `theorydb:"attr:lastSuccess" json:"last_success"`
	LastStateChange  time.Time `theorydb:"attr:lastStateChange" json:"last_state_change"`
	NextRetry        time.Time `theorydb:"attr:nextRetry" json:"next_retry"`
	BackoffDuration  int64     `theorydb:"attr:backoffDurationNanos" json:"backoff_duration_nanos"` // Store as nanoseconds for precision

	// Metrics
	TotalRequests  int64 `theorydb:"attr:totalRequests" json:"total_requests"`
	TotalFailures  int64 `theorydb:"attr:totalFailures" json:"total_failures"`
	TotalSuccesses int64 `theorydb:"attr:totalSuccesses" json:"total_successes"`

	// Metadata
	Reason string `theorydb:"attr:reason" json:"reason"` // Reason for last state change

	// TTL for automatic cleanup (30 days after last change)
	TTL int64 `theorydb:"ttl,attr:ttl" json:"ttl"`
}

// UpdateKeys sets the DynamoDB keys
func (c *CircuitBreakerState) UpdateKeys() error {
	c.PK = fmt.Sprintf("CIRCUIT#%s", c.InstanceID)
	c.SK = SKState

	// Set TTL to 30 days after last state change
	if !c.LastStateChange.IsZero() {
		c.TTL = c.LastStateChange.Add(30 * 24 * time.Hour).Unix()
	} else {
		c.TTL = time.Now().Add(30 * 24 * time.Hour).Unix()
	}
	return nil
}

// GetPK returns the partition key
func (c *CircuitBreakerState) GetPK() string {
	return c.PK
}

// GetSK returns the sort key
func (c *CircuitBreakerState) GetSK() string {
	return c.SK
}

// GetBackoffDuration returns the backoff duration as time.Duration
func (c *CircuitBreakerState) GetBackoffDuration() time.Duration {
	return time.Duration(c.BackoffDuration)
}

// SetBackoffDuration sets the backoff duration from time.Duration
func (c *CircuitBreakerState) SetBackoffDuration(d time.Duration) {
	c.BackoffDuration = int64(d)
}

// TableName returns the DynamoDB table backing CircuitBreakerState.
func (CircuitBreakerState) TableName() string {
	return MainTableName
}

// CircuitBreakerEvent represents a state change event for debugging and monitoring
type CircuitBreakerEvent struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// DynamoDB keys
	PK string `theorydb:"pk,attr:PK" json:"pk"` // CIRCUIT#<instance_id>
	SK string `theorydb:"sk,attr:SK" json:"sk"` // EVENT#<timestamp_nanos>

	// Event details
	InstanceID string    `theorydb:"attr:instanceID" json:"instance_id"`
	EventType  string    `theorydb:"attr:eventType" json:"event_type"` // state_change, metric
	NewStatus  string    `theorydb:"attr:newStatus" json:"new_status,omitempty"`
	OldStatus  string    `theorydb:"attr:oldStatus" json:"old_status,omitempty"`
	Reason     string    `theorydb:"attr:reason" json:"reason"`
	Timestamp  time.Time `theorydb:"attr:timestamp" json:"timestamp"`

	// For metric events
	Success   *bool  `theorydb:"attr:success" json:"success,omitempty"`
	Error     string `theorydb:"attr:error" json:"error,omitempty"`
	ErrorType string `theorydb:"attr:errorType" json:"error_type,omitempty"`

	// TTL for cleanup (7 days for events)
	TTL int64 `theorydb:"ttl,attr:ttl" json:"ttl"`
}

// UpdateKeys sets the DynamoDB keys for events
func (e *CircuitBreakerEvent) UpdateKeys() error {
	e.PK = fmt.Sprintf("CIRCUIT#%s", e.InstanceID)

	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now()
	}

	e.SK = fmt.Sprintf("EVENT#%d", e.Timestamp.UnixNano())

	// Set TTL to 7 days from now
	e.TTL = time.Now().Add(7 * 24 * time.Hour).Unix()
	return nil
}

// GetPK returns the partition key
func (e *CircuitBreakerEvent) GetPK() string {
	return e.PK
}

// GetSK returns the sort key
func (e *CircuitBreakerEvent) GetSK() string {
	return e.SK
}

// TableName returns the DynamoDB table backing CircuitBreakerEvent.
func (CircuitBreakerEvent) TableName() string {
	return MainTableName
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

// TableName returns the DynamoDB table backing CircuitBreakerConfig.
func (CircuitBreakerConfig) TableName() string {
	return MainTableName
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
