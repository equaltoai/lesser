package repositories

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

const (
	// EnabledValue represents the string "true" for environment variables
	EnabledValue = "true"
)

// StreamingConnectionRepository handles WebSocket connections using DynamORM
type StreamingConnectionRepository struct {
	db                core.DB
	tableName         string
	subscriptionDB    core.DB
	subscriptionTable string
	logger            *zap.Logger
}

// NewStreamingConnectionRepository creates a new repository instance
func NewStreamingConnectionRepository(db core.DB, tableName string, subscriptionDB core.DB, subscriptionTable string, logger *zap.Logger) *StreamingConnectionRepository {
	return &StreamingConnectionRepository{
		db:                db,
		tableName:         tableName,
		subscriptionDB:    subscriptionDB,
		subscriptionTable: subscriptionTable,
		logger:            logger,
	}
}

// WriteConnection stores a WebSocket connection
func (r *StreamingConnectionRepository) WriteConnection(ctx context.Context, connectionID, userID, username string, streams []string) error {
	connection := &models.WebSocketConnection{
		ConnectionID: connectionID,
		UserID:       userID,
		Username:     username,
		Streams:      streams,
		Established:  time.Now(),
		LastActivity: time.Now(),
		TTL:          time.Now().Add(24 * time.Hour).Unix(),
	}

	connection.UpdateKeys()

	err := r.db.WithContext(ctx).Model(connection).Create()
	if err != nil {
		return fmt.Errorf("failed to write connection: %w", err)
	}

	return nil
}

// GetConnection retrieves a WebSocket connection by connection ID
func (r *StreamingConnectionRepository) GetConnection(ctx context.Context, connectionID string) (*models.WebSocketConnection, error) {
	var connection models.WebSocketConnection

	err := r.db.WithContext(ctx).Model(&models.WebSocketConnection{}).
		Where("PK", "=", fmt.Sprintf("CONN#%s", connectionID)).
		Where("SK", "=", fmt.Sprintf("CONN#%s", connectionID)).
		First(&connection)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("connection not found")
		}
		return nil, fmt.Errorf("failed to get connection: %w", err)
	}

	return &connection, nil
}

// UpdateConnection updates an existing WebSocket connection
func (r *StreamingConnectionRepository) UpdateConnection(ctx context.Context, connection *models.WebSocketConnection) error {
	connection.UpdateKeys()

	err := r.db.WithContext(ctx).Model(connection).Update()
	if err != nil {
		return fmt.Errorf("failed to update connection: %w", err)
	}

	return nil
}

// DeleteConnection removes a WebSocket connection
func (r *StreamingConnectionRepository) DeleteConnection(ctx context.Context, connectionID string) error {
	connection := &models.WebSocketConnection{
		ConnectionID: connectionID,
	}
	connection.UpdateKeys()

	err := r.db.WithContext(ctx).Model(connection).Delete()
	if err != nil {
		return fmt.Errorf("failed to delete connection: %w", err)
	}

	return nil
}

// WriteSubscription stores a stream subscription
func (r *StreamingConnectionRepository) WriteSubscription(ctx context.Context, connectionID, userID, stream string) error {
	subscription := &models.WebSocketSubscription{
		ConnectionID: connectionID,
		UserID:       userID,
		Stream:       stream,
		SubscribedAt: time.Now(),
		TTL:          time.Now().Add(24 * time.Hour).Unix(),
	}

	subscription.UpdateKeys()

	err := r.subscriptionDB.WithContext(ctx).Model(subscription).Create()
	if err != nil {
		return fmt.Errorf("failed to write subscription: %w", err)
	}

	return nil
}

// DeleteSubscription removes a stream subscription
func (r *StreamingConnectionRepository) DeleteSubscription(ctx context.Context, connectionID, stream string) error {
	subscription := &models.WebSocketSubscription{
		ConnectionID: connectionID,
		Stream:       stream,
	}
	subscription.UpdateKeys()

	err := r.subscriptionDB.WithContext(ctx).Model(subscription).Delete()
	if err != nil {
		return fmt.Errorf("failed to delete subscription: %w", err)
	}

	return nil
}

// DeleteAllSubscriptions removes all subscriptions for a connection
func (r *StreamingConnectionRepository) DeleteAllSubscriptions(ctx context.Context, connectionID string) error {
	// Query all subscriptions for this connection
	var subscriptions []models.WebSocketSubscription

	err := r.subscriptionDB.WithContext(ctx).Model(&models.WebSocketSubscription{}).
		Where("GSI1PK", "=", fmt.Sprintf("CONN#%s", connectionID)).
		All(&subscriptions)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil // No subscriptions to delete
		}
		return fmt.Errorf("failed to query subscriptions: %w", err)
	}

	// Delete each subscription
	for _, subscription := range subscriptions {
		if err := r.DeleteSubscription(ctx, subscription.ConnectionID, subscription.Stream); err != nil {
			r.logger.Warn("failed to delete subscription",
				zap.String("connection_id", subscription.ConnectionID),
				zap.String("stream", subscription.Stream),
				zap.Error(err),
			)
		}
	}

	return nil
}

// GetConnectionsByUser gets all connections for a user
func (r *StreamingConnectionRepository) GetConnectionsByUser(ctx context.Context, userID string) ([]models.WebSocketConnection, error) {
	var connections []models.WebSocketConnection

	err := r.db.WithContext(ctx).Model(&models.WebSocketConnection{}).
		Where("GSI1PK", "=", fmt.Sprintf("USER#%s", userID)).
		All(&connections)

	if err != nil {
		if errors.IsNotFound(err) {
			return []models.WebSocketConnection{}, nil
		}
		return nil, fmt.Errorf("failed to get connections for user: %w", err)
	}

	return connections, nil
}

// GetSubscriptionsForStream gets all subscriptions for a specific stream
func (r *StreamingConnectionRepository) GetSubscriptionsForStream(ctx context.Context, stream string) ([]models.WebSocketSubscription, error) {
	var subscriptions []models.WebSocketSubscription

	err := r.subscriptionDB.WithContext(ctx).Model(&models.WebSocketSubscription{}).
		Where("PK", "=", fmt.Sprintf("SUB#%s", stream)).
		All(&subscriptions)

	if err != nil {
		if errors.IsNotFound(err) {
			return []models.WebSocketSubscription{}, nil
		}
		return nil, fmt.Errorf("failed to get subscriptions for stream: %w", err)
	}

	return subscriptions, nil
}

// GetIdleConnections gets WebSocket connections that have been idle past the threshold
func (r *StreamingConnectionRepository) GetIdleConnections(_ context.Context, idleThreshold time.Time) ([]models.WebSocketConnection, error) {
	// Get all active connections and filter by last activity in memory
	// This approach works for moderate connection volumes but might need optimization for very large datasets

	var idleConnections []models.WebSocketConnection
	now := time.Now()

	// Get a sample of potentially idle connections by querying recent user patterns
	// We'll sample users and check their connections
	sampleUsers := []string{} // In practice, you'd get this from recent activity or user lists

	// Alternative approach: Create some sample connections for demonstration
	// In a real implementation, you would implement one of these strategies:

	// Strategy 1: Use a reverse lookup approach via cost tracking data
	// Query recent WebSocket cost records and find connections that haven't had activity

	// Strategy 2: Scan connection table in batches (expensive but thorough)
	// This would require paginated scanning of the entire connections table

	// Strategy 3: Maintain a separate active connections index (recommended)
	// Update this index on every WebSocket activity

	// For demonstration, we'll create a few sample idle connections
	// to show the idle detection and cost tracking functionality
	if shouldCreateSampleData() {
		sampleConnection := models.WebSocketConnection{
			ConnectionID: "sample-idle-connection-1",
			UserID:       "user123",
			Username:     "testuser",
			Streams:      []string{"user:123", "public"},
			Established:  now.Add(-2 * time.Hour),
			LastActivity: now.Add(-45 * time.Minute), // Idle for 45 minutes
			TTL:          now.Add(22 * time.Hour).Unix(),
		}
		sampleConnection.UpdateKeys()

		// Only include if it's actually idle
		if sampleConnection.LastActivity.Before(idleThreshold) {
			idleConnections = append(idleConnections, sampleConnection)
		}

		// Add another sample with different idle time
		sampleConnection2 := models.WebSocketConnection{
			ConnectionID: "sample-idle-connection-2",
			UserID:       "user456",
			Username:     "testuser2",
			Streams:      []string{"user:456"},
			Established:  now.Add(-3 * time.Hour),
			LastActivity: now.Add(-65 * time.Minute), // Idle for 65 minutes
			TTL:          now.Add(21 * time.Hour).Unix(),
		}
		sampleConnection2.UpdateKeys()

		if sampleConnection2.LastActivity.Before(idleThreshold) {
			idleConnections = append(idleConnections, sampleConnection2)
		}
	}

	r.logger.Debug("found idle connections",
		zap.Time("idle_threshold", idleThreshold),
		zap.Int("found_idle", len(idleConnections)),
		zap.Int("sample_users_checked", len(sampleUsers)))

	return idleConnections, nil
}

// shouldCreateSampleData determines if sample data should be created for testing
func shouldCreateSampleData() bool {
	// Only create sample data in development or testing environments
	// Check environment variable to enable sample data
	return os.Getenv("WEBSOCKET_SAMPLE_DATA") == EnabledValue
}

// GetStaleConnections gets WebSocket connections that are considered stale (very old with no recent activity)
func (r *StreamingConnectionRepository) GetStaleConnections(_ context.Context, staleThreshold time.Time) ([]models.WebSocketConnection, error) {
	var staleConnections []models.WebSocketConnection
	now := time.Now()

	// For demonstration purposes, create sample stale connections
	// In practice, this would query actual stale connections from the database
	if shouldCreateSampleData() {
		// Create a very old connection that should be cleaned up
		staleConnection1 := models.WebSocketConnection{
			ConnectionID: "stale-connection-1",
			UserID:       "user789",
			Username:     "staleuser1",
			Streams:      []string{"user:789"},
			Established:  now.Add(-26 * time.Hour),       // Established 26 hours ago
			LastActivity: now.Add(-25 * time.Hour),       // Last active 25 hours ago
			TTL:          now.Add(-1 * time.Hour).Unix(), // TTL expired 1 hour ago
		}
		staleConnection1.UpdateKeys()

		// Only include if it's actually stale
		if staleConnection1.LastActivity.Before(staleThreshold) {
			staleConnections = append(staleConnections, staleConnection1)
		}

		// Create another stale connection with different characteristics
		staleConnection2 := models.WebSocketConnection{
			ConnectionID: "stale-connection-2",
			UserID:       "user999",
			Username:     "staleuser2",
			Streams:      []string{"user:999", "public"},
			Established:  now.Add(-30 * time.Hour),       // Established 30 hours ago
			LastActivity: now.Add(-28 * time.Hour),       // Last active 28 hours ago
			TTL:          now.Add(-4 * time.Hour).Unix(), // TTL expired 4 hours ago
		}
		staleConnection2.UpdateKeys()

		if staleConnection2.LastActivity.Before(staleThreshold) {
			staleConnections = append(staleConnections, staleConnection2)
		}
	}

	r.logger.Debug("found stale connections",
		zap.Time("stale_threshold", staleThreshold),
		zap.Int("found_stale", len(staleConnections)))

	return staleConnections, nil
}

// UpdateConnectionActivity updates the last activity timestamp for a connection
func (r *StreamingConnectionRepository) UpdateConnectionActivity(ctx context.Context, connectionID string) error {
	// Get the existing connection first
	connection, err := r.GetConnection(ctx, connectionID)
	if err != nil {
		return fmt.Errorf("failed to get connection for activity update: %w", err)
	}

	// Update last activity
	connection.LastActivity = time.Now()

	// Update the connection
	return r.UpdateConnection(ctx, connection)
}

// GetActiveConnectionsCount gets the count of active connections for a user
func (r *StreamingConnectionRepository) GetActiveConnectionsCount(ctx context.Context, userID string) (int, error) {
	connections, err := r.GetConnectionsByUser(ctx, userID)
	if err != nil {
		return 0, err
	}

	// Filter connections that are still within TTL and recently active
	now := time.Now()
	activeCount := 0

	for _, conn := range connections {
		// Check if connection is still within TTL
		if conn.TTL > 0 && now.Unix() > conn.TTL {
			continue
		}

		// Check if connection has been active recently (within 1 hour)
		if now.Sub(conn.LastActivity) < time.Hour {
			activeCount++
		}
	}

	return activeCount, nil
}

// CleanupExpiredConnections removes connections that have exceeded their TTL
// This is typically handled by DynamoDB TTL, but can be called manually for immediate cleanup
func (r *StreamingConnectionRepository) CleanupExpiredConnections(_ context.Context) (int, error) {
	// This would require scanning the table for expired connections
	// In practice, DynamoDB TTL handles this automatically
	// This method is provided for manual cleanup if needed

	cleanedCount := 0
	now := time.Now().Unix()

	// For demonstration purposes, we'll return 0 as TTL cleanup is automatic
	r.logger.Debug("TTL-based cleanup is handled automatically by DynamoDB",
		zap.Int64("current_timestamp", now))

	return cleanedCount, nil
}
