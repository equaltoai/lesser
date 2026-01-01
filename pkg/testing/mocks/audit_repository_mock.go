// Package mocks provides mock implementations for testing.
package mocks

import (
	"context"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
)

// MockAuditRepository is a mock implementation of interfaces.AuditRepository
// using testify/mock for expectation-based testing.
type MockAuditRepository struct {
	mock.Mock
}

// NewMockAuditRepository creates a new mock audit repository
func NewMockAuditRepository() *MockAuditRepository {
	return &MockAuditRepository{}
}

// ===== Core Audit Operations =====

// StoreAuditLog mocks the StoreAuditLog method
func (m *MockAuditRepository) StoreAuditLog(ctx context.Context, log *models.AuthAuditLog) error {
	args := m.Called(ctx, log)
	return args.Error(0)
}

// GetAuditLogByID mocks the GetAuditLogByID method
func (m *MockAuditRepository) GetAuditLogByID(ctx context.Context, id string, date time.Time) (*models.AuthAuditLog, error) {
	args := m.Called(ctx, id, date)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.AuthAuditLog), args.Error(1)
}

// ===== Query Operations =====

// GetUserAuditLogs mocks the GetUserAuditLogs method
func (m *MockAuditRepository) GetUserAuditLogs(ctx context.Context, username string, limit int, startTime, endTime time.Time) ([]*models.AuthAuditLog, error) {
	args := m.Called(ctx, username, limit, startTime, endTime)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.AuthAuditLog), args.Error(1)
}

// GetIPAuditLogs mocks the GetIPAuditLogs method
func (m *MockAuditRepository) GetIPAuditLogs(ctx context.Context, ipAddress string, limit int, startTime, endTime time.Time) ([]*models.AuthAuditLog, error) {
	args := m.Called(ctx, ipAddress, limit, startTime, endTime)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.AuthAuditLog), args.Error(1)
}

// GetSessionAuditLogs mocks the GetSessionAuditLogs method
func (m *MockAuditRepository) GetSessionAuditLogs(ctx context.Context, sessionID string) ([]*models.AuthAuditLog, error) {
	args := m.Called(ctx, sessionID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.AuthAuditLog), args.Error(1)
}

// GetSecurityEvents mocks the GetSecurityEvents method
func (m *MockAuditRepository) GetSecurityEvents(ctx context.Context, severity string, startTime, endTime time.Time, limit int, cursor string) ([]*models.AuthAuditLog, string, error) {
	args := m.Called(ctx, severity, startTime, endTime, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*models.AuthAuditLog), args.String(1), args.Error(2)
}

// ===== Analytics Operations =====

// GetRecentFailedLogins mocks the GetRecentFailedLogins method
func (m *MockAuditRepository) GetRecentFailedLogins(ctx context.Context, username string, duration time.Duration) (int, error) {
	args := m.Called(ctx, username, duration)
	return args.Int(0), args.Error(1)
}

// GetRecentIPFailures mocks the GetRecentIPFailures method
func (m *MockAuditRepository) GetRecentIPFailures(ctx context.Context, ipAddress string, duration time.Duration) (int, error) {
	args := m.Called(ctx, ipAddress, duration)
	return args.Int(0), args.Error(1)
}

// ===== Cleanup Operations =====

// CleanupOldLogs mocks the CleanupOldLogs method
func (m *MockAuditRepository) CleanupOldLogs(ctx context.Context, retentionDays int) error {
	args := m.Called(ctx, retentionDays)
	return args.Error(0)
}

// ===== Event Storage =====

// StoreAuditEvent mocks the StoreAuditEvent method
func (m *MockAuditRepository) StoreAuditEvent(ctx context.Context, eventType, severity, username, userID, ipAddress, userAgent, deviceName, sessionID, requestID string, success bool, failureReason string, metadata map[string]interface{}) error {
	args := m.Called(ctx, eventType, severity, username, userID, ipAddress, userAgent, deviceName, sessionID, requestID, success, failureReason, metadata)
	return args.Error(0)
}

// Ensure MockAuditRepository implements interfaces.AuditRepository
var _ interfaces.AuditRepository = (*MockAuditRepository)(nil)
