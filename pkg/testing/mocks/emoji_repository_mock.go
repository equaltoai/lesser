// Package mocks provides mock implementations for testing.
package mocks

import (
	"context"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/stretchr/testify/mock"
)

// MockEmojiRepository is a mock implementation of interfaces.EmojiRepository
// using testify/mock for expectation-based testing.
type MockEmojiRepository struct {
	mock.Mock
}

// NewMockEmojiRepository creates a new mock emoji repository
func NewMockEmojiRepository() *MockEmojiRepository {
	return &MockEmojiRepository{}
}

// ===== Core Emoji Operations =====

// CreateCustomEmoji mocks the CreateCustomEmoji method
func (m *MockEmojiRepository) CreateCustomEmoji(ctx context.Context, emoji *storage.CustomEmoji) error {
	args := m.Called(ctx, emoji)
	return args.Error(0)
}

// GetCustomEmoji mocks the GetCustomEmoji method
func (m *MockEmojiRepository) GetCustomEmoji(ctx context.Context, shortcode string) (*storage.CustomEmoji, error) {
	args := m.Called(ctx, shortcode)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.CustomEmoji), args.Error(1)
}

// GetCustomEmojis mocks the GetCustomEmojis method
func (m *MockEmojiRepository) GetCustomEmojis(ctx context.Context) ([]*storage.CustomEmoji, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.CustomEmoji), args.Error(1)
}

// UpdateCustomEmoji mocks the UpdateCustomEmoji method
func (m *MockEmojiRepository) UpdateCustomEmoji(ctx context.Context, emoji *storage.CustomEmoji) error {
	args := m.Called(ctx, emoji)
	return args.Error(0)
}

// DeleteCustomEmoji mocks the DeleteCustomEmoji method
func (m *MockEmojiRepository) DeleteCustomEmoji(ctx context.Context, shortcode string) error {
	args := m.Called(ctx, shortcode)
	return args.Error(0)
}

// ===== Remote Emoji Operations =====

// GetRemoteEmoji mocks the GetRemoteEmoji method
func (m *MockEmojiRepository) GetRemoteEmoji(ctx context.Context, shortcode, domain string) (*storage.CustomEmoji, error) {
	args := m.Called(ctx, shortcode, domain)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.CustomEmoji), args.Error(1)
}

// ===== Category and Search Operations =====

// GetCustomEmojisByCategory mocks the GetCustomEmojisByCategory method
func (m *MockEmojiRepository) GetCustomEmojisByCategory(ctx context.Context, category string) ([]*storage.CustomEmoji, error) {
	args := m.Called(ctx, category)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.CustomEmoji), args.Error(1)
}

// SearchEmojis mocks the SearchEmojis method
func (m *MockEmojiRepository) SearchEmojis(ctx context.Context, query string, limit int) ([]*storage.CustomEmoji, error) {
	args := m.Called(ctx, query, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.CustomEmoji), args.Error(1)
}

// ===== Popularity and Usage Operations =====

// GetPopularEmojis mocks the GetPopularEmojis method
func (m *MockEmojiRepository) GetPopularEmojis(ctx context.Context, domain string, limit int) ([]*storage.CustomEmoji, error) {
	args := m.Called(ctx, domain, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.CustomEmoji), args.Error(1)
}

// IncrementEmojiUsage mocks the IncrementEmojiUsage method
func (m *MockEmojiRepository) IncrementEmojiUsage(ctx context.Context, shortcode string) error {
	args := m.Called(ctx, shortcode)
	return args.Error(0)
}

// Ensure MockEmojiRepository implements interfaces.EmojiRepository
var _ interfaces.EmojiRepository = (*MockEmojiRepository)(nil)
