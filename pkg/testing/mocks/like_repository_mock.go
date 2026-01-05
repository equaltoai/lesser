// Package mocks provides mock implementations for testing.
package mocks

import (
	"context"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
)

// MockLikeRepository is a mock implementation of interfaces.LikeRepository
// using testify/mock for expectation-based testing.
type MockLikeRepository struct {
	mock.Mock
}

// NewMockLikeRepository creates a new mock like repository
func NewMockLikeRepository() *MockLikeRepository {
	return &MockLikeRepository{}
}

// CreateLike mocks the CreateLike method
func (m *MockLikeRepository) CreateLike(ctx context.Context, actor, object, statusAuthorID string) (*models.Like, error) {
	args := m.Called(ctx, actor, object, statusAuthorID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Like), args.Error(1)
}

// DeleteLike mocks the DeleteLike method
func (m *MockLikeRepository) DeleteLike(ctx context.Context, actor, object string) error {
	args := m.Called(ctx, actor, object)
	return args.Error(0)
}

// GetLike mocks the GetLike method
func (m *MockLikeRepository) GetLike(ctx context.Context, actor, object string) (*models.Like, error) {
	args := m.Called(ctx, actor, object)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Like), args.Error(1)
}

// GetObjectLikes mocks the GetObjectLikes method
func (m *MockLikeRepository) GetObjectLikes(ctx context.Context, objectID string, limit int, cursor string) ([]*models.Like, string, error) {
	args := m.Called(ctx, objectID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*models.Like), args.String(1), args.Error(2)
}

// GetActorLikes mocks the GetActorLikes method
func (m *MockLikeRepository) GetActorLikes(ctx context.Context, actorID string, limit int, cursor string) ([]*models.Like, string, error) {
	args := m.Called(ctx, actorID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*models.Like), args.String(1), args.Error(2)
}

// CountActorLikes mocks the CountActorLikes method
func (m *MockLikeRepository) CountActorLikes(ctx context.Context, actorID string) (int64, error) {
	args := m.Called(ctx, actorID)
	return args.Get(0).(int64), args.Error(1)
}

// HasLiked mocks the HasLiked method
func (m *MockLikeRepository) HasLiked(ctx context.Context, actor, object string) (bool, error) {
	args := m.Called(ctx, actor, object)
	return args.Bool(0), args.Error(1)
}

// CascadeDeleteLikes mocks the CascadeDeleteLikes method
func (m *MockLikeRepository) CascadeDeleteLikes(ctx context.Context, objectID string) error {
	args := m.Called(ctx, objectID)
	return args.Error(0)
}

// TombstoneObject mocks the TombstoneObject method
func (m *MockLikeRepository) TombstoneObject(ctx context.Context, objectID string, deletedBy string) error {
	args := m.Called(ctx, objectID, deletedBy)
	return args.Error(0)
}

// GetTombstone mocks the GetTombstone method
func (m *MockLikeRepository) GetTombstone(ctx context.Context, objectID string) (*storage.Tombstone, error) {
	args := m.Called(ctx, objectID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Tombstone), args.Error(1)
}

// GetLikeCount mocks the GetLikeCount method
func (m *MockLikeRepository) GetLikeCount(ctx context.Context, statusID string) (int64, error) {
	args := m.Called(ctx, statusID)
	return args.Get(0).(int64), args.Error(1)
}

// GetBoostCount mocks the GetBoostCount method
func (m *MockLikeRepository) GetBoostCount(ctx context.Context, statusID string) (int64, error) {
	args := m.Called(ctx, statusID)
	return args.Get(0).(int64), args.Error(1)
}

// IncrementReblogCount mocks the IncrementReblogCount method
func (m *MockLikeRepository) IncrementReblogCount(ctx context.Context, objectID string) error {
	args := m.Called(ctx, objectID)
	return args.Error(0)
}

// HasReblogged mocks the HasReblogged method
func (m *MockLikeRepository) HasReblogged(ctx context.Context, actorID, statusID string) (bool, error) {
	args := m.Called(ctx, actorID, statusID)
	return args.Bool(0), args.Error(1)
}

// CountForObject mocks the CountForObject method
func (m *MockLikeRepository) CountForObject(ctx context.Context, objectID string) (int64, error) {
	args := m.Called(ctx, objectID)
	return args.Get(0).(int64), args.Error(1)
}

// GetForObject mocks the GetForObject method
func (m *MockLikeRepository) GetForObject(ctx context.Context, objectID string, limit int, cursor string) ([]*models.Like, string, error) {
	args := m.Called(ctx, objectID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*models.Like), args.String(1), args.Error(2)
}

// GetLikedObjects mocks the GetLikedObjects method
func (m *MockLikeRepository) GetLikedObjects(ctx context.Context, actorID string, limit int, cursor string) ([]*models.Like, string, error) {
	args := m.Called(ctx, actorID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*models.Like), args.String(1), args.Error(2)
}

// Ensure MockLikeRepository implements interfaces.LikeRepository
var _ interfaces.LikeRepository = (*MockLikeRepository)(nil)
