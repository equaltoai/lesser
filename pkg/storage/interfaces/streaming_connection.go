// Package interfaces defines the repository interfaces for the Lesser application.
package interfaces //nolint:revive // Standard interfaces package name

import (
	"context"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
)

// StreamingConnectionRepository defines the interface for WebSocket connection operations.
// This handles WebSocket connections with complete lifecycle management.
type StreamingConnectionRepository interface {
	// Connection lifecycle operations
	WriteConnection(ctx context.Context, connectionID, userID, username string, streams []string) (*models.WebSocketConnection, error)
	GetConnection(ctx context.Context, connectionID string) (*models.WebSocketConnection, error)
	UpdateConnection(ctx context.Context, connection *models.WebSocketConnection) error
	DeleteConnection(ctx context.Context, connectionID string) error
	UpdateConnectionState(ctx context.Context, connectionID string, newState models.ConnectionState, reason string) error
	UpdateConnectionActivity(ctx context.Context, connectionID string) error

	// Subscription operations
	WriteSubscription(ctx context.Context, connectionID, userID, stream string) error
	DeleteSubscription(ctx context.Context, connectionID, stream string) error
	DeleteAllSubscriptions(ctx context.Context, connectionID string) error
	GetSubscriptionsForStream(ctx context.Context, stream string) ([]models.WebSocketSubscription, error)

	// Connection queries
	GetConnectionsByUser(ctx context.Context, userID string) ([]models.WebSocketConnection, error)
	GetConnectionsByState(ctx context.Context, state models.ConnectionState) ([]models.WebSocketConnection, error)
	GetIdleConnections(ctx context.Context, idleThreshold time.Time) ([]models.WebSocketConnection, error)
	GetStaleConnections(ctx context.Context, staleThreshold time.Time) ([]models.WebSocketConnection, error)
	GetHealthyConnections(ctx context.Context) ([]models.WebSocketConnection, error)
	GetUnhealthyConnections(ctx context.Context) ([]models.WebSocketConnection, error)

	// Connection counts
	GetActiveConnectionsCount(ctx context.Context, userID string) (int, error)
	GetTotalActiveConnectionsCount(ctx context.Context) (int, error)
	GetConnectionCountByState(ctx context.Context, state models.ConnectionState) (int, error)
	GetUserConnectionCount(ctx context.Context, userID string) (int, error)

	// Connection management
	MarkConnectionsIdle(ctx context.Context, idleThreshold time.Duration) (int, error)
	CloseTimedOutConnections(ctx context.Context) (int, error)
	ReclaimIdleConnections(ctx context.Context, maxIdleConnections int) (int, error)
	CleanupExpiredConnections(ctx context.Context) (int, error)

	// Resource limits
	EnforceResourceLimits(ctx context.Context, connectionID string, messageSize int64) error
	GetConnectionPool(ctx context.Context) (map[string]interface{}, error)

	// Message and activity tracking
	RecordConnectionMessage(ctx context.Context, connectionID string, sent bool, messageSize int64) error
	RecordConnectionError(ctx context.Context, connectionID string, errorMsg string) error
	RecordPing(ctx context.Context, connectionID string) error
	RecordPong(ctx context.Context, connectionID string) error
}
