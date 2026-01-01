// Package mocks provides mock implementations for testing.
package mocks

import (
	"context"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
)

// MockStreamingConnectionRepository is a mock implementation of interfaces.StreamingConnectionRepository
// using testify/mock for expectation-based testing.
type MockStreamingConnectionRepository struct {
	mock.Mock
}

// NewMockStreamingConnectionRepository creates a new mock streaming connection repository
func NewMockStreamingConnectionRepository() *MockStreamingConnectionRepository {
	return &MockStreamingConnectionRepository{}
}

// ===== Connection Lifecycle Operations =====

// WriteConnection mocks the WriteConnection method
func (m *MockStreamingConnectionRepository) WriteConnection(ctx context.Context, connectionID, userID, username string, streams []string) (*models.WebSocketConnection, error) {
	args := m.Called(ctx, connectionID, userID, username, streams)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.WebSocketConnection), args.Error(1)
}

// GetConnection mocks the GetConnection method
func (m *MockStreamingConnectionRepository) GetConnection(ctx context.Context, connectionID string) (*models.WebSocketConnection, error) {
	args := m.Called(ctx, connectionID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.WebSocketConnection), args.Error(1)
}

// UpdateConnection mocks the UpdateConnection method
func (m *MockStreamingConnectionRepository) UpdateConnection(ctx context.Context, connection *models.WebSocketConnection) error {
	args := m.Called(ctx, connection)
	return args.Error(0)
}

// DeleteConnection mocks the DeleteConnection method
func (m *MockStreamingConnectionRepository) DeleteConnection(ctx context.Context, connectionID string) error {
	args := m.Called(ctx, connectionID)
	return args.Error(0)
}

// UpdateConnectionState mocks the UpdateConnectionState method
func (m *MockStreamingConnectionRepository) UpdateConnectionState(ctx context.Context, connectionID string, newState models.ConnectionState, reason string) error {
	args := m.Called(ctx, connectionID, newState, reason)
	return args.Error(0)
}

// UpdateConnectionActivity mocks the UpdateConnectionActivity method
func (m *MockStreamingConnectionRepository) UpdateConnectionActivity(ctx context.Context, connectionID string) error {
	args := m.Called(ctx, connectionID)
	return args.Error(0)
}

// ===== Subscription Operations =====

// WriteSubscription mocks the WriteSubscription method
func (m *MockStreamingConnectionRepository) WriteSubscription(ctx context.Context, connectionID, userID, stream string) error {
	args := m.Called(ctx, connectionID, userID, stream)
	return args.Error(0)
}

// DeleteSubscription mocks the DeleteSubscription method
func (m *MockStreamingConnectionRepository) DeleteSubscription(ctx context.Context, connectionID, stream string) error {
	args := m.Called(ctx, connectionID, stream)
	return args.Error(0)
}

// DeleteAllSubscriptions mocks the DeleteAllSubscriptions method
func (m *MockStreamingConnectionRepository) DeleteAllSubscriptions(ctx context.Context, connectionID string) error {
	args := m.Called(ctx, connectionID)
	return args.Error(0)
}

// GetSubscriptionsForStream mocks the GetSubscriptionsForStream method
func (m *MockStreamingConnectionRepository) GetSubscriptionsForStream(ctx context.Context, stream string) ([]models.WebSocketSubscription, error) {
	args := m.Called(ctx, stream)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]models.WebSocketSubscription), args.Error(1)
}

// ===== Connection Queries =====

// GetConnectionsByUser mocks the GetConnectionsByUser method
func (m *MockStreamingConnectionRepository) GetConnectionsByUser(ctx context.Context, userID string) ([]models.WebSocketConnection, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]models.WebSocketConnection), args.Error(1)
}

// GetConnectionsByState mocks the GetConnectionsByState method
func (m *MockStreamingConnectionRepository) GetConnectionsByState(ctx context.Context, state models.ConnectionState) ([]models.WebSocketConnection, error) {
	args := m.Called(ctx, state)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]models.WebSocketConnection), args.Error(1)
}

// GetIdleConnections mocks the GetIdleConnections method
func (m *MockStreamingConnectionRepository) GetIdleConnections(ctx context.Context, idleThreshold time.Time) ([]models.WebSocketConnection, error) {
	args := m.Called(ctx, idleThreshold)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]models.WebSocketConnection), args.Error(1)
}

// GetStaleConnections mocks the GetStaleConnections method
func (m *MockStreamingConnectionRepository) GetStaleConnections(ctx context.Context, staleThreshold time.Time) ([]models.WebSocketConnection, error) {
	args := m.Called(ctx, staleThreshold)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]models.WebSocketConnection), args.Error(1)
}

// GetHealthyConnections mocks the GetHealthyConnections method
func (m *MockStreamingConnectionRepository) GetHealthyConnections(ctx context.Context) ([]models.WebSocketConnection, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]models.WebSocketConnection), args.Error(1)
}

// GetUnhealthyConnections mocks the GetUnhealthyConnections method
func (m *MockStreamingConnectionRepository) GetUnhealthyConnections(ctx context.Context) ([]models.WebSocketConnection, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]models.WebSocketConnection), args.Error(1)
}

// ===== Connection Counts =====

// GetActiveConnectionsCount mocks the GetActiveConnectionsCount method
func (m *MockStreamingConnectionRepository) GetActiveConnectionsCount(ctx context.Context, userID string) (int, error) {
	args := m.Called(ctx, userID)
	return args.Int(0), args.Error(1)
}

// GetTotalActiveConnectionsCount mocks the GetTotalActiveConnectionsCount method
func (m *MockStreamingConnectionRepository) GetTotalActiveConnectionsCount(ctx context.Context) (int, error) {
	args := m.Called(ctx)
	return args.Int(0), args.Error(1)
}

// GetConnectionCountByState mocks the GetConnectionCountByState method
func (m *MockStreamingConnectionRepository) GetConnectionCountByState(ctx context.Context, state models.ConnectionState) (int, error) {
	args := m.Called(ctx, state)
	return args.Int(0), args.Error(1)
}

// GetUserConnectionCount mocks the GetUserConnectionCount method
func (m *MockStreamingConnectionRepository) GetUserConnectionCount(ctx context.Context, userID string) (int, error) {
	args := m.Called(ctx, userID)
	return args.Int(0), args.Error(1)
}

// ===== Connection Management =====

// MarkConnectionsIdle mocks the MarkConnectionsIdle method
func (m *MockStreamingConnectionRepository) MarkConnectionsIdle(ctx context.Context, idleThreshold time.Duration) (int, error) {
	args := m.Called(ctx, idleThreshold)
	return args.Int(0), args.Error(1)
}

// CloseTimedOutConnections mocks the CloseTimedOutConnections method
func (m *MockStreamingConnectionRepository) CloseTimedOutConnections(ctx context.Context) (int, error) {
	args := m.Called(ctx)
	return args.Int(0), args.Error(1)
}

// ReclaimIdleConnections mocks the ReclaimIdleConnections method
func (m *MockStreamingConnectionRepository) ReclaimIdleConnections(ctx context.Context, maxIdleConnections int) (int, error) {
	args := m.Called(ctx, maxIdleConnections)
	return args.Int(0), args.Error(1)
}

// CleanupExpiredConnections mocks the CleanupExpiredConnections method
func (m *MockStreamingConnectionRepository) CleanupExpiredConnections(ctx context.Context) (int, error) {
	args := m.Called(ctx)
	return args.Int(0), args.Error(1)
}

// ===== Resource Limits =====

// EnforceResourceLimits mocks the EnforceResourceLimits method
func (m *MockStreamingConnectionRepository) EnforceResourceLimits(ctx context.Context, connectionID string, messageSize int64) error {
	args := m.Called(ctx, connectionID, messageSize)
	return args.Error(0)
}

// GetConnectionPool mocks the GetConnectionPool method
func (m *MockStreamingConnectionRepository) GetConnectionPool(ctx context.Context) (map[string]interface{}, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]interface{}), args.Error(1)
}

// ===== Message and Activity Tracking =====

// RecordConnectionMessage mocks the RecordConnectionMessage method
func (m *MockStreamingConnectionRepository) RecordConnectionMessage(ctx context.Context, connectionID string, sent bool, messageSize int64) error {
	args := m.Called(ctx, connectionID, sent, messageSize)
	return args.Error(0)
}

// RecordConnectionError mocks the RecordConnectionError method
func (m *MockStreamingConnectionRepository) RecordConnectionError(ctx context.Context, connectionID string, errorMsg string) error {
	args := m.Called(ctx, connectionID, errorMsg)
	return args.Error(0)
}

// RecordPing mocks the RecordPing method
func (m *MockStreamingConnectionRepository) RecordPing(ctx context.Context, connectionID string) error {
	args := m.Called(ctx, connectionID)
	return args.Error(0)
}

// RecordPong mocks the RecordPong method
func (m *MockStreamingConnectionRepository) RecordPong(ctx context.Context, connectionID string) error {
	args := m.Called(ctx, connectionID)
	return args.Error(0)
}

// Ensure MockStreamingConnectionRepository implements interfaces.StreamingConnectionRepository
var _ interfaces.StreamingConnectionRepository = (*MockStreamingConnectionRepository)(nil)
