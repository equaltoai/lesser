// Package mocks provides mock implementations for testing.
package mocks

import (
	"context"

	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
)

// MockSeveranceRepository is a mock implementation of interfaces.SeveranceRepository
// using testify/mock for expectation-based testing.
type MockSeveranceRepository struct {
	mock.Mock
}

// NewMockSeveranceRepository creates a new mock severance repository
func NewMockSeveranceRepository() *MockSeveranceRepository {
	return &MockSeveranceRepository{}
}

// ===== Severed Relationship Operations =====

// CreateSeveredRelationship mocks the CreateSeveredRelationship method
func (m *MockSeveranceRepository) CreateSeveredRelationship(ctx context.Context, severance *models.SeveredRelationship) error {
	args := m.Called(ctx, severance)
	return args.Error(0)
}

// GetSeveredRelationship mocks the GetSeveredRelationship method
func (m *MockSeveranceRepository) GetSeveredRelationship(ctx context.Context, id string) (*models.SeveredRelationship, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.SeveredRelationship), args.Error(1)
}

// ListSeveredRelationships mocks the ListSeveredRelationships method
func (m *MockSeveranceRepository) ListSeveredRelationships(ctx context.Context, localInstance string, filters interfaces.SeveranceFilters, limit int, cursor string) ([]*models.SeveredRelationship, string, error) {
	args := m.Called(ctx, localInstance, filters, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*models.SeveredRelationship), args.String(1), args.Error(2)
}

// UpdateSeveranceStatus mocks the UpdateSeveranceStatus method
func (m *MockSeveranceRepository) UpdateSeveranceStatus(ctx context.Context, id string, status models.SeveranceStatus) error {
	args := m.Called(ctx, id, status)
	return args.Error(0)
}

// ===== Affected Relationship Operations =====

// CreateAffectedRelationship mocks the CreateAffectedRelationship method
func (m *MockSeveranceRepository) CreateAffectedRelationship(ctx context.Context, affected *models.AffectedRelationship) error {
	args := m.Called(ctx, affected)
	return args.Error(0)
}

// GetAffectedRelationships mocks the GetAffectedRelationships method
func (m *MockSeveranceRepository) GetAffectedRelationships(ctx context.Context, severanceID string, limit int, cursor string) ([]*models.AffectedRelationship, string, error) {
	args := m.Called(ctx, severanceID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*models.AffectedRelationship), args.String(1), args.Error(2)
}

// ===== Reconnection Attempt Operations =====

// CreateReconnectionAttempt mocks the CreateReconnectionAttempt method
func (m *MockSeveranceRepository) CreateReconnectionAttempt(ctx context.Context, attempt *models.SeveranceReconnectionAttempt) error {
	args := m.Called(ctx, attempt)
	return args.Error(0)
}

// UpdateReconnectionAttempt mocks the UpdateReconnectionAttempt method
func (m *MockSeveranceRepository) UpdateReconnectionAttempt(ctx context.Context, attempt *models.SeveranceReconnectionAttempt) error {
	args := m.Called(ctx, attempt)
	return args.Error(0)
}

// GetReconnectionAttempt mocks the GetReconnectionAttempt method
func (m *MockSeveranceRepository) GetReconnectionAttempt(ctx context.Context, severanceID, attemptID string) (*models.SeveranceReconnectionAttempt, error) {
	args := m.Called(ctx, severanceID, attemptID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.SeveranceReconnectionAttempt), args.Error(1)
}

// GetReconnectionAttempts mocks the GetReconnectionAttempts method
func (m *MockSeveranceRepository) GetReconnectionAttempts(ctx context.Context, severanceID string) ([]*models.SeveranceReconnectionAttempt, error) {
	args := m.Called(ctx, severanceID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.SeveranceReconnectionAttempt), args.Error(1)
}

// Ensure MockSeveranceRepository implements interfaces.SeveranceRepository
var _ interfaces.SeveranceRepository = (*MockSeveranceRepository)(nil)
