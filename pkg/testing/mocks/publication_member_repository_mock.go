// Package mocks provides mock implementations for testing.
package mocks

import (
	"context"

	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
)

// MockPublicationMemberRepository is a mock implementation of interfaces.PublicationMemberRepository
// using testify/mock for expectation-based testing.
type MockPublicationMemberRepository struct {
	mock.Mock
}

// NewMockPublicationMemberRepository creates a new mock publication member repository
func NewMockPublicationMemberRepository() *MockPublicationMemberRepository {
	return &MockPublicationMemberRepository{}
}

// ===== Core CRUD Operations =====

// CreateMember mocks the CreateMember method
func (m *MockPublicationMemberRepository) CreateMember(ctx context.Context, member *models.PublicationMember) error {
	args := m.Called(ctx, member)
	return args.Error(0)
}

// GetMember mocks the GetMember method
func (m *MockPublicationMemberRepository) GetMember(ctx context.Context, publicationID, userID string) (*models.PublicationMember, error) {
	args := m.Called(ctx, publicationID, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.PublicationMember), args.Error(1)
}

// DeleteMember mocks the DeleteMember method
func (m *MockPublicationMemberRepository) DeleteMember(ctx context.Context, publicationID, userID string) error {
	args := m.Called(ctx, publicationID, userID)
	return args.Error(0)
}

// ===== List Operations =====

// ListMembers mocks the ListMembers method
func (m *MockPublicationMemberRepository) ListMembers(ctx context.Context, publicationID string) ([]*models.PublicationMember, error) {
	args := m.Called(ctx, publicationID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.PublicationMember), args.Error(1)
}

// ListMembershipsForUserPaginated mocks the ListMembershipsForUserPaginated method
func (m *MockPublicationMemberRepository) ListMembershipsForUserPaginated(ctx context.Context, userID string, limit int, cursor string) ([]*models.PublicationMember, string, error) {
	args := m.Called(ctx, userID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*models.PublicationMember), args.String(1), args.Error(2)
}

// Update mocks the Update method
func (m *MockPublicationMemberRepository) Update(ctx context.Context, member *models.PublicationMember) error {
	args := m.Called(ctx, member)
	return args.Error(0)
}

// Ensure MockPublicationMemberRepository implements interfaces.PublicationMemberRepository
var _ interfaces.PublicationMemberRepository = (*MockPublicationMemberRepository)(nil)
