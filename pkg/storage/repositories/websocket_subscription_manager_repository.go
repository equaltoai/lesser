package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// WebSocketSubscriptionManagerRepository handles WebSocket event subscriptions using DynamORM
type WebSocketSubscriptionManagerRepository struct {
	db        core.DB
	tableName string
	logger    *zap.Logger
}

// NewWebSocketSubscriptionManagerRepository creates a new repository instance
func NewWebSocketSubscriptionManagerRepository(db core.DB, tableName string, logger *zap.Logger) *WebSocketSubscriptionManagerRepository {
	return &WebSocketSubscriptionManagerRepository{
		db:        db,
		tableName: tableName,
		logger:    logger,
	}
}

// HandleConnect stores a new WebSocket connection
func (r *WebSocketSubscriptionManagerRepository) HandleConnect(ctx context.Context, connectionID, userID string) error {
	connection := &models.WebSocketEventConnection{
		ConnectionID: connectionID,
		UserID:       userID,
		ConnectedAt:  time.Now(),
		LastSeen:     time.Now(),
		TTL:          time.Now().Add(24 * time.Hour).Unix(), // Expire after 24 hours
	}

	connection.UpdateKeys()

	err := r.db.WithContext(ctx).Model(connection).Create()
	if err != nil {
		return fmt.Errorf("failed to store connection: %w", err)
	}

	return nil
}

// HandleDisconnect removes a WebSocket connection and its subscriptions
func (r *WebSocketSubscriptionManagerRepository) HandleDisconnect(ctx context.Context, connectionID string) error {
	// First, remove all subscriptions for this connection
	if err := r.CleanupSubscriptions(ctx, connectionID); err != nil {
		r.logger.Warn("failed to cleanup subscriptions during disconnect",
			zap.String("connection_id", connectionID),
			zap.Error(err),
		)
	}

	// Remove the connection itself
	connection := &models.WebSocketEventConnection{
		ConnectionID: connectionID,
	}
	connection.UpdateKeys()

	err := r.db.WithContext(ctx).Model(connection).Delete()
	if err != nil {
		return fmt.Errorf("failed to delete connection: %w", err)
	}

	return nil
}

// GetConnection retrieves a WebSocket connection by connection ID
func (r *WebSocketSubscriptionManagerRepository) GetConnection(ctx context.Context, connectionID string) (*models.WebSocketEventConnection, error) {
	var connection models.WebSocketEventConnection

	err := r.db.WithContext(ctx).Model(&models.WebSocketEventConnection{}).
		Where("PK", "=", fmt.Sprintf("CONNECTION#%s", connectionID)).
		Where("SK", "=", "METADATA").
		First(&connection)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("connection not found")
		}
		return nil, fmt.Errorf("failed to get connection: %w", err)
	}

	return &connection, nil
}

// CreateSubscription creates a new subscription
func (r *WebSocketSubscriptionManagerRepository) CreateSubscription(ctx context.Context, connectionID string, subscriptionType string, filter map[string]any) error {
	subscription := &models.WebSocketEventSubscription{
		ConnectionID:     connectionID,
		SubscriptionType: subscriptionType,
		Filter:           filter,
		CreatedAt:        time.Now(),
		TTL:              time.Now().Add(24 * time.Hour).Unix(),
	}

	subscription.UpdateKeys()

	err := r.db.WithContext(ctx).Model(subscription).Create()
	if err != nil {
		return fmt.Errorf("failed to store subscription: %w", err)
	}

	return nil
}

// DeleteSubscription removes a subscription
func (r *WebSocketSubscriptionManagerRepository) DeleteSubscription(ctx context.Context, connectionID, subscriptionType string) error {
	subscription := &models.WebSocketEventSubscription{
		ConnectionID:     connectionID,
		SubscriptionType: subscriptionType,
	}
	subscription.UpdateKeys()

	err := r.db.WithContext(ctx).Model(subscription).Delete()
	if err != nil {
		return fmt.Errorf("failed to delete subscription: %w", err)
	}

	return nil
}

// GetSubscriptionsForConnection gets all subscriptions for a connection
func (r *WebSocketSubscriptionManagerRepository) GetSubscriptionsForConnection(ctx context.Context, connectionID string) ([]models.WebSocketEventSubscription, error) {
	var subscriptions []models.WebSocketEventSubscription

	// Since DynamORM doesn't support BeginsWith, we'll get all and filter
	// For a more efficient implementation, you'd want to scan/query differently
	err := r.db.WithContext(ctx).Model(&models.WebSocketEventSubscription{}).
		Where("PK", "=", fmt.Sprintf("CONNECTION#%s", connectionID)).
		All(&subscriptions)

	// Filter results to only include subscriptions (SK starts with SUBSCRIPTION#)
	var filteredSubscriptions []models.WebSocketEventSubscription
	for _, sub := range subscriptions {
		if len(sub.SK) > 13 && sub.SK[:13] == "SUBSCRIPTION#" {
			filteredSubscriptions = append(filteredSubscriptions, sub)
		}
	}
	subscriptions = filteredSubscriptions

	if err != nil {
		if errors.IsNotFound(err) {
			return []models.WebSocketEventSubscription{}, nil
		}
		return nil, fmt.Errorf("failed to get subscriptions for connection: %w", err)
	}

	return subscriptions, nil
}

// GetSubscriptionsForType gets all subscriptions for a specific subscription type
func (r *WebSocketSubscriptionManagerRepository) GetSubscriptionsForType(ctx context.Context, subscriptionType string) ([]models.WebSocketEventSubscription, error) {
	var subscriptions []models.WebSocketEventSubscription

	err := r.db.WithContext(ctx).Model(&models.WebSocketEventSubscription{}).
		Where("GSI1PK", "=", fmt.Sprintf("SUBSCRIPTION#%s", subscriptionType)).
		All(&subscriptions)

	if err != nil {
		if errors.IsNotFound(err) {
			return []models.WebSocketEventSubscription{}, nil
		}
		return nil, fmt.Errorf("failed to get subscriptions for type: %w", err)
	}

	return subscriptions, nil
}

// CleanupSubscriptions removes all subscriptions for a connection
func (r *WebSocketSubscriptionManagerRepository) CleanupSubscriptions(ctx context.Context, connectionID string) error {
	// Get all subscriptions for this connection
	subscriptions, err := r.GetSubscriptionsForConnection(ctx, connectionID)
	if err != nil {
		return fmt.Errorf("failed to query subscriptions for cleanup: %w", err)
	}

	// Delete each subscription
	for _, subscription := range subscriptions {
		if err := r.DeleteSubscription(ctx, subscription.ConnectionID, subscription.SubscriptionType); err != nil {
			r.logger.Warn("failed to delete subscription during cleanup",
				zap.String("connection_id", subscription.ConnectionID),
				zap.String("subscription_type", subscription.SubscriptionType),
				zap.Error(err),
			)
		}
	}

	return nil
}

// GetAllConnections gets all active connections (mainly for broadcasting)
func (r *WebSocketSubscriptionManagerRepository) GetAllConnections(_ context.Context) ([]models.WebSocketEventConnection, error) {
	var connections []models.WebSocketEventConnection

	// Since we need all connections, we'll need to scan the table
	// In a real implementation, you might want to use a GSI or different access pattern
	// For now, this is not efficiently implemented as DynamORM doesn't support scanning by prefix easily
	// This method should be reconsidered for production use
	err := fmt.Errorf("GetAllConnections not efficiently supported by DynamORM - needs redesign")

	if err != nil {
		if errors.IsNotFound(err) {
			return []models.WebSocketEventConnection{}, nil
		}
		return nil, fmt.Errorf("failed to get all connections: %w", err)
	}

	return connections, nil
}

// GetUserConnections retrieves all active connection IDs for a user
func (r *WebSocketSubscriptionManagerRepository) GetUserConnections(ctx context.Context, userID string) ([]string, error) {
	var connections []models.WebSocketEventConnection

	// Query connections by GSI2 (UserID index)
	err := r.db.WithContext(ctx).Model(&models.WebSocketEventConnection{}).
		Where("GSI2PK", "=", fmt.Sprintf("USER#%s", userID)).
		Where("GSI2SK", "begins_with", "CONNECTION#").
		All(&connections)

	if err != nil {
		if errors.IsNotFound(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("failed to get user connections: %w", err)
	}

	// Extract connection IDs and filter for active connections
	var connectionIDs []string
	currentTime := time.Now()

	for _, conn := range connections {
		// Check if connection is still valid (not expired)
		if conn.TTL > 0 && currentTime.Unix() > conn.TTL {
			// Connection has expired, skip it
			continue
		}

		// Check if connection was seen recently (within last hour as a reasonable threshold)
		if !conn.LastSeen.IsZero() && currentTime.Sub(conn.LastSeen) > time.Hour {
			// Connection hasn't been seen recently, might be stale
			continue
		}

		connectionIDs = append(connectionIDs, conn.ConnectionID)
	}

	r.logger.Debug("retrieved user connections",
		zap.String("user_id", userID),
		zap.Int("total_connections", len(connections)),
		zap.Int("active_connections", len(connectionIDs)))

	return connectionIDs, nil
}
