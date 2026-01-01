// Package inmemory provides thread-safe in-memory implementations of repository interfaces.
package inmemory

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

// StreamingConnectionRepository is a thread-safe in-memory implementation of interfaces.StreamingConnectionRepository.
type StreamingConnectionRepository struct {
	mu sync.RWMutex

	// Connections by ID: connectionID -> WebSocketConnection
	connections map[string]*models.WebSocketConnection

	// Connections by user: userID -> []connectionID
	connectionsByUser map[string][]string

	// Connections by state: state -> []connectionID
	connectionsByState map[models.ConnectionState][]string

	// Subscriptions by key: stream_connectionID -> WebSocketSubscription
	subscriptions map[string]*models.WebSocketSubscription

	// Subscriptions by stream: stream -> []WebSocketSubscription
	subscriptionsByStream map[string][]*models.WebSocketSubscription

	// Subscriptions by connection: connectionID -> []stream
	subscriptionsByConn map[string][]string
}

// NewStreamingConnectionRepository creates a new in-memory streaming connection repository
func NewStreamingConnectionRepository() *StreamingConnectionRepository {
	return &StreamingConnectionRepository{
		connections:           make(map[string]*models.WebSocketConnection),
		connectionsByUser:     make(map[string][]string),
		connectionsByState:    make(map[models.ConnectionState][]string),
		subscriptions:         make(map[string]*models.WebSocketSubscription),
		subscriptionsByStream: make(map[string][]*models.WebSocketSubscription),
		subscriptionsByConn:   make(map[string][]string),
	}
}


// ===== Connection Lifecycle Operations =====

// WriteConnection stores a WebSocket connection
func (r *StreamingConnectionRepository) WriteConnection(_ context.Context, connectionID, userID, username string, streams []string) (*models.WebSocketConnection, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	connection := &models.WebSocketConnection{
		ConnectionID:   connectionID,
		UserID:         userID,
		Username:       username,
		Streams:        streams,
		Established:    now,
		LastActivity:   now,
		State:          models.ConnectionStateConnecting,
		StateChangedAt: now,
		MaxRetries:     5,
		IdleTimeout:    time.Hour * 2,
		MaxMessageSize: 1024 * 64,
		RateLimit:      100,
		RateLimitReset: now.Add(time.Minute),
		TTL:            now.Add(24 * time.Hour).Unix(),
		Metrics: models.ConnectionMetrics{
			ConnectionQuality: 1.0,
		},
		Info: models.ConnectionInfo{
			Protocol:   "websocket",
			APIVersion: "v1",
		},
	}

	r.connections[connectionID] = connection
	r.connectionsByUser[userID] = append(r.connectionsByUser[userID], connectionID)
	r.connectionsByState[models.ConnectionStateConnecting] = append(r.connectionsByState[models.ConnectionStateConnecting], connectionID)

	return connection, nil
}

// GetConnection retrieves a WebSocket connection by connection ID
func (r *StreamingConnectionRepository) GetConnection(_ context.Context, connectionID string) (*models.WebSocketConnection, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	connection, exists := r.connections[connectionID]
	if !exists {
		return nil, storage.ErrNotFound
	}
	return connection, nil
}

// UpdateConnection updates an existing WebSocket connection
func (r *StreamingConnectionRepository) UpdateConnection(_ context.Context, connection *models.WebSocketConnection) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.connections[connection.ConnectionID]; !exists {
		return storage.ErrNotFound
	}

	r.connections[connection.ConnectionID] = connection
	return nil
}

// DeleteConnection removes a WebSocket connection
func (r *StreamingConnectionRepository) DeleteConnection(_ context.Context, connectionID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	connection, exists := r.connections[connectionID]
	if !exists {
		return nil
	}

	delete(r.connections, connectionID)
	r.removeFromUserIndex(connection.UserID, connectionID)
	r.removeFromStateIndex(connection.State, connectionID)

	return nil
}

// UpdateConnectionState updates the connection state
func (r *StreamingConnectionRepository) UpdateConnectionState(_ context.Context, connectionID string, newState models.ConnectionState, reason string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	connection, exists := r.connections[connectionID]
	if !exists {
		return storage.ErrNotFound
	}

	oldState := connection.State
	connection.State = newState
	connection.StateChangedAt = time.Now()
	if reason != "" {
		connection.CloseReason = reason
	}

	r.removeFromStateIndex(oldState, connectionID)
	r.connectionsByState[newState] = append(r.connectionsByState[newState], connectionID)

	return nil
}

// UpdateConnectionActivity updates the last activity timestamp
func (r *StreamingConnectionRepository) UpdateConnectionActivity(_ context.Context, connectionID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	connection, exists := r.connections[connectionID]
	if !exists {
		return storage.ErrNotFound
	}

	connection.LastActivity = time.Now()
	return nil
}


// ===== Subscription Operations =====

// WriteSubscription stores a stream subscription
func (r *StreamingConnectionRepository) WriteSubscription(_ context.Context, connectionID, userID, stream string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := stream + "_" + connectionID
	subscription := &models.WebSocketSubscription{
		ConnectionID: connectionID,
		UserID:       userID,
		Stream:       stream,
		SubscribedAt: time.Now(),
		TTL:          time.Now().Add(24 * time.Hour).Unix(),
	}

	r.subscriptions[key] = subscription
	r.subscriptionsByStream[stream] = append(r.subscriptionsByStream[stream], subscription)
	r.subscriptionsByConn[connectionID] = append(r.subscriptionsByConn[connectionID], stream)

	return nil
}

// DeleteSubscription removes a stream subscription
func (r *StreamingConnectionRepository) DeleteSubscription(_ context.Context, connectionID, stream string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := stream + "_" + connectionID
	delete(r.subscriptions, key)

	// Remove from stream index
	var newStreamSubs []*models.WebSocketSubscription
	for _, sub := range r.subscriptionsByStream[stream] {
		if sub.ConnectionID != connectionID {
			newStreamSubs = append(newStreamSubs, sub)
		}
	}
	r.subscriptionsByStream[stream] = newStreamSubs

	// Remove from connection index
	var newConnStreams []string
	for _, s := range r.subscriptionsByConn[connectionID] {
		if s != stream {
			newConnStreams = append(newConnStreams, s)
		}
	}
	r.subscriptionsByConn[connectionID] = newConnStreams

	return nil
}

// DeleteAllSubscriptions removes all subscriptions for a connection
func (r *StreamingConnectionRepository) DeleteAllSubscriptions(_ context.Context, connectionID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	streams := r.subscriptionsByConn[connectionID]
	for _, stream := range streams {
		key := stream + "_" + connectionID
		delete(r.subscriptions, key)

		var newStreamSubs []*models.WebSocketSubscription
		for _, sub := range r.subscriptionsByStream[stream] {
			if sub.ConnectionID != connectionID {
				newStreamSubs = append(newStreamSubs, sub)
			}
		}
		r.subscriptionsByStream[stream] = newStreamSubs
	}

	delete(r.subscriptionsByConn, connectionID)
	return nil
}

// GetSubscriptionsForStream gets all subscriptions for a specific stream
func (r *StreamingConnectionRepository) GetSubscriptionsForStream(_ context.Context, stream string) ([]models.WebSocketSubscription, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	subs := r.subscriptionsByStream[stream]
	result := make([]models.WebSocketSubscription, len(subs))
	for i, sub := range subs {
		result[i] = *sub
	}
	return result, nil
}


// ===== Connection Queries =====

// GetConnectionsByUser gets all connections for a user
func (r *StreamingConnectionRepository) GetConnectionsByUser(_ context.Context, userID string) ([]models.WebSocketConnection, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var connections []models.WebSocketConnection
	for _, connID := range r.connectionsByUser[userID] {
		if conn, exists := r.connections[connID]; exists {
			connections = append(connections, *conn)
		}
	}
	return connections, nil
}

// GetConnectionsByState gets all connections in a specific state
func (r *StreamingConnectionRepository) GetConnectionsByState(_ context.Context, state models.ConnectionState) ([]models.WebSocketConnection, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var connections []models.WebSocketConnection
	for _, connID := range r.connectionsByState[state] {
		if conn, exists := r.connections[connID]; exists {
			connections = append(connections, *conn)
		}
	}
	return connections, nil
}

// GetIdleConnections gets connections that have been idle past the threshold
func (r *StreamingConnectionRepository) GetIdleConnections(_ context.Context, idleThreshold time.Time) ([]models.WebSocketConnection, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var idleConnections []models.WebSocketConnection
	for _, conn := range r.connections {
		if conn.LastActivity.Before(idleThreshold) {
			idleConnections = append(idleConnections, *conn)
		}
	}
	return idleConnections, nil
}

// GetStaleConnections gets connections that are considered stale
func (r *StreamingConnectionRepository) GetStaleConnections(_ context.Context, staleThreshold time.Time) ([]models.WebSocketConnection, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var staleConnections []models.WebSocketConnection
	now := time.Now().Unix()

	for _, conn := range r.connections {
		isStale := conn.LastActivity.Before(staleThreshold) || (conn.TTL > 0 && now > conn.TTL)
		if isStale {
			staleConnections = append(staleConnections, *conn)
		}
	}
	return staleConnections, nil
}

// GetHealthyConnections gets all healthy connections
func (r *StreamingConnectionRepository) GetHealthyConnections(_ context.Context) ([]models.WebSocketConnection, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var healthyConns []models.WebSocketConnection
	for _, conn := range r.connections {
		if conn.State == models.ConnectionStateConnected || conn.State == models.ConnectionStateIdle {
			if conn.IsHealthy() {
				healthyConns = append(healthyConns, *conn)
			}
		}
	}
	return healthyConns, nil
}

// GetUnhealthyConnections gets connections that need attention
func (r *StreamingConnectionRepository) GetUnhealthyConnections(_ context.Context) ([]models.WebSocketConnection, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var unhealthyConns []models.WebSocketConnection
	for _, conn := range r.connections {
		if conn.State == models.ConnectionStateError || !conn.IsHealthy() {
			unhealthyConns = append(unhealthyConns, *conn)
		}
	}
	return unhealthyConns, nil
}


// ===== Connection Counts =====

// GetActiveConnectionsCount gets the count of active connections for a user
func (r *StreamingConnectionRepository) GetActiveConnectionsCount(_ context.Context, userID string) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	count := 0
	for _, connID := range r.connectionsByUser[userID] {
		if conn, exists := r.connections[connID]; exists {
			if conn.State == models.ConnectionStateConnected || conn.State == models.ConnectionStateIdle {
				count++
			}
		}
	}
	return count, nil
}

// GetTotalActiveConnectionsCount gets the total count of active connections
func (r *StreamingConnectionRepository) GetTotalActiveConnectionsCount(_ context.Context) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	count := 0
	for _, conn := range r.connections {
		if conn.State == models.ConnectionStateConnected || conn.State == models.ConnectionStateIdle {
			count++
		}
	}
	return count, nil
}

// GetConnectionCountByState returns the number of connections in the provided state
func (r *StreamingConnectionRepository) GetConnectionCountByState(_ context.Context, state models.ConnectionState) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.connectionsByState[state]), nil
}

// GetUserConnectionCount returns the number of connections for a user
func (r *StreamingConnectionRepository) GetUserConnectionCount(_ context.Context, userID string) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.connectionsByUser[userID]), nil
}

// ===== Connection Management =====

// MarkConnectionsIdle marks inactive connections as idle
func (r *StreamingConnectionRepository) MarkConnectionsIdle(_ context.Context, idleThreshold time.Duration) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	markedCount := 0
	now := time.Now()

	for _, conn := range r.connections {
		if conn.State == models.ConnectionStateConnected && now.Sub(conn.LastActivity) > idleThreshold {
			r.removeFromStateIndex(conn.State, conn.ConnectionID)
			conn.State = models.ConnectionStateIdle
			conn.StateChangedAt = now
			r.connectionsByState[models.ConnectionStateIdle] = append(r.connectionsByState[models.ConnectionStateIdle], conn.ConnectionID)
			markedCount++
		}
	}

	return markedCount, nil
}

// CloseTimedOutConnections closes connections that have exceeded their idle timeout
func (r *StreamingConnectionRepository) CloseTimedOutConnections(_ context.Context) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	closedCount := 0
	now := time.Now()

	for _, conn := range r.connections {
		if conn.State == models.ConnectionStateIdle {
			idleDuration := now.Sub(conn.LastActivity)
			if idleDuration > conn.IdleTimeout {
				r.removeFromStateIndex(conn.State, conn.ConnectionID)
				conn.State = models.ConnectionStateClosing
				conn.StateChangedAt = now
				conn.CloseReason = fmt.Sprintf("Idle timeout after %v", idleDuration)
				conn.CloseCode = 1001
				r.connectionsByState[models.ConnectionStateClosing] = append(r.connectionsByState[models.ConnectionStateClosing], conn.ConnectionID)
				closedCount++
			}
		}
	}

	return closedCount, nil
}

// ReclaimIdleConnections proactively closes old idle connections
func (r *StreamingConnectionRepository) ReclaimIdleConnections(_ context.Context, maxIdleConnections int) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	idleConnIDs := r.connectionsByState[models.ConnectionStateIdle]
	if len(idleConnIDs) <= maxIdleConnections {
		return 0, nil
	}

	excessCount := len(idleConnIDs) - maxIdleConnections
	reclaimedCount := 0

	for i := 0; i < excessCount && i < len(idleConnIDs); i++ {
		connID := idleConnIDs[i]
		if conn, exists := r.connections[connID]; exists {
			r.removeFromStateIndex(conn.State, connID)
			conn.State = models.ConnectionStateClosing
			conn.StateChangedAt = time.Now()
			conn.CloseReason = "Resource reclamation - idle connection cleanup"
			conn.CloseCode = 1001
			r.connectionsByState[models.ConnectionStateClosing] = append(r.connectionsByState[models.ConnectionStateClosing], connID)
			reclaimedCount++
		}
	}

	return reclaimedCount, nil
}

// CleanupExpiredConnections removes connections that have exceeded their TTL
func (r *StreamingConnectionRepository) CleanupExpiredConnections(_ context.Context) (int, error) {
	// In-memory implementation doesn't use TTL cleanup
	return 0, nil
}


// ===== Resource Limits =====

// EnforceResourceLimits enforces resource limits on connections
func (r *StreamingConnectionRepository) EnforceResourceLimits(_ context.Context, connectionID string, messageSize int64) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	conn, exists := r.connections[connectionID]
	if !exists {
		return storage.ErrNotFound
	}

	if messageSize > conn.MaxMessageSize {
		return fmt.Errorf("message size %d exceeds limit %d", messageSize, conn.MaxMessageSize)
	}

	now := time.Now()
	if now.After(conn.RateLimitReset) {
		conn.CurrentRate = 0
		conn.RateLimitReset = now.Add(time.Minute)
	}

	if conn.CurrentRate >= conn.RateLimit {
		return fmt.Errorf("rate limit exceeded: %d messages per minute", conn.RateLimit)
	}

	return nil
}

// GetConnectionPool returns current connection pool statistics
func (r *StreamingConnectionRepository) GetConnectionPool(_ context.Context) (map[string]interface{}, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	totalActive := 0
	for _, conn := range r.connections {
		if conn.State == models.ConnectionStateConnected || conn.State == models.ConnectionStateIdle {
			totalActive++
		}
	}

	return map[string]interface{}{
		"total_active":    totalActive,
		"connected":       len(r.connectionsByState[models.ConnectionStateConnected]),
		"idle":            len(r.connectionsByState[models.ConnectionStateIdle]),
		"error":           len(r.connectionsByState[models.ConnectionStateError]),
		"closing":         len(r.connectionsByState[models.ConnectionStateClosing]),
		"max_per_user":    10,
		"max_total":       10000,
		"utilization_pct": float64(totalActive) / float64(10000) * 100,
	}, nil
}

// ===== Message and Activity Tracking =====

// RecordConnectionMessage records message statistics
func (r *StreamingConnectionRepository) RecordConnectionMessage(_ context.Context, connectionID string, sent bool, messageSize int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	conn, exists := r.connections[connectionID]
	if !exists {
		return storage.ErrNotFound
	}

	conn.RecordMessage(sent, messageSize)
	conn.CurrentRate++

	return nil
}

// RecordConnectionError records an error for a connection
func (r *StreamingConnectionRepository) RecordConnectionError(_ context.Context, connectionID string, errorMsg string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	conn, exists := r.connections[connectionID]
	if !exists {
		return storage.ErrNotFound
	}

	conn.IncrementError(errorMsg)

	if conn.Metrics.ErrorCount >= 10 {
		r.removeFromStateIndex(conn.State, connectionID)
		conn.State = models.ConnectionStateError
		conn.CloseReason = "Too many errors"
		r.connectionsByState[models.ConnectionStateError] = append(r.connectionsByState[models.ConnectionStateError], connectionID)
	}

	return nil
}

// RecordPing records a ping for a connection
func (r *StreamingConnectionRepository) RecordPing(_ context.Context, connectionID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	conn, exists := r.connections[connectionID]
	if !exists {
		return storage.ErrNotFound
	}

	conn.RecordPing()
	return nil
}

// RecordPong records a pong for a connection
func (r *StreamingConnectionRepository) RecordPong(_ context.Context, connectionID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	conn, exists := r.connections[connectionID]
	if !exists {
		return storage.ErrNotFound
	}

	conn.RecordPong()
	return nil
}


// ===== Helper Methods =====

// removeFromUserIndex removes a connection from the user index
func (r *StreamingConnectionRepository) removeFromUserIndex(userID, connectionID string) {
	var newConnIDs []string
	for _, connID := range r.connectionsByUser[userID] {
		if connID != connectionID {
			newConnIDs = append(newConnIDs, connID)
		}
	}
	r.connectionsByUser[userID] = newConnIDs
}

// removeFromStateIndex removes a connection from the state index
func (r *StreamingConnectionRepository) removeFromStateIndex(state models.ConnectionState, connectionID string) {
	var newConnIDs []string
	for _, connID := range r.connectionsByState[state] {
		if connID != connectionID {
			newConnIDs = append(newConnIDs, connID)
		}
	}
	r.connectionsByState[state] = newConnIDs
}

// Clear clears all data (test helper)
func (r *StreamingConnectionRepository) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.connections = make(map[string]*models.WebSocketConnection)
	r.connectionsByUser = make(map[string][]string)
	r.connectionsByState = make(map[models.ConnectionState][]string)
	r.subscriptions = make(map[string]*models.WebSocketSubscription)
	r.subscriptionsByStream = make(map[string][]*models.WebSocketSubscription)
	r.subscriptionsByConn = make(map[string][]string)
}

// Ensure StreamingConnectionRepository implements interfaces.StreamingConnectionRepository
var _ interfaces.StreamingConnectionRepository = (*StreamingConnectionRepository)(nil)
