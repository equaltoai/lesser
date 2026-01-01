// Package mocks provides mock implementations for testing.
package mocks

import (
	"context"

	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
)

// MockRevisionRepository is a mock implementation of interfaces.RevisionRepository
// using testify/mock for expectation-based testing.
type MockRevisionRepository struct {
	mock.Mock
}

// NewMockRevisionRepository creates a new mock revision repository
func NewMockRevisionRepository() *MockRevisionRepository {
	return &MockRevisionRepository{}
}

// ===== Core CRUD Operations =====

// CreateRevision mocks the CreateRevision method
func (m *MockRevisionRepository) CreateRevision(ctx context.Context, revision *models.Revision) error {
	args := m.Called(ctx, revision)
	return args.Error(0)
}

// GetRevision mocks the GetRevision method
func (m *MockRevisionRepository) GetRevision(ctx context.Context, objectID string, version int) (*models.Revision, error) {
	args := m.Called(ctx, objectID, version)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Revision), args.Error(1)
}

// ===== List Operations =====

// ListRevisions mocks the ListRevisions method
func (m *MockRevisionRepository) ListRevisions(ctx context.Context, objectID string, limit int) ([]*models.Revision, error) {
	args := m.Called(ctx, objectID, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.Revision), args.Error(1)
}

// ListRevisionsPaginated mocks the ListRevisionsPaginated method
func (m *MockRevisionRepository) ListRevisionsPaginated(ctx context.Context, objectID string, limit int, cursor string) ([]*models.Revision, string, error) {
	args := m.Called(ctx, objectID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*models.Revision), args.String(1), args.Error(2)
}

// Delete mocks the Delete method
func (m *MockRevisionRepository) Delete(ctx context.Context, pk, sk string) error {
	args := m.Called(ctx, pk, sk)
	return args.Error(0)
}

// Ensure MockRevisionRepository implements interfaces.RevisionRepository
var _ interfaces.RevisionRepository = (*MockRevisionRepository)(nil)
