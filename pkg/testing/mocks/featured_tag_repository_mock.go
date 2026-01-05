// Package mocks provides mock implementations for testing.
package mocks

import (
	"context"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/stretchr/testify/mock"
)

// MockFeaturedTagRepository is a mock implementation of interfaces.FeaturedTagRepository
// using testify/mock for expectation-based testing.
type MockFeaturedTagRepository struct {
	mock.Mock
}

// NewMockFeaturedTagRepository creates a new mock featured tag repository
func NewMockFeaturedTagRepository() *MockFeaturedTagRepository {
	return &MockFeaturedTagRepository{}
}

// ===== Core Featured Tag Operations =====

// CreateFeaturedTag mocks the CreateFeaturedTag method
func (m *MockFeaturedTagRepository) CreateFeaturedTag(ctx context.Context, tag *storage.FeaturedTag) error {
	args := m.Called(ctx, tag)
	return args.Error(0)
}

// DeleteFeaturedTag mocks the DeleteFeaturedTag method
func (m *MockFeaturedTagRepository) DeleteFeaturedTag(ctx context.Context, username, name string) error {
	args := m.Called(ctx, username, name)
	return args.Error(0)
}

// GetFeaturedTags mocks the GetFeaturedTags method
func (m *MockFeaturedTagRepository) GetFeaturedTags(ctx context.Context, username string) ([]*storage.FeaturedTag, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.FeaturedTag), args.Error(1)
}

// ===== Tag Suggestions =====

// GetTagSuggestions mocks the GetTagSuggestions method
func (m *MockFeaturedTagRepository) GetTagSuggestions(ctx context.Context, username string, limit int) ([]string, error) {
	args := m.Called(ctx, username, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]string), args.Error(1)
}

// Ensure MockFeaturedTagRepository implements interfaces.FeaturedTagRepository
var _ interfaces.FeaturedTagRepository = (*MockFeaturedTagRepository)(nil)
