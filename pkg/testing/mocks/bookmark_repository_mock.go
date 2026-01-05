// Package mocks provides mock implementations for testing.
package mocks

import (
	"context"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
)

// MockBookmarkRepository is a mock implementation of interfaces.BookmarkRepository
// using testify/mock for expectation-based testing.
type MockBookmarkRepository struct {
	mock.Mock
}

// NewMockBookmarkRepository creates a new mock bookmark repository
func NewMockBookmarkRepository() *MockBookmarkRepository {
	return &MockBookmarkRepository{}
}

// CreateBookmark mocks the CreateBookmark method
func (m *MockBookmarkRepository) CreateBookmark(ctx context.Context, username, objectID string) (*models.Bookmark, error) {
	args := m.Called(ctx, username, objectID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Bookmark), args.Error(1)
}

// DeleteBookmark mocks the DeleteBookmark method
func (m *MockBookmarkRepository) DeleteBookmark(ctx context.Context, username, objectID string) error {
	args := m.Called(ctx, username, objectID)
	return args.Error(0)
}

// GetBookmark mocks the GetBookmark method
func (m *MockBookmarkRepository) GetBookmark(ctx context.Context, username, objectID string) (*models.Bookmark, error) {
	args := m.Called(ctx, username, objectID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Bookmark), args.Error(1)
}

// GetUserBookmarks mocks the GetUserBookmarks method
func (m *MockBookmarkRepository) GetUserBookmarks(ctx context.Context, username string, limit int, cursor string) ([]*models.Bookmark, string, error) {
	args := m.Called(ctx, username, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*models.Bookmark), args.String(1), args.Error(2)
}

// IsBookmarked mocks the IsBookmarked method
func (m *MockBookmarkRepository) IsBookmarked(ctx context.Context, username, objectID string) (bool, error) {
	args := m.Called(ctx, username, objectID)
	return args.Bool(0), args.Error(1)
}

// CountUserBookmarks mocks the CountUserBookmarks method
func (m *MockBookmarkRepository) CountUserBookmarks(ctx context.Context, username string) (int64, error) {
	args := m.Called(ctx, username)
	return args.Get(0).(int64), args.Error(1)
}

// CheckBookmarksForStatuses mocks the CheckBookmarksForStatuses method
func (m *MockBookmarkRepository) CheckBookmarksForStatuses(ctx context.Context, username string, statusIDs []string) (map[string]bool, error) {
	args := m.Called(ctx, username, statusIDs)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]bool), args.Error(1)
}

// AddBookmark mocks the AddBookmark method
func (m *MockBookmarkRepository) AddBookmark(ctx context.Context, username, objectID string) error {
	args := m.Called(ctx, username, objectID)
	return args.Error(0)
}

// RemoveBookmark mocks the RemoveBookmark method
func (m *MockBookmarkRepository) RemoveBookmark(ctx context.Context, username, objectID string) error {
	args := m.Called(ctx, username, objectID)
	return args.Error(0)
}

// GetBookmarks mocks the GetBookmarks method
func (m *MockBookmarkRepository) GetBookmarks(ctx context.Context, username string, limit int, cursor string) ([]*storage.Bookmark, string, error) {
	args := m.Called(ctx, username, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.Bookmark), args.String(1), args.Error(2)
}

// CascadeDeleteUserBookmarks mocks the CascadeDeleteUserBookmarks method
func (m *MockBookmarkRepository) CascadeDeleteUserBookmarks(ctx context.Context, username string) error {
	args := m.Called(ctx, username)
	return args.Error(0)
}

// CascadeDeleteObjectBookmarks mocks the CascadeDeleteObjectBookmarks method
func (m *MockBookmarkRepository) CascadeDeleteObjectBookmarks(ctx context.Context, objectID string) error {
	args := m.Called(ctx, objectID)
	return args.Error(0)
}

// Ensure MockBookmarkRepository implements interfaces.BookmarkRepository
var _ interfaces.BookmarkRepository = (*MockBookmarkRepository)(nil)
