// Package mocks provides mock implementations for testing.
package mocks

import (
	"context"

	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
)

// MockMediaMetadataRepository is a mock implementation of interfaces.MediaMetadataRepository
// using testify/mock for expectation-based testing.
type MockMediaMetadataRepository struct {
	mock.Mock
}

// NewMockMediaMetadataRepository creates a new mock media metadata repository
func NewMockMediaMetadataRepository() *MockMediaMetadataRepository {
	return &MockMediaMetadataRepository{}
}

// CreateMediaMetadata mocks the CreateMediaMetadata method
func (m *MockMediaMetadataRepository) CreateMediaMetadata(ctx context.Context, metadata *models.MediaMetadata) error {
	args := m.Called(ctx, metadata)
	return args.Error(0)
}

// GetMediaMetadata mocks the GetMediaMetadata method
func (m *MockMediaMetadataRepository) GetMediaMetadata(ctx context.Context, mediaID string) (*models.MediaMetadata, error) {
	args := m.Called(ctx, mediaID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.MediaMetadata), args.Error(1)
}

// UpdateMediaMetadata mocks the UpdateMediaMetadata method
func (m *MockMediaMetadataRepository) UpdateMediaMetadata(ctx context.Context, metadata *models.MediaMetadata) error {
	args := m.Called(ctx, metadata)
	return args.Error(0)
}

// DeleteMediaMetadata mocks the DeleteMediaMetadata method
func (m *MockMediaMetadataRepository) DeleteMediaMetadata(ctx context.Context, mediaID string) error {
	args := m.Called(ctx, mediaID)
	return args.Error(0)
}

// GetMediaMetadataByStatus mocks the GetMediaMetadataByStatus method
func (m *MockMediaMetadataRepository) GetMediaMetadataByStatus(ctx context.Context, status string, limit int) ([]*models.MediaMetadata, error) {
	args := m.Called(ctx, status, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.MediaMetadata), args.Error(1)
}

// GetPendingMediaMetadata mocks the GetPendingMediaMetadata method
func (m *MockMediaMetadataRepository) GetPendingMediaMetadata(ctx context.Context, limit int) ([]*models.MediaMetadata, error) {
	args := m.Called(ctx, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.MediaMetadata), args.Error(1)
}

// GetProcessingMediaMetadata mocks the GetProcessingMediaMetadata method
func (m *MockMediaMetadataRepository) GetProcessingMediaMetadata(ctx context.Context, limit int) ([]*models.MediaMetadata, error) {
	args := m.Called(ctx, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.MediaMetadata), args.Error(1)
}

// MarkProcessingStarted mocks the MarkProcessingStarted method
func (m *MockMediaMetadataRepository) MarkProcessingStarted(ctx context.Context, mediaID string) error {
	args := m.Called(ctx, mediaID)
	return args.Error(0)
}

// MarkProcessingComplete mocks the MarkProcessingComplete method
func (m *MockMediaMetadataRepository) MarkProcessingComplete(ctx context.Context, mediaID string, result interfaces.ProcessingResult) error {
	args := m.Called(ctx, mediaID, result)
	return args.Error(0)
}

// MarkProcessingFailed mocks the MarkProcessingFailed method
func (m *MockMediaMetadataRepository) MarkProcessingFailed(ctx context.Context, mediaID string, errorMsg string) error {
	args := m.Called(ctx, mediaID, errorMsg)
	return args.Error(0)
}

// CleanupExpiredMetadata mocks the CleanupExpiredMetadata method
func (m *MockMediaMetadataRepository) CleanupExpiredMetadata(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

// Ensure MockMediaMetadataRepository implements interfaces.MediaMetadataRepository
var _ interfaces.MediaMetadataRepository = (*MockMediaMetadataRepository)(nil)
