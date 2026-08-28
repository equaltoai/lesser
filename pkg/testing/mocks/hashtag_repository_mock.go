// Package mocks provides mock implementations for testing.
package mocks

import (
	"context"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
)

// MockHashtagRepository is a mock implementation of interfaces.HashtagRepository
// using testify/mock for expectation-based testing.
type MockHashtagRepository struct {
	mock.Mock
}

// NewMockHashtagRepository creates a new mock hashtag repository
func NewMockHashtagRepository() *MockHashtagRepository {
	return &MockHashtagRepository{}
}

// IndexStatusHashtags mocks the IndexStatusHashtags method
func (m *MockHashtagRepository) IndexStatusHashtags(ctx context.Context, statusID string, authorID string, authorHandle string, statusURL string, content string, hashtags []string, published time.Time, visibility string) error {
	args := m.Called(ctx, statusID, authorID, authorHandle, statusURL, content, hashtags, published, visibility)
	return args.Error(0)
}

// RemoveStatusFromHashtagIndex mocks the RemoveStatusFromHashtagIndex method
func (m *MockHashtagRepository) RemoveStatusFromHashtagIndex(ctx context.Context, statusID string) error {
	args := m.Called(ctx, statusID)
	return args.Error(0)
}

// GetHashtagInfo mocks the GetHashtagInfo method
func (m *MockHashtagRepository) GetHashtagInfo(ctx context.Context, hashtag string) (*storage.Hashtag, error) {
	args := m.Called(ctx, hashtag)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Hashtag), args.Error(1)
}

// GetHashtagUsageHistory mocks the GetHashtagUsageHistory method
func (m *MockHashtagRepository) GetHashtagUsageHistory(ctx context.Context, hashtag string, days int) ([]int64, error) {
	args := m.Called(ctx, hashtag, days)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]int64), args.Error(1)
}

// GetHashtagActivity mocks the GetHashtagActivity method
func (m *MockHashtagRepository) GetHashtagActivity(ctx context.Context, hashtag string, since time.Time) ([]*storage.Activity, error) {
	args := m.Called(ctx, hashtag, since)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.Activity), args.Error(1)
}

// GetHashtagStats mocks the GetHashtagStats method
func (m *MockHashtagRepository) GetHashtagStats(ctx context.Context, hashtag string) (any, error) {
	args := m.Called(ctx, hashtag)
	return args.Get(0), args.Error(1)
}

// GetHashtagTimelineAdvanced mocks the GetHashtagTimelineAdvanced method
func (m *MockHashtagRepository) GetHashtagTimelineAdvanced(ctx context.Context, hashtag string, maxID *string, limit int, visibility string) ([]*storage.StatusSearchResult, error) {
	args := m.Called(ctx, hashtag, maxID, limit, visibility)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.StatusSearchResult), args.Error(1)
}

// GetMultiHashtagTimeline mocks the GetMultiHashtagTimeline method
func (m *MockHashtagRepository) GetMultiHashtagTimeline(ctx context.Context, hashtags []string, maxID *string, limit int, userID string) ([]*storage.StatusSearchResult, error) {
	args := m.Called(ctx, hashtags, maxID, limit, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.StatusSearchResult), args.Error(1)
}

// FollowHashtag mocks the FollowHashtag method
func (m *MockHashtagRepository) FollowHashtag(ctx context.Context, userID, hashtag string) error {
	args := m.Called(ctx, userID, hashtag)
	return args.Error(0)
}

// UnfollowHashtag mocks the UnfollowHashtag method
func (m *MockHashtagRepository) UnfollowHashtag(ctx context.Context, userID, hashtag string) error {
	args := m.Called(ctx, userID, hashtag)
	return args.Error(0)
}

// IsFollowingHashtag mocks the IsFollowingHashtag method
func (m *MockHashtagRepository) IsFollowingHashtag(ctx context.Context, userID, hashtag string) (bool, error) {
	args := m.Called(ctx, userID, hashtag)
	return args.Bool(0), args.Error(1)
}

// GetHashtagFollow mocks the GetHashtagFollow method
func (m *MockHashtagRepository) GetHashtagFollow(ctx context.Context, userID string, hashtag string) (*models.HashtagFollow, error) {
	args := m.Called(ctx, userID, hashtag)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.HashtagFollow), args.Error(1)
}

// GetHashtagMute mocks the GetHashtagMute method
func (m *MockHashtagRepository) GetHashtagMute(ctx context.Context, userID string, hashtag string) (*models.HashtagMute, error) {
	args := m.Called(ctx, userID, hashtag)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.HashtagMute), args.Error(1)
}

// Ensure MockHashtagRepository implements interfaces.HashtagRepository
var _ interfaces.HashtagRepository = (*MockHashtagRepository)(nil)
