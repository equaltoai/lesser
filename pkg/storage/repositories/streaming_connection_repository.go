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

// StreamingConnectionRepository handles WebSocket connections using DynamORM
type StreamingConnectionRepository struct {
	db               core.DB
	tableName        string
	subscriptionDB   core.DB
	subscriptionTable string
	logger           *zap.Logger
}

// NewStreamingConnectionRepository creates a new repository instance
func NewStreamingConnectionRepository(db core.DB, tableName string, subscriptionDB core.DB, subscriptionTable string, logger *zap.Logger) *StreamingConnectionRepository {
	return &StreamingConnectionRepository{
		db:               db,
		tableName:        tableName,
		subscriptionDB:   subscriptionDB,
		subscriptionTable: subscriptionTable,
		logger:           logger,
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