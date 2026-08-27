// Package mocks provides mock implementations for testing.
package mocks

import (
	"context"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
)

// MockDLQRepository is a mock implementation of interfaces.DLQRepository
// using testify/mock for expectation-based testing.
type MockDLQRepository struct {
	mock.Mock
}

// NewMockDLQRepository creates a new mock DLQ repository
func NewMockDLQRepository() *MockDLQRepository {
	return &MockDLQRepository{}
}

// ===== Core DLQ Operations =====

// CreateDLQMessage mocks the CreateDLQMessage method
func (m *MockDLQRepository) CreateDLQMessage(ctx context.Context, message *models.DLQMessage) error {
	args := m.Called(ctx, message)
	return args.Error(0)
}

// GetDLQMessage mocks the GetDLQMessage method
func (m *MockDLQRepository) GetDLQMessage(ctx context.Context, id string) (*models.DLQMessage, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.DLQMessage), args.Error(1)
}

// UpdateDLQMessage mocks the UpdateDLQMessage method
func (m *MockDLQRepository) UpdateDLQMessage(ctx context.Context, message *models.DLQMessage) error {
	args := m.Called(ctx, message)
	return args.Error(0)
}

// DeleteDLQMessage mocks the DeleteDLQMessage method
func (m *MockDLQRepository) DeleteDLQMessage(ctx context.Context, message *models.DLQMessage) error {
	args := m.Called(ctx, message)
	return args.Error(0)
}

// BatchUpdateDLQMessages mocks the BatchUpdateDLQMessages method
func (m *MockDLQRepository) BatchUpdateDLQMessages(ctx context.Context, messages []*models.DLQMessage) error {
	args := m.Called(ctx, messages)
	return args.Error(0)
}

// ===== Query Operations =====

// GetDLQMessagesByService mocks the GetDLQMessagesByService method
func (m *MockDLQRepository) GetDLQMessagesByService(ctx context.Context, service string, date time.Time, limit int, cursor string) ([]*models.DLQMessage, string, error) {
	args := m.Called(ctx, service, date, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*models.DLQMessage), args.String(1), args.Error(2)
}

// GetDLQMessagesByServiceDateRange mocks the GetDLQMessagesByServiceDateRange method
func (m *MockDLQRepository) GetDLQMessagesByServiceDateRange(ctx context.Context, service string, startDate, endDate time.Time, limit int) ([]*models.DLQMessage, error) {
	args := m.Called(ctx, service, startDate, endDate, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.DLQMessage), args.Error(1)
}

// GetDLQMessagesByErrorType mocks the GetDLQMessagesByErrorType method
func (m *MockDLQRepository) GetDLQMessagesByErrorType(ctx context.Context, errorType string, limit int, cursor string) ([]*models.DLQMessage, string, error) {
	args := m.Called(ctx, errorType, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*models.DLQMessage), args.String(1), args.Error(2)
}

// GetDLQMessagesForReprocessing mocks the GetDLQMessagesForReprocessing method
func (m *MockDLQRepository) GetDLQMessagesForReprocessing(ctx context.Context, service string, status string, limit int, cursor string) ([]*models.DLQMessage, string, error) {
	args := m.Called(ctx, service, status, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*models.DLQMessage), args.String(1), args.Error(2)
}

// GetDLQMessagesByStatus mocks the GetDLQMessagesByStatus method
func (m *MockDLQRepository) GetDLQMessagesByStatus(ctx context.Context, service, status string, limit int, cursor string) ([]*models.DLQMessage, string, error) {
	args := m.Called(ctx, service, status, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*models.DLQMessage), args.String(1), args.Error(2)
}

// SearchDLQMessages mocks the SearchDLQMessages method
func (m *MockDLQRepository) SearchDLQMessages(ctx context.Context, filter *interfaces.DLQSearchFilter) ([]*models.DLQMessage, string, error) {
	args := m.Called(ctx, filter)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*models.DLQMessage), args.String(1), args.Error(2)
}

// ===== Analytics Operations =====

// GetDLQAnalytics mocks the GetDLQAnalytics method
func (m *MockDLQRepository) GetDLQAnalytics(ctx context.Context, service string, timeRange interfaces.DLQTimeRange) (*interfaces.DLQAnalytics, error) {
	args := m.Called(ctx, service, timeRange)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.DLQAnalytics), args.Error(1)
}

// GetDLQTrends mocks the GetDLQTrends method
func (m *MockDLQRepository) GetDLQTrends(ctx context.Context, service string, days int) (*interfaces.DLQTrends, error) {
	args := m.Called(ctx, service, days)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.DLQTrends), args.Error(1)
}

// AnalyzeFailurePatterns mocks the AnalyzeFailurePatterns method
func (m *MockDLQRepository) AnalyzeFailurePatterns(ctx context.Context, service string, days int) (map[string]*interfaces.DLQSimilarityGroup, error) {
	args := m.Called(ctx, service, days)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]*interfaces.DLQSimilarityGroup), args.Error(1)
}

// ===== Retry Operations =====

// SendToDeadLetterQueue mocks the SendToDeadLetterQueue method
func (m *MockDLQRepository) SendToDeadLetterQueue(ctx context.Context, service, messageID, messageBody, errorType, errorMessage string, isPermanent bool) error {
	args := m.Called(ctx, service, messageID, messageBody, errorType, errorMessage, isPermanent)
	return args.Error(0)
}

// RetryFailedMessage mocks the RetryFailedMessage method
func (m *MockDLQRepository) RetryFailedMessage(ctx context.Context, messageID string) error {
	args := m.Called(ctx, messageID)
	return args.Error(0)
}

// GetRetryableMessages mocks the GetRetryableMessages method
func (m *MockDLQRepository) GetRetryableMessages(ctx context.Context, service string, limit int) ([]*models.DLQMessage, error) {
	args := m.Called(ctx, service, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.DLQMessage), args.Error(1)
}

// ===== Cleanup Operations =====

// CleanupExpiredMessages mocks the CleanupExpiredMessages method
func (m *MockDLQRepository) CleanupExpiredMessages(ctx context.Context, before time.Time) (int, error) {
	args := m.Called(ctx, before)
	return args.Int(0), args.Error(1)
}

// ===== Health Monitoring =====

// MonitorDLQHealth mocks the MonitorDLQHealth method
func (m *MockDLQRepository) MonitorDLQHealth(ctx context.Context, service string) (*interfaces.DLQHealthStatus, error) {
	args := m.Called(ctx, service)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.DLQHealthStatus), args.Error(1)
}

// Ensure MockDLQRepository implements interfaces.DLQRepository
var _ interfaces.DLQRepository = (*MockDLQRepository)(nil)
