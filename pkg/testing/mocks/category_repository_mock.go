// Package mocks provides mock implementations for testing.
package mocks

import (
	"context"

	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	dynamormcore "github.com/theory-cloud/tabletheory/pkg/core"
)

// MockCategoryRepository is a mock implementation of interfaces.CategoryRepository
// using testify/mock for expectation-based testing.
type MockCategoryRepository struct {
	mock.Mock
}

// NewMockCategoryRepository creates a new mock category repository
func NewMockCategoryRepository() *MockCategoryRepository {
	return &MockCategoryRepository{}
}

// ===== Database Access =====

// GetDB mocks the GetDB method
func (m *MockCategoryRepository) GetDB() dynamormcore.DB {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(dynamormcore.DB)
}

// ===== Core CRUD Operations =====

// CreateCategory mocks the CreateCategory method
func (m *MockCategoryRepository) CreateCategory(ctx context.Context, category *models.Category) error {
	args := m.Called(ctx, category)
	return args.Error(0)
}

// GetCategory mocks the GetCategory method
func (m *MockCategoryRepository) GetCategory(ctx context.Context, id string) (*models.Category, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Category), args.Error(1)
}

// Update mocks the Update method
func (m *MockCategoryRepository) Update(ctx context.Context, category *models.Category) error {
	args := m.Called(ctx, category)
	return args.Error(0)
}

// Delete mocks the Delete method
func (m *MockCategoryRepository) Delete(ctx context.Context, pk, sk string) error {
	args := m.Called(ctx, pk, sk)
	return args.Error(0)
}

// ===== List Operations =====

// ListCategories mocks the ListCategories method
func (m *MockCategoryRepository) ListCategories(ctx context.Context, parentID *string, limit int) ([]*models.Category, error) {
	args := m.Called(ctx, parentID, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.Category), args.Error(1)
}

// ===== Count Operations =====

// UpdateArticleCount mocks the UpdateArticleCount method
func (m *MockCategoryRepository) UpdateArticleCount(ctx context.Context, categoryID string, delta int) error {
	args := m.Called(ctx, categoryID, delta)
	return args.Error(0)
}

// Ensure MockCategoryRepository implements interfaces.CategoryRepository
var _ interfaces.CategoryRepository = (*MockCategoryRepository)(nil)
