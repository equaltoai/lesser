// Package streaming provides error recovery mechanisms for WebSocket connections
package streaming

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/theory-cloud/apptheory/pkg/streamer"
	"go.uber.org/zap"
)

// JobQueue defines the interface for job queue operations to avoid import cycles
type JobQueue interface {
	QueueDelayedJob(ctx context.Context, queueName string, messageBody interface{}, delaySeconds int32) error
}

// ErrorRecoveryManager handles connection error recovery and reconnection strategies
type ErrorRecoveryManager struct {
	connRepo  interfaces.StreamingConnectionRepository
	apiClient streamer.Client
	jobQueue  JobQueue
	logger    *zap.Logger

	// Configuration
	maxRetries     int
	baseRetryDelay time.Duration
	maxRetryDelay  time.Duration
	jitterFactor   float64
	circuitBreaker *CircuitBreaker
	enableBackoff  bool
}

// RetryJobMessage represents a streaming connection retry job
type RetryJobMessage struct {
	ConnectionID  string `json:"connection_id"`
	RetryCount    int    `json:"retry_count"`
	Timestamp     int64  `json:"timestamp"`
	OriginalError string `json:"original_error,omitempty"`
}

// ErrorRecoveryConfig contains configuration for error recovery
type ErrorRecoveryConfig struct {
	MaxRetries     int           // Maximum number of retry attempts
	BaseRetryDelay time.Duration // Base delay between retries
	MaxRetryDelay  time.Duration // Maximum delay between retries
	JitterFactor   float64       // Jitter factor for randomizing delays (0.0-1.0)
	EnableBackoff  bool          // Enable exponential backoff
}

// DefaultErrorRecoveryConfig returns default error recovery configuration
func DefaultErrorRecoveryConfig() *ErrorRecoveryConfig {
	return &ErrorRecoveryConfig{
		MaxRetries:     5,
		BaseRetryDelay: time.Second * 2,
		MaxRetryDelay:  time.Minute * 5,
		JitterFactor:   0.1,
		EnableBackoff:  true,
	}
}

// CircuitBreakerState represents the current state of a circuit breaker
type CircuitBreakerState int

const (
	// CircuitBreakerClosed indicates the circuit breaker is closed (normal operation)
	CircuitBreakerClosed CircuitBreakerState = iota
	// CircuitBreakerOpen indicates the circuit breaker is open (blocking requests)
	CircuitBreakerOpen
	// CircuitBreakerHalfOpen indicates the circuit breaker is testing if service is recovered
	CircuitBreakerHalfOpen
)

// CircuitBreaker implements the circuit breaker pattern for connection recovery
type CircuitBreaker struct {
	state            CircuitBreakerState
	failures         int
	maxFailures      int
	timeout          time.Duration
	lastFailureTime  time.Time
	successCount     int
	halfOpenMaxTries int
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(maxFailures int, timeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		state:            CircuitBreakerClosed,
		maxFailures:      maxFailures,
		timeout:          timeout,
		halfOpenMaxTries: 3,
	}
}

// CanExecute returns whether the circuit breaker allows execution
func (cb *CircuitBreaker) CanExecute() bool {
	switch cb.state {
	case CircuitBreakerClosed:
		return true
	case CircuitBreakerOpen:
		// Check if timeout has elapsed
		if time.Since(cb.lastFailureTime) > cb.timeout {
			cb.state = CircuitBreakerHalfOpen
			cb.successCount = 0
			return true
		}
		return false
	case CircuitBreakerHalfOpen:
		return cb.successCount < cb.halfOpenMaxTries
	}
	return false
}

// RecordSuccess records a successful operation
func (cb *CircuitBreaker) RecordSuccess() {
	switch cb.state {
	case CircuitBreakerClosed:
		cb.failures = 0
	case CircuitBreakerHalfOpen:
		cb.successCount++
		if cb.successCount >= cb.halfOpenMaxTries {
			cb.state = CircuitBreakerClosed
			cb.failures = 0
		}
	}
}

// RecordFailure records a failed operation
func (cb *CircuitBreaker) RecordFailure() {
	cb.failures++
	cb.lastFailureTime = time.Now()

	switch cb.state {
	case CircuitBreakerClosed:
		if cb.failures >= cb.maxFailures {
			cb.state = CircuitBreakerOpen
		}
	case CircuitBreakerHalfOpen:
		cb.state = CircuitBreakerOpen
	}
}

// GetState returns the current circuit breaker state
func (cb *CircuitBreaker) GetState() CircuitBreakerState {
	return cb.state
}

// NewErrorRecoveryManager creates a new error recovery manager
func NewErrorRecoveryManager(
	connRepo interfaces.StreamingConnectionRepository,
	apiClient streamer.Client,
	jobQueue JobQueue,
	logger *zap.Logger,
	config *ErrorRecoveryConfig,
) *ErrorRecoveryManager {
	if config == nil {
		config = DefaultErrorRecoveryConfig()
	}

	if logger == nil {
		logger = zap.NewNop()
	}

	return &ErrorRecoveryManager{
		connRepo:       connRepo,
		apiClient:      apiClient,
		jobQueue:       jobQueue,
		logger:         logger.With(zap.String("component", "error_recovery")),
		maxRetries:     config.MaxRetries,
		baseRetryDelay: config.BaseRetryDelay,
		maxRetryDelay:  config.MaxRetryDelay,
		jitterFactor:   config.JitterFactor,
		enableBackoff:  config.EnableBackoff,
		circuitBreaker: NewCircuitBreaker(10, time.Minute*5), // 10 failures in 5 minutes trips breaker
	}
}

// HandleConnectionError processes a connection error and determines recovery strategy
func (erm *ErrorRecoveryManager) HandleConnectionError(ctx context.Context, connectionID string, err error) error {
	erm.logger.Info("handling connection error",
		zap.String("connection_id", connectionID),
		zap.Error(err))

	// Get the connection
	conn, getErr := erm.connRepo.GetConnection(ctx, connectionID)
	if getErr != nil {
		return errors.FailedToGet("streaming connection", getErr).WithMetadata("connection_id", connectionID)
	}

	// Record the error
	if recordErr := erm.connRepo.RecordConnectionError(ctx, connectionID, err.Error()); recordErr != nil {
		erm.logger.Error("failed to record connection error",
			zap.String("connection_id", connectionID),
			zap.Error(recordErr))
	}

	// Determine if we should attempt recovery
	if !erm.shouldAttemptRecovery(conn, err) {
		erm.logger.Info("not attempting recovery for connection",
			zap.String("connection_id", connectionID),
			zap.String("reason", "recovery criteria not met"))
		recoveryErr := errors.StreamingRecoveryFailed(connectionID, conn.RetryCount, err).AsNonRetryable()
		return erm.markConnectionClosedWithError(ctx, connectionID, recoveryErr)
	}

	// Check circuit breaker
	if !erm.circuitBreaker.CanExecute() {
		erm.logger.Info("circuit breaker preventing recovery attempt",
			zap.String("connection_id", connectionID),
			zap.String("breaker_state", fmt.Sprintf("%d", erm.circuitBreaker.GetState())))
		cbErr := errors.StreamingCircuitBreakerOpen(connectionID).WithMetadata("breaker_state", fmt.Sprintf("%d", erm.circuitBreaker.GetState()))
		return erm.markConnectionClosedWithError(ctx, connectionID, cbErr)
	}

	// Attempt recovery
	return erm.attemptRecovery(ctx, conn, err)
}

// shouldAttemptRecovery determines if recovery should be attempted for a connection
func (erm *ErrorRecoveryManager) shouldAttemptRecovery(conn *models.WebSocketConnection, err error) bool {
	// Don't attempt recovery if connection has exceeded retry limit
	if conn.RetryCount >= conn.MaxRetries {
		return false
	}

	// Don't attempt recovery for certain permanent error types
	// Use centralized error code checking instead of string matching
	if appErr, ok := errors.AsAppError(err); ok {
		switch appErr.Code {
		case errors.CodeAuthFailed, errors.CodeUnauthorized, errors.CodeForbidden,
			errors.CodeAccountSuspended, errors.CodeTooManyConnections, errors.CodeRateLimited:
			return false
		}
	} else {
		// Fallback to string matching for non-AppError errors
		errorStr := err.Error()
		permanentErrors := []string{
			"authentication failed",
			"unauthorized",
			"forbidden",
			"connection limit exceeded",
			"rate limit exceeded",
		}

		for _, permErr := range permanentErrors {
			if len(errorStr) > len(permErr) && errorStr[:len(permErr)] == permErr {
				return false
			}
		}
	}

	// Check connection health and quality
	if !conn.IsHealthy() && conn.Metrics.ConnectionQuality < 0.3 {
		return false
	}

	return true
}

// attemptRecovery attempts to recover a connection with exponential backoff
func (erm *ErrorRecoveryManager) attemptRecovery(ctx context.Context, conn *models.WebSocketConnection, err error) error {
	conn.RetryCount++

	// Calculate retry delay with exponential backoff and jitter
	delay := erm.calculateRetryDelay(conn.RetryCount)

	erm.logger.Info("scheduling connection recovery",
		zap.String("connection_id", conn.ConnectionID),
		zap.Int("retry_count", conn.RetryCount),
		zap.Duration("delay", delay))

	// Update connection state
	conn.UpdateState(models.ConnectionStateError)
	conn.CloseReason = fmt.Sprintf("Connection error, retry %d/%d: %v", conn.RetryCount, conn.MaxRetries, err)

	if updateErr := erm.connRepo.UpdateConnection(ctx, conn); updateErr != nil {
		erm.logger.Error("failed to update connection for retry",
			zap.String("connection_id", conn.ConnectionID),
			zap.Error(updateErr))
	}

	// Schedule retry using SQS job queue for reliability
	if err := erm.scheduleRetryJob(ctx, conn.ConnectionID, conn.RetryCount, delay, err); err != nil {
		erm.logger.Error("failed to schedule retry job, falling back to goroutine",
			zap.String("connection_id", conn.ConnectionID),
			zap.Error(err))

		// Fallback to goroutine if job queue fails
		go func() {
			time.Sleep(delay)
			erm.executeRecovery(context.Background(), conn.ConnectionID)
		}()
	}

	return nil
}

// calculateRetryDelay calculates the delay before the next retry attempt
func (erm *ErrorRecoveryManager) calculateRetryDelay(retryCount int) time.Duration {
	if !erm.enableBackoff {
		return erm.baseRetryDelay
	}

	// Exponential backoff: baseDelay * 2^(retryCount-1)
	delay := float64(erm.baseRetryDelay) * math.Pow(2, float64(retryCount-1))

	// Apply jitter to avoid thundering herd
	// #nosec G404 - Using math/rand for jitter is acceptable for backoff timing
	jitter := delay * erm.jitterFactor * (2*rand.Float64() - 1) // Random between -jitterFactor and +jitterFactor
	delay += jitter

	// Clamp to maximum delay
	if time.Duration(delay) > erm.maxRetryDelay {
		delay = float64(erm.maxRetryDelay)
	}

	return time.Duration(delay)
}

// executeRecovery executes the actual recovery attempt
func (erm *ErrorRecoveryManager) executeRecovery(ctx context.Context, connectionID string) {
	erm.logger.Info("executing connection recovery",
		zap.String("connection_id", connectionID))

	// Get current connection state
	conn, err := erm.connRepo.GetConnection(ctx, connectionID)
	if err != nil {
		erm.logger.Error("failed to get connection for recovery",
			zap.String("connection_id", connectionID),
			zap.Error(err))
		erm.circuitBreaker.RecordFailure()
		return
	}

	// Check if connection is still in error state
	if conn.State != models.ConnectionStateError {
		erm.logger.Info("connection no longer in error state, skipping recovery",
			zap.String("connection_id", connectionID),
			zap.String("current_state", string(conn.State)))
		return
	}

	// Attempt to restore connection to connecting state
	// For serverless WebSocket (AWS API Gateway), we implement recovery by:
	// 1. Validating connection health via ping
	// 2. Resynchronizing connection state
	// 3. Re-establishing subscriptions if needed
	conn.UpdateState(models.ConnectionStateConnecting)
	conn.CloseReason = "Recovery attempt in progress"

	// Record recovery attempt for metrics
	erm.recordRecoveryAttempt(ctx, conn)

	if err := erm.connRepo.UpdateConnection(ctx, conn); err != nil {
		erm.logger.Error("failed to update connection during recovery",
			zap.String("connection_id", connectionID),
			zap.Error(err))
		erm.circuitBreaker.RecordFailure()
		return
	}

	// Test connection health by sending a ping message
	// In serverless WebSocket, we can't "reconnect" - we can only validate if connection is still alive
	recoverySuccess := erm.validateConnectionHealth(ctx, conn)

	if recoverySuccess {
		erm.handleRecoverySuccess(ctx, conn)
		erm.circuitBreaker.RecordSuccess()
	} else {
		erm.handleRecoveryFailure(ctx, conn)
		erm.circuitBreaker.RecordFailure()
	}
}

// validateConnectionHealth tests if the WebSocket connection is still alive by sending a ping
func (erm *ErrorRecoveryManager) validateConnectionHealth(ctx context.Context, conn *models.WebSocketConnection) bool {
	if erm.apiClient == nil {
		erm.logger.Warn("no API client available, falling back to simulation",
			zap.String("connection_id", conn.ConnectionID))
		return erm.simulateRecoveryAttempt(conn)
	}

	// Create a ping message to test connection health
	pingMessage := map[string]interface{}{
		"type":      "ping",
		"timestamp": time.Now().Unix(),
		"id":        fmt.Sprintf("recovery_%d", time.Now().UnixNano()),
	}

	messageBytes, err := json.Marshal(pingMessage)
	if err != nil {
		erm.logger.Error("failed to marshal ping message",
			zap.String("connection_id", conn.ConnectionID),
			zap.Error(err))
		return false
	}

	// Try to send ping message to validate connection
	err = erm.apiClient.PostToConnection(ctx, conn.ConnectionID, messageBytes)

	if err != nil {
		erm.logger.Debug("connection health check failed",
			zap.String("connection_id", conn.ConnectionID),
			zap.Error(err))
		return false
	}

	erm.logger.Debug("connection health check passed",
		zap.String("connection_id", conn.ConnectionID))
	return true
}

// simulateRecoveryAttempt simulates a recovery attempt (fallback when API client unavailable)
func (erm *ErrorRecoveryManager) simulateRecoveryAttempt(conn *models.WebSocketConnection) bool {
	// Simple simulation based on connection quality and retry count
	baseSuccessRate := conn.Metrics.ConnectionQuality
	retryPenalty := float64(conn.RetryCount) * 0.1
	successRate := baseSuccessRate - retryPenalty

	if successRate < 0.1 {
		successRate = 0.1 // Minimum 10% chance
	}

	// #nosec G404 - Using math/rand for probabilistic success simulation is acceptable
	return rand.Float64() < successRate
}

// handleRecoverySuccess handles successful connection recovery
func (erm *ErrorRecoveryManager) handleRecoverySuccess(ctx context.Context, conn *models.WebSocketConnection) {
	erm.logger.Info("connection recovery succeeded",
		zap.String("connection_id", conn.ConnectionID),
		zap.Int("retry_count", conn.RetryCount))

	// Reset connection to healthy state
	conn.UpdateState(models.ConnectionStateConnected)
	conn.CloseReason = ""
	conn.LastActivity = time.Now()
	conn.RetryCount = 0               // Reset retry count on success
	conn.Metrics.ErrorCount = 0       // Reset error count
	conn.CalculateConnectionQuality() // Recalculate quality

	if err := erm.connRepo.UpdateConnection(ctx, conn); err != nil {
		erm.logger.Error("failed to update connection after successful recovery",
			zap.String("connection_id", conn.ConnectionID),
			zap.Error(err))
		return
	}

	// Attempt to resynchronize connection state
	if err := erm.ResynchronizeConnection(ctx, conn); err != nil {
		erm.logger.Warn("connection recovered but resynchronization failed",
			zap.String("connection_id", conn.ConnectionID),
			zap.Error(err))
		// Don't fail the recovery - connection is still functional
	}
}

// handleRecoveryFailure handles failed connection recovery
func (erm *ErrorRecoveryManager) handleRecoveryFailure(ctx context.Context, conn *models.WebSocketConnection) {
	erm.logger.Info("connection recovery failed",
		zap.String("connection_id", conn.ConnectionID),
		zap.Int("retry_count", conn.RetryCount),
		zap.Int("max_retries", conn.MaxRetries))

	if conn.RetryCount >= conn.MaxRetries {
		// Max retries exceeded, mark connection as closed
		maxRetriesErr := errors.StreamingRecoveryFailed(conn.ConnectionID, conn.RetryCount, errors.NewAppError(errors.CodeInternal, errors.CategoryStreaming, "max retries exceeded")).
			WithMetadata("max_retries", conn.MaxRetries).AsNonRetryable()
		_ = erm.markConnectionClosedWithError(ctx, conn.ConnectionID, maxRetriesErr) // Database errors logged internally
	} else {
		// Schedule another retry
		conn.UpdateState(models.ConnectionStateError)
		conn.CloseReason = fmt.Sprintf("Recovery failed, retry %d/%d", conn.RetryCount, conn.MaxRetries)

		if err := erm.connRepo.UpdateConnection(ctx, conn); err != nil {
			erm.logger.Error("failed to update connection after recovery failure",
				zap.String("connection_id", conn.ConnectionID),
				zap.Error(err))
		}

		// Schedule next retry using SQS job queue
		delay := erm.calculateRetryDelay(conn.RetryCount + 1)
		recoveryError := errors.NewAppError(errors.CodeConnectionClosed, errors.CategoryStreaming, "Recovery attempt failed").
			WithMetadata("connection_id", conn.ConnectionID).
			WithMetadata("retry_count", conn.RetryCount)
		if err := erm.scheduleRetryJob(ctx, conn.ConnectionID, conn.RetryCount, delay, recoveryError); err != nil {
			erm.logger.Error("failed to schedule next retry job, using fallback goroutine",
				zap.String("connection_id", conn.ConnectionID),
				zap.Error(err))

			// Fallback to goroutine if job queue fails
			go func() {
				time.Sleep(delay)
				erm.executeRecovery(context.Background(), conn.ConnectionID)
			}()
		}
	}
}

// markConnectionClosedWithError marks a connection as permanently closed with error details
func (erm *ErrorRecoveryManager) markConnectionClosedWithError(ctx context.Context, connectionID string, appErr *errors.AppError) error {
	conn, err := erm.connRepo.GetConnection(ctx, connectionID)
	if err != nil {
		erm.logger.Error("failed to get connection for closing", zap.Error(err))
		return appErr // Return original error even if DB update fails
	}

	conn.UpdateState(models.ConnectionStateClosed)
	conn.CloseReason = appErr.Error()
	conn.CloseCode = 1011 // Internal Error

	if err := erm.connRepo.UpdateConnection(ctx, conn); err != nil {
		erm.logger.Error("failed to update connection status during close",
			zap.String("connection_id", connectionID),
			zap.Error(err))
		// Continue - we still want to return the original error
	}

	erm.logger.Info("connection marked as closed with error",
		zap.String("connection_id", connectionID),
		zap.String("error_code", string(appErr.Code)),
		zap.String("error_category", string(appErr.Category)),
		zap.String("reason", appErr.Error()))

	// Return the original error for upstream handling
	return appErr
}

// scheduleRetryJob schedules a retry job using SQS queue for reliability
func (erm *ErrorRecoveryManager) scheduleRetryJob(ctx context.Context, connectionID string, retryCount int, delay time.Duration, originalErr error) error {
	if erm.jobQueue == nil {
		return errors.ConfigurationMissing("streaming retry job queue").WithMetadata("connection_id", connectionID)
	}

	retryMsg := RetryJobMessage{
		ConnectionID:  connectionID,
		RetryCount:    retryCount,
		Timestamp:     time.Now().Unix(),
		OriginalError: originalErr.Error(),
	}

	// Convert delay to seconds for SQS DelaySeconds (max 900 seconds = 15 minutes)
	delaySeconds := int32(delay.Seconds())
	maxSQSDelay := int32(900) // 15 minutes maximum for SQS
	if delaySeconds > maxSQSDelay {
		delaySeconds = maxSQSDelay
	}

	err := erm.jobQueue.QueueDelayedJob(ctx, "streaming-retry", retryMsg, delaySeconds)
	if err != nil {
		return errors.NewAppError(errors.CodeSQSProcessingFailed, errors.CategoryLambda, "Failed to queue streaming retry job").
			WithInternalError(err).
			WithMetadata("connection_id", connectionID).
			WithMetadata("retry_count", retryCount).
			AsRetryable()
	}

	erm.logger.Info("scheduled retry job for connection",
		zap.String("connection_id", connectionID),
		zap.Int("retry_count", retryCount),
		zap.Duration("delay", delay),
		zap.Int32("sqs_delay_seconds", delaySeconds))

	return nil
}

// ProcessRetryJob processes a retry job from the queue
func (erm *ErrorRecoveryManager) ProcessRetryJob(ctx context.Context, msg RetryJobMessage) error {
	erm.logger.Info("processing retry job",
		zap.String("connection_id", msg.ConnectionID),
		zap.Int("retry_count", msg.RetryCount),
		zap.Int64("original_timestamp", msg.Timestamp))

	// Validate connection still exists and is in error state
	conn, err := erm.connRepo.GetConnection(ctx, msg.ConnectionID)
	if err != nil {
		erm.logger.Warn("connection not found for retry job, skipping",
			zap.String("connection_id", msg.ConnectionID),
			zap.Error(err))
		return nil // Don't error - connection may have been cleaned up
	}

	// Only process if connection is still in error state and retry count matches
	if conn.State != models.ConnectionStateError {
		erm.logger.Info("connection no longer in error state, skipping retry job",
			zap.String("connection_id", msg.ConnectionID),
			zap.String("current_state", string(conn.State)))
		return nil
	}

	if conn.RetryCount > msg.RetryCount {
		erm.logger.Info("connection retry count has advanced, skipping stale retry job",
			zap.String("connection_id", msg.ConnectionID),
			zap.Int("msg_retry_count", msg.RetryCount),
			zap.Int("current_retry_count", conn.RetryCount))
		return nil
	}

	// Execute the recovery attempt
	erm.executeRecovery(ctx, msg.ConnectionID)
	return nil
}

// recordRecoveryAttempt records metrics for a recovery attempt
func (erm *ErrorRecoveryManager) recordRecoveryAttempt(_ context.Context, conn *models.WebSocketConnection) {
	erm.logger.Debug("recording recovery attempt",
		zap.String("connection_id", conn.ConnectionID),
		zap.Int("retry_count", conn.RetryCount),
		zap.Float64("connection_quality", conn.Metrics.ConnectionQuality))

	// Update connection metrics (using existing fields)
	conn.Metrics.ErrorCount = 0    // Reset error count on recovery attempt
	conn.LastActivity = time.Now() // Update activity time
}

// ResynchronizeConnection attempts to resynchronize a recovered connection
func (erm *ErrorRecoveryManager) ResynchronizeConnection(ctx context.Context, conn *models.WebSocketConnection) error {
	erm.logger.Info("resynchronizing connection after recovery",
		zap.String("connection_id", conn.ConnectionID))

	// Send connection state sync message
	syncMessage := map[string]interface{}{
		"type":      "connection_sync",
		"timestamp": time.Now().Unix(),
		"user_id":   conn.UserID,
		"streams":   conn.Streams,
		"quality":   conn.Metrics.ConnectionQuality,
	}

	if erm.apiClient != nil {
		messageBytes, err := json.Marshal(syncMessage)
		if err != nil {
			return errors.MarshalingFailed("streaming sync message", err).WithMetadata("connection_id", conn.ConnectionID)
		}

		err = erm.apiClient.PostToConnection(ctx, conn.ConnectionID, messageBytes)

		if err != nil {
			erm.logger.Warn("failed to send sync message, connection may be stale",
				zap.String("connection_id", conn.ConnectionID),
				zap.Error(err))
			return errors.StreamingSyncFailed(conn.ConnectionID, err)
		}

		erm.logger.Debug("sync message sent successfully",
			zap.String("connection_id", conn.ConnectionID))
	}

	return nil
}

// PerformHealthCheck performs a comprehensive health check on a connection
func (erm *ErrorRecoveryManager) PerformHealthCheck(ctx context.Context, connectionID string) (*HealthCheckResult, error) {
	conn, err := erm.connRepo.GetConnection(ctx, connectionID)
	if err != nil {
		return nil, errors.FailedToGet("streaming connection for health check", err).WithMetadata("connection_id", connectionID)
	}

	result := &HealthCheckResult{
		ConnectionID:      connectionID,
		Timestamp:         time.Now(),
		IsHealthy:         false,
		QualityScore:      conn.Metrics.ConnectionQuality,
		LatencyMs:         0,
		PacketLoss:        0,
		RecommendedAction: "none",
	}

	// Test connection with ping
	pingSuccess := erm.validateConnectionHealth(ctx, conn)
	if !pingSuccess {
		result.RecommendedAction = "disconnect"
		return result, nil
	}

	// Calculate health metrics
	result.IsHealthy = conn.IsHealthy()
	result.LatencyMs = erm.measureConnectionLatency(ctx, conn)
	result.PacketLoss = erm.calculatePacketLoss(conn)

	// Determine recommended action based on health
	if result.QualityScore < 0.3 {
		result.RecommendedAction = "consider_disconnect"
	} else if result.QualityScore < 0.6 {
		result.RecommendedAction = "monitor"
	} else {
		result.RecommendedAction = "healthy"
	}

	return result, nil
}

// HealthCheckResult contains the results of a connection health check
type HealthCheckResult struct {
	ConnectionID      string    `json:"connection_id"`
	Timestamp         time.Time `json:"timestamp"`
	IsHealthy         bool      `json:"is_healthy"`
	QualityScore      float64   `json:"quality_score"`
	LatencyMs         int64     `json:"latency_ms"`
	PacketLoss        float64   `json:"packet_loss"`
	RecommendedAction string    `json:"recommended_action"`
}

// measureConnectionLatency measures round-trip latency for a connection
func (erm *ErrorRecoveryManager) measureConnectionLatency(ctx context.Context, conn *models.WebSocketConnection) int64 {
	if erm.apiClient == nil {
		return 0
	}

	start := time.Now()

	// Send a latency test message
	testMessage := map[string]interface{}{
		"type":      "latency_test",
		"timestamp": start.UnixNano(),
		"test_id":   fmt.Sprintf("lat_%d", start.UnixNano()),
	}

	messageBytes, err := json.Marshal(testMessage)
	if err != nil {
		return 0
	}

	err = erm.apiClient.PostToConnection(ctx, conn.ConnectionID, messageBytes)

	if err != nil {
		return 0
	}

	// Measure API call latency as proxy for connection latency
	// In serverless WebSocket (API Gateway), this is the best available metric
	return time.Since(start).Milliseconds()
}

// calculatePacketLoss calculates packet loss based on connection metrics
func (erm *ErrorRecoveryManager) calculatePacketLoss(conn *models.WebSocketConnection) float64 {
	if conn.Metrics.MessagesSent == 0 {
		return 0
	}

	// Estimate packet loss based on error count vs messages sent
	return float64(conn.Metrics.ErrorCount) / float64(conn.Metrics.MessagesSent) * 100
}

// GetRecoveryStats returns current error recovery statistics
func (erm *ErrorRecoveryManager) GetRecoveryStats() map[string]interface{} {
	return map[string]interface{}{
		"max_retries":      erm.maxRetries,
		"base_retry_delay": erm.baseRetryDelay.String(),
		"max_retry_delay":  erm.maxRetryDelay.String(),
		"jitter_factor":    erm.jitterFactor,
		"backoff_enabled":  erm.enableBackoff,
		"circuit_breaker": map[string]interface{}{
			"state":        fmt.Sprintf("%d", erm.circuitBreaker.GetState()),
			"failures":     erm.circuitBreaker.failures,
			"max_failures": erm.circuitBreaker.maxFailures,
			"timeout":      erm.circuitBreaker.timeout.String(),
		},
	}
}
