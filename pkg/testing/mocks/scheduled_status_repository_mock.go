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

// MockScheduledStatusRepository is a mock implementation of interfaces.ScheduledStatusRepository
// using testify/mock for expectation-based testing.
type MockScheduledStatusRepository struct {
	mock.Mock
}

// NewMockScheduledStatusRepository creates a new mock scheduled status repository
func NewMockScheduledStatusRepository() *MockScheduledStatusRepository {
	return &MockScheduledStatusRepository{}
}

// CreateScheduledStatus mocks the CreateScheduledStatus method
func (m *MockScheduledStatusRepository) CreateScheduledStatus(ctx context.Context, scheduled *storage.ScheduledStatus) error {
	args := m.Called(ctx, scheduled)
	return args.Error(0)
}

// GetScheduledStatus mocks the GetScheduledStatus method
func (m *MockScheduledStatusRepository) GetScheduledStatus(ctx context.Context, id string) (*storage.ScheduledStatus, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.ScheduledStatus), args.Error(1)
}

// GetScheduledStatuses mocks the GetScheduledStatuses method
func (m *MockScheduledStatusRepository) GetScheduledStatuses(ctx context.Context, username string, limit int, cursor string) ([]*storage.ScheduledStatus, string, error) {
	args := m.Called(ctx, username, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.ScheduledStatus), args.String(1), args.Error(2)
}

// UpdateScheduledStatus mocks the UpdateScheduledStatus method
func (m *MockScheduledStatusRepository) UpdateScheduledStatus(ctx context.Context, scheduled *storage.ScheduledStatus) error {
	args := m.Called(ctx, scheduled)
	return args.Error(0)
}

// DeleteScheduledStatus mocks the DeleteScheduledStatus method
func (m *MockScheduledStatusRepository) DeleteScheduledStatus(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

// GetDueScheduledStatuses mocks the GetDueScheduledStatuses method
func (m *MockScheduledStatusRepository) GetDueScheduledStatuses(ctx context.Context, before time.Time, limit int) ([]*storage.ScheduledStatus, error) {
	args := m.Called(ctx, before, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.ScheduledStatus), args.Error(1)
}

// MarkScheduledStatusPublished mocks the MarkScheduledStatusPublished method
func (m *MockScheduledStatusRepository) MarkScheduledStatusPublished(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

// GetScheduledStatusMedia mocks the GetScheduledStatusMedia method
func (m *MockScheduledStatusRepository) GetScheduledStatusMedia(ctx context.Context, id string) ([]*models.Media, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.Media), args.Error(1)
}

// SetMediaRepository mocks the SetMediaRepository method
func (m *MockScheduledStatusRepository) SetMediaRepository(mediaRepo interfaces.MediaRepositoryInterface) {
	m.Called(mediaRepo)
}

// Ensure MockScheduledStatusRepository implements interfaces.ScheduledStatusRepository
var _ interfaces.ScheduledStatusRepository = (*MockScheduledStatusRepository)(nil)
