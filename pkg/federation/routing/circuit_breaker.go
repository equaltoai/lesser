package routing

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"go.uber.org/zap"
)

// DistributedCircuitBreaker implements circuit breaker pattern with DynamoDB persistence
type DistributedCircuitBreaker struct {
	db        *dynamodb.Client
	tableName string
	logger    *zap.Logger

	// Local state cache
	circuits sync.Map // instanceID -> *circuitState

	// Configuration
	config *CircuitBreakerConfig
}

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

type circuitState struct {
	InstanceID       string
	Status           CircuitStatus
	FailureCount     int
	SuccessCount     int
	ConsecutiveFails int
	LastFailure      time.Time
	LastSuccess      time.Time
	LastStateChange  time.Time
	NextRetry        time.Time
	BackoffDuration  time.Duration

	// Metrics
	TotalRequests  int64
	TotalFailures  int64
	TotalSuccesses int64

	mu sync.RWMutex
}

// NewDistributedCircuitBreaker creates a new circuit breaker
func NewDistributedCircuitBreaker(db *dynamodb.Client, tableName string, logger *zap.Logger, config *CircuitBreakerConfig) *DistributedCircuitBreaker {
	if config == nil {
		config = &CircuitBreakerConfig{
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

	dcb := &DistributedCircuitBreaker{
		db:        db,
		tableName: tableName,
		logger:    logger,
		config:    config,
	}

	// Start background sync
	go dcb.syncWithDynamoDB()

	// Start recovery checker
	go dcb.checkRecovery()

	return dcb
}

// Open opens the circuit for an instance
func (dcb *DistributedCircuitBreaker) Open(instanceID string, reason string) error {
	state := dcb.getOrCreateState(instanceID)

	state.mu.Lock()
	defer state.mu.Unlock()

	previousStatus := state.Status
	state.Status = CircuitOpen
	state.LastStateChange = time.Now()
	state.NextRetry = time.Now().Add(dcb.config.OpenTimeout)

	// Apply exponential backoff
	if state.BackoffDuration == 0 {
		state.BackoffDuration = dcb.config.OpenTimeout
	} else {
		state.BackoffDuration = time.Duration(float64(state.BackoffDuration) * dcb.config.BackoffMultiplier)
		if state.BackoffDuration > dcb.config.MaxBackoff {
			state.BackoffDuration = dcb.config.MaxBackoff
		}
	}

	// Persist to DynamoDB
	if err := dcb.persistState(state, reason); err != nil {
		return fmt.Errorf("persist circuit state: %w", err)
	}

	dcb.logger.Warn("circuit opened",
		zap.String("instanceID", instanceID),
		zap.String("previousStatus", string(previousStatus)),
		zap.String("reason", reason),
		zap.Duration("backoff", state.BackoffDuration))

	return nil
}

// Close closes the circuit for an instance
func (dcb *DistributedCircuitBreaker) Close(instanceID string) error {
	state := dcb.getOrCreateState(instanceID)

	state.mu.Lock()
	defer state.mu.Unlock()

	if state.Status == CircuitClosed {
		return nil // Already closed
	}

	state.Status = CircuitClosed
	state.LastStateChange = time.Now()
	state.FailureCount = 0
	state.SuccessCount = 0
	state.ConsecutiveFails = 0
	state.BackoffDuration = 0

	// Persist to DynamoDB
	if err := dcb.persistState(state, "circuit closed"); err != nil {
		return fmt.Errorf("persist circuit state: %w", err)
	}

	dcb.logger.Info("circuit closed",
		zap.String("instanceID", instanceID))

	return nil
}

// HalfOpen puts the circuit in half-open state for testing
func (dcb *DistributedCircuitBreaker) HalfOpen(instanceID string) error {
	state := dcb.getOrCreateState(instanceID)

	state.mu.Lock()
	defer state.mu.Unlock()

	if state.Status != CircuitOpen {
		return fmt.Errorf("circuit must be open to transition to half-open")
	}

	state.Status = CircuitHalfOpen
	state.LastStateChange = time.Now()
	state.NextRetry = time.Now().Add(dcb.config.HalfOpenTimeout)

	// Persist to DynamoDB
	if err := dcb.persistState(state, "testing recovery"); err != nil {
		return fmt.Errorf("persist circuit state: %w", err)
	}

	dcb.logger.Info("circuit half-open",
		zap.String("instanceID", instanceID))

	return nil
}

// IsOpen checks if the circuit is open
func (dcb *DistributedCircuitBreaker) IsOpen(instanceID string) bool {
	state := dcb.getOrCreateState(instanceID)

	state.mu.RLock()
	defer state.mu.RUnlock()

	return state.Status == CircuitOpen
}

// CanAttempt checks if a request can be attempted
func (dcb *DistributedCircuitBreaker) CanAttempt(instanceID string) bool {
	state := dcb.getOrCreateState(instanceID)

	state.mu.RLock()
	defer state.mu.RUnlock()

	switch state.Status {
	case CircuitClosed:
		return true

	case CircuitOpen:
		// Check if it's time to test
		if time.Now().After(state.NextRetry) {
			// Transition to half-open (async)
			go dcb.HalfOpen(instanceID)
			return true
		}
		return false

	case CircuitHalfOpen:
		// Allow limited attempts
		return true

	default:
		return true
	}
}

// RecordSuccess records a successful request
func (dcb *DistributedCircuitBreaker) RecordSuccess(instanceID string) error {
	state := dcb.getOrCreateState(instanceID)

	state.mu.Lock()
	defer state.mu.Unlock()

	state.SuccessCount++
	state.TotalSuccesses++
	state.TotalRequests++
	state.LastSuccess = time.Now()
	state.ConsecutiveFails = 0

	// Check state transitions
	switch state.Status {
	case CircuitHalfOpen:
		if state.SuccessCount >= dcb.config.SuccessThreshold {
			// Close the circuit
			state.Status = CircuitClosed
			state.LastStateChange = time.Now()
			state.FailureCount = 0
			state.SuccessCount = 0
			state.BackoffDuration = 0

			dcb.logger.Info("circuit recovered",
				zap.String("instanceID", instanceID))
		}

	case CircuitOpen:
		// Shouldn't happen, but handle gracefully
		state.Status = CircuitHalfOpen
		state.LastStateChange = time.Now()
	}

	// Update metrics in DynamoDB (async)
	go dcb.updateMetrics(instanceID, true, nil)

	return nil
}

// RecordFailure records a failed request
func (dcb *DistributedCircuitBreaker) RecordFailure(instanceID string, err error) error {
	state := dcb.getOrCreateState(instanceID)

	state.mu.Lock()
	defer state.mu.Unlock()

	state.FailureCount++
	state.TotalFailures++
	state.TotalRequests++
	state.ConsecutiveFails++
	state.LastFailure = time.Now()

	// Determine error type for better decision making
	errorType := dcb.classifyError(err)

	// Check state transitions
	switch state.Status {
	case CircuitClosed:
		// Check if we should open the circuit
		if state.ConsecutiveFails >= dcb.config.FailureThreshold {
			state.Status = CircuitOpen
			state.LastStateChange = time.Now()
			state.NextRetry = time.Now().Add(dcb.config.OpenTimeout)

			reason := fmt.Sprintf("consecutive failures: %d, error: %v", state.ConsecutiveFails, errorType)
			go dcb.persistState(state, reason)

			dcb.logger.Warn("circuit opened due to failures",
				zap.String("instanceID", instanceID),
				zap.Int("failures", state.ConsecutiveFails),
				zap.String("errorType", errorType))
		}

	case CircuitHalfOpen:
		// Single failure returns to open
		state.Status = CircuitOpen
		state.LastStateChange = time.Now()
		state.NextRetry = time.Now().Add(state.BackoffDuration)
		state.SuccessCount = 0

		reason := fmt.Sprintf("half-open test failed: %v", errorType)
		go dcb.persistState(state, reason)

		dcb.logger.Warn("circuit reopened",
			zap.String("instanceID", instanceID),
			zap.String("errorType", errorType))
	}

	// Update metrics in DynamoDB (async)
	go dcb.updateMetrics(instanceID, false, err)

	return nil
}

// GetStatus returns the current circuit status
func (dcb *DistributedCircuitBreaker) GetStatus(instanceID string) CircuitStatus {
	state := dcb.getOrCreateState(instanceID)

	state.mu.RLock()
	defer state.mu.RUnlock()

	return state.Status
}

// GetMetrics returns circuit breaker metrics
func (dcb *DistributedCircuitBreaker) GetMetrics(instanceID string) map[string]interface{} {
	state := dcb.getOrCreateState(instanceID)

	state.mu.RLock()
	defer state.mu.RUnlock()

	successRate := float64(0)
	if state.TotalRequests > 0 {
		successRate = float64(state.TotalSuccesses) / float64(state.TotalRequests)
	}

	return map[string]interface{}{
		"status":           state.Status,
		"totalRequests":    state.TotalRequests,
		"totalFailures":    state.TotalFailures,
		"totalSuccesses":   state.TotalSuccesses,
		"successRate":      successRate,
		"consecutiveFails": state.ConsecutiveFails,
		"lastFailure":      state.LastFailure,
		"lastSuccess":      state.LastSuccess,
		"nextRetry":        state.NextRetry,
		"backoffDuration":  state.BackoffDuration,
	}
}

// Helper methods

func (dcb *DistributedCircuitBreaker) getOrCreateState(instanceID string) *circuitState {
	if state, ok := dcb.circuits.Load(instanceID); ok {
		return state.(*circuitState)
	}

	// Create new state
	state := &circuitState{
		InstanceID:      instanceID,
		Status:          CircuitClosed,
		LastStateChange: time.Now(),
	}

	// Try to load from DynamoDB
	if err := dcb.loadState(state); err != nil {
		dcb.logger.Debug("no existing circuit state",
			zap.String("instanceID", instanceID))
	}

	actual, _ := dcb.circuits.LoadOrStore(instanceID, state)
	return actual.(*circuitState)
}

func (dcb *DistributedCircuitBreaker) persistState(state *circuitState, reason string) error {
	item := map[string]types.AttributeValue{
		"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("CIRCUIT#%s", state.InstanceID)},
		"SK": &types.AttributeValueMemberS{Value: "STATE"},

		"Status":           &types.AttributeValueMemberS{Value: string(state.Status)},
		"FailureCount":     &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", state.FailureCount)},
		"SuccessCount":     &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", state.SuccessCount)},
		"ConsecutiveFails": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", state.ConsecutiveFails)},
		"LastStateChange":  &types.AttributeValueMemberS{Value: state.LastStateChange.Format(time.RFC3339)},
		"NextRetry":        &types.AttributeValueMemberS{Value: state.NextRetry.Format(time.RFC3339)},
		"BackoffDuration":  &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", state.BackoffDuration.Nanoseconds())},
		"Reason":           &types.AttributeValueMemberS{Value: reason},

		// Metrics
		"TotalRequests":  &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", state.TotalRequests)},
		"TotalFailures":  &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", state.TotalFailures)},
		"TotalSuccesses": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", state.TotalSuccesses)},

		// TTL for cleanup (30 days after last change)
		"TTL": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", state.LastStateChange.Add(30*24*time.Hour).Unix())},
	}

	if !state.LastFailure.IsZero() {
		item["LastFailure"] = &types.AttributeValueMemberS{Value: state.LastFailure.Format(time.RFC3339)}
	}
	if !state.LastSuccess.IsZero() {
		item["LastSuccess"] = &types.AttributeValueMemberS{Value: state.LastSuccess.Format(time.RFC3339)}
	}

	// Store state change event
	eventItem := map[string]types.AttributeValue{
		"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("CIRCUIT#%s", state.InstanceID)},
		"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("EVENT#%d", time.Now().UnixNano())},

		"EventType": &types.AttributeValueMemberS{Value: "state_change"},
		"NewStatus": &types.AttributeValueMemberS{Value: string(state.Status)},
		"Reason":    &types.AttributeValueMemberS{Value: reason},
		"Timestamp": &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},

		// TTL for cleanup (7 days)
		"TTL": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", time.Now().Add(7*24*time.Hour).Unix())},
	}

	// Use transaction to ensure consistency
	transactInput := &dynamodb.TransactWriteItemsInput{
		TransactItems: []types.TransactWriteItem{
			{
				Put: &types.Put{
					TableName: aws.String(dcb.tableName),
					Item:      item,
				},
			},
			{
				Put: &types.Put{
					TableName: aws.String(dcb.tableName),
					Item:      eventItem,
				},
			},
		},
	}

	_, err := dcb.db.TransactWriteItems(context.Background(), transactInput)
	return err
}

func (dcb *DistributedCircuitBreaker) loadState(state *circuitState) error {
	getInput := &dynamodb.GetItemInput{
		TableName: aws.String(dcb.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("CIRCUIT#%s", state.InstanceID)},
			"SK": &types.AttributeValueMemberS{Value: "STATE"},
		},
	}

	result, err := dcb.db.GetItem(context.Background(), getInput)
	if err != nil {
		return err
	}

	if result.Item == nil {
		return fmt.Errorf("state not found")
	}

	// Parse state
	if v, ok := result.Item["Status"].(*types.AttributeValueMemberS); ok {
		state.Status = CircuitStatus(v.Value)
	}
	if v, ok := result.Item["FailureCount"].(*types.AttributeValueMemberN); ok {
		if _, err := fmt.Sscanf(v.Value, "%d", &state.FailureCount); err != nil {
			dcb.logger.Warn("failed to parse FailureCount", zap.String("value", v.Value), zap.Error(err))
		}
	}
	if v, ok := result.Item["SuccessCount"].(*types.AttributeValueMemberN); ok {
		if _, err := fmt.Sscanf(v.Value, "%d", &state.SuccessCount); err != nil {
			dcb.logger.Warn("failed to parse SuccessCount", zap.String("value", v.Value), zap.Error(err))
		}
	}
	if v, ok := result.Item["ConsecutiveFails"].(*types.AttributeValueMemberN); ok {
		if _, err := fmt.Sscanf(v.Value, "%d", &state.ConsecutiveFails); err != nil {
			dcb.logger.Warn("failed to parse ConsecutiveFails", zap.String("value", v.Value), zap.Error(err))
		}
	}
	if v, ok := result.Item["TotalRequests"].(*types.AttributeValueMemberN); ok {
		if _, err := fmt.Sscanf(v.Value, "%d", &state.TotalRequests); err != nil {
			dcb.logger.Warn("failed to parse TotalRequests", zap.String("value", v.Value), zap.Error(err))
		}
	}
	if v, ok := result.Item["TotalFailures"].(*types.AttributeValueMemberN); ok {
		if _, err := fmt.Sscanf(v.Value, "%d", &state.TotalFailures); err != nil {
			dcb.logger.Warn("failed to parse TotalFailures", zap.String("value", v.Value), zap.Error(err))
		}
	}
	if v, ok := result.Item["TotalSuccesses"].(*types.AttributeValueMemberN); ok {
		if _, err := fmt.Sscanf(v.Value, "%d", &state.TotalSuccesses); err != nil {
			dcb.logger.Warn("failed to parse TotalSuccesses", zap.String("value", v.Value), zap.Error(err))
		}
	}

	// Parse timestamps
	if v, ok := result.Item["LastStateChange"].(*types.AttributeValueMemberS); ok {
		state.LastStateChange, _ = time.Parse(time.RFC3339, v.Value)
	}
	if v, ok := result.Item["NextRetry"].(*types.AttributeValueMemberS); ok {
		state.NextRetry, _ = time.Parse(time.RFC3339, v.Value)
	}
	if v, ok := result.Item["LastFailure"].(*types.AttributeValueMemberS); ok {
		state.LastFailure, _ = time.Parse(time.RFC3339, v.Value)
	}
	if v, ok := result.Item["LastSuccess"].(*types.AttributeValueMemberS); ok {
		state.LastSuccess, _ = time.Parse(time.RFC3339, v.Value)
	}

	// Parse backoff duration
	if v, ok := result.Item["BackoffDuration"].(*types.AttributeValueMemberN); ok {
		var nanos int64
		if _, err := fmt.Sscanf(v.Value, "%d", &nanos); err != nil {
			dcb.logger.Warn("failed to parse BackoffDuration", zap.String("value", v.Value), zap.Error(err))
		}
		state.BackoffDuration = time.Duration(nanos)
	}

	return nil
}

func (dcb *DistributedCircuitBreaker) updateMetrics(instanceID string, success bool, err error) {
	// Store metric event
	item := map[string]types.AttributeValue{
		"PK":        &types.AttributeValueMemberS{Value: fmt.Sprintf("CIRCUIT#%s", instanceID)},
		"SK":        &types.AttributeValueMemberS{Value: fmt.Sprintf("METRIC#%d", time.Now().UnixNano())},
		"Success":   &types.AttributeValueMemberBOOL{Value: success},
		"Timestamp": &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
		"TTL":       &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", time.Now().Add(24*time.Hour).Unix())},
	}

	if err != nil {
		item["Error"] = &types.AttributeValueMemberS{Value: err.Error()}
		item["ErrorType"] = &types.AttributeValueMemberS{Value: dcb.classifyError(err)}
	}

	putInput := &dynamodb.PutItemInput{
		TableName: aws.String(dcb.tableName),
		Item:      item,
	}

	_, putErr := dcb.db.PutItem(context.Background(), putInput)
	if putErr != nil {
		dcb.logger.Warn("failed to store metric", zap.Error(putErr))
	}
}

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

func (dcb *DistributedCircuitBreaker) syncWithDynamoDB() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		// Sync local state with DynamoDB
		dcb.circuits.Range(func(key, value interface{}) bool {
			state := value.(*circuitState)

			// Only sync if there have been changes
			state.mu.RLock()
			needsSync := time.Since(state.LastStateChange) < time.Minute
			state.mu.RUnlock()

			if needsSync {
				if err := dcb.loadState(state); err != nil {
					dcb.logger.Debug("sync failed",
						zap.String("instanceID", state.InstanceID),
						zap.Error(err))
				}
			}

			return true
		})
	}
}

func (dcb *DistributedCircuitBreaker) checkRecovery() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		dcb.circuits.Range(func(key, value interface{}) bool {
			state := value.(*circuitState)

			state.mu.RLock()
			shouldRecover := state.Status == CircuitOpen && time.Now().After(state.NextRetry)
			instanceID := state.InstanceID
			state.mu.RUnlock()

			if shouldRecover {
				if err := dcb.HalfOpen(instanceID); err != nil {
					dcb.logger.Warn("failed to transition to half-open",
						zap.String("instanceID", instanceID),
						zap.Error(err))
				}
			}

			return true
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[0:len(substr)] == substr || len(s) > len(substr) && contains(s[1:], substr)
}
