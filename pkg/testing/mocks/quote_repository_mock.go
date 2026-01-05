// Package mocks provides mock implementations for testing.
package mocks

import (
	"context"

	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
)

// MockQuoteRepository is a mock implementation of interfaces.QuoteRepository
// using testify/mock for expectation-based testing.
type MockQuoteRepository struct {
	mock.Mock
}

// NewMockQuoteRepository creates a new mock quote repository
func NewMockQuoteRepository() *MockQuoteRepository {
	return &MockQuoteRepository{}
}

// ===== Quote Relationship Operations =====

// CreateQuoteRelationship mocks the CreateQuoteRelationship method
func (m *MockQuoteRepository) CreateQuoteRelationship(ctx context.Context, relationship *models.QuoteRelationship) error {
	args := m.Called(ctx, relationship)
	return args.Error(0)
}

// GetQuoteRelationship mocks the GetQuoteRelationship method
func (m *MockQuoteRepository) GetQuoteRelationship(ctx context.Context, quoteStatusID, targetStatusID string) (*models.QuoteRelationship, error) {
	args := m.Called(ctx, quoteStatusID, targetStatusID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.QuoteRelationship), args.Error(1)
}

// UpdateQuoteRelationship mocks the UpdateQuoteRelationship method
func (m *MockQuoteRepository) UpdateQuoteRelationship(ctx context.Context, relationship *models.QuoteRelationship) error {
	args := m.Called(ctx, relationship)
	return args.Error(0)
}

// DeleteQuoteRelationship mocks the DeleteQuoteRelationship method
func (m *MockQuoteRepository) DeleteQuoteRelationship(ctx context.Context, quoteStatusID, targetStatusID string) error {
	args := m.Called(ctx, quoteStatusID, targetStatusID)
	return args.Error(0)
}

// ===== Quote Query Operations =====

// GetQuotesForStatus mocks the GetQuotesForStatus method
func (m *MockQuoteRepository) GetQuotesForStatus(ctx context.Context, statusID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.QuoteRelationship], error) {
	args := m.Called(ctx, statusID, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*models.QuoteRelationship]), args.Error(1)
}

// GetQuotesByUser mocks the GetQuotesByUser method
func (m *MockQuoteRepository) GetQuotesByUser(ctx context.Context, userID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.QuoteRelationship], error) {
	args := m.Called(ctx, userID, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*models.QuoteRelationship]), args.Error(1)
}

// ===== Quote Permissions Operations =====

// CreateQuotePermissions mocks the CreateQuotePermissions method
func (m *MockQuoteRepository) CreateQuotePermissions(ctx context.Context, permissions *models.QuotePermissions) error {
	args := m.Called(ctx, permissions)
	return args.Error(0)
}

// GetQuotePermissions mocks the GetQuotePermissions method
func (m *MockQuoteRepository) GetQuotePermissions(ctx context.Context, username string) (*models.QuotePermissions, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.QuotePermissions), args.Error(1)
}

// UpdateQuotePermissions mocks the UpdateQuotePermissions method
func (m *MockQuoteRepository) UpdateQuotePermissions(ctx context.Context, permissions *models.QuotePermissions) error {
	args := m.Called(ctx, permissions)
	return args.Error(0)
}

// DeleteQuotePermissions mocks the DeleteQuotePermissions method
func (m *MockQuoteRepository) DeleteQuotePermissions(ctx context.Context, username string) error {
	args := m.Called(ctx, username)
	return args.Error(0)
}

// ===== Quote Count Operations =====

// GetQuoteCount mocks the GetQuoteCount method
func (m *MockQuoteRepository) GetQuoteCount(ctx context.Context, statusID string) (int64, error) {
	args := m.Called(ctx, statusID)
	return args.Get(0).(int64), args.Error(1)
}

// IncrementQuoteCount mocks the IncrementQuoteCount method
func (m *MockQuoteRepository) IncrementQuoteCount(ctx context.Context, statusID string) error {
	args := m.Called(ctx, statusID)
	return args.Error(0)
}

// DecrementQuoteCount mocks the DecrementQuoteCount method
func (m *MockQuoteRepository) DecrementQuoteCount(ctx context.Context, statusID string) error {
	args := m.Called(ctx, statusID)
	return args.Error(0)
}

// ===== Quote Withdrawal Operations =====

// WithdrawQuotes mocks the WithdrawQuotes method
func (m *MockQuoteRepository) WithdrawQuotes(ctx context.Context, noteID, userID string) (int, error) {
	args := m.Called(ctx, noteID, userID)
	return args.Int(0), args.Error(1)
}

// Ensure MockQuoteRepository implements interfaces.QuoteRepository
var _ interfaces.QuoteRepository = (*MockQuoteRepository)(nil)
