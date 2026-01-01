// Package mocks provides mock implementations for testing.
package mocks

import (
	"context"

	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	dynamormcore "github.com/pay-theory/dynamorm/pkg/core"
	"github.com/stretchr/testify/mock"
)

// MockPublicationRepository is a mock implementation of interfaces.PublicationRepository
// using testify/mock for expectation-based testing.
type MockPublicationRepository struct {
	mock.Mock
}

// NewMockPublicationRepository creates a new mock publication repository
func NewMockPublicationRepository() *MockPublicationRepository {
	return &MockPublicationRepository{}
}

// ===== Database Access =====

// GetDB mocks the GetDB method
func (m *MockPublicationRepository) GetDB() dynamormcore.DB {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(dynamormcore.DB)
}

// ===== Core CRUD Operations =====

// CreatePublication mocks the CreatePublication method
func (m *MockPublicationRepository) CreatePublication(ctx context.Context, publication *models.Publication) error {
	args := m.Called(ctx, publication)
	return args.Error(0)
}

// GetPublication mocks the GetPublication method
func (m *MockPublicationRepository) GetPublication(ctx context.Context, id string) (*models.Publication, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Publication), args.Error(1)
}

// Update mocks the Update method
func (m *MockPublicationRepository) Update(ctx context.Context, publication *models.Publication) error {
	args := m.Called(ctx, publication)
	return args.Error(0)
}

// Delete mocks the Delete method
func (m *MockPublicationRepository) Delete(ctx context.Context, pk, sk string) error {
	args := m.Called(ctx, pk, sk)
	return args.Error(0)
}

// Ensure MockPublicationRepository implements interfaces.PublicationRepository
var _ interfaces.PublicationRepository = (*MockPublicationRepository)(nil)
