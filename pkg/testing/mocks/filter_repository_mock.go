// Package mocks provides mock implementations for testing.
package mocks

import (
	"context"

	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
)

// MockFilterRepository is a mock implementation of interfaces.FilterRepository
// using testify/mock for expectation-based testing.
type MockFilterRepository struct {
	mock.Mock
}

// NewMockFilterRepository creates a new mock filter repository
func NewMockFilterRepository() *MockFilterRepository {
	return &MockFilterRepository{}
}

// ===== Filter CRUD Operations =====

// CreateFilter mocks the CreateFilter method
func (m *MockFilterRepository) CreateFilter(ctx context.Context, filter *models.Filter) error {
	args := m.Called(ctx, filter)
	return args.Error(0)
}

// GetFilter mocks the GetFilter method
func (m *MockFilterRepository) GetFilter(ctx context.Context, filterID string) (*models.Filter, error) {
	args := m.Called(ctx, filterID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Filter), args.Error(1)
}

// UpdateFilter mocks the UpdateFilter method
func (m *MockFilterRepository) UpdateFilter(ctx context.Context, filter *models.Filter) error {
	args := m.Called(ctx, filter)
	return args.Error(0)
}

// ===== Filter Query Operations =====

// GetUserFilters mocks the GetUserFilters method
func (m *MockFilterRepository) GetUserFilters(ctx context.Context, username string) ([]*models.Filter, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.Filter), args.Error(1)
}

// GetActiveFilters mocks the GetActiveFilters method
func (m *MockFilterRepository) GetActiveFilters(ctx context.Context, username string, context []string) ([]*models.Filter, error) {
	args := m.Called(ctx, username, context)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.Filter), args.Error(1)
}

// ===== Filter Keyword Operations =====

// AddFilterKeyword mocks the AddFilterKeyword method
func (m *MockFilterRepository) AddFilterKeyword(ctx context.Context, keyword *models.FilterKeyword) error {
	args := m.Called(ctx, keyword)
	return args.Error(0)
}

// GetFilterKeywords mocks the GetFilterKeywords method
func (m *MockFilterRepository) GetFilterKeywords(ctx context.Context, filterID string) ([]*models.FilterKeyword, error) {
	args := m.Called(ctx, filterID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.FilterKeyword), args.Error(1)
}

// ===== Filter Status Operations =====

// AddFilterStatus mocks the AddFilterStatus method
func (m *MockFilterRepository) AddFilterStatus(ctx context.Context, filterStatus *models.FilterStatus) error {
	args := m.Called(ctx, filterStatus)
	return args.Error(0)
}

// GetFilterStatuses mocks the GetFilterStatuses method
func (m *MockFilterRepository) GetFilterStatuses(ctx context.Context, filterID string) ([]*models.FilterStatus, error) {
	args := m.Called(ctx, filterID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.FilterStatus), args.Error(1)
}

// ===== Filter Evaluation Operations =====

// EvaluateFilters mocks the EvaluateFilters method
func (m *MockFilterRepository) EvaluateFilters(ctx context.Context, username string, content string, context []string) ([]*models.Filter, error) {
	args := m.Called(ctx, username, content, context)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.Filter), args.Error(1)
}

// CheckContentFiltered mocks the CheckContentFiltered method
func (m *MockFilterRepository) CheckContentFiltered(ctx context.Context, username, statusID string, context []string) (bool, []*models.Filter, error) {
	args := m.Called(ctx, username, statusID, context)
	if args.Get(1) == nil {
		return args.Bool(0), nil, args.Error(2)
	}
	return args.Bool(0), args.Get(1).([]*models.Filter), args.Error(2)
}

// Ensure MockFilterRepository implements interfaces.FilterRepository
var _ interfaces.FilterRepository = (*MockFilterRepository)(nil)
