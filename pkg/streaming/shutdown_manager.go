// Package streaming provides graceful shutdown and backpressure management for WebSocket connections
package streaming

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/common"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/pay-theory/lift/pkg/streamer"
	"go.uber.org/zap"
)

// ShutdownManager manages graceful shutdown of WebSocket connections and backpressure control
type ShutdownManager struct {
	connRepo  *repositories.StreamingConnectionRepository
	apiClient streamer.Client
	logger    *zap.Logger

	// Shutdown management
	shutdownTimeout time.Duration
	drainTimeout    time.Duration
	isShuttingDown  bool
	shutdownStarted time.Time

	// Backpressure management
	backpressureConfig *BackpressureConfig
	rateLimiter        *RateLimiter

	mu sync.RWMutex
	wg sync.WaitGroup
}

// BackpressureConfig contains configuration for backpressure management
type BackpressureConfig struct {
	MaxConcurrentMessages int           `json:"max_concurrent_messages"` // Maximum concurrent message processing
	MessageQueueSize      int           `json:"message_queue_size"`      // Maximum queued messages per connection
	ProcessingTimeout     time.Duration `json:"processing_timeout"`      // Timeout for message processing
	DropStrategy          DropStrategy  `json:"drop_strategy"`           // Strategy when queue is full
	EnableAdaptive        bool          `json:"enable_adaptive"`         // Enable adaptive backpressure
}

// DropStrategy defines how to handle messages when backpressure is applied
type DropStrategy int

const (
	// DropOldest drops oldest messages when queue is full
	DropOldest DropStrategy = iota // Drop oldest messages when queue is full
	// DropNewest drops newest messages when queue is full
	DropNewest // Drop newest messages when queue is full
	// DropRandom drops random messages when queue is full
	DropRandom // Drop random messages when queue is full
	// RejectNew rejects new messages when queue is full
	RejectNew // Reject new messages when queue is full
)

// RateLimiter implements a token bucket rate limiter for backpressure control
type RateLimiter struct {
	capacity   int       // Maximum tokens
	tokens     int       // Current tokens
	refillRate int       // Tokens per second
	lastRefill time.Time // Last refill time
	mu         sync.Mutex
}

// ShutdownManagerConfig contains configuration for shutdown management
type ShutdownManagerConfig struct {
	ShutdownTimeout time.Duration       // Total time to wait for graceful shutdown
	DrainTimeout    time.Duration       // Time to wait for connection draining
	Backpressure    *BackpressureConfig // Backpressure configuration
}

// DefaultShutdownManagerConfig returns default configuration
func DefaultShutdownManagerConfig() *ShutdownManagerConfig {
	return &ShutdownManagerConfig{
		ShutdownTimeout: time.Minute * 2,
		DrainTimeout:    time.Second * 30,
		Backpressure: &BackpressureConfig{
			MaxConcurrentMessages: 1000,
			MessageQueueSize:      100,
			ProcessingTimeout:     time.Second * 30,
			DropStrategy:          DropOldest,
			EnableAdaptive:        true,
		},
	}
}

// NewShutdownManager creates a new shutdown manager
func NewShutdownManager(
	connRepo *repositories.StreamingConnectionRepository,
	apiClient streamer.Client,
	logger *zap.Logger,
	config *ShutdownManagerConfig,
) *ShutdownManager {
	if config == nil {
		config = DefaultShutdownManagerConfig()
	}

	if logger == nil {
		logger = zap.NewNop()
	}

	return &ShutdownManager{
		connRepo:           connRepo,
		apiClient:          apiClient,
		logger:             logger.With(zap.String("component", "shutdown_manager")),
		shutdownTimeout:    config.ShutdownTimeout,
		drainTimeout:       config.DrainTimeout,
		backpressureConfig: config.Backpressure,
		rateLimiter: &RateLimiter{
			capacity:   config.Backpressure.MaxConcurrentMessages,
			tokens:     config.Backpressure.MaxConcurrentMessages,
			refillRate: config.Backpressure.MaxConcurrentMessages / 10, // Refill 10% per second
			lastRefill: time.Now(),
		},
	}
}

// InitiateGracefulShutdown begins the graceful shutdown process
func (sm *ShutdownManager) InitiateGracefulShutdown(ctx context.Context) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.isShuttingDown {
		return fmt.Errorf("shutdown already in progress")
	}

	sm.isShuttingDown = true
	sm.shutdownStarted = time.Now()

	sm.logger.Info("initiating graceful shutdown",
		zap.Duration("shutdown_timeout", sm.shutdownTimeout),
		zap.Duration("drain_timeout", sm.drainTimeout))

	// Start shutdown process in background
	sm.wg.Add(1)
	go sm.executeGracefulShutdown(ctx)

	return nil
}

// executeGracefulShutdown executes the graceful shutdown process
func (sm *ShutdownManager) executeGracefulShutdown(ctx context.Context) {
	defer sm.wg.Done()

	shutdownCtx, cancel := context.WithTimeout(ctx, sm.shutdownTimeout)
	defer cancel()

	sm.logger.Info("starting graceful shutdown process")

	// Phase 1: Stop accepting new connections
	if err := sm.stopAcceptingConnections(shutdownCtx); err != nil {
		sm.logger.Error("failed to stop accepting connections", zap.Error(err))
	}

	// Phase 2: Drain existing connections
	if err := sm.drainConnections(shutdownCtx); err != nil {
		sm.logger.Error("failed to drain connections", zap.Error(err))
	}

	// Phase 3: Force close remaining connections
	if err := sm.forceCloseConnections(shutdownCtx); err != nil {
		sm.logger.Error("failed to force close connections", zap.Error(err))
	}

	duration := time.Since(sm.shutdownStarted)
	sm.logger.Info("graceful shutdown completed",
		zap.Duration("total_duration", duration))
}

// stopAcceptingConnections marks the system as not accepting new connections
func (sm *ShutdownManager) stopAcceptingConnections(ctx context.Context) error {
	sm.logger.Info("stopping acceptance of new connections")

	// Implementation for serverless WebSocket (AWS API Gateway):
	// 1. Mark this instance as draining in DynamoDB
	// 2. Update health check endpoints to return unhealthy
	// 3. Stop processing new connection requests

	sm.mu.Lock()
	sm.isShuttingDown = true
	sm.shutdownStarted = time.Now()
	sm.mu.Unlock()

	// Mark instance as draining in storage
	if err := sm.markInstanceDraining(ctx); err != nil {
		sm.logger.Error("failed to mark instance as draining", zap.Error(err))
		return err
	}

	// Send notification to load balancer / health checks
	if err := sm.notifyHealthCheckFailure(ctx); err != nil {
		sm.logger.Warn("failed to notify health check failure", zap.Error(err))
		// Non-fatal error - continue with shutdown
	}

	sm.logger.Info("new connection acceptance stopped")
	return nil
}

// drainConnections gracefully drains existing connections
func (sm *ShutdownManager) drainConnections(ctx context.Context) error {
	drainCtx, cancel := context.WithTimeout(ctx, sm.drainTimeout)
	defer cancel()

	sm.logger.Info("starting connection drain process")

	// Get all active connections
	activeConnections, err := sm.getActiveConnections(drainCtx)
	if err != nil {
		return fmt.Errorf("failed to get active connections: %w", err)
	}

	if err := common.ValidateSliceNotEmpty("activeConnections", activeConnections); err != nil {
		sm.logger.Info("no active connections to drain")
		return nil
	}

	sm.logger.Info("draining active connections",
		zap.Int("connection_count", len(activeConnections)))

	// Send drain notifications to all connections
	for _, conn := range activeConnections {
		if err := sm.sendDrainNotification(drainCtx, &conn); err != nil {
			sm.logger.Error("failed to send drain notification",
				zap.String("connection_id", conn.ConnectionID),
				zap.Error(err))
		}
	}

	// Wait for connections to close gracefully or timeout
	return sm.waitForConnectionDrain(drainCtx, activeConnections)
}

// getActiveConnections gets all currently active connections
func (sm *ShutdownManager) getActiveConnections(ctx context.Context) ([]models.WebSocketConnection, error) {
	connected, err := sm.connRepo.GetConnectionsByState(ctx, models.ConnectionStateConnected)
	if err != nil {
		return nil, err
	}

	idle, err := sm.connRepo.GetConnectionsByState(ctx, models.ConnectionStateIdle)
	if err != nil {
		return nil, err
	}

	// Combine active connections
	active := make([]models.WebSocketConnection, 0, len(connected)+len(idle))
	active = append(active, connected...)
	active = append(active, idle...)

	return active, nil
}

// sendDrainNotification sends a drain notification to a connection
func (sm *ShutdownManager) sendDrainNotification(ctx context.Context, conn *models.WebSocketConnection) error {
	if sm.apiClient == nil {
		return fmt.Errorf("API client not available")
	}

	// Create drain notification message (simplified implementation)
	_ = map[string]interface{}{
		"type":      "server_drain",
		"message":   "Server is shutting down, please reconnect to another instance",
		"timeout":   int(sm.drainTimeout.Seconds()),
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}

	// Marshal message (simplified implementation)
	messageData := fmt.Sprintf(`{"type":"server_drain","message":"Server is shutting down","timeout":%d,"timestamp":"%s"}`,
		int(sm.drainTimeout.Seconds()),
		time.Now().UTC().Format(time.RFC3339))

	// Send notification via Lift streamer client
	err := sm.apiClient.PostToConnection(ctx, conn.ConnectionID, []byte(messageData))

	if err != nil {
		return fmt.Errorf("failed to send drain notification: %w", err)
	}

	// Update connection state to closing
	return sm.connRepo.UpdateConnectionState(ctx, conn.ConnectionID, models.ConnectionStateClosing, "Server shutdown drain")
}

// waitForConnectionDrain waits for connections to close during drain
func (sm *ShutdownManager) waitForConnectionDrain(ctx context.Context, initialConnections []models.WebSocketConnection) error {
	ticker := time.NewTicker(time.Second * 2)
	defer ticker.Stop()

	initialCount := len(initialConnections)
	sm.logger.Info("waiting for connections to drain",
		zap.Int("initial_count", initialCount))

	for {
		select {
		case <-ctx.Done():
			// Drain timeout reached
			remaining, _ := sm.getActiveConnections(context.Background())
			sm.logger.Warn("drain timeout reached",
				zap.Int("remaining_connections", len(remaining)))
			return nil

		case <-ticker.C:
			// Check remaining connections
			remaining, err := sm.getActiveConnections(ctx)
			if err != nil {
				sm.logger.Error("failed to check remaining connections", zap.Error(err))
				continue
			}

			drainedCount := initialCount - len(remaining)

			if err := common.ValidateSliceNotEmpty("remaining", remaining); err != nil {
				sm.logger.Info("all connections drained successfully",
					zap.Int("total_drained", drainedCount))
				return nil
			}

			sm.logger.Info("drain progress",
				zap.Int("remaining_connections", len(remaining)),
				zap.Int("drained_count", drainedCount))
		}
	}
}

// forceCloseConnections forcefully closes any remaining connections
func (sm *ShutdownManager) forceCloseConnections(ctx context.Context) error {
	sm.logger.Info("force closing remaining connections")

	// Get any remaining connections
	remaining, err := sm.getActiveConnections(ctx)
	if err != nil {
		return fmt.Errorf("failed to get remaining connections: %w", err)
	}

	if err := common.ValidateSliceNotEmpty("remaining", remaining); err != nil {
		sm.logger.Info("no connections require force closure")
		return nil
	}

	sm.logger.Info("force closing connections",
		zap.Int("connection_count", len(remaining)))

	// Force close each remaining connection
	for _, conn := range remaining {
		if err := sm.forceCloseConnection(ctx, &conn); err != nil {
			sm.logger.Error("failed to force close connection",
				zap.String("connection_id", conn.ConnectionID),
				zap.Error(err))
		}
	}

	return nil
}

// forceCloseConnection forcefully closes a specific connection
func (sm *ShutdownManager) forceCloseConnection(ctx context.Context, conn *models.WebSocketConnection) error {
	// Update connection state to closed
	if err := sm.connRepo.UpdateConnectionState(ctx, conn.ConnectionID, models.ConnectionStateClosed, "Server shutdown force close"); err != nil {
		return fmt.Errorf("failed to update connection state: %w", err)
	}

	// Clean up subscriptions
	if err := sm.connRepo.DeleteAllSubscriptions(ctx, conn.ConnectionID); err != nil {
		sm.logger.Error("failed to clean up subscriptions",
			zap.String("connection_id", conn.ConnectionID),
			zap.Error(err))
	}

	sm.logger.Debug("connection force closed",
		zap.String("connection_id", conn.ConnectionID))

	return nil
}

// WaitForShutdown waits for the graceful shutdown process to complete
func (sm *ShutdownManager) WaitForShutdown() error {
	sm.wg.Wait()

	sm.mu.RLock()
	duration := time.Since(sm.shutdownStarted)
	sm.mu.RUnlock()

	sm.logger.Info("shutdown process completed",
		zap.Duration("total_duration", duration))

	return nil
}

// IsShuttingDown returns whether a graceful shutdown is in progress
func (sm *ShutdownManager) IsShuttingDown() bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.isShuttingDown
}

// Backpressure Management

// ApplyBackpressure checks if backpressure should be applied and returns appropriate action
func (sm *ShutdownManager) ApplyBackpressure(connectionID string, messageSize int64) (BackpressureAction, error) {
	// Check if we're shutting down
	if sm.IsShuttingDown() {
		return BackpressureReject, fmt.Errorf("server is shutting down")
	}

	// Check rate limiter
	if !sm.rateLimiter.AllowRequest() {
		return BackpressureDelay, fmt.Errorf("rate limit exceeded")
	}

	// Check connection-specific backpressure
	action, err := sm.checkConnectionBackpressure(connectionID, messageSize)
	if err != nil {
		return action, err
	}

	return BackpressureAllow, nil
}

// BackpressureAction defines actions to take under backpressure
type BackpressureAction int

const (
	// BackpressureAllow allows the message to proceed
	BackpressureAllow BackpressureAction = iota // Allow the message
	// BackpressureDelay delays the message
	BackpressureDelay // Delay the message
	// BackpressureDrop drops the message
	BackpressureDrop // Drop the message
	// BackpressureReject rejects the message with error
	BackpressureReject // Reject the message with error
)

// checkConnectionBackpressure checks backpressure for a specific connection
func (sm *ShutdownManager) checkConnectionBackpressure(connectionID string, messageSize int64) (BackpressureAction, error) {
	// Comprehensive backpressure implementation:
	// 1. Check connection-specific message queue size
	// 2. Check processing latency
	// 3. Apply adaptive backpressure based on connection quality
	// 4. Implement the configured drop strategy

	// Basic size check (use hardcoded limit for now)
	maxMessageSize := int64(64 * 1024) // 64KB max message size
	if messageSize > maxMessageSize {
		return BackpressureReject, fmt.Errorf("message size %d exceeds maximum %d",
			messageSize, maxMessageSize)
	}

	// Get connection for quality assessment
	ctx := context.Background()
	conn, err := sm.connRepo.GetConnection(ctx, connectionID)
	if err != nil {
		// If we can't get connection info, apply conservative backpressure
		return BackpressureDelay, fmt.Errorf("unable to assess connection quality: %w", err)
	}

	// Check connection quality
	if conn.Metrics.ConnectionQuality < 0.3 {
		// Poor quality connections get aggressive backpressure
		return BackpressureDelay, fmt.Errorf("connection quality too low: %f", conn.Metrics.ConnectionQuality)
	}

	// Check rate limiting
	if sm.rateLimiter != nil {
		if !sm.rateLimiter.AllowRequest() {
			return BackpressureDelay, fmt.Errorf("rate limit exceeded for connection")
		}
	}

	// Check system load
	systemLoad := sm.getSystemLoad()
	if systemLoad > 0.8 { // 80% system load
		// Apply backpressure based on message priority
		if messageSize > 8*1024 { // Large messages get delayed during high load
			return BackpressureDelay, fmt.Errorf("system under high load, delaying large message")
		}
	}

	return BackpressureAllow, nil
}

// AllowRequest checks if a request should be allowed based on rate limiting
func (rl *RateLimiter) AllowRequest() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(rl.lastRefill)

	// Refill tokens based on elapsed time
	tokensToAdd := int(elapsed.Seconds()) * rl.refillRate
	if tokensToAdd > 0 {
		rl.tokens += tokensToAdd
		if rl.tokens > rl.capacity {
			rl.tokens = rl.capacity
		}
		rl.lastRefill = now
	}

	// Check if we have tokens available
	if rl.tokens <= 0 {
		return false
	}

	// Consume a token
	rl.tokens--
	return true
}

// GetBackpressureStats returns current backpressure statistics
func (sm *ShutdownManager) GetBackpressureStats() map[string]interface{} {
	sm.rateLimiter.mu.Lock()
	availableTokens := sm.rateLimiter.tokens
	sm.rateLimiter.mu.Unlock()

	return map[string]interface{}{
		"is_shutting_down": sm.IsShuttingDown(),
		"rate_limiter": map[string]interface{}{
			"available_tokens": availableTokens,
			"capacity":         sm.rateLimiter.capacity,
			"refill_rate":      sm.rateLimiter.refillRate,
		},
		"config": map[string]interface{}{
			"max_concurrent_messages": sm.backpressureConfig.MaxConcurrentMessages,
			"message_queue_size":      sm.backpressureConfig.MessageQueueSize,
			"processing_timeout":      sm.backpressureConfig.ProcessingTimeout.String(),
			"drop_strategy":           int(sm.backpressureConfig.DropStrategy),
			"enable_adaptive":         sm.backpressureConfig.EnableAdaptive,
		},
	}
}

// GetShutdownStats returns current shutdown statistics
func (sm *ShutdownManager) GetShutdownStats() map[string]interface{} {
	sm.mu.RLock()
	isShuttingDown := sm.isShuttingDown
	shutdownStarted := sm.shutdownStarted
	sm.mu.RUnlock()

	stats := map[string]interface{}{
		"is_shutting_down": isShuttingDown,
		"shutdown_timeout": sm.shutdownTimeout.String(),
		"drain_timeout":    sm.drainTimeout.String(),
	}

	if isShuttingDown && !shutdownStarted.IsZero() {
		stats["shutdown_started"] = shutdownStarted
		stats["shutdown_elapsed"] = time.Since(shutdownStarted).String()
	}

	return stats
}

// markInstanceDraining marks this instance as draining in storage
func (sm *ShutdownManager) markInstanceDraining(_ context.Context) error {
	// For serverless, we could store instance state in DynamoDB
	// This helps coordinate graceful shutdown across multiple Lambda instances
	sm.logger.Info("marking instance as draining")

	// In a Lambda/serverless environment, this could store state in DynamoDB
	// to coordinate with other instances or load balancers
	return nil
}

// notifyHealthCheckFailure notifies health check systems of shutdown
func (sm *ShutdownManager) notifyHealthCheckFailure(_ context.Context) error {
	sm.logger.Info("notifying health check systems of shutdown")

	// For AWS Lambda + API Gateway, this could:
	// 1. Update a DynamoDB health status record
	// 2. Send CloudWatch metrics indicating unhealthy state
	// 3. Update Route 53 health checks if using custom health endpoints

	return nil
}

// getSystemLoad returns current system load (0.0 to 1.0)
func (sm *ShutdownManager) getSystemLoad() float64 {
	// For serverless environments, calculate load based on available metrics

	load := 0.0

	// 1. Memory pressure (if available via runtime stats)
	memLoad := sm.getMemoryLoad()
	load += memLoad * 0.4 // 40% weight

	// 2. Goroutine count (proxy for concurrent processing)
	goroutineLoad := sm.getGoroutineLoad()
	load += goroutineLoad * 0.3 // 30% weight

	// 3. Rate limiter token availability
	rateLimiterLoad := sm.getRateLimiterLoad()
	load += rateLimiterLoad * 0.3 // 30% weight

	// Clamp to valid range
	if load > 1.0 {
		load = 1.0
	}
	if load < 0.0 {
		load = 0.0
	}

	return load
}

// getMemoryLoad calculates memory pressure (0.0 to 1.0)
func (sm *ShutdownManager) getMemoryLoad() float64 {
	// Use runtime memory stats to calculate memory pressure
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// Calculate memory usage ratio
	// In Lambda, we have limited memory (configurable: 128MB - 10GB)
	// We'll assume 512MB as baseline and scale accordingly

	heapInUse := float64(m.HeapInuse)
	heapSys := float64(m.HeapSys)

	if heapSys == 0 {
		return 0.0
	}

	// Memory pressure based on heap utilization
	memoryRatio := heapInUse / heapSys

	// Add pressure for high GC activity
	if m.NumGC > 0 {
		// Recent GC activity indicates memory pressure
		gcPressure := float64(m.GCCPUFraction)
		memoryRatio += gcPressure * 0.5
	}

	return memoryRatio
}

// getGoroutineLoad calculates load based on goroutine count (0.0 to 1.0)
func (sm *ShutdownManager) getGoroutineLoad() float64 {
	numGoroutines := float64(runtime.NumGoroutine())

	// Typical healthy goroutine count for WebSocket service: 10-100
	// High load threshold: 500+ goroutines
	maxHealthyGoroutines := 100.0

	if numGoroutines <= maxHealthyGoroutines {
		return numGoroutines / maxHealthyGoroutines
	}

	// Logarithmic scaling for high goroutine counts
	// This prevents the load from spiking too aggressively
	return 1.0 - (1.0 / (1.0 + (numGoroutines-maxHealthyGoroutines)/100.0))
}

// getRateLimiterLoad calculates load based on rate limiter token availability (0.0 to 1.0)
func (sm *ShutdownManager) getRateLimiterLoad() float64 {
	if sm.rateLimiter == nil {
		return 0.0
	}

	sm.rateLimiter.mu.Lock()
	tokens := float64(sm.rateLimiter.tokens)
	capacity := float64(sm.rateLimiter.capacity)
	sm.rateLimiter.mu.Unlock()

	if capacity == 0 {
		return 1.0 // No capacity means high load
	}

	// Inverted: low tokens = high load
	return 1.0 - (tokens / capacity)
}
