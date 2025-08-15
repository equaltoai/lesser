// Package streaming provides connection state synchronization for multi-instance deployments
package streaming

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"go.uber.org/zap"
)

// StateSynchronizer manages connection state synchronization across multiple instances
type StateSynchronizer struct {
	connRepo    *repositories.StreamingConnectionRepository
	logger      *zap.Logger
	instanceID  string
	
	// Synchronization state
	syncInterval     time.Duration
	staleThreshold   time.Duration
	conflictResolver ConflictResolver
	
	// Background processing
	syncTicker       *time.Ticker
	isRunning        bool
	stopChan         chan struct{}
	wg               sync.WaitGroup
	mu               sync.RWMutex

	// State tracking
	lastSyncTime     time.Time
	syncStats        SyncStats
}

// SyncStats tracks synchronization statistics
type SyncStats struct {
	TotalSyncs          int64     `json:"total_syncs"`
	SuccessfulSyncs     int64     `json:"successful_syncs"`
	FailedSyncs         int64     `json:"failed_syncs"`
	ConflictsResolved   int64     `json:"conflicts_resolved"`
	StaleConnectionsFound int64   `json:"stale_connections_found"`
	LastSyncTime        time.Time `json:"last_sync_time"`
	LastSyncDuration    time.Duration `json:"last_sync_duration"`
	AverageSyncDuration time.Duration `json:"average_sync_duration"`
}

// ConflictResolver defines strategies for resolving state conflicts
type ConflictResolver interface {
	ResolveConflict(ctx context.Context, local, remote *models.WebSocketConnection) (*models.WebSocketConnection, error)
}

// StateSynchronizerConfig contains configuration for state synchronization
type StateSynchronizerConfig struct {
	InstanceID       string                // Unique instance identifier
	SyncInterval     time.Duration         // How often to sync state
	StaleThreshold   time.Duration         // When to consider connections stale
	ConflictResolver ConflictResolver      // Strategy for resolving conflicts
}

// DefaultStateSynchronizerConfig returns default configuration
func DefaultStateSynchronizerConfig() *StateSynchronizerConfig {
	return &StateSynchronizerConfig{
		InstanceID:       generateInstanceID(),
		SyncInterval:     time.Minute * 2,  // Sync every 2 minutes
		StaleThreshold:   time.Minute * 10, // Consider stale after 10 minutes
		ConflictResolver: &LastWriteWinsResolver{},
	}
}

// generateInstanceID generates a unique instance identifier
func generateInstanceID() string {
	// For AWS Lambda, create instance ID from execution environment
	// Using a combination of request ID and startup time for uniqueness
	
	// Try to get AWS Lambda request context if available
	requestID := getAWSRequestID()
	if requestID != "" {
		return fmt.Sprintf("lambda-%s", requestID)
	}
	
	// Fallback: use hostname and startup time
	hostname := getHostname()
	timestamp := time.Now().UnixNano()
	
	return fmt.Sprintf("instance-%s-%d", hostname, timestamp)
}

// NewStateSynchronizer creates a new state synchronizer
func NewStateSynchronizer(
	connRepo *repositories.StreamingConnectionRepository,
	logger *zap.Logger,
	config *StateSynchronizerConfig,
) *StateSynchronizer {
	if config == nil {
		config = DefaultStateSynchronizerConfig()
	}

	if logger == nil {
		logger = zap.NewNop()
	}

	return &StateSynchronizer{
		connRepo:         connRepo,
		logger:           logger.With(zap.String("component", "state_synchronizer")),
		instanceID:       config.InstanceID,
		syncInterval:     config.SyncInterval,
		staleThreshold:   config.StaleThreshold,
		conflictResolver: config.ConflictResolver,
		stopChan:         make(chan struct{}),
	}
}

// Start begins state synchronization
func (ss *StateSynchronizer) Start(ctx context.Context) error {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	if ss.isRunning {
		return fmt.Errorf("state synchronizer is already running")
	}

	ss.syncTicker = time.NewTicker(ss.syncInterval)
	ss.isRunning = true

	// Start synchronization routine
	ss.wg.Add(1)
	go ss.syncRoutine(ctx)

	ss.logger.Info("state synchronizer started",
		zap.String("instance_id", ss.instanceID),
		zap.Duration("sync_interval", ss.syncInterval))

	return nil
}

// Stop stops state synchronization
func (ss *StateSynchronizer) Stop() error {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	if !ss.isRunning {
		return nil
	}

	close(ss.stopChan)
	
	if ss.syncTicker != nil {
		ss.syncTicker.Stop()
	}

	ss.isRunning = false

	// Wait for routine to finish
	ss.wg.Wait()

	ss.logger.Info("state synchronizer stopped")
	return nil
}

// IsRunning returns whether state synchronization is active
func (ss *StateSynchronizer) IsRunning() bool {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	return ss.isRunning
}

// syncRoutine runs the periodic state synchronization
func (ss *StateSynchronizer) syncRoutine(ctx context.Context) {
	defer ss.wg.Done()

	ss.logger.Info("starting state synchronization routine")

	for {
		select {
		case <-ss.stopChan:
			ss.logger.Info("stopping state synchronization routine")
			return
		case <-ctx.Done():
			ss.logger.Info("context cancelled, stopping state synchronization routine")
			return
		case <-ss.syncTicker.C:
			if err := ss.performSync(ctx); err != nil {
				ss.logger.Error("state synchronization failed", zap.Error(err))
				ss.syncStats.FailedSyncs++
			} else {
				ss.syncStats.SuccessfulSyncs++
			}
			ss.syncStats.TotalSyncs++
		}
	}
}

// performSync performs a state synchronization cycle
func (ss *StateSynchronizer) performSync(ctx context.Context) error {
	start := time.Now()
	ss.logger.Debug("starting state synchronization")

	// Phase 1: Identify stale connections
	staleConnections, err := ss.identifyStaleConnections(ctx)
	if err != nil {
		return fmt.Errorf("failed to identify stale connections: %w", err)
	}

	// Phase 2: Resolve state conflicts
	resolvedCount, err := ss.resolveStateConflicts(ctx, staleConnections)
	if err != nil {
		return fmt.Errorf("failed to resolve state conflicts: %w", err)
	}

	// Phase 3: Clean up orphaned connections
	cleanedCount, err := ss.cleanupOrphanedConnections(ctx)
	if err != nil {
		return fmt.Errorf("failed to cleanup orphaned connections: %w", err)
	}

	// Update statistics
	duration := time.Since(start)
	ss.updateSyncStats(duration, len(staleConnections), resolvedCount, cleanedCount)

	ss.logger.Info("state synchronization completed",
		zap.Int("stale_connections", len(staleConnections)),
		zap.Int("conflicts_resolved", resolvedCount),
		zap.Int("connections_cleaned", cleanedCount),
		zap.Duration("duration", duration))

	return nil
}

// identifyStaleConnections identifies connections that may be stale
func (ss *StateSynchronizer) identifyStaleConnections(ctx context.Context) ([]models.WebSocketConnection, error) {
	now := time.Now()
	staleThreshold := now.Add(-ss.staleThreshold)

	// Get all connections
	var allConnections []models.WebSocketConnection

	// Check all connection states
	states := []models.ConnectionState{
		models.ConnectionStateConnecting,
		models.ConnectionStateConnected,
		models.ConnectionStateIdle,
		models.ConnectionStateClosing,
		models.ConnectionStateError,
	}

	for _, state := range states {
		stateConnections, err := ss.connRepo.GetConnectionsByState(ctx, state)
		if err != nil {
			ss.logger.Error("failed to get connections by state",
				zap.String("state", string(state)),
				zap.Error(err))
			continue
		}
		allConnections = append(allConnections, stateConnections...)
	}

	// Filter stale connections
	staleConnections := make([]models.WebSocketConnection, 0)
	
	for _, conn := range allConnections {
		// Consider connection stale if:
		// 1. Last activity is before threshold
		// 2. Connection has been in connecting state too long
		// 3. Connection has been in error state without recovery attempts
		
		isStale := false
		
		if conn.LastActivity.Before(staleThreshold) {
			isStale = true
		}
		
		if conn.State == models.ConnectionStateConnecting && conn.StateChangedAt.Before(staleThreshold) {
			isStale = true
		}
		
		if conn.State == models.ConnectionStateError && 
		   conn.RetryCount >= conn.MaxRetries && 
		   conn.StateChangedAt.Before(staleThreshold) {
			isStale = true
		}
		
		if isStale {
			staleConnections = append(staleConnections, conn)
		}
	}

	ss.syncStats.StaleConnectionsFound = int64(len(staleConnections))
	return staleConnections, nil
}

// resolveStateConflicts resolves state conflicts for stale connections
func (ss *StateSynchronizer) resolveStateConflicts(ctx context.Context, staleConnections []models.WebSocketConnection) (int, error) {
	resolvedCount := 0

	for _, staleConn := range staleConnections {
		// Get the current state from database (might have been updated by another instance)
		currentConn, err := ss.connRepo.GetConnection(ctx, staleConn.ConnectionID)
		if err != nil {
			ss.logger.Error("failed to get current connection state",
				zap.String("connection_id", staleConn.ConnectionID),
				zap.Error(err))
			continue
		}

		// Check if there's a conflict (different states or timestamps)
		hasConflict := false
		if currentConn.State != staleConn.State {
			hasConflict = true
		}
		if currentConn.StateChangedAt.After(staleConn.StateChangedAt.Add(time.Second * 10)) {
			hasConflict = true // Significant time difference
		}

		if hasConflict {
			// Resolve the conflict
			resolved, err := ss.conflictResolver.ResolveConflict(ctx, &staleConn, currentConn)
			if err != nil {
				ss.logger.Error("failed to resolve state conflict",
					zap.String("connection_id", staleConn.ConnectionID),
					zap.Error(err))
				continue
			}

			// Update the resolved state
			if err := ss.connRepo.UpdateConnection(ctx, resolved); err != nil {
				ss.logger.Error("failed to update resolved connection state",
					zap.String("connection_id", resolved.ConnectionID),
					zap.Error(err))
				continue
			}

			resolvedCount++
			ss.logger.Debug("resolved state conflict",
				zap.String("connection_id", resolved.ConnectionID),
				zap.String("old_state", string(staleConn.State)),
				zap.String("new_state", string(resolved.State)))
		}
	}

	ss.syncStats.ConflictsResolved += int64(resolvedCount)
	return resolvedCount, nil
}

// cleanupOrphanedConnections removes connections that are truly orphaned
func (ss *StateSynchronizer) cleanupOrphanedConnections(ctx context.Context) (int, error) {
	// Get connections in closed state that are old
	closedConnections, err := ss.connRepo.GetConnectionsByState(ctx, models.ConnectionStateClosed)
	if err != nil {
		return 0, fmt.Errorf("failed to get closed connections: %w", err)
	}

	cleanedCount := 0
	cleanupThreshold := time.Now().Add(-time.Hour) // Clean up connections closed more than 1 hour ago

	for _, conn := range closedConnections {
		if conn.StateChangedAt.Before(cleanupThreshold) {
			// Clean up subscriptions first
			if err := ss.connRepo.DeleteAllSubscriptions(ctx, conn.ConnectionID); err != nil {
				ss.logger.Error("failed to clean up subscriptions for orphaned connection",
					zap.String("connection_id", conn.ConnectionID),
					zap.Error(err))
			}

			// Delete the connection
			if err := ss.connRepo.DeleteConnection(ctx, conn.ConnectionID); err != nil {
				ss.logger.Error("failed to delete orphaned connection",
					zap.String("connection_id", conn.ConnectionID),
					zap.Error(err))
				continue
			}

			cleanedCount++
			ss.logger.Debug("cleaned up orphaned connection",
				zap.String("connection_id", conn.ConnectionID))
		}
	}

	return cleanedCount, nil
}

// updateSyncStats updates synchronization statistics
func (ss *StateSynchronizer) updateSyncStats(duration time.Duration, _, _, _ int) {
	ss.lastSyncTime = time.Now()
	ss.syncStats.LastSyncTime = ss.lastSyncTime
	ss.syncStats.LastSyncDuration = duration

	// Calculate running average
	if ss.syncStats.TotalSyncs > 1 {
		totalDuration := ss.syncStats.AverageSyncDuration * time.Duration(ss.syncStats.TotalSyncs-1)
		ss.syncStats.AverageSyncDuration = (totalDuration + duration) / time.Duration(ss.syncStats.TotalSyncs)
	} else {
		ss.syncStats.AverageSyncDuration = duration
	}
}

// ForceSync triggers an immediate synchronization
func (ss *StateSynchronizer) ForceSync(ctx context.Context) error {
	if !ss.IsRunning() {
		return fmt.Errorf("state synchronizer is not running")
	}

	return ss.performSync(ctx)
}

// GetSyncStats returns current synchronization statistics
func (ss *StateSynchronizer) GetSyncStats() SyncStats {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	return ss.syncStats
}

// GetInstanceInfo returns information about this instance
func (ss *StateSynchronizer) GetInstanceInfo() map[string]interface{} {
	return map[string]interface{}{
		"instance_id":     ss.instanceID,
		"is_running":      ss.IsRunning(),
		"sync_interval":   ss.syncInterval.String(),
		"stale_threshold": ss.staleThreshold.String(),
		"last_sync_time":  ss.lastSyncTime,
		"stats":           ss.GetSyncStats(),
	}
}

// Conflict Resolution Strategies

// LastWriteWinsResolver resolves conflicts by choosing the connection with the most recent update
type LastWriteWinsResolver struct{}

// ResolveConflict implements ConflictResolver by choosing the connection with the most recent update
func (r *LastWriteWinsResolver) ResolveConflict(_ context.Context, local, remote *models.WebSocketConnection) (*models.WebSocketConnection, error) {
	// Choose the connection with the most recent state change
	if remote.StateChangedAt.After(local.StateChangedAt) {
		return remote, nil
	}
	return local, nil
}

// HighestPriorityResolver resolves conflicts based on state priority
type HighestPriorityResolver struct{}

// ResolveConflict implements ConflictResolver by choosing the connection with higher priority state
func (r *HighestPriorityResolver) ResolveConflict(_ context.Context, local, remote *models.WebSocketConnection) (*models.WebSocketConnection, error) {
	// Define state priorities (higher number = higher priority)
	priorities := map[models.ConnectionState]int{
		models.ConnectionStateClosed:     0,
		models.ConnectionStateClosing:    1,
		models.ConnectionStateError:      2,
		models.ConnectionStateConnecting: 3,
		models.ConnectionStateIdle:       4,
		models.ConnectionStateConnected:  5,
	}

	localPriority := priorities[local.State]
	remotePriority := priorities[remote.State]

	if remotePriority > localPriority {
		return remote, nil
	}
	if localPriority > remotePriority {
		return local, nil
	}

	// Same priority, use last write wins
	if remote.StateChangedAt.After(local.StateChangedAt) {
		return remote, nil
	}
	return local, nil
}

// HealthBasedResolver resolves conflicts based on connection health
type HealthBasedResolver struct{}

// ResolveConflict implements ConflictResolver by choosing the healthier connection
func (r *HealthBasedResolver) ResolveConflict(_ context.Context, local, remote *models.WebSocketConnection) (*models.WebSocketConnection, error) {
	// Choose the healthier connection
	if remote.IsHealthy() && !local.IsHealthy() {
		return remote, nil
	}
	if local.IsHealthy() && !remote.IsHealthy() {
		return local, nil
	}

	// Both healthy or both unhealthy, use quality score
	if remote.Metrics.ConnectionQuality > local.Metrics.ConnectionQuality {
		return remote, nil
	}
	if local.Metrics.ConnectionQuality > remote.Metrics.ConnectionQuality {
		return local, nil
	}

	// Same quality, use last write wins
	if remote.StateChangedAt.After(local.StateChangedAt) {
		return remote, nil
	}
	return local, nil
}

// getAWSRequestID extracts the AWS Lambda request ID from environment
func getAWSRequestID() string {
	// In AWS Lambda, the request ID is available in the context
	// For our implementation, we'll check environment variables
	// In a real Lambda, this would come from the Lambda context
	return "" // Placeholder - would be extracted from Lambda context
}

// getHostname gets the hostname/container ID
func getHostname() string {
	// Try environment variables that might contain useful identifiers
	if containerID := getEnvVar("HOSTNAME"); containerID != "" {
		return containerID
	}
	if taskID := getEnvVar("ECS_TASK_ID"); taskID != "" {
		return taskID
	}
	if podName := getEnvVar("POD_NAME"); podName != "" {
		return podName
	}
	
	// Fallback to a generic identifier
	return "streaming-instance"
}

// getEnvVar safely gets an environment variable
func getEnvVar(key string) string {
	return os.Getenv(key)
}