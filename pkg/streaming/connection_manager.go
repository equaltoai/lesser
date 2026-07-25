// Package streaming provides connection lifecycle management for WebSocket streaming
package streaming

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/theory-cloud/apptheory/v2/pkg/streamer"
	"go.uber.org/zap"
)

// ConnectionManager manages WebSocket connection lifecycle, health checks, and resource management
type ConnectionManager struct {
	connRepo          interfaces.StreamingConnectionRepository
	apiClient         streamer.Client
	logger            *zap.Logger
	healthCheckTicker *time.Ticker
	cleanupTicker     *time.Ticker
	stopChan          chan struct{}
	wg                sync.WaitGroup
	mu                sync.RWMutex
	isRunning         bool

	// Configuration
	healthCheckInterval time.Duration
	cleanupInterval     time.Duration
	pingTimeout         time.Duration
	idleThreshold       time.Duration
	maxIdleConnections  int
}

// ConnectionManagerConfig contains configuration for the connection manager
type ConnectionManagerConfig struct {
	HealthCheckInterval time.Duration // How often to run health checks
	CleanupInterval     time.Duration // How often to run cleanup tasks
	PingTimeout         time.Duration // How long to wait for pong response
	IdleThreshold       time.Duration // When to mark connections as idle
	MaxIdleConnections  int           // Maximum number of idle connections to keep
}

// DefaultConnectionManagerConfig returns default configuration
func DefaultConnectionManagerConfig() *ConnectionManagerConfig {
	return &ConnectionManagerConfig{
		HealthCheckInterval: time.Minute * 2,  // Check every 2 minutes
		CleanupInterval:     time.Minute * 10, // Cleanup every 10 minutes
		PingTimeout:         time.Second * 30, // 30 second ping timeout
		IdleThreshold:       time.Minute * 30, // Mark idle after 30 minutes
		MaxIdleConnections:  100,              // Keep max 100 idle connections
	}
}

// NewConnectionManager creates a new connection manager
func NewConnectionManager(
	connRepo interfaces.StreamingConnectionRepository,
	apiClient streamer.Client,
	logger *zap.Logger,
	config *ConnectionManagerConfig,
) *ConnectionManager {
	if config == nil {
		config = DefaultConnectionManagerConfig()
	}

	if logger == nil {
		logger = zap.NewNop()
	}

	return &ConnectionManager{
		connRepo:            connRepo,
		apiClient:           apiClient,
		logger:              logger.With(zap.String("component", "connection_manager")),
		healthCheckInterval: config.HealthCheckInterval,
		cleanupInterval:     config.CleanupInterval,
		pingTimeout:         config.PingTimeout,
		idleThreshold:       config.IdleThreshold,
		maxIdleConnections:  config.MaxIdleConnections,
		stopChan:            make(chan struct{}),
	}
}

// Start begins the connection management background tasks
func (cm *ConnectionManager) Start(ctx context.Context) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cm.isRunning {
		return fmt.Errorf("connection manager is already running")
	}

	cm.healthCheckTicker = time.NewTicker(cm.healthCheckInterval)
	cm.cleanupTicker = time.NewTicker(cm.cleanupInterval)
	cm.isRunning = true

	// Start health check routine
	cm.wg.Add(1)
	go cm.healthCheckRoutine(ctx)

	// Start cleanup routine
	cm.wg.Add(1)
	go cm.cleanupRoutine(ctx)

	cm.logger.Info("connection manager started",
		zap.Duration("health_check_interval", cm.healthCheckInterval),
		zap.Duration("cleanup_interval", cm.cleanupInterval))

	return nil
}

// Stop stops the connection management background tasks
func (cm *ConnectionManager) Stop() error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if !cm.isRunning {
		return nil
	}

	close(cm.stopChan)

	if cm.healthCheckTicker != nil {
		cm.healthCheckTicker.Stop()
	}

	if cm.cleanupTicker != nil {
		cm.cleanupTicker.Stop()
	}

	cm.isRunning = false

	// Wait for routines to finish
	cm.wg.Wait()

	cm.logger.Info("connection manager stopped")
	return nil
}

// IsRunning returns whether the connection manager is currently running
func (cm *ConnectionManager) IsRunning() bool {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.isRunning
}

// healthCheckRoutine runs periodic health checks on connections
func (cm *ConnectionManager) healthCheckRoutine(ctx context.Context) {
	defer cm.wg.Done()

	cm.logger.Info("starting health check routine")

	for {
		select {
		case <-cm.stopChan:
			cm.logger.Info("stopping health check routine")
			return
		case <-ctx.Done():
			cm.logger.Info("context cancelled, stopping health check routine")
			return
		case <-cm.healthCheckTicker.C:
			if err := cm.runHealthCheck(ctx); err != nil {
				cm.logger.Error("health check failed", zap.Error(err))
			}
		}
	}
}

// cleanupRoutine runs periodic cleanup tasks
func (cm *ConnectionManager) cleanupRoutine(ctx context.Context) {
	defer cm.wg.Done()

	cm.logger.Info("starting cleanup routine")

	for {
		select {
		case <-cm.stopChan:
			cm.logger.Info("stopping cleanup routine")
			return
		case <-ctx.Done():
			cm.logger.Info("context cancelled, stopping cleanup routine")
			return
		case <-cm.cleanupTicker.C:
			if err := cm.runCleanup(ctx); err != nil {
				cm.logger.Error("cleanup failed", zap.Error(err))
			}
		}
	}
}

// runHealthCheck performs health checks on all active connections
func (cm *ConnectionManager) runHealthCheck(ctx context.Context) error {
	start := time.Now()
	cm.logger.Debug("starting health check cycle")

	// Get all active connections (connected + idle)
	connectedConns, err := cm.connRepo.GetConnectionsByState(ctx, models.ConnectionStateConnected)
	if err != nil {
		return fmt.Errorf("failed to get connected connections: %w", err)
	}

	idleConns, err := cm.connRepo.GetConnectionsByState(ctx, models.ConnectionStateIdle)
	if err != nil {
		return fmt.Errorf("failed to get idle connections: %w", err)
	}

	allConns := append(connectedConns, idleConns...)

	if err := common.ValidateSliceNotEmpty("allConns", allConns); err != nil {
		cm.logger.Debug("no active connections to health check")
		return nil
	}

	healthyCount := 0
	unhealthyCount := 0
	pingedCount := 0
	markedIdleCount := 0

	// Check each connection
	for _, conn := range allConns {
		// Check if connection should be marked as idle
		if conn.State == models.ConnectionStateConnected && time.Since(conn.LastActivity) > cm.idleThreshold {
			conn.UpdateState(models.ConnectionStateIdle)
			if err := cm.connRepo.UpdateConnection(ctx, &conn); err != nil {
				cm.logger.Error("failed to mark connection as idle",
					zap.String("connection_id", conn.ConnectionID),
					zap.Error(err))
			} else {
				markedIdleCount++
			}
		}

		// Check connection health
		if conn.IsHealthy() {
			healthyCount++

			// Send ping to connected connections to check responsiveness
			if conn.State == models.ConnectionStateConnected {
				if err := cm.sendPing(ctx, &conn); err != nil {
					cm.logger.Warn("failed to send ping",
						zap.String("connection_id", conn.ConnectionID),
						zap.Error(err))
					// Record error but don't fail the health check
					if recordErr := cm.connRepo.RecordConnectionError(ctx, conn.ConnectionID, fmt.Sprintf("ping failed: %v", err)); recordErr != nil {
						cm.logger.Error("failed to record connection error", zap.Error(recordErr))
					}
				} else {
					pingedCount++
				}
			}
		} else {
			unhealthyCount++
			cm.logger.Warn("unhealthy connection detected",
				zap.String("connection_id", conn.ConnectionID),
				zap.String("user_id", conn.UserID),
				zap.Int32("error_count", conn.Metrics.ErrorCount),
				zap.Float64("quality", conn.Metrics.ConnectionQuality))
		}
	}

	duration := time.Since(start)
	cm.logger.Info("health check completed",
		zap.Int("total_connections", len(allConns)),
		zap.Int("healthy_connections", healthyCount),
		zap.Int("unhealthy_connections", unhealthyCount),
		zap.Int("pinged_connections", pingedCount),
		zap.Int("marked_idle", markedIdleCount),
		zap.Duration("duration", duration))

	return nil
}

// runCleanup performs cleanup tasks on connections
func (cm *ConnectionManager) runCleanup(ctx context.Context) error {
	start := time.Now()
	cm.logger.Debug("starting cleanup cycle")

	// Mark connections as idle if they've been inactive
	idleCount, err := cm.connRepo.MarkConnectionsIdle(ctx, cm.idleThreshold)
	if err != nil {
		cm.logger.Error("failed to mark connections as idle", zap.Error(err))
	}

	// Close timed-out connections
	closedCount, err := cm.connRepo.CloseTimedOutConnections(ctx)
	if err != nil {
		cm.logger.Error("failed to close timed-out connections", zap.Error(err))
	}

	// Reclaim excess idle connections
	reclaimedCount, err := cm.connRepo.ReclaimIdleConnections(ctx, cm.maxIdleConnections)
	if err != nil {
		cm.logger.Error("failed to reclaim idle connections", zap.Error(err))
	}

	// Clean up expired connections (TTL cleanup)
	expiredCount, err := cm.connRepo.CleanupExpiredConnections(ctx)
	if err != nil {
		cm.logger.Error("failed to cleanup expired connections", zap.Error(err))
	}

	duration := time.Since(start)
	cm.logger.Info("cleanup completed",
		zap.Int("marked_idle", idleCount),
		zap.Int("closed_timeout", closedCount),
		zap.Int("reclaimed_idle", reclaimedCount),
		zap.Int("expired_cleaned", expiredCount),
		zap.Duration("duration", duration))

	return nil
}

// sendPing sends a ping message to a connection
func (cm *ConnectionManager) sendPing(ctx context.Context, conn *models.WebSocketConnection) error {
	if cm.apiClient == nil {
		return fmt.Errorf("API client not available")
	}

	// Create ping message
	pingMessage := map[string]interface{}{
		"type":      "ping",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}

	// Marshal message
	messageBytes, err := marshalMessage(pingMessage)
	if err != nil {
		return fmt.Errorf("failed to marshal ping message: %w", err)
	}

	// Send ping via Lift streamer client
	err = cm.apiClient.PostToConnection(ctx, conn.ConnectionID, messageBytes)

	if err != nil {
		return fmt.Errorf("failed to send ping: %w", err)
	}

	// Record ping in connection metrics
	return cm.connRepo.RecordPing(ctx, conn.ConnectionID)
}

// GetConnectionStats returns current connection statistics
func (cm *ConnectionManager) GetConnectionStats(ctx context.Context) (map[string]interface{}, error) {
	poolStats, err := cm.connRepo.GetConnectionPool(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get connection pool stats: %w", err)
	}

	// Add manager-specific stats
	poolStats["manager_running"] = cm.IsRunning()
	poolStats["health_check_interval"] = cm.healthCheckInterval.String()
	poolStats["cleanup_interval"] = cm.cleanupInterval.String()
	poolStats["idle_threshold"] = cm.idleThreshold.String()
	poolStats["max_idle_connections"] = cm.maxIdleConnections

	return poolStats, nil
}

// ForceHealthCheck triggers an immediate health check
func (cm *ConnectionManager) ForceHealthCheck(ctx context.Context) error {
	return cm.runHealthCheck(ctx)
}

// ForceCleanup triggers an immediate cleanup
func (cm *ConnectionManager) ForceCleanup(ctx context.Context) error {
	return cm.runCleanup(ctx)
}

// Helper function to marshal messages consistently
func marshalMessage(data interface{}) ([]byte, error) {
	// This would use the same JSON marshaling as the streaming service
	// For now, we'll use a simple implementation
	if jsonData, ok := data.(map[string]interface{}); ok {
		result := "{"
		first := true
		for k, v := range jsonData {
			if !first {
				result += ","
			}
			result += fmt.Sprintf("\"%s\":\"%v\"", k, v)
			first = false
		}
		result += "}"
		return []byte(result), nil
	}
	return nil, fmt.Errorf("unsupported data type")
}
